// Unstable Build LLC ("COMPANY") CONFIDENTIAL
//
// Unpublished Copyright (c) 2017-2026 Unstable Build, All Rights Reserved.
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

package idelsp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/unstablebuild/rune-go-sdk/api/browserapi"
	"github.com/unstablebuild/rune-go-sdk/api/schemeapi"
	"github.com/unstablebuild/rune-go-sdk/api/semanticapi"
	"github.com/unstablebuild/rune-go-sdk/api/textapi"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"github.com/unstablebuild/rune-go-sdk/iterator"
	"github.com/unstablebuild/rune-go-sdk/retry"
	"unstable.build/go-tui/debug"
)

// ErrNoServer is returned when no language server is
// available for the requested file.
var ErrNoServer = errors.New("no language server")

// Callback extends the SDK's LSPCallback with methods for tracking
// document version changes and waiting for the server to process them.
// This allows pull-diagnostics to wait until the server has processed
// a recently sent didChange before issuing the request.
type Callback interface {
	semanticapi.LSPCallback
	// FileDidChange reports that a file changed and how the next
	// WaitFileProcessed should wait for the server to reconcile it.
	// version is the editor's document version, open reports whether
	// the file is open in the editor, and oob reports whether the
	// change is out-of-band (a file watcher event or a
	// workspace/didChangeWatchedFiles triggered by a tool such as
	// apply_patch) rather than a versioned editor edit. An
	// out-of-band change to an open file makes the wait block for a
	// strictly newer version (using version as the floor), while one
	// to a closed file waits for the next diagnostics push.
	FileDidChange(uri string, version int32, open, oob bool)
	// InvalidateAllPending marks every URI tracked by the callback
	// as having a pending unversioned change, so that the next
	// WaitFileProcessed call for any of those URIs blocks until a
	// fresh publishDiagnostics push arrives. This is used when an
	// out-of-band workspace event (e.g. a file deletion) can
	// invalidate diagnostics for unrelated files in the same
	// package, and we want to avoid returning a stale snapshot from
	// the LSP cache.
	InvalidateAllPending()
	WaitFileProcessed(ctx context.Context, uri string) error
}

// Config provides optional configuration for a Manager.
type Config struct {
	Callback           Callback
	MaxRetries         uint
	InitializeTimeout  time.Duration
	CloseTimeout       time.Duration
	EventHandleTimeout time.Duration
	NoInitializeServer bool
	WorkDoneProgress   bool
	// ScheduleNextTick hops onto the host event loop. When nil,
	// notifications fire directly from background goroutines, which
	// races workspaceManagerHandler.focus reads in notis.inFocus.
	ScheduleNextTick func(func()) bool
}

// Manager is a multi-language LSP server manager.
// It implements semanticapi.LSP and textapi.EventHandler.
type Manager struct {
	cfg           Config
	evs           chan textapi.Event
	mu            sync.Mutex
	tokenSeq      int64
	rootURI       string
	fileSystem    schemeapi.FileSystem
	executor      schemeapi.Executor
	notifications browserapi.Notifications
	pkgManager    PkgManager
	callback      Callback
	maxRetries    uint
	servers       map[string]server
	files         map[string]*file
	pendingOpens  map[string]textapi.Event
	ctx           context.Context
	cancel        context.CancelFunc
	log           *slog.Logger
}

// tokenFor returns existing if non-nil, or generates a new
// integer ProgressToken when WorkDoneProgress is enabled.
func (m *Manager) tokenFor(
	existing *semanticapi.ProgressToken,
) *semanticapi.ProgressToken {
	if existing != nil || !m.cfg.WorkDoneProgress {
		return existing
	}
	id := int(atomic.AddInt64(&m.tokenSeq, 1))
	return &semanticapi.ProgressToken{
		IntegerValue: id, IsInteger: true,
	}
}

var (
	_ semanticapi.LSP      = (*Manager)(nil)
	_ textapi.EventHandler = (*Manager)(nil)
)

// New creates a new Manager with the given dependencies and configuration.
func New(
	uri workspaceapi.URI, fileSystem schemeapi.FileSystem,
	executor schemeapi.Executor,
	pkgManager PkgManager, notifications browserapi.Notifications,
	opener browserapi.ResourceOpener,
	cfg Config,
) *Manager {
	const eventsBufferSize = 5

	if cfg.MaxRetries == 0 {
		cfg.MaxRetries = 3
	}
	if cfg.InitializeTimeout == 0 {
		cfg.InitializeTimeout = 3 * time.Second
	}
	if cfg.CloseTimeout == 0 {
		cfg.CloseTimeout = 3 * time.Second
	}
	if cfg.EventHandleTimeout == 0 {
		cfg.EventHandleTimeout = 1 * time.Second
	}
	ctx, cancel := context.WithCancel(context.Background())
	ret := &Manager{
		cfg:           cfg,
		log:           slog.With("struct", "idelsp.Manager", "workspace", convertURI(uri)),
		rootURI:       convertURI(uri),
		fileSystem:    fileSystem,
		executor:      executor,
		pkgManager:    pkgManager,
		notifications: notifications,
		callback:      cfg.Callback,
		maxRetries:    cfg.MaxRetries,
		servers:       make(map[string]server),
		files:         make(map[string]*file),
		pendingOpens:  make(map[string]textapi.Event),
		ctx:           ctx,
		cancel:        cancel,
		evs:           make(chan textapi.Event, eventsBufferSize),
	}
	go debug.CapturePanicReport(func() {
		ret.handleEvs()
	})
	return ret
}

// Close shuts down all active language servers.
func (m *Manager) Close() error {
	defer m.cancel()
	m.mu.Lock()
	servers := make([]server, 0, len(m.servers))
	for _, s := range m.servers {
		servers = append(servers, s)
	}
	m.mu.Unlock()

	var errs []error
	ctx, cancel := context.WithTimeout(
		context.Background(), m.cfg.CloseTimeout,
	)
	defer cancel()
	for _, s := range servers {
		if err := s.stop(ctx); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// Handle implements textapi.EventHandler.
func (m *Manager) Handle(
	_ context.Context, ev textapi.Event,
) bool {
	m.log.Debug("received event", "type", ev.Type, "file", ev.URI)
	select {
	case m.evs <- ev:
		return false
	case <-m.ctx.Done():
		return true
	}
}

func (m *Manager) handleEvs() {
	// ensure Handle doesn't block if this goroutine panics
	// and no goroutine is listening on m.evs.
	defer m.cancel()
	for {
		select {
		case <-m.ctx.Done():
			return
		case ev := <-m.evs:
			err := m.handle(ev)
			if err != nil {
				m.log.Error("handle event", "error", err)
			}
		}
	}
}

func (m *Manager) handle(ev textapi.Event) error {
	uri := convertURI(ev.URI)

	ctx, cancel := context.WithTimeout(context.Background(),
		m.cfg.EventHandleTimeout)
	defer cancel()

	switch ev.Type {
	case textapi.EventTypeOpen:
		m.log.Debug("process open", "file", uri, "size", len(ev.Content))
		srv, err := m.ensureServer(ctx, ev.URI)
		if err != nil {
			if m.cfg.NoInitializeServer {
				m.mu.Lock()
				m.pendingOpens[uri] = ev
				m.mu.Unlock()
				m.log.Debug("tracked pending open for server init", "file", uri)
				return nil
			}
			return err
		}
		// ensureServer may have consumed most of the
		// EventHandleTimeout while starting the server (it
		// uses its own InitializeTimeout internally). Reset
		// the deadline so the didOpen notify gets a fresh
		// timeout.
		cancel()
		openCtx, openCancel := context.WithTimeout(
			context.Background(), m.cfg.EventHandleTimeout)
		defer openCancel()
		f, err := m.ensureFile(ev.URI, ev.Content, srv.config().id)
		if err != nil {
			return err
		}
		return srv.notify(openCtx, "textDocument/didOpen",
			semanticapi.DidOpenTextDocumentParams{
				TextDocument: semanticapi.TextDocumentItem{
					URI:        uri,
					LanguageID: f.languageID,
					Version:    f.version,
					Text:       f.content,
				},
			})

	case textapi.EventTypeClose:
		srv, err := m.serverForURI(uri)
		if err != nil {
			if m.cfg.NoInitializeServer {
				m.mu.Lock()
				delete(m.pendingOpens, uri)
				delete(m.files, uri)
				m.mu.Unlock()
				return nil
			}
			return err
		}
		notifyErr := srv.notify(ctx, "textDocument/didClose",
			semanticapi.DidCloseTextDocumentParams{
				TextDocument: semanticapi.TextDocumentIdentifier{
					URI: uri,
				},
			})
		// Drop the cached entry regardless of notify outcome: once
		// we send didClose (or fail trying), the server side is no
		// longer guaranteed to track this URI and our cached copy
		// would only leak memory until process exit.
		m.mu.Lock()
		delete(m.files, uri)
		m.mu.Unlock()
		return notifyErr

	case textapi.EventTypeEdit:
		srv, err := m.serverForURI(uri)
		if err != nil {
			return err
		}
		uriStr := convertURI(ev.URI)
		f, ok := m.getFile(uriStr)
		if !ok {
			return errors.New("received edit event for non-open file")
		}
		m.mu.Lock()
		m.files[f.docID.URI].version++
		version := m.files[f.docID.URI].version
		m.mu.Unlock()
		err = srv.notify(ctx, "textDocument/didChange",
			semanticapi.DidChangeTextDocumentParams{
				TextDocument: semanticapi.VersionedTextDocumentIdentifier{
					Version: version,
					URI:     uri,
				},
				ContentChanges: []semanticapi.TextDocumentContentChangeEvent{
					{
						Range: &semanticapi.Range{
							Start: semanticapi.Position{
								Line:      uint32(ev.Start.Y),
								Character: uint32(ev.Start.X),
							},
							End: semanticapi.Position{
								Line:      uint32(ev.End.Y),
								Character: uint32(ev.End.X),
							},
						},
						Text: ev.Content,
					},
				},
			})
		if err == nil {
			m.callback.FileDidChange(uri, version, true, false)
		}
		return err

	case textapi.EventTypeFlush:
		srv, err := m.serverForURI(uri)
		if err != nil {
			return err
		}
		f, err := m.ensureFile(ev.URI, ev.Content, srv.config().id)
		if err != nil {
			return err
		}
		return srv.notify(ctx, "textDocument/didSave",
			semanticapi.DidSaveTextDocumentParams{
				TextDocument: semanticapi.TextDocumentIdentifier{
					URI: uri,
				},
				Text: f.content,
			})

	case textapi.EventTypeCreate:
		m.fileDidChangeOOB(uri)
		return m.broadcastNotify(ctx,
			workspaceapi.URI{}, "workspace/didChangeWatchedFiles",
			semanticapi.DidChangeWatchedFilesParams{
				Changes: []semanticapi.FileEvent{
					{
						URI:  uri,
						Type: semanticapi.FileChangeTypeCreated,
					},
				},
			})

	case textapi.EventTypeChange:
		m.fileDidChangeOOB(uri)
		return m.broadcastNotify(ctx,
			workspaceapi.URI{}, "workspace/didChangeWatchedFiles",
			semanticapi.DidChangeWatchedFilesParams{
				Changes: []semanticapi.FileEvent{
					{
						URI:  uri,
						Type: semanticapi.FileChangeTypeChanged,
					},
				},
			})

	case textapi.EventTypeRemove:
		m.fileDidChangeOOB(uri)
		return m.broadcastNotify(ctx,
			workspaceapi.URI{}, "workspace/didChangeWatchedFiles",
			semanticapi.DidChangeWatchedFilesParams{
				Changes: []semanticapi.FileEvent{
					{
						URI:  uri,
						Type: semanticapi.FileChangeTypeDeleted,
					},
				},
			})

	case textapi.EventTypeRename:
		// textapi.EventTypeRename doesn't contain the old/new path mapping
		// so we simply tell the LSP server that the file has changed
		// in which case it will try to read it and succeed (rename target)
		// or fail (rename source), and apply the right changes internally.
		m.fileDidChangeOOB(uri)
		return m.broadcastNotify(ctx,
			workspaceapi.URI{}, "workspace/didChangeWatchedFiles",
			semanticapi.DidChangeWatchedFilesParams{
				Changes: []semanticapi.FileEvent{
					{
						URI:  uri,
						Type: semanticapi.FileChangeTypeChanged,
					},
				},
			})
	default:
		return fmt.Errorf("extraneous event %v", ev.Type)
	}
}

func (m *Manager) fileDidChangeOOB(uri string) {
	f, open := m.getFile(uri)
	var version int32
	if open {
		version = f.version
	}
	m.callback.FileDidChange(uri, version, open, true)
}

func (m *Manager) getFile(uriStr string) (*file, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	f, ok := m.files[uriStr]
	return f, ok
}

func (m *Manager) ensureFile(
	uri workspaceapi.URI,
	content string, languageID string,
) (*file, error) {
	uriStr := convertURI(uri)
	if languageID == "" || uri == (workspaceapi.URI{}) {
		panic("empty params for ensuring available file")
	}
	f, ok := m.getFile(uriStr)
	if ok {
		if content != "" {
			m.files[uriStr].content = content
			m.files[uriStr].version++
		}
		return f, nil
	}
	if content == "" {
		f, err := m.fileSystem.Open(uri.Path())
		if err != nil {
			return nil, fmt.Errorf("open file for reading: %w", err)
		}
		defer f.Close() // nolint:errcheck
		data, err := io.ReadAll(f)
		if err != nil {
			return nil, fmt.Errorf("read workspace file: %w", err)
		}
		content = string(data)
	} else if len(content) == 0 || content[len(content)-1] != '\n' {
		// editor trims last EOL but LSP servers expect it
		content += "\n"
	}
	f = newFile(uri, content, languageID)
	m.mu.Lock()
	m.files[uriStr] = f
	m.mu.Unlock()

	return f, nil
}

func (m *Manager) ensureServer(
	_ context.Context, filename workspaceapi.URI,
) (server, error) {
	lang, err := languageForFile(filename)
	if err != nil {
		return nil, err
	}

	m.mu.Lock()
	if srv, ok := m.servers[lang.id]; ok {
		m.mu.Unlock()
		return srv, nil
	}
	m.mu.Unlock()

	if m.cfg.NoInitializeServer {
		return nil, fmt.Errorf("server not initialized and auto-initialize config is false")
	}

	ctx, cancel := context.WithTimeout(m.ctx, m.cfg.InitializeTimeout)
	defer cancel()
	return m.initializeServer(ctx, lang, autoInitParams(m.rootURI))
}

func (m *Manager) initializeServer(
	ctx context.Context, lang langConfig, params semanticapi.InitializeParams,
) (*langServer, error) {
	if lang.command == "" {
		return nil, errors.New("language configuration with empty command")
	}
	srv := m.buildChild(ctx, lang, lang.id, params)
	if err := srv.start(ctx); err != nil {
		return nil, err
	}

	m.mu.Lock()
	m.servers[lang.id] = srv
	m.mu.Unlock()

	m.sendPendingOpens(lang.id, srv)

	go debug.CapturePanicReport(func() {
		m.watchServer(&lang, srv)
	})
	return srv, nil
}

// buildChild resolves the binary for lang and constructs an
// un-started langServer. serverName is the identity reported to the
// callback handler (and used to key diagnostics by source); for a
// single-server language it is the language id, for a multi-server
// child it is the child's command so each backend's diagnostics
// accumulate independently.
func (m *Manager) buildChild(
	ctx context.Context, lang langConfig, serverName string,
	params semanticapi.InitializeParams,
) *langServer {
	var binPath string
	if filepath.IsAbs(lang.command) {
		binPath = lang.command
	} else {
		var err error
		binPath, err = m.findBinary(ctx, &lang)
		if err != nil {
			m.log.Debug(
				"find lsp executable, falling back to using PATH",
				"executable", lang.command, "error", err,
			)
			binPath = lang.command
		}
	}
	srv := newLangServer(
		m.ctx, lang, binPath, m.executor, m.rootURI,
		newCallbackAdapter(m.callback, serverName, m.rootURI),
		params,
	)
	srv.serverName = serverName
	return srv
}

// childName derives the server-name identity for a child backend
// from its command string: the base name of the executable
// (e.g. "ty server" -> "ty", "/usr/bin/ruff" -> "ruff").
func childName(command string) string {
	argv := strings.Fields(command)
	if len(argv) == 0 {
		return command
	}
	return filepath.Base(argv[0])
}

// initializeMultiServer builds and starts a multiLangServer for lang.
// The default child serves any method without an explicit route;
// alternates maps an LSP method to a command string, and one extra
// child is spawned per distinct alternate command. Each child is
// supervised independently so a single crash does not tear down the
// others.
func (m *Manager) initializeMultiServer(
	ctx context.Context, lang langConfig,
	alternates map[string]string, params semanticapi.InitializeParams,
) (*multiLangServer, error) {
	if lang.command == "" {
		return nil, errors.New("language configuration with empty command")
	}

	defaultChild := m.buildChild(ctx, lang, childName(lang.command), params)
	children := []*langServer{defaultChild}
	routes := make(map[string]int)

	// Dedup alternate commands so two methods sharing one command
	// map to a single child.
	cmdIndex := make(map[string]int)
	for method, cmd := range alternates {
		if cmd == "" || cmd == lang.command {
			continue
		}
		idx, ok := cmdIndex[cmd]
		if !ok {
			argv := strings.Split(cmd, " ")
			childCfg := langConfig{id: lang.id, command: argv[0], args: argv[1:]}
			child := m.buildChild(ctx, childCfg, childName(cmd), params)
			children = append(children, child)
			idx = len(children) - 1
			cmdIndex[cmd] = idx
		}
		routes[method] = idx
	}

	mls := &multiLangServer{cfg: lang, routes: routes}
	mls.children = make([]server, len(children))
	for i, c := range children {
		mls.children[i] = c
	}

	if err := mls.start(ctx); err != nil {
		_ = mls.Close()
		return nil, err
	}

	m.mu.Lock()
	m.servers[lang.id] = mls
	m.mu.Unlock()

	for _, child := range children {
		m.sendPendingOpens(lang.id, child)
		watched := child
		go debug.CapturePanicReport(func() {
			m.watchServer(&lang, watched)
		})
	}
	return mls, nil
}

// installRestarted swaps a freshly restarted child into the
// manager's routing for lang. When the language is backed by a
// multiLangServer, only the crashed child is replaced (by pointer
// identity) so the other children keep running; otherwise the
// single registered server is replaced wholesale.
func (m *Manager) installRestarted(lang langConfig, old, restarted *langServer) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if mls, ok := m.servers[lang.id].(*multiLangServer); ok {
		mls.replaceChild(old, restarted)
		return
	}
	m.servers[lang.id] = restarted
}

// sendPendingOpens sends didOpen for files that were opened before this
// server existed. Only files whose language matches langID are sent;
// the rest stay in pendingOpens for a future server init.
func (m *Manager) sendPendingOpens(langID string, srv *langServer) {
	m.mu.Lock()
	var opens []textapi.Event
	for uri, ev := range m.pendingOpens {
		cfg, err := languageForFilename(uri)
		if err != nil || cfg.id != langID {
			continue
		}
		opens = append(opens, ev)
		delete(m.pendingOpens, uri)
	}
	m.mu.Unlock()

	ctx, cancel := context.WithTimeout(
		context.Background(), m.cfg.EventHandleTimeout,
	)
	defer cancel()

	for _, ev := range opens {
		uri := convertURI(ev.URI)
		f, err := m.ensureFile(ev.URI, ev.Content, langID)
		if err != nil {
			m.log.Error("send pending open", "error", err, "file", uri)
			continue
		}
		_ = srv.notify(ctx, "textDocument/didOpen",
			semanticapi.DidOpenTextDocumentParams{
				TextDocument: semanticapi.TextDocumentItem{
					URI:        uri,
					LanguageID: f.languageID,
					Version:    f.version,
					Text:       f.content,
				},
			})
	}
}

func (m *Manager) findBinary(
	ctx context.Context, lang *langConfig,
) (string, error) {
	files, err := m.pkgManager.LibDir(
		ctx, lang.id,
	)
	if err != nil {
		return "", fmt.Errorf("lib dir: %w", err)
	}
	defer func() { _ = files.Close() }()

	paths, err := iterator.ToSlice(ctx, files)
	if err != nil {
		return "", fmt.Errorf("lib dir iterator: %w", err)
	}

	for _, file := range paths {
		candidate := filepath.Base(file)
		if candidate != lang.command {
			continue
		}
		if _, err := os.Stat(file); err == nil {
			return file, nil
		}
	}
	return "", fmt.Errorf(
		"%s not found in any package directory",
		lang.command,
	)
}

func (m *Manager) watchServer(
	lang *langConfig, srv *langServer,
) {
	defer func() {
		err := srv.Close()
		if err != nil {
			m.log.Warn("jsonrpc2 connection close", "error", err)
		}
	}()

	ch := srv.watcher
	if ch == nil {
		return
	}

	select {
	case <-m.ctx.Done():
		return
	case err := <-ch:
		if err == nil {
			m.log.Debug("server exited gracefully", "language", lang.id)
			return
		}
		m.log.Warn("server crashed",
			"language", lang.id, "error", err)
		srv.mu.Lock()
		stopCalled := srv.stopCalled
		srv.mu.Unlock()
		if stopCalled {
			return
		}
	case <-srv.conn.Done():
		// The jsonrpc2 connection died (e.g. read or write error)
		// while the child process is still running. This leaves the
		// editor unable to communicate even though the server looks
		// alive. Kill the orphaned process and fall through into the
		// restart loop.
		srv.mu.Lock()
		stopCalled := srv.stopCalled
		srv.mu.Unlock()
		if stopCalled {
			return
		}
		m.log.Warn("jsonrpc2 connection died, killing orphan process",
			"language", lang.id, "pid", srv.pid)
		if err := m.executor.Signal(srv.pid, syscall.SIGKILL); err != nil {
			m.log.Warn("kill orphan lsp process",
				"language", lang.id, "pid", srv.pid, "error", err)
		}
	}

	strategy := retry.CombinedStrategy(
		retry.LimitStrategy(m.maxRetries),
		retry.ExponentialStrategy(500*time.Millisecond, 5*time.Second),
	)

	retryErr := retry.Retry(m.ctx, strategy,
		func(ctx context.Context) (bool, error) {
			ctx, cancel := context.WithTimeout(ctx, m.cfg.InitializeTimeout)
			defer cancel()

			m.log.Debug("restarting lsp server", "language", lang.id)
			newSrv := newLangServer(
				m.ctx, srv.cfg, srv.binPath, m.executor, m.rootURI,
				newCallbackAdapter(m.callback, srv.serverName, m.rootURI),
				srv.params,
			)
			newSrv.serverName = srv.serverName

			if err := newSrv.start(ctx); err != nil {
				return true, err
			}
			m.installRestarted(*lang, srv, newSrv)

			m.reopenFiles(ctx, lang.id, newSrv)
			go debug.CapturePanicReport(func() {
				m.watchServer(lang, newSrv)
			})
			return false, nil
		})

	if retryErr != nil && m.notifications != nil {
		m.cfg.ScheduleNextTick(func() {
			_, _ = m.notifications.Notify(
				browserapi.LevelError,
				"LSP server %s failed after %d retries: %s",
				lang.command, m.maxRetries, retryErr,
			)
		})
	}
}

func (m *Manager) reopenFiles(
	ctx context.Context, langID string,
	srv *langServer,
) {
	m.mu.Lock()
	var files []*file
	for _, file := range m.files {
		if file.languageID == langID {
			files = append(files, file)
		}
	}
	m.mu.Unlock()

	for _, f := range files {
		_ = srv.notify(ctx, "textDocument/didOpen",
			semanticapi.DidOpenTextDocumentParams{
				TextDocument: semanticapi.TextDocumentItem{
					URI:        f.docID.URI,
					LanguageID: langID,
					Version:    f.version,
					Text:       f.content,
				},
			})
	}
}

func (m *Manager) serverForURI(uri string) (server, error) {
	m.mu.Lock()
	cfg, err := languageForFilename(uri)
	if err != nil {
		m.mu.Unlock()
		return nil, err
	}
	srv, ok := m.servers[cfg.id]
	m.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("%w: server %s not running", ErrNoServer, cfg.id)
	}
	return srv, nil
}

func (m *Manager) allServers() []server {
	m.mu.Lock()
	defer m.mu.Unlock()
	servers := make(
		[]server, 0, len(m.servers),
	)
	for _, s := range m.servers {
		servers = append(servers, s)
	}
	return servers
}

func (m *Manager) broadcastNotify(
	ctx context.Context, uri workspaceapi.URI, method string, params any,
) (ret error) {
	if uri == (workspaceapi.URI{}) {
		for _, srv := range m.allServers() {
			if err := srv.notify(ctx, method, params); err != nil {
				ret = errors.Join(ret, err)
			}
		}
		return
	}

	uriStr := convertURI(uri)

	// Tier 1: watcher-based routing — servers that registered
	// file watchers matching this URI. This is not supported for now.

	// Tier 2: language-based routing — match file extension to a known language.
	cfg, err := languageForFile(uri)
	if err == nil {
		for _, srv := range m.allServers() {
			if srv.config().id != cfg.id {
				continue
			}
			if err := srv.notify(ctx, method, params); err != nil {
				ret = errors.Join(ret, err)
			}
		}
		return ret
	}

	// Tier 3: broadcast to all running servers.
	m.log.Debug("broadcasting to all servers", "method", method, "uri", uriStr)
	for _, srv := range m.allServers() {
		if err := srv.notify(ctx, method, params); err != nil {
			ret = errors.Join(ret, err)
		}
	}
	return ret
}

func autoInitParams(rootURI string) semanticapi.InitializeParams {
	capabilities := map[string]any{
		"textDocument": map[string]any{
			"implementation": map[string]any{
				"linkSupport": true,
			},
			"completion":     map[string]any{},
			"hover":          map[string]any{},
			"signatureHelp":  map[string]any{},
			"definition":     map[string]any{},
			"references":     map[string]any{},
			"documentSymbol": map[string]any{},
			"formatting":     map[string]any{},
			"rename": map[string]any{
				"prepareSupport": true,
			},
			"codeAction": map[string]any{},
			"foldingRange": map[string]any{
				"lineFoldingOnly": false,
			},
			"selectionRange":    map[string]any{},
			"documentHighlight": map[string]any{},
			"callHierarchy":     map[string]any{},
			"codeLens":          map[string]any{},
			"inlayHint":         map[string]any{},
			"semanticTokens": map[string]any{
				"requests": map[string]any{
					"full":  true,
					"range": true,
				},
				"tokenTypes": []string{
					"namespace", "type", "class",
					"enum", "interface", "struct",
					"typeParameter", "parameter",
					"variable", "property",
					"enumMember", "event",
					"function", "method", "macro",
					"keyword", "modifier",
					"comment", "string", "number",
					"regexp", "operator",
					"decorator", "label",
				},
				"tokenModifiers": []string{
					"declaration", "definition",
					"readonly", "static",
					"deprecated", "abstract",
					"async", "modification",
					"documentation",
					"defaultLibrary",
				},
				"formats": []string{"relative"},
			},
		},
		"workspace": map[string]any{
			"symbol":      map[string]any{},
			"diagnostics": map[string]any{},
		},
		"window": map[string]any{
			"workDoneProgress": true,
			"showDocument": map[string]any{
				"support": true,
			},
		},
	}
	capabilitiesData, err := json.Marshal(capabilities)
	if err != nil {
		panic("marshal capabilities")
	}
	return semanticapi.InitializeParams{
		RootURI:      rootURI,
		Capabilities: json.RawMessage(capabilitiesData),
	}
}
