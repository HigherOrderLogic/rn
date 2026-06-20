// Unstable Build LLC ("COMPANY") CONFIDENTIAL
//
// Unpublished Copyright (c) 2023-2024 Unstable Build, All Rights Reserved.
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
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"runtime"
	"sync"
	"syscall"
	"time"

	multierr "github.com/ernestrc/go-multierror"
	extbrowser "github.com/ernestrc/sensible/browser"
	log "github.com/sirupsen/logrus"
	flag "github.com/spf13/pflag"
	"github.com/unstablebuild/blue/logging"
	"github.com/unstablebuild/blue/release"
	"github.com/unstablebuild/blue/release/cdnrelease"
	"github.com/unstablebuild/rune-go-sdk/api/browserapi"
	"github.com/unstablebuild/rune-go-sdk/api/config"
	"github.com/unstablebuild/rune-go-sdk/api/storageapi"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"github.com/unstablebuild/rune-go-sdk/term"
	"github.com/unstablebuild/rune-go-sdk/tui"
	"golang.org/x/oauth2"
	"golang.org/x/sys/unix"
	"google.golang.org/grpc"
	"unstable.build/go-tui/cmd/rune/crashreport"
	"unstable.build/go-tui/cmd/rune/ide/apiclient"
	"unstable.build/go-tui/component/shader"
	"unstable.build/go-tui/debug"
	"unstable.build/go-tui/extension/extensionv2"
	"unstable.build/go-tui/ide"
	"unstable.build/go-tui/ide/ideplan"
	"unstable.build/go-tui/llm/llmrpc"
	"unstable.build/go-tui/rpc"
	"unstable.build/go-tui/term/gui"
	"unstable.build/go-tui/text"
	"unstable.build/go-tui/workspace"
	"unstable.build/go-tui/workspace/workspacerpc"
	"unstable.build/go-tui/workspace/workspacessh"
)

const doubleClickTimeout = 500 * time.Millisecond

// defaultWebsiteAddress is the production base URL of the Rune
// website. The upgrade prompt opens "<base>/checkout?source=rune" on
// click; the website honors the source query parameter to stash a
// "return to Rune" intent during the OAuth + Stripe round trip.
const defaultWebsiteAddress = "https://rune.build"

var (
	apicfg = apiclient.DefaultConfig()
	// Version is a combination of Tag and Commit, representing
	// this executables version.
	version string

	configFilename          = "config.yaml"
	configStarFilename      = "config.star"
	workspaceConfigFilename = ".rune/config.yaml"
	home                    string

	defaultConfigPath string
	flagConfigPath    *string
	defaultDataPath   string
	flagDataPath      *string

	flagVersion   = flag.BoolP("version", "v", false, "Print version information and exit")
	flagWorkspace = flag.StringP("workspace", "w", cwdURI().String(),
		"Set the initial workspace to open in the format [scheme:][//[userinfo@]host][/]path")
	flagFPS = flag.BoolP("fps", "f", false, "Render FPS on GUI mode")
	flagGUI = flag.BoolP("gui", "G", false, "Run Rune in manual GUI mode")
	flagTUI = flag.Bool("hardcore", false, "Run Rune in manual TUI mode")

	// marked hidden
	flagWorkspaceServer = flag.StringP("workspace-server", "x", "",
		"Run a workspace server from standard input and output")
	flagWorkspaceServerLogFile = flag.StringP("workspace-server-log", "o", "",
		"Log workspace server TRACE level logs to file")
	flagHTTPAddress = flag.String("rune-http-address", apicfg.HTTPEndpointAddress,
		"Rune HTTP API endpoint host/port pair")
	flagGRPCAddress = flag.String("rune-grpc-address", apicfg.GRPCEndpointAddress,
		"Rune GRPC API endpoint host/port pair")
	flagGRPCInsecure = flag.Bool("rune-grpc-insecure", apicfg.InsecureTransport,
		"If set to false, does not use GRPC over TLS or oauth2 credentials.")
	flagTelemetryPeriod = flag.Duration("rune-telemetry-period", apicfg.TelemetryPeriod,
		"How often to send aggregated usage data (requires --rune-enable-telemetry).")
	flagReleaseCollection = flag.String("rune-release-collection",
		apicfg.ReleaseCollection,
		"Collection name for the release manager.")
	flagZdotDir = flag.String("rune-zdotdir", "", "Initial ZDOTDIR directory when using default OS shell via $SHELL.")

	flagWebsiteAddress = flag.String("rune-website-address", defaultWebsiteAddress,
		"Base URL of the Rune website. Used to build the checkout URL "+
			"opened by the upgrade prompt during bootstrap and lockdown.")
)

func init() {
	var err error
	home, err = os.UserHomeDir()
	if err != nil {
		log.Error(err)
		home = "."
	}

	defaultConfigPath = resolveDefaultConfigPath(defaultDataPath)
	flagConfigPath = flag.StringP("config", "c", defaultConfigPath,
		"Use this file for configuring rune")

	defaultDataPath = path.Join(home, ".rune")
	flagDataPath = flag.StringP("datadir", "d", defaultDataPath,
		"Set temporary data directory")

	version = fmt.Sprintf("%s (HEAD is %s)", debug.Tag, debug.Commit)
}

func resolveDefaultConfigPath(dataDir string) string {
	yamlPath := path.Join(dataDir, configFilename)
	starPath := path.Join(dataDir, configStarFilename)
	if _, err := os.Stat(yamlPath); err == nil {
		return yamlPath
	}
	if _, err := os.Stat(starPath); err == nil {
		return starPath
	}
	return yamlPath
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
	l := log.New()
	var logger *slog.Logger

	newScheme := workspace.NewFileScheme
	if serverLogs := *flagWorkspaceServerLogFile; serverLogs != "" {
		f, err := workspace.OpenFile(serverLogs,
			os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
		if err != nil {
			log.Fatal(err)
		}
		l.SetOutput(f)
		l.SetLevel(log.TraceLevel)
		l.SetFormatter(logging.LogrusLogdFormatter{})
		rpc.EnableGRPCLogging(f, f, f)
		logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		}))

		defer f.Close()
		defer func() {
			_ = f.Sync()
		}()

		newScheme = workspace.LoggingScheme("file", newScheme)

	} else {
		logger = slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{}))
		l.SetOutput(io.Discard)
		l.SetLevel(log.PanicLevel)
		rpc.DisableGRPCLogging()
	}
	slog.SetDefault(logger)

	l.Tracef("Initialized debug logger")

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
		grpc.MaxRecvMsgSize(llmrpc.MaxRecvMsgSize),
		grpc.MaxSendMsgSize(llmrpc.MaxSendMsgSize),
	)
	signal.Notify(ch)

	defer close(quitch)
	defer signal.Reset()

	go debug.CapturePanicReport(func() {
		defer grpcServer.Stop()
		for {
			select {
			case sig := <-ch:
				switch sig {
				case syscall.SIGTERM, syscall.SIGINT:
					l.Infof("Received %v signal: cleaning up...", sig)
					return
				case syscall.SIGKILL:
					l.Info("Received SIGKILL signal: exiting")
					os.Exit(1)
				case syscall.SIGURG:
					/* received when socket urgent data is ready to be read */
				default:
					l.Debugf("Received unhandled signal: %#v", sig)
				}
			case <-quitch:
				return
			}
		}
	})

	uri, err := workspaceapi.CurrentUserHostURI(*flagWorkspaceServer)
	if err != nil {
		l.Error(err)
		return 2
	}
	scheme, err := newScheme(context.Background(), config.NopConfig(), uri)
	if err != nil {
		l.Error(err)
		return 3
	}
	defer scheme.Close()

	server := workspacerpc.NewServer(scheme, new(sync.Mutex),
		workspacerpc.CommandAuthorizerFunc(
			func(context.Context, workspaceapi.Cmd) error { return nil }))
	defer func() {
		_ = server.Stop()
	}()

	err = workspacessh.StartSchemeServer(l, server, grpcServer)
	if err != nil {
		l.Error(err)
		return 4
	}
	l.Tracef("StartSchemeServer returned with no error")
	return 0
}

func main() {
	if err := flag.CommandLine.MarkHidden("rune-http-address"); err != nil {
		panic(err)
	}
	if err := flag.CommandLine.MarkHidden("rune-grpc-address"); err != nil {
		panic(err)
	}
	if err := flag.CommandLine.MarkHidden("rune-grpc-insecure"); err != nil {
		panic(err)
	}
	if err := flag.CommandLine.MarkHidden("workspace-server"); err != nil {
		panic(err)
	}
	if err := flag.CommandLine.MarkHidden("workspace-server-log"); err != nil {
		panic(err)
	}
	if err := flag.CommandLine.MarkHidden("rune-telemetry-period"); err != nil {
		panic(err)
	}
	if err := flag.CommandLine.MarkHidden("rune-release-collection"); err != nil {
		panic(err)
	}
	if err := flag.CommandLine.MarkHidden("rune-zdotdir"); err != nil {
		panic(err)
	}
	if err := flag.CommandLine.MarkHidden("rune-website-address"); err != nil {
		panic(err)
	}
	if err := flag.CommandLine.MarkHidden("hardcore"); err != nil {
		panic(err)
	}

	flag.ErrHelp = errors.New("")
	flag.Usage = func() {
		fmt.Printf("Rune %s\n\n", version)
		flag.PrintDefaults()
	}

	flag.Parse()

	exec, _ := os.Executable()
	// If no manual tui/gui flag was set, assume we were launched as a desktop
	// app and inject the same defaults the platform launcher would normally pass.
	if !*flagGUI && !*flagTUI && *flagWorkspaceServer == "" {
		if err := os.MkdirAll(*flagDataPath, 0777); err != nil {
			fmt.Fprintf(os.Stderr, "mkdir datadir %q: %s",
				*flagDataPath, err)
		}
		// gracefully degrade; launch without zsh customization
		// which looses ensuring modal vte works well with zsh
		zdotDir := ""
		if src, ok := bundleZdotDir(runtime.GOOS, exec); ok {
			dst := filepath.Join(*flagDataPath, "zdot")
			if err := installZdotDir(src, dst); err != nil {
				fmt.Fprintf(os.Stderr,
					"install zdot %q -> %q: %s", src, dst, err)
			} else {
				zdotDir = dst
			}
		}
		defaults, ok := appLaunchArgs(runtime.GOOS, zdotDir)
		if ok {
			os.Args = append(os.Args[:1], append(defaults, os.Args[1:]...)...)

			// best effort redirect stdout/err to <datadir>/launch.log
			// so runtime fatal stderr dumps can later be ingested as
			// crash reports.
			logPath := crashreport.DefaultLaunchLogPath(*flagDataPath)
			f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
			if err == nil {
				// syscall.Dup2 isn't defined on linux/arm64 (the
				// kernel only exposes Dup3 there); golang.org/x/sys/unix
				// papers over the difference.
				fd := int(f.Fd())
				_ = unix.Dup2(fd, int(os.Stderr.Fd()))
			}
			flag.Parse()
		}
	}

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
	defaultConfigPath = resolveDefaultConfigPath(*flagDataPath)
	if !flag.Lookup("config").Changed {
		*flagConfigPath = defaultConfigPath
	}

	// Set up the crash report directory under the data path so reports
	// are stored durably rather than in the OS temp dir.
	reportsDir := filepath.Join(*flagDataPath, "reports")
	if err := os.MkdirAll(reportsDir, 0777); err != nil {
		fmt.Fprintf(os.Stderr, "mkdir reports %q: %s", reportsDir, err)
	} else {
		debug.ReportsDir = reportsDir
	}

	var code int
	_, err, ok := debug.CapturePanicReportWith(
		reportsDir, debug.Package, debug.Tag, func() {
			code = run()
		})
	if ok {
		os.Exit(code)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "fatal error: %v", err)
		log.Errorf("fatal error: %v", err)
	}
	os.Exit(4)
}

var zdotFiles = []string{".zshenv", ".zprofile", ".zshrc", ".zlogin"}

func appLaunchArgs(goos, zdotDir string) ([]string, bool) {
	var args []string
	if zdotDir != "" {
		args = append(args, "--rune-zdotdir="+zdotDir)
	}
	switch goos {
	case "darwin", "linux":
		return append(args, "-G", "-w", ""), true
	default:
		return nil, false
	}
}

func bundleZdotDir(goos, execPath string) (string, bool) {
	switch goos {
	case "darwin":
		macosDir := filepath.Dir(execPath)
		contentsDir := filepath.Dir(macosDir)
		resourcesDir := filepath.Join(contentsDir, "Resources")
		return filepath.Join(resourcesDir, "zdot"), true
	case "linux":
		appDir, ok := linuxAppDir(execPath)
		if !ok {
			return "", false
		}
		return filepath.Join(appDir, "share", "zdot"), true
	default:
		return "", false
	}
}

func installZdotDir(srcDir, dstDir string) error {
	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		return err
	}
	for _, name := range zdotFiles {
		src := filepath.Join(srcDir, name)
		data, err := os.ReadFile(src)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return err
		}
		dst := filepath.Join(dstDir, name)
		if err := os.WriteFile(dst, data, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func linuxAppDir(execPath string) (string, bool) {
	binDir := filepath.Dir(execPath)
	if filepath.Base(binDir) != "bin" {
		return "", false
	}

	appDir := filepath.Dir(binDir)
	if filepath.Base(appDir) != "rune.app" {
		return "", false
	}

	return appDir, true
}

func run() int {
	var filenames []string

	if *flagVersion {
		fmt.Printf("Rune %s\n", version)
		return 0
	}

	filenames = append(filenames, flag.Args()...)

	if err := setupRuneBinPATH(*flagDataPath); err != nil {
		log.Errorf("installed executables will not be available: "+
			"set the PATH env variable: %v", err)
	}

	// TUI inherits the parent-shell PATH the user already exported, so it
	// skips the login SHELL PATH resolve.
	var pathDone <-chan error
	if !*flagTUI {
		pathDone = startLoginShellPATHResolve(*flagDataPath)
	}

	rpc.DisableGRPCLogging()

	debug.StartPProfOnSignal()

	if *flagWorkspaceServer != "" {
		if err := <-pathDone; err != nil {
			log.Errorf("could not resolve login shell PATH; tools on "+
				"it (e.g. homebrew, mise) may be unavailable: %v", err)
		}
		code := startWorkspaceServer()
		return code
	}

	var mu sync.Mutex
	ctx := context.Background()
	runner, err := extensionv2.NewRunner(ctx, &mu, *flagDataPath)
	if err != nil {
		err = fmt.Errorf("new extension runner: %v", err)
		fmt.Fprintf(os.Stderr, "%s", err)
		return 1
	}

	if *flagConfigPath != defaultConfigPath {
		if _, err := os.Stat(*flagConfigPath); err != nil {
			err = fmt.Errorf("stat %q: %s", *flagConfigPath, err)
			fmt.Printf("%s", err)
			return 1
		}
	}

	if *flagGUI {
		return runGUI(filenames, runner, &mu, pathDone)
	} else if *flagTUI {
		return runTUI(filenames, runner, &mu)
	} else {
		fmt.Fprintf(os.Stderr, "--gui must be set if running on %s\n",
			runtime.GOOS)
		return 1
	}
}

func runTUI(
	filenames []string, runner ide.ExtensionsRunner,
	mu *sync.Mutex,
) int {
	opts := []ide.Option{
		ide.WithExtensionsRunner(runner),
		ide.WithInitShader(
			initShader,
			initShaderFPS,
			initShaderDuration),
		ide.WithShutdownShader(
			func(defaultAttr term.Attributes) shader.Shader {
				return shutdownShader(defaultAttr)
			}, 30, shutdownShaderDuration),
		ide.WithLoadingShader(loadingShader, loadingShaderFPS, loadingShaderDuration),
		ide.WithOpenShader(openShader, openShaderFPS, openShaderDuration),
		ide.WithLocker(mu),
		ide.WithConfigFilename(workspaceConfigFilename),
		ide.WithDefaultWallpaper(makeWallpaper()),
		ide.WithDefaultConfigStarlark(defaultStarlarkConfig, true, true),
		ide.WithScheduleNextTick(func(fn func()) bool {
			return tui.PublishEvent(term.Event{Type: term.EventInterrupt, UserFunc: fn})
		}),
		ide.WithZdotDir(*flagZdotDir),
		ide.WithScheme(docsScheme, newDocsSchemeFunc(*flagConfigPath)),
		ide.WithStreamingOpen(true),
	}
	opts = append(opts, embeddedTutorialOptions()...)
	if debug.DebugBuild == "true" {
		opts = append(opts, ide.WithDebugCommands(true))
	}

	scheduleNextTick := func(fn func()) bool {
		return tui.PublishEvent(term.Event{Type: term.EventInterrupt, UserFunc: fn})
	}

	storage := newRuneStorage(*flagDataPath)
	client, releaseManager := newAPIClient(storage)
	defer client.Close()

	checkoutURL, _ := mustResolveBootstrapURLs(*flagWebsiteAddress)
	var ideRef *ide.IDE
	opts = append(opts,
		ide.WithReleaseManager(releaseManager),
		ide.WithPlanSource(ide.PlanSourceConfig{
			Source:      ideplan.NewJWTSource(client.CachedTokenSource(), nil),
			CheckoutURL: checkoutURL,
			OnReSignIn: func() {
				lockdownReSignIn(client, ideRef, scheduleNextTick)
			},
		}),
	)

	i, err := ide.New(*flagWorkspace, *flagConfigPath,
		*flagDataPath, storage, opts...)
	if err != nil {
		fmt.Printf("%s", err)
		return 1
	}
	ideRef = i

	err = subscribeOtherCommands(i, client, *flagConfigPath)
	if err != nil {
		log.Errorf("subscribe to commands: %v", err)
	}

	openFiles(i, filenames)

	scheduleCrashReportCheck(i, client, *flagDataPath, scheduleNextTick)

	upgradeCtx, upgradeCancel := context.WithCancel(context.Background())
	defer upgradeCancel()
	upgradeMgr := scheduleUpgradeCheck(upgradeCtx, i, apiclient.DefaultDownloadsHost,
		scheduleNextTick)
	if err := subscribeUpgradeCommands(i, upgradeMgr); err != nil {
		log.Errorf("subscribe upgrade commands: %v", err)
	}
	defer func() {
		_ = upgradeMgr.Close()
	}()

	var ret error
	if err := doRunTUI(mu, i); err != nil {
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

func runGUI(
	filenames []string, runner ide.ExtensionsRunner,
	mu *sync.Mutex, pathDone <-chan error,
) int {
	setEnvForGUI(*flagDataPath)

	// Capture the launch command for guiwindownew. Visit iterates
	// only over flags that were explicitly set (including
	// macOS-injected defaults after the second flag.Parse), so
	// positional filename args are naturally excluded.
	execPath, _ := os.Executable()
	var launchArgs []string
	flag.CommandLine.Visit(func(f *flag.Flag) {
		launchArgs = append(launchArgs, fmt.Sprintf("--%s=%s", f.Name, f.Value.String()))
	})
	launchCmd := append([]string{execPath}, launchArgs...)

	chdirerr := os.Chdir(home)
	if chdirerr != nil {
		chdirerr = fmt.Errorf("cd %s: %w", home, chdirerr)
	}

	publishChan := make(chan term.Event, 4096)
	publishEvent := func(ev term.Event) bool {
		select {
		case publishChan <- ev:
			return true
		default:
			return false
		}
	}

	checkoutURL, signupURL := mustResolveBootstrapURLs(*flagWebsiteAddress)
	root, err := newBootstrapHandler(
		*flagDataPath, *flagConfigPath,
		*flagWorkspace, *flagZdotDir, filenames,
		launchCmd, runner, mu, publishEvent,
		checkoutURL, signupURL,
		func(u *url.URL) error { return extbrowser.Browse(u) },
		text.NewSystemClipboard(),
	)
	if err != nil {
		fmt.Printf("ide: %s", err)
		log.Errorf("ide: %v", err)
		return 1
	}

	browser := root.browser()
	if chdirerr != nil {
		_, _ = browser.Notify(browserapi.LevelError, "%v", chdirerr)
	}
	cfg, ok, err := getGUIConfig(root.config())
	if err != nil {
		_, _ = browser.Notify(browserapi.LevelError, "%v", err)
	}
	if !ok {
		cfg = config.NopConfig()
	}

	env := getGUIEnvVars(browser, cfg)
	go debug.CapturePanicReport(func() {
		if err := <-pathDone; err != nil {
			_, _ = browser.Notify(browserapi.LevelError,
				"could not resolve your login shell PATH; tools on it "+
					"(e.g. homebrew, mise) may be unavailable: %v", err)
		}
		env.Iterate(func(k string, value any) {
			os.Setenv(k, evalVar(value))
		})
	})

	defaultColorTheme := getGUIDefaultColorTheme(browser, cfg)
	themes := getGUIColorThemes(browser, cfg)
	transparentWindow := getGUITransparentWindow(browser, cfg)
	options := []gui.Option{
		gui.WithColorThemes(defaultColorTheme, themes),
		gui.WithFontDPI(getGUIFontDPI(browser, cfg)),
		gui.WithFontSize(getGUIFontSize(browser, cfg)),
		gui.WithFontFamily(getGUIFontFamily(browser, cfg)),
		gui.WithColumnWidthOffset(getGUIColumnWidthOffset(browser, cfg)),
		gui.WithLineHeightOffset(getGUILineHeightOffset(browser, cfg)),
		gui.WithScrollMultiplier(getGUIScrollMultiplier(browser, cfg)),
		gui.WithRenderOffset(0, 10),
		gui.WithLigatures(getGUILigatures(browser, cfg)),
		gui.WithTransparentWindow(transparentWindow),
		gui.WithBackgroundBlur(getGUIBackgroundBlur(browser, cfg)),
		gui.WithPublishChannel(publishChan),
		gui.WithLocker(mu),
		gui.WithPrintFPS(*flagFPS),
		gui.WithKeyMapping(getGUIKeyMapping(browser, cfg)),
	}

	storage := root.storage
	width, height, ok := getLastSize(storage)
	if ok {
		options = append(options, gui.WithSize(width, height))
	}
	x, y, ok := getLastPosition(storage)
	if ok {
		options = append(options, gui.WithPosition(x, y))
	}

	g, err := gui.New(root, options...)
	if err != nil {
		fmt.Printf("gui: %s", err)
		log.Errorf("gui: %v", err)
		return 1
	}
	defer func() { _ = root.Close() }()
	root.attachGUI(g, transparentWindow)

	if fg, bg := getGUIWindowOpacity(browser, cfg); transparentWindow && (fg != 1 || bg != 1) {
		g.SetOpacity(bg, fg)
	}
	if root.alreadyBootstrapped() {
		if err := root.setupConfiguredIDE(root.realIDE, root.client); err != nil {
			fmt.Printf("setup ide: %s", err)
			log.Errorf("setup ide: %v", err)
			return 1
		}
	}
	err = g.Run("Rune")
	if err != nil && !errors.Is(err, gui.ErrHandlerExited) {
		fmt.Printf("%s", err)
		log.Errorf("run: %v", err)
		saveLastSize(storage, g)
		saveLastPosition(storage, g)
		return 1
	}

	saveLastSize(storage, g)
	saveLastPosition(storage, g)
	return 0
}

// newAPIClient constructs the production apiclient.Client and its
// release.Manager from a shared storage. Both can be passed to
// ide.New via WithReleaseManager / WithPlanSource without creating an
// IDE → apiclient → IDE cycle.
func newAPIClient(storage storageapi.Service) (*apiclient.Client, release.Manager) {
	apicfg := apiclient.DefaultConfig()
	apicfg.HTTPEndpointAddress = *flagHTTPAddress
	apicfg.GRPCEndpointAddress = *flagGRPCAddress
	apicfg.InsecureTransport = *flagGRPCInsecure
	apicfg.TelemetryPeriod = *flagTelemetryPeriod
	apicfg.ReleaseCollection = *flagReleaseCollection
	apicfg.WebsiteAddress = *flagWebsiteAddress
	client := apiclient.New(storage, apicfg, *flagDataPath)
	httpClient := oauth2.NewClient(context.Background(), client.OAuthTokenSource())
	arch := fmt.Sprintf("%s-%s", runtime.GOOS, runtime.GOARCH)
	releaseManager := cdnrelease.NewManager(httpClient, *flagHTTPAddress+"/api/releases/"+arch)
	return client, releaseManager
}

// newBootstrapAPIClient builds a slim apiclient.Client suitable for
// driving the pre-swap login prompt. Telemetry is disabled and the
// client is NOT subscribed to IDE events or used as a release manager
// source — those wirings live in setupReleaseManager and belong to the
// configured IDE only. Tokens acquired by this client are written to
// the shared on-disk auth partition keyed by dataDir, so the real
// client constructed after the swap reads them back transparently.
func newBootstrapAPIClient(storage storageapi.Service, openBrowser func(*url.URL) error) *apiclient.Client {
	apicfg := apiclient.DefaultConfig()
	apicfg.HTTPEndpointAddress = *flagHTTPAddress
	apicfg.GRPCEndpointAddress = *flagGRPCAddress
	apicfg.InsecureTransport = *flagGRPCInsecure
	apicfg.TelemetryPeriod = *flagTelemetryPeriod
	apicfg.ReleaseCollection = *flagReleaseCollection
	apicfg.WebsiteAddress = *flagWebsiteAddress
	apicfg.EnableTelemetry = false
	apicfg.OpenBrowser = openBrowser
	return apiclient.New(storage, apicfg, *flagDataPath)
}

func doRunTUI(mu *sync.Mutex, i *ide.IDE) error {
	err := tui.Run(i.Ready(),
		tui.WithLocker(mu),
		tui.WithDefaultAttributes(i.DefaultAttributes()),
		tui.WithInputMode(i.InputMode()),
	)
	if err != nil {
		return fmt.Errorf("tui run: %w", err)
	}

	return nil
}

func openFiles(i *ide.IDE, filenames []string) {
	for _, file := range filenames {
		uri, err := workspaceapi.CurrentUserHostURI(file)
		if err != nil {
			err = fmt.Errorf("get uri: %w", err)
			_, _ = i.Notifications().Notify(browserapi.LevelError, err.Error())
			continue
		}
		err = i.Open(uri)
		if err != nil {
			_, _ = i.Notifications().Notify(browserapi.LevelError, err.Error())
			continue
		}
	}
}

func evalVar(value any) (ret string) {
	str, ok := value.(string)
	if !ok {
		ret = fmt.Sprintf("%v", value)
		return
	}
	ret = os.ExpandEnv(str)
	return
}
