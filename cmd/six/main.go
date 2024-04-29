package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"path"
	"runtime"
	"sync"
	"syscall"

	"github.com/ernestrc/blue/logging"
	multierr "github.com/ernestrc/go-multierror"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"unstable.build/go-tui/api/config"
	workspaceapi "unstable.build/go-tui/api/workspace"
	"unstable.build/go-tui/browser"
	"unstable.build/go-tui/debug"
	"unstable.build/go-tui/extension"
	"unstable.build/go-tui/extension/process"
	"unstable.build/go-tui/ide"
	"unstable.build/go-tui/proto"
	"unstable.build/go-tui/workspace"
	workspacepb "unstable.build/go-tui/workspace/rpc"
	"unstable.build/go-tui/workspace/ssh"
)

const (
	pprofAddr           = ":2860"
	configFilename      = ".sixrc"
	sixDefaultWallpaper = `
███████╗██╗██╗ ██╗
██╔════╝██║██████║
███████╗██║╚═██╔═╝
╚════██║██║██████╗
███████║██║██╔═██║
╚══════╝╚═╝╚═╝ ╚═╝`
)

var (
	// compile-time variables
	Tag     = "development"
	Commit  = "HEAD"
	Version string

	defaultConfigPath string
	flagConfigPath    *string
	defaultDataPath   string
	flagDataPath      *string

	flagRecover                = flag.String("r", "", "recover from recovery file")
	flagPprof                  = flag.Bool("p", false, fmt.Sprintf("start pprof server at %s", pprofAddr))
	flagVersion                = flag.Bool("v", false, "print version information")
	flagWorkspace              = flag.String("w", cwdURI().String(), "workspaceapi.URI")
	flagWorkspaceServer        = flag.String("x", "", "runs workspace server from standard input and output")
	flagWorkspaceServerLogFile = flag.String("o", "", "log workspace server TRACE level logs to given file")
)

func init() {
	home, err := os.UserHomeDir()
	if err != nil {
		log.Error(err)
		home = "."
	}
	defaultConfigPath = path.Join(home, configFilename)
	flagConfigPath = flag.String("c", defaultConfigPath, "config file path")
	defaultDataPath = path.Join(home, ".six")
	flagDataPath = flag.String("d", defaultDataPath, "data directory path")

	Version = fmt.Sprintf("%s (HEAD is %s)", Tag, Commit)
}

func cwdURI() workspaceapi.URI {
	wd, err := os.Getwd()
	if err != nil {
		log.Fatalf("Failed to get working directory: %s", err)
	}
	uri, err := workspaceapi.CurrentUserHostURI(wd)
	if err != nil {
		log.Fatalf("Failed to parse working directory as URI %s: %s", wd, err)
	}
	return uri
}

func startWorkspaceServer() int {
	newScheme := workspace.NewFileScheme
	if serverLogs := *flagWorkspaceServerLogFile; serverLogs != "" {
		f, err := workspace.OpenFile(serverLogs,
			os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
		if err != nil {
			log.Fatal(err)
		}
		log.SetOutput(f)
		log.SetLevel(log.TraceLevel)
		log.SetFormatter(logging.LogrusLogdFormatter{})
		proto.EnableGRPCLogging(f, f, f)

		defer f.Close()
		defer f.Sync()

		newScheme = workspace.LoggingScheme("file", newScheme)

	} else {
		log.SetOutput(ioutil.Discard)
		log.SetLevel(log.PanicLevel)
		proto.DisableGRPCLogging()
	}

	log.Tracef("Initialized debug logger")

	// log unhandled signals for debugging
	ch := make(chan os.Signal, 1)
	quitch := make(chan struct{})
	grpcServer := grpc.NewServer(
		grpc.ChainUnaryInterceptor(
			proto.UnaryLoggingRecoveryInterceptor(),
		),
		grpc.ChainStreamInterceptor(
			proto.StreamLoggingRecoveryInterceptor(),
		),
	)
	signal.Notify(ch)

	defer close(quitch)
	defer signal.Reset()

	go func() {
		defer grpcServer.Stop()
		for {
			select {
			case sig := <-ch:
				switch sig {
				case syscall.SIGTERM, syscall.SIGINT:
					log.Infof("Received %v signal: cleaning up...", sig)
					return
				case syscall.SIGKILL:
					log.Info("Received SIGKILL signal: exiting")
					os.Exit(1)
				case syscall.SIGURG:
					/* received when socket urgent data is ready to be read */
				default:
					log.Debugf("Received unhandled signal: %#v", sig)
				}
			case <-quitch:
				return
			}
		}
	}()

	uri, err := workspaceapi.CurrentUserHostURI(*flagWorkspaceServer)
	if err != nil {
		log.Error(err)
		return 2
	}
	scheme, err := newScheme(context.Background(), config.NopConfig(), uri)
	if err != nil {
		log.Error(err)
		return 3
	}
	defer scheme.Close()

	server := workspacepb.NewServer(scheme, new(sync.Mutex))
	defer server.Stop()

	err = ssh.StartSchemeServer(log.StandardLogger(), server, grpcServer)
	if err != nil {
		log.Error(err)
		return 4
	}
	log.Tracef("StartSchemeServer returned with no error")
	return 0
}

func main() {
	var code int
	ok, path, err := debug.CapturePanicReportDir(".", "six", Tag, func() {
		code = run()
	})
	if ok {
		os.Exit(code)
	}
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Saved crash report %q\n", path)
	os.Exit(4)
}

func extensionRunner(
	locker sync.Locker,
	uri workspaceapi.URI,
	res map[extension.Permission]extension.ResourceRegistrar,
	dataDir string,
	notifications browser.Notifications,
) (extension.Runner, error) {
	notifications = &protectedNotifications{notifications: notifications, locker: locker}
	extensionOpts := []process.Option{
		process.WithLocker(locker),
		process.WithWorkspace(uri),
		process.WithDataDir(dataDir),
		process.WithPackageName("six"),
		process.WithPackageVersion(Tag),
		process.WithNotifications(notifications),
	}
	return process.NewManager(extension.GrantAll(res), extensionOpts...)
}

func run() int {
	var err error
	var filenames []string

	flag.Parse()

	if *flagVersion {
		fmt.Printf("Six %s\n", Version)
		return 0
	}

	for _, file := range flag.Args() {
		filenames = append(filenames, file)
	}

	if *flagPprof {
		runtime.SetBlockProfileRate(1)
		runtime.SetMutexProfileFraction(1)
		go func() {
			log.Println(http.ListenAndServe(pprofAddr, nil))
		}()
	}

	proto.DisableGRPCLogging()

	if *flagWorkspaceServer != "" {
		code := startWorkspaceServer()
		return code
	}

	if *flagConfigPath != defaultConfigPath {
		if _, err := os.Stat(*flagConfigPath); err != nil {
			err = fmt.Errorf("stat %q: %s", *flagConfigPath, err)
			fmt.Printf("%s", err)
			return 1
		}
	}

	// ensure that data path exists
	if _, err := os.Stat(*flagDataPath); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			err = fmt.Errorf("stat %q: %s", *flagDataPath, err)
			fmt.Printf("%s", err)
			return 1
		}
		if err := os.Mkdir(*flagDataPath, 0777); err != nil {
			err = fmt.Errorf("mkdir %q: %s", *flagDataPath, err)
			fmt.Printf("%s", err)
			return 1
		}
	}

	var eventLoopMutex sync.Mutex
	opts := []ide.Option{
		ide.WithExtensionsRunner(ide.FuncExtensionsRunner(extensionRunner)),
		ide.WithLocker(&eventLoopMutex),
		ide.WithConfigFilename(configFilename),
		ide.WithDefaultWallpaper(sixDefaultWallpaper),
	}

	var i *ide.IDE
	if *flagRecover != "" && len(filenames) != 0 {
		i, err = ide.NewRecovery(*flagWorkspace, *flagConfigPath,
			filenames[0], *flagRecover, *flagDataPath, opts...)
	} else if *flagRecover != "" {
		err = fmt.Errorf("flag -r requires to pass the original filename")
	} else {
		i, err = ide.New(*flagWorkspace, *flagConfigPath,
			*flagDataPath, filenames, opts...)
	}

	if err != nil {
		fmt.Printf("%s", err)
		return 1
	}

	var ret error
	if err := i.Run(); err != nil {
		ret = multierr.Append(ret, err)
	}
	if err := i.Close(); err != nil {
		ret = multierr.Append(ret, err)
	}
	if ret != nil {
		fmt.Printf("%s", ret)
		log.Error(ret)
		return 1
	}

	return 0
}
