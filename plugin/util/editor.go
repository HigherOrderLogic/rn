package util

import (
	"io"

	"github.com/ernestrc/go-tui/editor"
	"github.com/ernestrc/go-tui/plugin"
	"github.com/ernestrc/go-tui/proto"
	log "github.com/sirupsen/logrus"
)

// CommandEventHandler combines EventHandler with CommandHandler.
type CommandEventHandler interface {
	editor.EventHandler
	editor.CommandHandler
	io.Closer
}

type editorGrantee struct {
	broker     proto.MuxBroker
	ed         editor.Editor
	handler    CommandEventHandler
	newHandler func(editor.Editor, []plugin.Grant, proto.MuxBroker, plugin.Config) (CommandEventHandler, error)
	cmds       []string
	pconfig    plugin.Config
	err        error
}

func (t *editorGrantee) Connected(broker proto.MuxBroker, config plugin.Config) {
	log.Infof("plugin connected; config: %#v", config)
	t.broker = broker
	t.pconfig = config
}

func (t *editorGrantee) subscribeToEvents(grants []plugin.Grant) error {
	evs := []editor.EventType{
		editor.EventTypeClose,
		editor.EventTypeFlush,
		editor.EventTypeOpen,
		editor.EventTypeInsert,
		editor.EventTypeDelete,
	}
	h, err := t.newHandler(t.ed, grants, t.broker, t.pconfig)
	if err != nil {
		return err
	}
	t.handler = h
	for _, ev := range evs {
		err := t.ed.SubscribeEditor(ev, h)
		if err != nil {
			return err
		}
	}

	for _, cmd := range t.cmds {
		if err = t.ed.Register(cmd, h); err != nil {
			return err
		}
	}
	log.Debugf("subscribed to events %+v", evs)
	return nil
}

func (t *editorGrantee) PermissionGranted(grants []plugin.Grant) {
	log.Infof("permissions granted: %v", grants)

	for _, grant := range grants {
		var err error
		switch grant.Permission {
		case plugin.PermissionEditor:
			t.ed, err = plugin.Editor(grant.Token, t.broker)
			if err == nil {
				err = t.subscribeToEvents(grants)
			}
		}
		if err != nil {
			log.Errorf("PermissionGranted: %+v: %s", grants, err)
		}
	}
}

func (t *editorGrantee) PermissionDenied(perms []plugin.Permission) {
	log.Fatalf("Could not start plugin due to missing permissions: "+
		"denied: %v; required: %v", perms, requiredPermissions)
}

func (t *editorGrantee) Shutdown(reason string) error {
	log.Warningf("plugin being shutdown: %s", reason)
	if t.handler != nil {
		return t.handler.Close()
	}
	return nil
}

func (t *editorGrantee) Health() error {
	return nil
}

// ServeEditorEventHandler calls fn to build a CommandEventHandler,
// subsribes it to all editor events and registers it as the CommandHandler
// of cmds. It also requests extraPerms, in addition to plugin.PermissionEditor.
// All granted permissions are returned in the fn callback. If one of the
// permissions is denied, the plugin will exit with an error.
func ServeEditorEventHandler(
	cmds []string,
	fn func(editor.Editor, []plugin.Grant, proto.MuxBroker, plugin.Config) (CommandEventHandler, error),
	extraPerms ...plugin.Permission,
) {
	perms := []plugin.Permission{
		plugin.PermissionEditor,
	}
	perms = append(perms, extraPerms...)
	s := &editorGrantee{cmds: cmds, newHandler: fn}
	plugin.Serve(s, perms...)
}
