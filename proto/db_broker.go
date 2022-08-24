package proto

import (
	context "context"
	fmt "fmt"
	math "math"
	"net"
	"sync"
	"time"

	"github.com/ernestrc/blue/datastore/document"
	"github.com/ernestrc/blue/retry"
	multierr "github.com/ernestrc/go-multierror"
	"github.com/ernestrc/go-tui/util"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
)

// satisfies to MuxBroker
type dbBroker struct {
	mu        sync.Mutex
	id        uint32
	svc       document.Service
	listeners map[uint32]net.Listener
	logger    *log.Logger
}

type nextIDDoc struct {
	ID uint32
}

type listenerDoc struct {
	Address string
}

// NewDatastoreBroker provides brokerage by employing a datastore to share
// connection information.
func NewDatastoreBroker(svc document.Service, logger *log.Logger) MuxBroker {
	ret := new(dbBroker)
	ret.svc = svc
	ret.logger = logger
	// start with 1 so zero-valued uint32 can be interpreted as not valid
	ret.id = 1
	ret.listeners = make(map[uint32]net.Listener)
	return ret
}

func makeListenerDocumentKey(i uint32) string {
	return fmt.Sprintf("LISTENER: %d", i)
}

func makeNextIDDocument(i uint32) (string, nextIDDoc) {
	return fmt.Sprintf("ID: %d", i), nextIDDoc{i}
}

func makeListenerDocument(i uint32, listener net.Listener) (string, listenerDoc) {
	return makeListenerDocumentKey(i), listenerDoc{Address: listener.Addr().String()}
}

func (t *dbBroker) NextId() uint32 {
	t.mu.Lock()
	defer t.mu.Unlock()

	ctx := context.Background()
	retries := 3 // retries for unknown errors
	retry.Retry(ctx, retry.LimitStrategy(math.MaxInt32), func(ctx context.Context) bool {
		t.id++
		key, doc := makeNextIDDocument(t.id)
		err := t.svc.Create(ctx, key, doc)
		if err == document.ErrAlreadyExists {
			return true
		}
		if err != nil {
			if retries == 0 {
				panic(err)
			}
			retries--
			return true
		}
		return false
	})

	return t.id
}

func (t *dbBroker) Cleanup(ID uint32) (ret error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if err := t.svc.Delete(context.Background(), makeListenerDocumentKey(ID)); err != nil {
		ret = multierr.Append(ret, err)
	}
	l, ok := t.listeners[ID]
	if !ok {
		return
	}
	if err := l.Close(); err != nil {
		ret = multierr.Append(ret, err)
	}
	return
}

func (t *dbBroker) Accept(id uint32) (net.Listener, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	listener, err := util.TempUnixListener()
	if err != nil {
		return nil, err
	}
	key, doc := makeListenerDocument(id, listener)
	err = t.svc.Create(context.Background(), key, doc)
	if err != nil {
		return nil, err
	}
	t.listeners[id] = listener
	return listener, nil
}

func (t *dbBroker) AcceptAndServe(
	ID uint32, srv func(opts []grpc.ServerOption) MuxServer,
) {
	lis, err := t.Accept(ID)
	if err != nil {
		panic(err)
	}
	server := srv([]grpc.ServerOption{})
	go server.Serve(lis)
}

func (t *dbBroker) Dial(ID uint32) (conn MuxConn, err error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	var lis listenerDoc
	err = t.svc.Get(context.Background(), makeListenerDocumentKey(ID), &lis)
	if err != nil {
		return
	}

	opts := []grpc.DialOption{grpc.WithInsecure(), grpc.WithDialer(
		func(_ string, _ time.Duration) (net.Conn, error) {
			addr, err := net.ResolveUnixAddr("unix", lis.Address)
			if err != nil {
				return nil, err
			}
			return net.Dial(addr.Network(), addr.String())
		},
	)}
	conn, err = grpc.Dial("", opts...)
	if err != nil {
		return
	}

	if t.logger != nil && t.logger.IsLevelEnabled(log.TraceLevel) {
		conn = loggingConn{t.logger, conn}
	}
	return
}

func (t *dbBroker) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	for _, lis := range t.listeners {
		_ = lis.Close()
	}
	t.listeners = nil
	return t.svc.Close()
}
