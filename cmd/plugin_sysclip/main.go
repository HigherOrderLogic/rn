package main

import (
	"sync"

	"github.com/ernestrc/go-tui/plugin"
	"github.com/ernestrc/go-tui/proto"
	"github.com/ernestrc/go-tui/text"

	"github.com/atotto/clipboard"
	log "github.com/sirupsen/logrus"
)

var (
	requiredPermissions = []plugin.Permission{
		plugin.PermissionClipboard,
	}
)

type systemClipboard struct {
	mu         sync.Mutex
	broker     proto.MuxBroker
	registerID string
}

func (c *systemClipboard) Paste() (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	text, err := clipboard.ReadAll()
	if err != nil {
		return "", err
	}
	return text, nil
}

func (c *systemClipboard) Copy(data string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	return clipboard.WriteAll(data)
}

func (c *systemClipboard) Connected(broker proto.MuxBroker, config plugin.Config) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if clipboard.Unsupported {
		log.Fatal("clipboard package does not support this system: terminating plugin")
	}

	log.Infof("plugin connected; config: %#v", config)
	c.broker = broker
	c.registerID = text.DefaultRegisterID

	registerID, err := config.GetString("register")
	if err != nil {
		log.Infof("coud not read 'register' property: %v", err)
		return
	}
	c.registerID = registerID
}

func (c *systemClipboard) PermissionGranted(grants []plugin.Grant) {
	log.Infof("permissions granted: %v", grants)

	grant := grants[0]

	switch grant.Permission {
	case plugin.PermissionClipboard:
		clipboard, err := plugin.GetClipboard(grant.Token, c.broker)
		if err == nil {
			err = clipboard.SetRegister(c.registerID, c)
		}
		if err != nil {
			log.Errorf("PermissionGranted: %+v: %s", grants, err)
		}
	}
}

func (c *systemClipboard) PermissionDenied(perms []plugin.Permission) {
	log.Fatalf("Could not start plugin due to missing permissions: "+
		"denied: %v; required: %v", perms, requiredPermissions)
}

func (c *systemClipboard) Shutdown(reason string) error {
	log.Warningf("plugin being shutdown: %s", reason)
	return nil
}

func (c *systemClipboard) Health() error {
	return nil
}

func main() {
	s := &systemClipboard{}
	plugin.Serve(s, requiredPermissions...)
}
