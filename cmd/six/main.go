// Unstable Build LLC ("COMPANY") CONFIDENTIAL
//
// Unpublished Copyright (c) 2017-2024 Unstable Build, All Rights Reserved.
//
// NOTICE: All information contained herein is, and remains the property of COMPANY.
// The intellectual and technical concepts contained herein are proprietary to
// COMPANY and may be covered by U.S. and Foreign Patents, patents in process,
// and are protected by trade secret or copyright law. Dissemination of this information
// or reproduction of this material is strictly forbidden unless prior written permission
// is obtained from COMPANY. Access to the source code contained herein is hereby
// forbidden to anyone except current COMPANY employees, managers or contractors who
// have executed Confidentiality and Non-disclosure agreements explicitly covering such access.
//
// The copyright notice above does not evidence any actual or intended publication or
// disclosure of this source code, which includes information that is confidential and/or
// proprietary, and is a trade secret, of COMPANY. ANY REPRODUCTION, MODIFICATION,
// DISTRIBUTION, PUBLIC  PERFORMANCE, OR PUBLIC DISPLAY OF OR THROUGH USE OF THIS SOURCE CODE
// WITHOUT  THE EXPRESS WRITTEN CONSENT OF COMPANY IS STRICTLY PROHIBITED, AND IN
// VIOLATION OF APPLICABLE LAWS AND INTERNATIONAL TREATIES. THE RECEIPT OR POSSESSION OF
// THIS SOURCE CODE AND/OR RELATED INFORMATION DOES NOT CONVEY OR IMPLY ANY RIGHTS TO
// REPRODUCE, DISCLOSE OR DISTRIBUTE ITS CONTENTS, OR TO MANUFACTURE, USE, OR SELL
// ANYTHING THAT IT MAY DESCRIBE, IN WHOLE OR IN PART.

package main

import (
	"bytes"
	"context"
	_ "embed"
	"errors"
	"flag"
	"fmt"
	"image"
	"image/png"
	"io"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"path"
	"runtime"
	"sync"
	"syscall"
	"time"

	multierr "github.com/ernestrc/go-multierror"
	log "github.com/sirupsen/logrus"
	"github.com/unstablebuild/blue/logging"
	"google.golang.org/grpc"
	"unstable.build/go-tui"
	"unstable.build/go-tui/api/config"
	"unstable.build/go-tui/api/workspaceapi"
	"unstable.build/go-tui/browser"
	"unstable.build/go-tui/component"
	"unstable.build/go-tui/component/asciiart"
	"unstable.build/go-tui/component/shader"
	"unstable.build/go-tui/component/shader/glslshader"
	"unstable.build/go-tui/debug"
	"unstable.build/go-tui/extension"
	"unstable.build/go-tui/extension/extensionv2"
	"unstable.build/go-tui/ide"
	"unstable.build/go-tui/rpc"
	"unstable.build/go-tui/term"
	"unstable.build/go-tui/workspace"
	"unstable.build/go-tui/workspace/workspacerpc"
	"unstable.build/go-tui/workspace/workspacessh"
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
	version string

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

	version = fmt.Sprintf("%s (HEAD is %s)", debug.Tag, debug.Commit)
}

func cwdURI() workspaceapi.URI {
	wd, err := os.Getwd()
	if err != nil {
		log.Fatalf("get working directory: %s", err)
	}
	uri, err := workspaceapi.CurrentUserHostURI(wd)
	if err != nil {
		log.Fatalf("parse working directory as URI %s: %s", wd, err)
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
		rpc.EnableGRPCLogging(f, f, f)

		//nolint:errcheck
		defer f.Close()
		//nolint:errcheck
		defer f.Sync()

		newScheme = workspace.LoggingScheme("file", newScheme)

	} else {
		log.SetOutput(io.Discard)
		log.SetLevel(log.PanicLevel)
		rpc.DisableGRPCLogging()
	}

	log.Tracef("Initialized debug logger")

	// log unhandled signals for debugging
	ch := make(chan os.Signal, 1)
	quitch := make(chan struct{})
	grpcServer := grpc.NewServer(
		grpc.ChainUnaryInterceptor(
			rpc.UnaryReportRecoveryInterceptor(),
		),
		grpc.ChainStreamInterceptor(
			rpc.StreamReportRecoveryInterceptor(),
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

	server := workspacerpc.NewServer(scheme, new(sync.Mutex))
	//nolint:errcheck
	defer server.Stop()

	err = workspacessh.StartSchemeServer(log.StandardLogger(), server, grpcServer)
	if err != nil {
		log.Error(err)
		return 4
	}
	log.Tracef("StartSchemeServer returned with no error")
	return 0
}

func main() {
	flag.Parse()

	// ensure that data path exists
	if _, err := os.Stat(*flagDataPath); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			err = fmt.Errorf("stat %q: %s", *flagDataPath, err)
			fmt.Printf("%s", err)
			os.Exit(1)
		}
		if err := os.MkdirAll(*flagDataPath, 0777); err != nil {
			err = fmt.Errorf("mkdir %q: %s", *flagDataPath, err)
			fmt.Printf("%s", err)
			os.Exit(1)
		}
	}

	var code int
	ok, path, err := debug.CapturePanicReportWith(
		debug.ReportsDir, debug.Package, debug.Tag, func() {
			code = run()
		})
	if ok {
		os.Exit(code)
	}
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Saved crash report file://%v\n", path)
	os.Exit(4)
}

func run() int {
	ctx := context.Background()

	if *flagVersion {
		fmt.Printf("Six %s\n", version)
		return 0
	}

	var err error
	var filenames []string
	filenames = append(filenames, flag.Args()...)

	if *flagPprof {
		runtime.SetBlockProfileRate(1)
		runtime.SetMutexProfileFraction(1)
		go func() {
			log.Println(http.ListenAndServe(pprofAddr, nil))
		}()
	}

	rpc.DisableGRPCLogging()

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

	var eventLoopMutex sync.Mutex

	grantor := extension.GrantAll()
	extensionOpts := []extensionv2.Option{
		extensionv2.WithPackageName(debug.Package),
		extensionv2.WithPackageVersion(debug.Tag),
	}
	runner, err := extensionv2.NewRunner(ctx, &eventLoopMutex,
		grantor, *flagDataPath, extensionOpts...)
	if err != nil {
		err = fmt.Errorf("new extension runner: %v", err)
		fmt.Printf("%s", err)
		return 1
	}

	unstableBuildLogo := unstableBuildLogo()
	opts := []ide.Option{
		ide.WithExtensionsRunner(runner),
		ide.WithInitShader(
			func(defaultAttr term.Attributes) shader.Shader {
				return glslshader.Burning(
					glslshader.BurningPresetGentle(unstableBuildLogo, true),
					defaultAttr,
					10*time.Second, 60,
				)
			}, 60, 10*time.Second),
		ide.WithShutdownShader(
			func(defaultAttr term.Attributes) shader.Shader {
				return glslshader.Incendium(
					glslshader.DefaultIncendiumParams(), defaultAttr, 60,
				)
			}, 60, 10*time.Second),
		ide.WithLocker(&eventLoopMutex),
		ide.WithConfigFilename(configFilename),
		ide.WithDefaultWallpaper(makeWallpaper(unstableBuildLogo)),
		ide.WithDefaultConfigYAML(defaultConfig),
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

//go:embed unstable_build_logo.png
var unstableBuildLogoBytes []byte

func unstableBuildLogo() image.Image {
	img, err := png.Decode(bytes.NewReader(unstableBuildLogoBytes))
	if err != nil {
		panic(fmt.Errorf("png decode: %v", err))
	}
	return img
}

func makeWallpaper(img image.Image) browser.Wallpaper {
	return browser.Wallpaper{
		NewComponent: func() tui.Component {
			cfg := asciiart.DefaultConfig()
			cfg.Color = true
			cfg.MaintainAspectRatio = true
			cfg.DensityCharacters = "\u2009▓▓▓▓▓▓▓▓▓"
			image := asciiart.NewComponent(img, cfg)
			return component.NewSpan(image, component.SpanConfig{
				PadHorizontalPerc: 0.4,
				PadVerticalPerc:   0.2,
				ContentAlignment:  component.SpanAlignmentCentered,
			})
		},
	}
}
