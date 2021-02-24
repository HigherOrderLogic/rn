package util

import (
	"fmt"
	"io"
	"sync"

	"github.com/ernestrc/go-tui"
	"github.com/ernestrc/go-tui/browser"
	"github.com/ernestrc/go-tui/plugin"
	"github.com/ernestrc/go-tui/proto"
	"github.com/ernestrc/go-tui/term"
	log "github.com/sirupsen/logrus"
)

var requiredPermissions = []plugin.Permission{
	plugin.PermissionBrowserWindowManager,
	plugin.PermissionBrowserEventSubscriber,
}

type keySplitHandler struct {
	mu     sync.Mutex
	config KeySplitHandlerConfig

	broker  proto.MuxBroker
	wm      browser.WindowManager
	s       browser.EventSubscriber
	pconfig plugin.Config
	grants  []plugin.Grant
	h       tui.Handler
	win     browser.Window
}

func (t *keySplitHandler) Connected(broker proto.MuxBroker, config plugin.Config) {
	log.Infof("plugin connected; config: %v", config)
	t.broker = broker
	t.pconfig = config
}

func (t *keySplitHandler) closeHandler() bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.h == nil {
		return false
	}

	defer func() {
		t.h = nil
	}()

	if closer, ok := t.h.(io.Closer); ok {
		t.mu.Unlock()
		defer t.mu.Lock()
		err := closer.Close()
		if err != nil {
			log.Errorf("error closing plugin: %v", err)
		}
	}
	return true
}

func (t *keySplitHandler) cleanWindow() bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.win == nil {
		return false
	}

	defer func() {
		t.win = nil
	}()

	t.mu.Unlock()
	defer t.mu.Lock()

	t.closeWindow()

	return true
}

func (t *keySplitHandler) closeWindow() {
	err := t.win.Close()
	if err != nil {
		log.Errorf("error closing plugin window: %s", err)
	}
}

func (t *keySplitHandler) exitClean() {
	log.Info("received exit signal; cleaning resources...")
	if t.cleanWindow() {
		log.Debug("cleaned window")
	}
	if t.closeHandler() {
		log.Debug("closed handler")
	}
}

func (t *keySplitHandler) handleKeyEvent() {
	t.mu.Lock()
	win := t.win
	wm := t.wm
	t.mu.Unlock()

	if win != nil {
		log.Debug("received key event but win is already open")
		return
	}
	if wm == nil {
		log.Warn("could not handle key event: could not resolve wm permission")
		return
	}

	focus, err := wm.Focus()
	if err != nil {
		log.Errorf("failed to get focus: %s", err)
		return
	}

	h, err := t.config.Handler(t.grants, t.broker, focus, t.pconfig)
	if err != nil {
		log.Errorf("error building window handler: %v", err)
		return
	}

	win, err = t.config.Split(t.wm, browser.CallbackHandler(h, t.exitClean))
	if err != nil {
		log.Errorf("error opening new window: %s", err)
		return
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	t.h = h
	t.win = win
}

func (t *keySplitHandler) Handle(ev term.Event) (exit bool) {
	if ev == t.config.Key {
		t.handleKeyEvent()
	}
	return
}

func (t *keySplitHandler) subscribeToEvents() error {
	err := t.s.Subscribe(t.config.Key, t)
	if err != nil {
		return err
	}
	return nil
}

func (t *keySplitHandler) PermissionGranted(grants []plugin.Grant) {
	t.mu.Lock()
	defer t.mu.Unlock()

	var err error
	log.Infof("permissions granted: %+v", grants)

	for _, g := range grants {
		switch g.Permission {
		case plugin.PermissionBrowserWindowManager:
			t.wm, err = plugin.WindowManager(g.Token, t.broker)
		case plugin.PermissionBrowserEventSubscriber:
			t.s, err = plugin.EventSubscriber(g.Token, t.broker)
			if err == nil {
				err = t.subscribeToEvents()
			}
		}
		if err != nil {
			log.Errorf("PermissionGranted: %+v: %s", g.Permission, err)
		}
	}

	t.grants = grants
}

func (t *keySplitHandler) PermissionDenied(perms []plugin.Permission) {
	log.Fatalf("Could not start plugin due to missing permissions: "+
		"denied: %v; required: %v", perms, requiredPermissions)
}

func (t *keySplitHandler) Shutdown(reason string) error {
	log.Warningf("plugin being shutdown: %s", reason)
	t.closeHandler()
	t.cleanWindow()
	return nil
}

func (t *keySplitHandler) Health() error {
	return nil
}

// KeySplitHandlerConfig provides the configuration required to
// use KeySplitHandler plugin.Grantee helper. See KeySplitHandler
// for more details.
type KeySplitHandlerConfig struct {
	Key term.Event

	// Split is one of the following WindowManager split methods:
	//   - SplitVerticalRight(Handler) (Window, error)
	//   - SplitVerticalLeft(Handler) (Window, error)
	//   - SplitHorizontalAbove(Handler) (Window, error)
	//   - SplitHorizontalBelow(Handler) (Window, error)
	Split func(browser.WindowManager, browser.Handler) (browser.Window, error)

	// Handler is the constructor used to install a handler
	// on the split window. The focus argument represents
	// the window in focus when key event was fired.
	// If returned Handler satisfies io.Closer, then Close will be called
	// when split window is closed.
	Handler func([]plugin.Grant, proto.MuxBroker, browser.Window,
		plugin.Config) (tui.Handler, error)

	// Permissions to be requested for Handler.
	Permissions []plugin.Permission
}

// ServeKeySplitHandler serves a plugin.Grantee that opens a split window
// with a new handler when key event is fired. The key subscribed to
// the type of window split and the handler used is configured with config.
// If either is not set, this function panics.
// Note that this function never returns.
func ServeKeySplitHandler(config KeySplitHandlerConfig) {
	if config.Handler == nil || (config.Key == term.Event{}) || config.Split == nil {
		panic(fmt.Sprintf("invalid key split handler configuration: %#v", config))
	}
	perms := append(config.Permissions, requiredPermissions...)
	plugin.Serve(&keySplitHandler{config: config}, perms...)
}
