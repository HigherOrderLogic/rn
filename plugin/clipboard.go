package plugin

import (
	"errors"
	"io"
	"sync"

	multierr "github.com/ernestrc/go-multierror"
	"github.com/ernestrc/go-tui/proto"
	"github.com/ernestrc/go-tui/text"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
)

const (
	// PermissionClipboard requests access to the editor.
	PermissionClipboard Permission = "_PermClipboard"
)

// ClipboardRegister is the interface that wraps the basic Copy, Paste methods
// for terminal applications that use multiple "registers" for short-lived text storage.
type ClipboardRegister interface {
	Paste() (string, error)
	Copy(string) error
}

// ClipboardSetter is the interface that wraps the basic methods SetRegister.
type ClipboardSetter interface {
	// SetRegister sets the register to be used for registerID. If there's already
	// a register installed for registerID, then it should be overwritten.
	SetRegister(registerID string, r ClipboardRegister) error
}

// Clipboard combines a ClipboardSetter with a ClipboardRegister.
type Clipboard interface {
	ClipboardSetter

	// ClipboardRegister for the default registerID
	ClipboardRegister
}

// ClipboardManager satisfies text.Clipboard by means of a plugin.ClipboardSetter
// which can be used to install arbitrary plugin.ClipboardRegister implementations.
type ClipboardManager struct {
	s *clipboardServer
	// text.Clipboard is re-used but each implementation is only
	// used for its registered registerID.
	registers map[string]*pluginRegister
}

// NewClipboardManager allocates storage for a new ClipboardManager and initializes it.
func NewClipboardManager() *ClipboardManager {
	ret := new(ClipboardManager)
	ret.Init()
	return ret
}

// avoid conflict of Copy/Paste methods
type clipboardManagerServer ClipboardManager

// Init initializes this ClipboardManager.
func (s *ClipboardManager) Init() {
	s.registers = make(map[string]*pluginRegister)
}

// Serve satisfies ResourceServer.
func (s *ClipboardManager) Serve(
	pluginID string, grantID uint32, broker proto.MuxBroker,
	l *log.Logger, lock sync.Locker,
) {
	broker.AcceptAndServe(grantID, func(opts []grpc.ServerOption) proto.MuxServer {
		lock.Lock()
		defer lock.Unlock()

		// create a new server every time Serve is called
		// so ClipboardManager can be shared across workspaces
		var srv proto.MuxServer
		if l != nil && l.IsLevelEnabled(log.TraceLevel) {
			srv = proto.LoggingGRPCServer(l, opts...)
		} else {
			srv = proto.GRPCServer(opts...)
		}
		grpc := srv.GRPC()

		// uses this ClipboardManager as the clipboard implementation
		// for all resource requests.
		s.s = newClipboardServer(l, broker, (*clipboardManagerServer)(s), lock)
		proto.RegisterClipboardServer(grpc, s.s)
		proto.RegisterClipboardRegisterServer(grpc, s.s.defaultRegisterServer)

		return srv
	})
}

func (s *clipboardManagerServer) SetRegister(
	registerID string, r ClipboardRegister,
) error {
	return (*ClipboardManager)(s).SetRegister(registerID, r)
}

func (s *clipboardManagerServer) Paste() (d string, err error) {
	data, err := (*ClipboardManager)(s).Paste(text.DefaultRegisterID)
	if err != nil {
		return "", err
	}
	return data.Text, nil
}

func (s *clipboardManagerServer) Copy(data string) error {
	d := text.ClipboardData{Text: data}
	return (*ClipboardManager)(s).Copy(text.DefaultRegisterID, d)
}

// Paste satisfies ClipboardRegister.
func (s *ClipboardManager) Paste(registerID string) (d text.ClipboardData, err error) {
	reg, ok := s.registers[registerID]
	if !ok {
		return
	}

	return reg.Paste(registerID)
}

func newMultiRegister(initial ClipboardRegister) (*pluginRegister, string) {
	multi := newMultiClipboard()
	token := multi.add(initial)
	reg := newPluginRegister(multi)
	return reg, token
}

// Copy satisfies ClipboardRegister.
func (s *ClipboardManager) Copy(registerID string, data text.ClipboardData) error {
	reg, ok := s.registers[registerID]
	if !ok {
		adapter := newTextRegister(registerID, text.NewInMemoryClipboard())
		reg, _ = newMultiRegister(adapter)
		s.registers[registerID] = reg
	}

	return reg.Copy(registerID, data)
}

func (s *ClipboardManager) tryAddCloseHook(
	r ClipboardRegister, rm *pluginRegister, token string,
) {
	if client, ok := r.(*clipboardRegisterClient); ok {
		// hook should only be called via calls to ClipboardManager.Close(),
		// or via connection monitoring, in which case both
		// should already be synchronizing over state
		client.hook = func() error {
			rm.r.(*multiClipboard).remove(token)
			return nil
		}
	}
}

// SetRegister satisfies ClipboardSetter.
func (s *ClipboardManager) SetRegister(registerID string, r ClipboardRegister) error {
	if registerID == "" {
		return errors.New("Invalid registerID")
	}
	pr, ok := s.registers[registerID]
	if !ok {
		rm, token := newMultiRegister(r)
		s.registers[registerID] = rm
		s.tryAddCloseHook(r, rm, token)
		return nil
	}

	if multi, ok := pr.r.(*multiClipboard); ok {
		token := multi.add(r)
		s.tryAddCloseHook(r, pr, token)
		return nil
	}

	// replace with multi
	mr, token := newMultiRegister(pr.r)
	s.registers[registerID] = mr
	s.tryAddCloseHook(r, mr, token)
	return nil
}

// Close releases all resources associated with this ClipboardManager.
func (s *ClipboardManager) Close() (ret error) {
	for _, r := range s.registers {
		if err := r.Close(); err != nil {
			ret = multierr.Append(ret, err)
		}
	}

	if s.s != nil {
		// avoid deadlock in case this goroutine
		// is holding the resource mutex passed to the
		// server
		go s.s.Close()
	}

	return
}

// adapts a text.Clipboard into ClipboardRegister
type textRegister struct {
	registerID string
	r          text.Clipboard
}

func newTextRegister(registerID string, r text.Clipboard) *textRegister {
	ret := new(textRegister)
	ret.r = r
	ret.registerID = registerID
	return ret
}

func (r *textRegister) Paste() (d string, err error) {
	data, err := r.r.Paste(r.registerID)
	if err != nil {
		return "", err
	}
	return data.Text, nil
}

func (r *textRegister) Copy(data string) error {
	return r.r.Copy(r.registerID, text.ClipboardData{Text: data})
}

func (r *textRegister) Close() error {
	return nil
}

// adapts a ClipboardRegister into text.Clipboard
type pluginRegister struct {
	// plugins cannot provide storage for text.ClipboardData's empty interface field
	metadata interface{}
	r        ClipboardRegister
}

func newPluginRegister(r ClipboardRegister) *pluginRegister {
	ret := new(pluginRegister)
	ret.r = r
	return ret
}

func (r *pluginRegister) Paste(registerID string) (d text.ClipboardData, err error) {
	d.Text, err = r.r.Paste()
	if err != nil {
		return
	}
	d.Metadata = r.metadata
	return
}

func (r *pluginRegister) Copy(registerID string, data text.ClipboardData) error {
	err := r.r.Copy(data.Text)
	if err != nil {
		return err
	}
	r.metadata = data.Metadata
	return nil
}

func (r *pluginRegister) Close() error {
	if closer, ok := r.r.(io.Closer); ok {
		return closer.Close()
	}
	return nil
}

func dialClipboard(token uint32, broker proto.MuxBroker) (
	Clipboard, error,
) {
	conn, err := broker.Dial(token)
	if err != nil {
		return nil, err
	}
	c := newClipboardClient(&pluginLogger, broker, conn)
	return c, nil
}

// GetClipboard acquires the remote plugin.ClipboardSetter
// with the given permission token and broker.
func GetClipboard(token uint32, broker proto.MuxBroker) (
	Clipboard, error,
) {
	return dialClipboard(token, broker)
}
