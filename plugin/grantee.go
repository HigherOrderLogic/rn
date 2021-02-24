package plugin

import (
	"context"
	"os"
	"time"

	"github.com/ernestrc/go-tui/proto"
	"github.com/hashicorp/go-plugin"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
)

// Permission represents a type of resource access.
type Permission string

// Permissions is a set of Permission.
type Permissions map[Permission]struct{}

// Grant binds a granted Permission with a Token that
// can be used with the plugin API.
type Grant struct {
	Token uint32
	Permission
}

// Grantee needs to be implemented by plugins that want
// to access plugin host resources.
type Grantee interface {
	Connected(proto.MuxBroker, Config)
	PermissionGranted([]Grant)
	PermissionDenied([]Permission)
	Shutdown(reason string) error
	Health() error
}

// handshakeConfigs are used to just do a basic handshake between
// a plugin and host. If the handshake fails, a user friendly error is shown.
// This prevents users from executing bad plugins or executing a plugin
// directory. It is a UX feature, not a security feature.
var handshakeConfig = plugin.HandshakeConfig{
	ProtocolVersion:  1,
	MagicCookieKey:   "TUI_PLUGIN",
	MagicCookieValue: "kombucha_for_dogs",
}

const typeGranteePlugin = "tui_grantee_plugin"

type granteePlugin struct {
	plugin.Plugin
	logger    *log.Logger
	requested []Permission
	grantee   Grantee
	grantor   Grantor
	keepAlive time.Duration
	broker    proto.MuxBroker
}

// GRPCServer satisfies plugin.GRPCPlugin
func (p *granteePlugin) GRPCServer(_ *plugin.GRPCBroker, s *grpc.Server) error {
	server := newGranteeServer(p.broker, p.grantee, p.requested, p.keepAlive)
	if p.logger.IsLevelEnabled(log.TraceLevel) {
		server = &loggingGranteeServer{Logger: p.logger, GranteeServer: server}
	}
	proto.RegisterGranteeServer(s, server)
	return nil
}

// GRPCClient satisfies plugin.GRPCPlugin
func (p *granteePlugin) GRPCClient(
	ctx context.Context, _ *plugin.GRPCBroker, c *grpc.ClientConn,
) (interface{}, error) {
	pbClient := proto.NewGranteeClient(c)
	if p.logger.IsLevelEnabled(log.TraceLevel) {
		pbClient = &loggingGranteeClient{Logger: p.logger, GranteeClient: pbClient}
	}
	client := newGranteeClient(p.broker, pbClient)
	return client, nil
}

// Serve attempts to request the given permissions for Grantee
// and serves it as a plugin. This function never returns.
func Serve(grantee Grantee, request ...Permission) {
	SetLoggingOutput(os.Stderr)

	pluginMap := map[string]plugin.Plugin{
		typeGranteePlugin: &granteePlugin{
			logger:    &pluginLogger,
			requested: request,
			grantee:   grantee,
			keepAlive: defaultHealthCheckTicker,
			broker:    initClientBroker(&pluginLogger),
		},
	}

	plugin.Serve(&plugin.ServeConfig{
		HandshakeConfig: handshakeConfig,
		Plugins:         pluginMap,
		Logger:          NewHCLogLogrus(&pluginLogger),
		GRPCServer:      plugin.DefaultGRPCServer,
	})
}
