package plugin

import (
	"fmt"
	"os/exec"

	"github.com/hashicorp/go-plugin"
	log "github.com/sirupsen/logrus"
)

const (
	envLogLevel = "go_tui_log_level"
)

func makeLogLevelEnv(l log.Level) string {
	return fmt.Sprintf("%s=%s", envLogLevel, l)
}

func goPluginGranteeBuilder(m *Manager) pluginBuilder {
	return func(pluginID, path string, grantor Grantor, logger *log.Logger) (*granteeClient, error) {
		pluginMap := map[string]plugin.Plugin{
			typeGranteePlugin: &granteePlugin{broker: m.broker, logger: logger, grantor: grantor},
		}

		cmd := exec.Command(path)
		// if URI is zero-valued, then Path returns an empty string
		// which fits the default in exec.Cmd.Dir which is to not
		// set the command's dir.
		cmd.Dir = m.config.workspace.Path()
		cmd.Env = append(cmd.Env, makeBrokerRemoteAddrEnv(m.brokerAddr.String()))
		cmd.Env = append(cmd.Env, makeLogLevelEnv(logger.GetLevel()))

		config := &plugin.ClientConfig{
			HandshakeConfig:  handshakeConfig,
			Plugins:          pluginMap,
			Cmd:              cmd,
			Logger:           NewHCLogLogrus(logger),
			AllowedProtocols: []plugin.Protocol{plugin.ProtocolGRPC},
			// TODO we should validate integrity of plugins
			// SecureConfig:    &secureCfg,
		}
		client := plugin.NewClient(config)

		rpcClient, err := client.Client()
		if err != nil {
			return nil, err
		}

		raw, err := rpcClient.Dispense(typeGranteePlugin)
		if err != nil {
			return nil, err
		}

		grantee := raw.(*granteeClient)
		grantee.bindPluginClient(client)

		return grantee, nil
	}
}
