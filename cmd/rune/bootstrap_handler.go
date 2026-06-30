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
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/unstablebuild/blue/release"
	"github.com/unstablebuild/rune-go-sdk/api/browserapi"
	"github.com/unstablebuild/rune-go-sdk/api/config"
	"github.com/unstablebuild/rune-go-sdk/api/storageapi"
	"github.com/unstablebuild/rune-go-sdk/clipboard"
	"github.com/unstablebuild/rune-go-sdk/component"
	sdkhandler "github.com/unstablebuild/rune-go-sdk/handler"
	"github.com/unstablebuild/rune-go-sdk/term"
	"github.com/unstablebuild/rune-go-sdk/tui"
	"unstable.build/go-tui/browser"
	"unstable.build/go-tui/cmd/rune/ide/apiclient"
	"unstable.build/go-tui/debug"
	"unstable.build/go-tui/ide"
	"unstable.build/go-tui/ide/idepkg"
	"unstable.build/go-tui/ide/ideplan"
	"unstable.build/go-tui/ide/ideupgrade"
	"unstable.build/go-tui/term/gui"
)

type bootstrapHandler struct {
	inner             tui.Handler
	dataDir           string
	storage           storageapi.Service
	configPath        string
	workspace         string
	zdotDir           string
	filenames         []string
	launchCmd         []string
	runner            ide.ExtensionsRunner
	mu                *sync.Mutex
	publishEvent      func(term.Event) bool
	checkoutURL       string
	signupURL         string
	openBrowser       func(*url.URL) error
	clip              clipboard.Register
	preIDE            *ide.IDE
	realIDE           *ide.IDE
	g                 *gui.GUI
	transparentWindow bool
	initialThemeAttr  term.Attributes
	client            *apiclient.Client
	upgradeMgr        *ideupgrade.Manager
	upgradeCancel     context.CancelFunc
	bootstrapClient   *apiclient.Client
	lastTabsClick     time.Time
	clickCount        int
	lastResizeW       int
	lastResizeH       int
	chosenEditor      string
	closingPreIDE     bool
}

func newBootstrapHandler(
	dataDir, configPath, workspace, zdotDir string,
	filenames, launchCmd []string,
	runner ide.ExtensionsRunner,
	mu *sync.Mutex,
	publishEvent func(term.Event) bool,
	checkoutURL, signupURL string,
	openBrowser func(*url.URL) error,
	clip clipboard.Register,
) (*bootstrapHandler, error) {
	bh := &bootstrapHandler{
		dataDir:      dataDir,
		storage:      newRuneStorage(dataDir),
		configPath:   configPath,
		workspace:    workspace,
		zdotDir:      zdotDir,
		filenames:    filenames,
		launchCmd:    launchCmd,
		runner:       runner,
		mu:           mu,
		publishEvent: publishEvent,
		checkoutURL:  checkoutURL,
		signupURL:    signupURL,
		openBrowser:  openBrowser,
		clip:         clip,
	}

	if isBootstrapped(dataDir) {
		client, releaseManager := newAPIClient(bh.storage)
		realIDE, err := bh.buildConfiguredIDE(client, releaseManager, false)
		if err != nil {
			_ = client.Close()
			return nil, fmt.Errorf("build configured ide: %w", err)
		}
		bh.client = client
		bh.realIDE = realIDE
		bh.inner = realIDE.Ready()
		return bh, nil
	}

	preIDE, err := bh.buildPreIDE()
	if err != nil {
		return nil, fmt.Errorf("new pre-config ide: %w", err)
	}
	bh.preIDE = preIDE
	bh.inner = preIDE.Ready()
	client := newBootstrapAPIClient(preIDE.Storage(), openBrowser)
	bh.bootstrapClient = client
	bh.openBootstrapFlow()
	return bh, nil
}

func isBootstrapped(dataDir string) bool {
	if _, err := os.Stat(filepath.Join(dataDir, configFilename)); err == nil {
		return true
	}
	if _, err := os.Stat(filepath.Join(dataDir, configStarFilename)); err == nil {
		return true
	}
	return false
}

func (b *bootstrapHandler) buildPreIDE() (*ide.IDE, error) {
	opts := []ide.Option{
		ide.WithExtensionsRunner(b.runner),
		ide.WithLocker(b.mu),
		ide.WithDefaultConfigStarlark(defaultStarlarkConfig, true, false),
		ide.WithDefaultWallpaper(makeWallpaper()),
		ide.WithTabBarOffset(13),
		ide.WithTabBarHeight(2),
		ide.WithWorkspacesBarHeight(2),
		ide.WithWorkspacesBarOffset(2),
		ide.WithWorkspacesIcon('1'),
		ide.WithWorkspacesBarFrame(false),
		ide.WithStreamingOpen(true),
		ide.WithBell(func() {}),
		ide.WithPublishEvent(b.publishEvent),
		ide.WithScheduleNextTick(b.scheduleNextTick),
		ide.WithZdotDir(b.zdotDir),
		ide.WithTabsClickCallback(b.handleTabsClick),
		ide.WithoutHomePrompt(),
	}
	preIDE, err := ide.New("", b.configPath, b.dataDir, b.storage, opts...)
	if err != nil {
		return nil, err
	}
	b.applyInitialThemeAttr(preIDE)
	return preIDE, nil
}

func (b *bootstrapHandler) buildConfiguredIDE(
	client *apiclient.Client, releaseManager release.Manager,
	startingTutorial bool,
) (*ide.IDE, error) {
	opts := []ide.Option{
		ide.WithExtensionsRunner(b.runner),
		ide.WithInitShader(initShader, initShaderFPS, initShaderDuration),
		ide.WithShutdownShader(shutdownShader, shutdownShaderFPS, shutdownShaderDuration),
		ide.WithLoadingShader(loadingShader, loadingShaderFPS, loadingShaderDuration),
		ide.WithOpenShader(openShader, openShaderFPS, openShaderDuration),
		ide.WithStreamingOpen(true),
		ide.WithLocker(b.mu),
		ide.WithConfigFilename(workspaceConfigFilename),
		ide.WithDefaultWallpaper(makeThemedWallpaper(b.wallpaperTheme)),
		ide.WithTabBarOffset(13),
		ide.WithTabBarHeight(2),
		ide.WithWorkspacesBarHeight(2),
		ide.WithWorkspacesBarOffset(2),
		ide.WithWorkspacesIcon('1'),
		ide.WithWorkspacesBarFrame(false),
		ide.WithDefaultConfigStarlark(defaultStarlarkConfig, true, false),
		ide.WithBell(func() {}),
		ide.WithPublishEvent(b.publishEvent),
		ide.WithScheduleNextTick(b.scheduleNextTick),
		ide.WithZdotDir(b.zdotDir),
		ide.WithScheme(docsScheme, newDocsSchemeFunc(b.configPath)),
		ide.WithTabsClickCallback(b.handleTabsClick),
		ide.WithPackageConfigMergeHook(b.guiEnvLiveApplyHook),
		ide.WithDispatchOnPreview(cmdSetTheme,
			func(cmd string, args ...string) (component.Responsive, func(), bool) {
				if cmd != cmdSetTheme || b.g == nil {
					return nil, nil, false
				}
				if len(args) == 0 {
					return nil, nil, false
				}
				theme := b.g.Theme()
				_, err := b.g.SetTheme(args[0])
				if err != nil {
					return nil, nil, false
				}
				return nil, func() {
					theme, err := b.g.SetTheme(theme)
					if err == nil && b.realIDE != nil {
						b.realIDE.SetDefaultAttributes(term.Attributes{
							Fg: term.FromTcellColor(theme.Foreground),
							Bg: term.FromTcellColor(theme.Background),
						})
					}
				}, true
			}),
	}
	opts = append(opts, embeddedTutorialOptions()...)
	if startingTutorial {
		opts = append(opts, ide.WithStartingTutorial("basics"))
	}
	if debug.DebugBuild == "true" {
		opts = append(opts, ide.WithDebugCommands(true))
	}
	opts = append(opts,
		ide.WithReleaseManager(releaseManager),
		ide.WithPlanSource(ide.PlanSourceConfig{
			Source:      ideplan.NewJWTSource(client.CachedTokenSource(), nil),
			CheckoutURL: b.checkoutURL,
			OnReSignIn: func() {
				lockdownReSignIn(client, b.realIDE, b.scheduleNextTick)
			},
		}),
	)
	realIDE, err := ide.New(b.workspace, b.configPath, b.dataDir,
		b.storage, opts...)
	if err != nil {
		return nil, err
	}
	b.applyInitialThemeAttr(realIDE)
	return realIDE, nil
}

func (b *bootstrapHandler) attachGUI(g *gui.GUI, transparentWindow bool) {
	b.g = g
	b.transparentWindow = transparentWindow
}

// wallpaperTheme reports the live GUI theme name for the themed wallpaper.
// It returns "" before the GUI is attached, which selects the default logo.
func (b *bootstrapHandler) wallpaperTheme() string {
	if b.g == nil {
		return ""
	}
	return b.g.Theme()
}

// applyInitialThemeAttr must run before Ready(): Ready() captures
// defAttr to materialize the init shader, so a later
// SetDefaultAttributes would not propagate into the running shader
// (RUNE-203). Seeding both the pre-bootstrap and configured IDEs
// with the same attrs also avoids a color jump across performSwap.
func (b *bootstrapHandler) applyInitialThemeAttr(i *ide.IDE) {
	b.initialThemeAttr = resolveInitialThemeAttr(i.Browser(), i.Config())
	i.SetDefaultAttributes(b.initialThemeAttr)
}

func (b *bootstrapHandler) browser() browser.Browser {
	if b.realIDE != nil {
		return b.realIDE.Browser()
	}
	return b.preIDE.Browser()
}

func (b *bootstrapHandler) notifications() browserapi.Notifications {
	return bootstrapNotifications{b: b}
}

// notifyError surfaces a bootstrap-flow failure to the user. The
// bootstrap UI has no log pane, so log-only reporting is invisible.
func (b *bootstrapHandler) notifyError(context string, err error) {
	log.Errorf("bootstrap %s: %v", context, err)
	_, nerr := b.notifications().Notify(browserapi.LevelError, "%s: %v", context, err)
	if nerr != nil {
		log.Warnf("bootstrap %s: notify: %v", context, nerr)
	}
}

func (b *bootstrapHandler) alreadyBootstrapped() bool {
	return b.realIDE != nil
}

func (b *bootstrapHandler) config() config.Config {
	if b.realIDE != nil {
		return b.realIDE.Config()
	}
	return b.preIDE.Config()
}

func (b *bootstrapHandler) guiEnvLiveApplyHook(
	event idepkg.ConfigMergeEvent,
) (idepkg.ConfigMergeResult, error) {
	if !event.TouchesPath("gui", "env") {
		return idepkg.ConfigMergeResult{}, nil
	}
	rootCfg, err := ide.Config(b.configPath)
	if err != nil {
		return idepkg.ConfigMergeResult{}, fmt.Errorf("reload config for gui.env: %w", err)
	}
	guiCfg, ok, err := getGUIConfig(rootCfg)
	if err != nil {
		return idepkg.ConfigMergeResult{}, fmt.Errorf("load gui config: %w", err)
	}
	if !ok {
		return idepkg.ConfigMergeResult{}, nil
	}
	if env, err := getGUIEnvVars(guiCfg); err != nil {
		return idepkg.ConfigMergeResult{}, fmt.Errorf("decode gui.env: %w", err)
	} else if err := applyGUIEnvVars(env); err != nil {
		return idepkg.ConfigMergeResult{}, fmt.Errorf("apply gui.env: %w", err)
	}
	return idepkg.ConfigMergeResult{LiveApplied: true}, nil
}

func (b *bootstrapHandler) setupConfiguredIDE(
	i *ide.IDE, client *apiclient.Client,
) error {
	i.SetDefaultAttributes(b.initialThemeAttr)

	b.client = client
	scheduleCrashReportCheck(i, client, b.dataDir, b.scheduleNextTick)

	var errs []error
	if err := subscribeCommands(b.g, client, i,
		b.transparentWindow, b.configPath, b.launchCmd); err != nil {
		errs = append(errs, fmt.Errorf("subscribe to GUI commands: %w", err))
	}

	openFiles(i, b.filenames)

	upgradeCtx, upgradeCancel := context.WithCancel(context.Background())
	b.upgradeCancel = upgradeCancel
	b.upgradeMgr = scheduleUpgradeCheck(upgradeCtx, i,
		apiclient.DefaultDownloadsHost, b.scheduleNextTick)
	if err := subscribeUpgradeCommands(i, b.upgradeMgr); err != nil {
		errs = append(errs, fmt.Errorf("subscribe upgrade commands: %w", err))
	}
	return errors.Join(errs...)
}

func (b *bootstrapHandler) scheduleNextTick(fn func()) bool {
	return b.publishEvent(term.Event{Type: term.EventInterrupt, UserFunc: fn})
}

// handleTabsClick maximizes the GUI window on a double-click of the
// tabs bar. Installed on both buildPreIDE and buildConfiguredIDE so
// the affordance is available during the bootstrap UI as well as the
// configured IDE.
func (b *bootstrapHandler) handleTabsClick(_ int) bool {
	if b.clickCount == 0 || time.Since(b.lastTabsClick) < doubleClickTimeout {
		b.clickCount++
	} else {
		b.clickCount = 1
	}
	b.lastTabsClick = time.Now()
	if b.clickCount == 2 && b.g != nil {
		b.g.MaximizeWindow()
		return true
	}
	return false
}

func (b *bootstrapHandler) Resize(w, h int) {
	b.lastResizeW = w
	b.lastResizeH = h
	b.inner.Resize(w, h)
}

func (b *bootstrapHandler) Draw(w term.Writer) { b.inner.Draw(w) }

func (b *bootstrapHandler) Cursor() (term.Coordinates, term.CursorStyle, bool) {
	return b.inner.Cursor()
}

func (b *bootstrapHandler) Selection() (string, bool) {
	return b.inner.Selection()
}

func (b *bootstrapHandler) Handle(ev term.Event) (exit, handled bool) {
	if b.realIDE == nil && shouldSwallowBootstrapEvent(ev) {
		return false, true
	}
	return b.inner.Handle(ev)
}

func (b *bootstrapHandler) performSwap() error {
	if err := b.writeOverrideConfig(); err != nil {
		return fmt.Errorf("write bootstrap override: %w", err)
	}

	b.configPath = filepath.Join(b.dataDir, configFilename)

	b.mu.Unlock()
	defer b.mu.Lock()

	client, releaseManager := newAPIClient(b.storage)
	realIDE, err := b.buildConfiguredIDE(client, releaseManager, true)
	if err != nil {
		_ = client.Close()
		return fmt.Errorf("build configured ide: %w", err)
	}
	b.realIDE = realIDE
	setupErr := b.setupConfiguredIDE(realIDE, client)
	b.inner = realIDE.Ready()
	if b.lastResizeW > 0 && b.lastResizeH > 0 {
		b.inner.Resize(b.lastResizeW, b.lastResizeH)
	}

	var closeErr error
	if b.preIDE != nil {
		b.closingPreIDE = true
		if cerr := b.preIDE.Close(); cerr != nil {
			closeErr = fmt.Errorf("close pre-config ide: %w", cerr)
		}
		b.preIDE = nil
	}
	return errors.Join(setupErr, closeErr)
}

func (b *bootstrapHandler) writeOverrideConfig() error {
	body, err := renderOverride(b.chosenEditor)
	if err != nil {
		return fmt.Errorf("render override: %w", err)
	}
	path := filepath.Join(b.dataDir, configFilename)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		return fmt.Errorf("write override config %q: %w", path, err)
	}
	return nil
}

func (b *bootstrapHandler) Close() error {
	b.closingPreIDE = true
	var errs []error
	if b.upgradeMgr != nil {
		if err := b.upgradeMgr.Close(); err != nil {
			errs = append(errs, err)
		}
		b.upgradeMgr = nil
	}
	if b.upgradeCancel != nil {
		b.upgradeCancel()
		b.upgradeCancel = nil
	}
	if b.client != nil {
		if err := b.client.Close(); err != nil {
			errs = append(errs, err)
		}
		b.client = nil
	}
	if b.realIDE != nil {
		if err := b.realIDE.Close(); err != nil {
			errs = append(errs, err)
		}
		b.realIDE = nil
	}
	if b.preIDE != nil {
		if err := b.preIDE.Close(); err != nil {
			errs = append(errs, err)
		}
		b.preIDE = nil
	}
	if b.storage != nil {
		if err := b.storage.Close(); err != nil {
			errs = append(errs, err)
		}
		b.storage = nil
	}
	return errors.Join(errs...)
}

const (
	editorModal    = "modal"
	editorModeless = "modeless"
)

// Vim-mode option labels double as map keys in optionToChoice; they
// must stay byte-identical between the prompt and the callback.
const (
	optVimYes = "   Yes   "
	optVimNo  = "   No    "
)

var (
	bootstrapVimKeys = []term.KeyComb{
		{Ch: 'y'}, {Ch: 'n'},
	}

	bootstrapWelcomeKeys = []term.KeyComb{
		{Ch: 'g'},
	}

	bootstrapLoginChoiceKeys = []term.KeyComb{
		{Ch: 'l'}, {Ch: 's'},
	}

	bootstrapLoginWaitKeys = []term.KeyComb{
		{Ch: 'p'}, {Ch: 'c'},
	}

	bootstrapUpgradeKeys = []term.KeyComb{
		{Ch: 'u'},
	}
)

const (
	optWelcomeGo = " Let's go "

	optLoginSignIn = "  Sign in  "
	optLoginSignUp = "  Sign up  "
	optLoginCancel = "  Cancel  "
	optUpgradePro  = "  Upgrade to Pro  "
	optLoginCopy   = "  Copy URL  "
)

func (b *bootstrapHandler) openBootstrapFlow() {
	b.openWelcomePrompt()
}

func (b *bootstrapHandler) openWelcomePrompt() {
	msg := "## Welcome to Rune\n\n" +
		"Glad you're here. Let's get everything set up.\n\n" +
		"Learning a new editor is hard, and it can feel daunting at first. We've all been there. " +
		"These first steps are designed to make that process easier, and we promise that once Rune starts to click, " +
		"the payoff will be huge."
	guard := b.promptGuard()
	b.preIDE.Prompt(
		msg,
		[]string{optWelcomeGo},
		bootstrapWelcomeKeys,
		sdkhandler.FuncPromptHandler(
			guard.onSelect(func(_ int, _ string) {
				b.openVimPrompt()
			}),
			guard.onClose(b.openWelcomePrompt),
		),
	)
}

func (b *bootstrapHandler) openVimPrompt() {
	msg := "## Choose your key bindings\n" +
		"Rune ships with two built-in editors, so pick the one that feels like home.\n\n" +
		"Know vim? Pick **Yes** and you get it **everywhere**, not just in editor " +
		"buffers: the terminal, input boxes, and the file explorer all share the same " +
		"modes, motions, operators, and macros. The same muscle memory across the " +
		"whole IDE.\n\n" +
		"Used to VS Code, Cursor, or a plain text editor? Pick **No** and Rune uses " +
		"those familiar, standard key bindings everywhere instead.\n\n" +
		"Either choice sets the default editor and key bindings for files, the file " +
		"explorer, and every input across Rune. You can fine-tune it later in your " +
		"config.\n\n" +
		"**Enable vim mode?**"
	guard := b.promptGuard()
	b.preIDE.Prompt(
		msg,
		[]string{optVimYes, optVimNo},
		bootstrapVimKeys,
		sdkhandler.FuncPromptHandler(
			guard.onSelect(func(_ int, option string) {
				b.chosenEditor = optionToChoice(option)
				_ = b.openLoginPrompt()
			}),
			guard.onClose(b.openVimPrompt),
		),
	)
}

func (b *bootstrapHandler) openLoginPrompt() error {
	msg := "## Sign in or sign up\n" +
		"Much of the industry is betting that we won't be writing code for much longer.\n" +
		"Unstable Build is betting that the technologists, the systems programmers, the\n" +
		"hackers who love this craft, the ones who'd rather read the source than the\n" +
		"docs, the ones who check out the branch and run it locally before they approve\n" +
		"the PR, and the ones everyone wants on their on-call rotation when prod is on\n" +
		"fire, will outlive every company betting against them.\n\n" +
		"**Rune is how we stay irreplaceable**.\n\n" +
		"Independent and user-supported. Your subscription keeps it that way.\n\n" +
		"$10/month or $100/year. Cancel anytime."
	guard := b.promptGuard()
	b.preIDE.Prompt(
		msg,
		[]string{optLoginSignIn, optLoginSignUp},
		bootstrapLoginChoiceKeys,
		sdkhandler.FuncPromptHandler(
			guard.onSelect(func(_ int, option string) {
				var err error
				switch option {
				case optLoginSignIn:
					err = b.startLogin()
				case optLoginSignUp:
					err = b.openBrowserURL(b.signupURL)
					b.scheduleNextTick(func() { _ = b.openLoginPrompt() })
				}
				if err != nil {
					b.notifyError("login could not start", err)
				}
			}),
			guard.onClose(func() { _ = b.openLoginPrompt() }),
		),
	)
	return nil
}

// startLogin returns a Purge failure rather than swallowing it: a
// stale cached token would short-circuit Login and silently pin the
// bootstrap to the previous identity.
func (b *bootstrapHandler) startLogin() error {
	// A still-valid cached token would short-circuit TokenCtx and
	// resolve Login without opening the browser, so the user could
	// never switch identities from the bootstrap Sign in button.
	if ts := b.bootstrapClient.CachedTokenSource(); ts != nil {
		if err := ts.Purge(); err != nil {
			return fmt.Errorf("purge cached token: %w", err)
		}
	}
	loginCtx, loginCancel := context.WithCancel(context.Background())
	session := b.bootstrapClient.Login(loginCtx)

	// loginCoord shares state between the URL and Done watchers
	// so the wait prompt is never left stranded when Done resolves
	// before the URL tick lands. The done flag is set on the event
	// loop inside Done's tick, after which the URL tick must be a
	// no-op even if it already had a Window in hand.
	coord := &loginCoord{loginCancel: loginCancel}

	go debug.CapturePanicReport(func() {
		u, ok := <-session.URL
		if !ok {
			return
		}
		b.scheduleNextTick(func() {
			coord.mu.Lock()
			if coord.done || coord.cancelled {
				coord.mu.Unlock()
				return
			}
			coord.mu.Unlock()
			win := b.mountLoginWaitPrompt(u.String(), coord)
			coord.mu.Lock()
			if coord.done || coord.cancelled {
				coord.mu.Unlock()
				if win != nil {
					_ = win.Close()
				}
				return
			}
			coord.window = win
			coord.mu.Unlock()
		})
	})

	go debug.CapturePanicReport(func() {
		err, ok := <-session.Done
		handleLoginDone(loginDoneArgs{
			ok:           ok,
			err:          err,
			coord:        coord,
			scheduleTick: b.scheduleNextTick,
			decide: func(ctx context.Context) (ideplan.Decision, error) {
				source := ideplan.NewJWTSource(b.bootstrapClient.CachedTokenSource(), nil)
				return source.Decision(ctx)
			},
			onSwap:        b.performSwap,
			onUpgrade:     b.openUpgradePrompt,
			onLoginPrompt: b.openLoginPrompt,
			notifyError:   b.notifyError,
		})
	})
	return nil
}

type loginDoneArgs struct {
	ok            bool
	err           error
	coord         *loginCoord
	scheduleTick  func(func()) bool
	decide        func(context.Context) (ideplan.Decision, error)
	onSwap        func() error
	onUpgrade     func() error
	onLoginPrompt func() error
	notifyError   func(context string, err error)
}

// handleLoginDone schedules the wait-window close before any
// follow-up work so the user is never left staring at the "Follow
// browser instructions" prompt while decide() makes a synchronous
// claims read.
func handleLoginDone(a loginDoneArgs) {
	a.scheduleTick(func() {
		a.coord.mu.Lock()
		a.coord.done = true
		win := a.coord.window
		a.coord.window = nil
		a.coord.mu.Unlock()
		if win != nil {
			_ = win.Close()
		}
	})

	a.coord.mu.Lock()
	cancelled := a.coord.cancelled
	a.coord.mu.Unlock()
	if cancelled {
		return
	}
	if !a.ok || a.err != nil {
		if a.err != nil {
			a.notifyError("login", a.err)
		}
		a.scheduleTick(func() {
			if err := a.onLoginPrompt(); err != nil {
				a.notifyError("open login prompt", err)
			}
		})
		return
	}
	dec, derr := a.decide(context.Background())
	if derr != nil {
		a.notifyError("read claims", derr)
		a.scheduleTick(func() {
			if err := a.onLoginPrompt(); err != nil {
				a.notifyError("open login prompt", err)
			}
		})
		return
	}
	if dec.Status == ideplan.StatusActive {
		a.scheduleTick(func() {
			if err := a.onSwap(); err != nil {
				a.notifyError("finish bootstrap", err)
			}
		})
		return
	}
	a.scheduleTick(func() {
		if err := a.onUpgrade(); err != nil {
			a.notifyError("open upgrade prompt", err)
		}
	})
}

// loginCoord serializes the two goroutines that drive the bootstrap
// login wait prompt. Without this coordination the URL goroutine
// could mount a wait prompt on top of whatever the Done goroutine
// has already shown (upgrade prompt, choice prompt, performSwap'd
// IDE), leaving two prompts stacked.
type loginCoord struct {
	mu          sync.Mutex
	window      browser.Window
	done        bool
	cancelled   bool
	loginCancel context.CancelFunc
}

func (b *bootstrapHandler) mountLoginWaitPrompt(
	oauthURL string, coord *loginCoord,
) browser.Window {
	coord.mu.Lock()
	stop := coord.cancelled || coord.done
	coord.mu.Unlock()
	if stop {
		return nil
	}
	header := "**Follow the instructions in your browser.**\n\n"
	msg := header +
		"If your browser did not open automatically, copy this link:\n\n" +
		"`" + oauthURL + "`"
	guard := b.promptGuard()
	return b.preIDE.Prompt(
		msg,
		[]string{optLoginCopy, optLoginCancel},
		bootstrapLoginWaitKeys,
		sdkhandler.FuncPromptHandler(
			guard.onSelect(func(_ int, option string) {
				switch option {
				case optLoginCopy:
					guard.advanced = false // stay in this prompt
					level, msg := b.copyBootstrapURL(oauthURL)
					if _, err := b.notifications().Notify(level, "%s", msg); err != nil {
						log.Warnf("bootstrap copy url: notify: %v", err)
					}
					b.mountLoginWaitPrompt(oauthURL, coord)
				case optLoginCancel:
					coord.mu.Lock()
					coord.cancelled = true
					if coord.window != nil {
						_ = coord.window.Close()
						coord.window = nil
					}
					cancel := coord.loginCancel
					coord.mu.Unlock()
					if cancel != nil {
						cancel()
					}
					_ = b.openLoginPrompt()
				}
			}),
			guard.onClose(func() {
				b.mountLoginWaitPrompt(oauthURL, coord)
			}),
		),
	)
}

// copyBootstrapURL writes the OAuth URL to the clipboard.
func (b *bootstrapHandler) copyBootstrapURL(oauthURL string) (browserapi.NotificationLevel, string) {
	if err := b.clip.Copy(clipboard.DefaultRegisterID, clipboard.Data{Text: oauthURL}); err != nil {
		return browserapi.LevelWarn,
			fmt.Sprintf("copy to clipboard failed: %v", err)
	}
	return browserapi.LevelSuccess, "OAuth URL copied to clipboard"
}

func (b *bootstrapHandler) openUpgradePrompt() error {
	msg := "**Upgrade to Rune Pro to unlock the IDE.**\n\n" +
		"Your account is signed in but has no paid plan. Click below\n" +
		"to open the checkout page in your browser. Once you complete\n" +
		"checkout, return here and click Sign in again."
	guard := b.promptGuard()
	b.preIDE.Prompt(
		msg,
		[]string{optUpgradePro},
		bootstrapUpgradeKeys,
		sdkhandler.FuncPromptHandler(
			guard.onSelect(func(_ int, _ string) {
				if err := b.openBrowserURL(b.checkoutURL); err != nil {
					b.notifyError("open checkout page", err)
				}
				_ = b.openLoginPrompt()
			}),
			guard.onClose(func() { _ = b.openUpgradePrompt() }),
		),
	)
	return nil
}

// mustResolveBootstrapURLs panics on parse failure because in
// practice nobody sets -rune-website-address; the default points at
// production and a parse failure means the build itself is broken.
func mustResolveBootstrapURLs(raw string) (checkout, signup string) {
	base, err := url.Parse(raw)
	if err != nil {
		panic(fmt.Sprintf("parse -rune-website-address %q: %v", raw, err))
	}
	signupBase := *base
	signupBase.Path = "/signup"
	return checkoutURL(base).String(), signupBase.String()
}

func checkoutURL(base *url.URL) *url.URL {
	const (
		checkoutPath   = "/checkout"
		checkoutSource = "rune"
	)
	u := *base
	u.Path = checkoutPath
	q := u.Query()
	q.Set("source", checkoutSource)
	u.RawQuery = q.Encode()
	return &u
}

func (b *bootstrapHandler) openBrowserURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("parse url %q: %w", raw, err)
	}
	if err := b.openBrowser(u); err != nil {
		return fmt.Errorf("open browser: %w", err)
	}
	return nil
}

// guardedPromptChain re-opens a bootstrap prompt that was closed
// without a user selection (Esc, mouse dismissal, …). The SDK
// invokes OnSelect synchronously before OnClose, so "advanced"
// reliably distinguishes the two paths.
type guardedPromptChain struct {
	advanced bool
	closing  func() bool
}

func (g *guardedPromptChain) onSelect(next func(int, string)) func(int, string) {
	return func(idx int, option string) {
		g.advanced = true
		next(idx, option)
	}
}

func (g *guardedPromptChain) onClose(reopen func()) func() error {
	return func() error {
		if !g.advanced && !g.closing() {
			reopen()
		}
		return nil
	}
}

// promptGuard builds the chain guard for a bootstrap prompt. The
// closing check stops the reopen-on-dismiss chain while the preIDE
// is being torn down (shutdown or swap to the configured IDE), where
// every window close fires OnClose and reopening would resurrect
// prompt windows mid-Close forever.
func (b *bootstrapHandler) promptGuard() *guardedPromptChain {
	return &guardedPromptChain{closing: func() bool { return b.closingPreIDE }}
}

// shouldSwallowBootstrapEvent must NOT swallow Esc: the SDK prompt
// relies on Esc to exit, and guardedPromptChain.onClose re-opens
// the prompt right after.
func shouldSwallowBootstrapEvent(ev term.Event) bool {
	if ev.Type != term.EventKey {
		return false
	}
	// preIDE only loads embedded defaults — no user config exists
	// yet — so ':' is hardcoded as the command-prompt activation key.
	if ev.Mod == 0 && ev.Ch == ':' {
		return true
	}
	// Quit / close keybindings from rune.star and
	// override_modeless.yaml:
	//   <m-q> quit
	//   <m-w> windowclose, <a-w> tabclose, <c-w> tabclose
	//   <m-s-w> / <s-m-w> windowclose (modeless overrides)
	if ev.Ch == 'q' && ev.Mod&term.ModMeta != 0 {
		return true
	}
	if ev.Ch == 'w' && ev.Mod != 0 {
		switch {
		case ev.Mod&term.ModMeta != 0,
			ev.Mod&term.ModAlt != 0,
			ev.Mod&term.ModCtrl != 0:
			return true
		}
	}
	return false
}

func optionToChoice(option string) string {
	switch option {
	case optVimNo:
		return editorModeless
	}
	return editorModal
}
