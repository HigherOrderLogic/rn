package main

import (
	"context"
	"net/http"
	_ "net/http/pprof"
	"strconv"
	"sync"
	"sync/atomic"

	multierr "github.com/ernestrc/go-multierror"
	"github.com/ernestrc/go-tui/browser"
	"github.com/ernestrc/go-tui/plugin"
	"github.com/ernestrc/go-tui/proto"
	"github.com/ernestrc/go-tui/term"
	"github.com/ernestrc/go-tui/text"
	"github.com/ernestrc/go-tui/workspace"
	log "github.com/sirupsen/logrus"
)

const (
	cmdSplitWindowTerminal = "splitWindowTerminal"
	cmdTerminalTab         = "newTerminal"
)

var (
	requiredPermissions = []plugin.Permission{
		plugin.PermissionBrowserWindowManager,
		plugin.PermissionEditor,
		plugin.PermissionWorkspace,
		plugin.PermissionBrowserEventPublisher,
		plugin.PermissionBrowserMessenger,
	}
	commands = []string{
		cmdSplitWindowTerminal,
		cmdTerminalTab,
	}
)

type emulatorGrantee struct {
	mu     sync.Mutex
	broker proto.MuxBroker

	wp workspace.Workspace
	wm browser.WindowManager
	p  browser.EventPublisher
	ed text.Editor
	m  browser.Messenger

	defattr    term.Attributes
	shell      string
	initialCmd string
	counter    int32
}

func (e *emulatorGrantee) Connected(broker proto.MuxBroker, config plugin.Config) {
	e.mu.Lock()
	defer e.mu.Unlock()

	log.Infof("plugin connected; config: %#v", config)
	e.broker = broker

	attr, err := plugin.GetAttributes(config, "attr")
	if err != nil && err != plugin.ErrNotFound {
		log.Errorf("coud not read 'attr' property: %v", err)
		return
	}
	e.defattr = attr

	shell, err := config.GetString("shell")
	if err != nil && err != plugin.ErrNotFound {
		log.Errorf("coud not read 'shell' property: %v", err)
		return
	}
	e.shell = shell

	initialCmd, err := config.GetString("cmd")
	if err != nil && err != plugin.ErrNotFound {
		log.Errorf("coud not read 'initalCmd' property: %v", err)
		return
	}
	e.initialCmd = initialCmd
}

func (e *emulatorGrantee) PermissionGranted(grants []plugin.Grant) {
	log.Infof("permissions granted: %v", grants)

	var err error
	for _, g := range grants {
		switch g.Permission {
		case plugin.PermissionBrowserEventPublisher:
			e.p, err = plugin.EventPublisher(g.Token, e.broker)
		case plugin.PermissionBrowserWindowManager:
			e.wm, err = plugin.WindowManager(g.Token, e.broker)
		case plugin.PermissionWorkspace:
			e.wp, err = plugin.Workspace(g.Token, e.broker)
		case plugin.PermissionBrowserMessenger:
			e.m, err = plugin.Messenger(g.Token, e.broker)
		case plugin.PermissionEditor:
			e.ed, err = plugin.Editor(g.Token, e.broker)
			if err == nil {
				for _, cmd := range commands {
					subsErr := e.ed.SubscribeCommand(cmd, e)
					if subsErr != nil {
						err = multierr.Append(err, subsErr)
					}
				}
			}
		}
		if err != nil {
			log.Errorf("PermissionGranted: %+v: %s", g.Permission, err)
		}
	}
}

func (e *emulatorGrantee) PermissionDenied(perms []plugin.Permission) {
	log.Fatalf("Could not start plugin due to missing permissions: "+
		"denied: %v; required: %v", perms, requiredPermissions)
}

func (e *emulatorGrantee) Shutdown(reason string) error {
	log.Debugf("plugin being shutdown: %s", reason)
	return nil
}

func (e *emulatorGrantee) Health() error {
	return nil
}

func (e *emulatorGrantee) HandleCommand(
	ctx context.Context, cmd text.Command,
) (exit bool) {
	switch cmd.Name {
	case cmdSplitWindowTerminal, cmdTerminalTab:
	default:
		log.Warningf("HandleCommand: unknown command %q", cmd.Name)
		return
	}

	counter := atomic.AddInt32(&e.counter, 1)
	log.Tracef("HandleCommand: creating new emulator handler")
	h, err := NewHandler(e.wm, e.wp, e.p, e.m, e.defattr, e.shell,
		e.initialCmd, strconv.Itoa(int(counter)))
	if err != nil {
		log.Errorf("NewHandler: %s", err)
		return
	}

	log.Tracef("HandleCommand: created new emulator handler: %p", h)

	switch cmd.Name {
	case cmdSplitWindowTerminal:
		_, err = e.wm.Split(browser.OrientationDefault, h)
		if err != nil {
			log.Errorf("Split: %s", err)
			_ = h.Close()
			return
		}
	case cmdTerminalTab:
		uri, err := h.URI()
		if err != nil {
			log.Errorf("URI: %s", err)
			_ = h.Close()
			return
		}
		t, err := e.wm.Tab(uri, h.Title(), h)
		if err != nil {
			_ = h.Close()
			log.Errorf("Tab: %s", err)
			return
		}
		win, err := e.wm.Focus()
		if err != nil {
			_ = h.Close()
			log.Errorf("Focus: %s", err)
			return
		}
		err = win.SetContent(t)
		if err != nil {
			_ = h.Close()
			log.Errorf("SetContent: %s", err)
			return
		}
	}
	log.Debugf("HandleCommand: success %q", cmd.Name)
	return
}

func main() {
	go func() {
		log.Println(http.ListenAndServe("localhost:6068", nil))
	}()

	s := emulatorGrantee{}
	plugin.Serve(&s, requiredPermissions...)
}
