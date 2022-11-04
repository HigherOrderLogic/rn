package plugin

import (
	"path/filepath"
	"sync"

	"github.com/ernestrc/blue/document"
	docrpc "github.com/ernestrc/blue/document/rpc"
	bproto "github.com/ernestrc/blue/document/rpc/proto"
	"github.com/ernestrc/blue/encoding/toml"
	multierr "github.com/ernestrc/go-multierror"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"unstable.build/go-tui/debug"
	"unstable.build/go-tui/proto"
	"unstable.build/go-tui/storage"
)

const (
	// PermissionStorage requests access to persistent storage.
	PermissionStorage = "_PermStorage"
)

type pluginResource struct {
	svc    document.Service
	srv    proto.MuxServer
	server *docrpc.Server
}

type storageResourceServer struct {
	mu              sync.Mutex
	storageDir      string
	pluginResources map[string]pluginResource
}

func newStorageResourceServer(storageDir string) *storageResourceServer {
	ret := new(storageResourceServer)
	ret.storageDir = storageDir
	ret.pluginResources = make(map[string]pluginResource)
	return ret
}

func (s *storageResourceServer) setupStorage(pluginID string) document.Service {
	path := filepath.Join(s.storageDir, ".dbplugin", filepath.Clean(pluginID))
	svc, err := storage.New(path, toml.Marshaler())
	if err != nil {
		log.Warnf("Failed to setup storage for plugin %q: %v."+
			"Fallback to in-memory", pluginID, err)
		return document.NewInMemoryService()
	}
	return svc
}

func (s *storageResourceServer) Serve(
	pluginID string, grantID uint32, broker proto.MuxBroker,
	lock sync.Locker,
) error {
	return acceptAndServe(broker, grantID,
		func(opts []grpc.ServerOption) proto.MuxServer {
			s.mu.Lock()
			defer s.mu.Unlock()
			res, ok := s.pluginResources[pluginID]
			if ok {
				return res.srv
			}
			var srv proto.MuxServer
			l := debug.StandardLogger()
			if l.IsLevelEnabled(log.TraceLevel) {
				srv = proto.LoggingGRPCServer(l, opts...)
			} else {
				srv = proto.GRPCServer(opts...)
			}
			grpc := srv.GRPC()
			svc := s.setupStorage(pluginID)
			res = pluginResource{
				srv:    srv,
				server: new(docrpc.Server),
				svc:    svc,
			}
			res.server.Init(svc, toml.Marshaler(), grpc)
			bproto.RegisterDocumentStoreServer(grpc, res.server)
			s.pluginResources[pluginID] = res
			return res.srv
		})
}

func (s *storageResourceServer) Close() (ret error) {
	for _, res := range s.pluginResources {
		// docrpc.Server closes grpc.Server
		if err := res.server.Close(); err != nil {
			ret = multierr.Append(ret, err)
		}
	}
	return ret
}

// StorageResource returns a map of Permission to a ResourceServer
// capable of serving a document.Service.
func StorageResources(storageDir string) map[Permission]ResourceServer {
	s := newStorageResourceServer(storageDir)
	return map[Permission]ResourceServer{
		PermissionStorage: s,
	}
}

func dialStorage(token uint32, broker proto.MuxBroker) (
	document.Service, error,
) {
	if c, ok := clients.Load(token); ok {
		return c.(document.Service), nil
	}
	conn, err := broker.Dial(token)
	if err != nil {
		return nil, err
	}
	c := new(docrpc.Client)
	c.Init(conn, toml.Marshaler())
	clients.Store(token, c)
	return c, nil
}

// Storage acquires a client to persistent storage with
// the given token.
func Storage(token uint32, broker proto.MuxBroker) (
	document.Service, error,
) {
	return dialStorage(token, broker)
}
