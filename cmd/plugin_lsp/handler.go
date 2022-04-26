package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/ernestrc/go-tui/browser"
	"github.com/ernestrc/go-tui/cell"
	"github.com/ernestrc/go-tui/handler"
	"github.com/ernestrc/go-tui/handler/search"
	"github.com/ernestrc/go-tui/plugin"
	plugutil "github.com/ernestrc/go-tui/plugin/util"
	"github.com/ernestrc/go-tui/proto"
	"github.com/ernestrc/go-tui/term"
	"github.com/ernestrc/go-tui/text"
	"github.com/ernestrc/go-tui/text/vi"
	"github.com/ernestrc/go-tui/workspace"
	"github.com/ernestrc/golang-internal-tools/fakenet"
	"github.com/ernestrc/golang-internal-tools/jsonrpc2"
	"github.com/ernestrc/golang-internal-tools/lsp"
	"github.com/ernestrc/golang-internal-tools/lsp/lsprpc"
	"github.com/ernestrc/golang-internal-tools/lsp/protocol"
	"github.com/ernestrc/golang-internal-tools/lsp/source"
	"github.com/ernestrc/golang-internal-tools/span"
	log "github.com/sirupsen/logrus"
)

const (
	maxHoverColumns          = 90
	defaultRpcTimeout        = 10 * time.Second
	defaultConnectTimeout    = 10 * time.Second
	defaultDisconnectTimeout = 1 * time.Second
	firstFileVersion         = 1
	commandNextDiagnostic    = "lspNextDiagnostic"
	commandPrevDiagnostic    = "lspPrevDiagnostic"
	commandHover             = "lspHover"
	commandGoToDef           = "lspGoToDefinition"
	commandFormat            = "lspFormat"
	commandOrganizeImports   = "lspOrganizeImports"
	commandReferences        = "lspReferences"
	commandAddWorkspace      = "lspAddWorkspaceFolder"
	commandRemoveWorkspace   = "lspRemoveWorkspaceFolder"
	handleBackpressureEvs    = 64
	referencesWindowWidth    = 50
	referencesWindowHeight   = 15
)

var (
	lspHandlerCommands = []string{
		commandNextDiagnostic, commandPrevDiagnostic,
		commandHover, commandGoToDef, commandFormat, commandReferences,
		commandAddWorkspace, commandRemoveWorkspace, commandOrganizeImports,
	}
	lspHandlerEvents = []text.EventType{
		text.EventTypeClose,
		text.EventTypeFlush,
		text.EventTypeOpen,
		text.EventTypeEdit,
	}
	lspHandlerPermissions = []plugin.Permission{
		plugin.PermissionBrowserWindowManager,
		plugin.PermissionBrowserResourceOpener,
		plugin.PermissionBrowserMessenger,
		plugin.PermissionWorkspace,
	}
	defaultSemanticTokensListID = "lsp_syntax_highlighting"
	defaultDiagnosticListID     = "lsp_diagnostic"
	defaultDiagnosticAttr       = map[protocol.DiagnosticSeverity]term.Attributes{
		protocol.SeverityError:       {Bg: term.ColorRed, Fg: term.ColorWhite},
		protocol.SeverityWarning:     {Bg: term.ColorYellow, Fg: term.ColorBlack},
		protocol.SeverityInformation: {Bg: term.ColorBlue, Fg: term.ColorWhite},
		protocol.SeverityHint:        {Bg: term.ColorGreen, Fg: term.ColorWhite},
	}
	defaultSemanticTypeAttr = map[string]term.Attributes{
		"namespace":     {},
		"type":          {},
		"class":         {},
		"enum":          {},
		"interface":     {Fg: term.ColorYellow},
		"struct":        {},
		"typeParameter": {},
		"parameter":     {},
		"variable":      {},
		"property":      {},
		"enumMember":    {},
		"event":         {},
		"method":        {},
		"function":      {},
		"member":        {},
		"macro":         {},
		"keyword":       {Fg: term.ColorYellow},
		"modifier":      {Fg: term.ColorYellow},
		"comment":       {Fg: term.ColorBlue},
		"string":        {Fg: term.ColorMagenta},
		"number":        {Fg: term.ColorRed},
		"regexp":        {},
		"operator":      {},
	}

	matcherString = map[source.SymbolMatcher]string{
		source.SymbolFuzzy:           "fuzzy",
		source.SymbolCaseSensitive:   "caseSensitive",
		source.SymbolCaseInsensitive: "caseInsensitive",
	}
)

type file struct {
	uri        workspace.URI
	languageID string
	handler    text.Handler
	docID      protocol.TextDocumentIdentifier

	// handler use getters
	_version     int32
	_cells       [][]term.Cell
	_diagnostics []protocol.Diagnostic
}

type execServer struct {
	langID string
	cmd    workspace.Pid
	srv    protocol.Server
	caps   protocol.ServerCapabilities
}

type lspEditorHandler struct {
	mu     sync.Mutex
	evChan chan text.Event

	ed   text.Editor
	wm   browser.WindowManager
	m    browser.Messenger
	o    browser.ResourceOpener
	exec workspace.Workspace

	semanticTypesAttr    map[string]term.Attributes
	diagnosticAttr       map[protocol.DiagnosticSeverity]term.Attributes
	semanticTokensListID string
	diagnosticListID     string
	rpcTimeout           time.Duration
	connectTimeout       time.Duration
	disconnectTimeout    time.Duration

	cwd               string
	exit              bool
	files             map[string]*file
	pendingDiagnostic map[string][]protocol.Diagnostic
	pendingGoTo       map[string]protocol.Range
	serversCfg        map[string]interface{}
	servers           map[string]execServer
	cancelTokensReq   func()
}

func sendInitializeRequest(
	ctx context.Context, cwd string, server execServer,
) (*protocol.InitializeResult, error) {
	params := &protocol.ParamInitialize{}
	// NOTE: gopls doesn't respect workspaces if rootURI is set
	if server.langID == ".go" {
		params.Capabilities.Workspace.WorkspaceFolders = true
		params.WorkspaceFolders = []protocol.WorkspaceFolder{makeWorkspaceFolder(cwd)}
	} else {
		params.RootURI = protocol.URIFromPath(cwd)
		params.RootPath = cwd
		// TODO validate compat
		// params.Path = params.RootPath // backwards compat with tsserver
	}

	params.Capabilities.Workspace.Configuration = false
	params.Capabilities.Workspace.Symbol = new(protocol.WorkspaceSymbolClientCapabilities)
	params.Capabilities.Workspace.Symbol.SymbolKind.ValueSet = []protocol.SymbolKind{}

	// Make sure to respect configured options when sending initialize request.
	opts := source.DefaultOptions().Clone()

	params.Capabilities.TextDocument.Hover = protocol.HoverClientCapabilities{
		ContentFormat: []protocol.MarkupKind{opts.PreferredContentFormat},
	}
	params.Capabilities.Workspace.ApplyEdit = true
	params.Capabilities.Workspace.WorkspaceEdit = new(protocol.WorkspaceEditClientCapabilities)
	params.Capabilities.Workspace.WorkspaceEdit.DocumentChanges = true
	params.Capabilities.TextDocument.CodeAction.CodeActionLiteralSupport.CodeActionKind.ValueSet = []protocol.CodeActionKind{}
	params.Capabilities.TextDocument.CodeAction.ResolveSupport.Properties = []string{}
	params.Capabilities.TextDocument.Completion.CompletionItem.TagSupport.ValueSet = []protocol.CompletionItemTag{}
	params.Capabilities.TextDocument.Completion.CompletionItem.ResolveSupport.Properties = []string{}
	params.Capabilities.TextDocument.Completion.CompletionItem.InsertTextModeSupport.ValueSet = []protocol.InsertTextMode{}
	params.Capabilities.TextDocument.TypeDefinition.LinkSupport = false
	params.Capabilities.TextDocument.DocumentSymbol.HierarchicalDocumentSymbolSupport = opts.HierarchicalDocumentSymbolSupport
	params.Capabilities.TextDocument.DocumentSymbol.SymbolKind.ValueSet = []protocol.SymbolKind{}
	params.Capabilities.TextDocument.DocumentSymbol.TagSupport.ValueSet = []protocol.SymbolTag{}
	params.Capabilities.TextDocument.SemanticTokens.Formats = []string{"relative"}
	params.Capabilities.TextDocument.SemanticTokens.Requests.Range = true
	params.Capabilities.TextDocument.SemanticTokens.Requests.Full = true
	params.Capabilities.TextDocument.SemanticTokens.TokenTypes = lsp.SemanticTypes()
	params.Capabilities.TextDocument.SemanticTokens.TokenModifiers = lsp.SemanticModifiers()
	params.Capabilities.TextDocument.PublishDiagnostics.TagSupport.ValueSet = []protocol.DiagnosticTag{protocol.Unnecessary}
	params.Capabilities.TextDocument.PublishDiagnostics.VersionSupport = true
	params.InitializationOptions = map[string]interface{}{
		"symbolMatcher":  matcherString[opts.SymbolMatcher],
		"semanticTokens": true,
	}

	res, err := server.srv.Initialize(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("lsp.Server.Initialize: %v", err)
	}
	err = server.srv.Initialized(ctx, &protocol.InitializedParams{})
	if err != nil {
		return nil, fmt.Errorf("lsp.Server.Initialized: %v", err)
	}

	return res, nil
}

// parseAddr parses listen into a network, and address.
func parseAddr(listen string) (network string, address string) {
	if listen == lsprpc.AutoNetwork {
		return lsprpc.AutoNetwork, ""
	}
	if parts := strings.SplitN(listen, ";", 2); len(parts) == 2 {
		return parts[0], parts[1]
	}
	return "tcp", listen
}

func streamRPC(cc jsonrpc2.Conn, h *lspEditorHandler) {
	ch := lspClientHandler{h: h}
	ctx := context.Background()

	cc.Go(ctx,
		protocol.Handlers(protocol.ClientHandler(&ch, jsonrpc2.MethodNotFound)))
	<-cc.Done()
	err := cc.Err()
	if err != nil {
		log.Errorf("jsonrpc2 processing goroutine error: %v", err)
	}
}

func initializeConnection(
	h *lspEditorHandler, reader io.ReadCloser, writer io.WriteCloser,
) (protocol.Server, error) {
	conn := fakenet.NewConn("stdio", reader, writer)
	stream := jsonrpc2.NewHeaderStream(conn)
	cc := jsonrpc2.NewConn(stream)
	server := protocol.ServerDispatcher(cc)
	go streamRPC(cc, h)

	return server, nil
}

func (h *lspEditorHandler) getPipes(pid workspace.Pid) (
	io.WriteCloser, io.ReadCloser, io.ReadCloser, error,
) {
	stdin, err := h.exec.StdinPipe(pid)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to create stdin pipe: %v", err)
	}
	stdout, err := h.exec.StdoutPipe(pid)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to create stdout pipe: %v", err)
	}

	stderr, err := h.exec.StderrPipe(pid)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to create stderr pipe: %v", err)
	}

	return stdin, stdout, stderr, nil
}

func (h *lspEditorHandler) parseCmd(arg interface{}) (workspace.Pid, error) {
	str, ok := arg.(string)
	cmd := strings.Split(str, " ")
	if !ok || len(cmd) == 0 {
		return 0, fmt.Errorf("invalid command: %v", arg)
	}

	return h.exec.Command(cmd[0], cmd[1:]...)
}

func logStderr(langID string, stderr io.ReadCloser) {
	reader := bufio.NewReader(stderr)
	for {
		line, err := reader.ReadString('\n')
		log.Debugf("%s: %s", langID, line)
		if err != nil {
			if err != io.EOF {
				log.Errorf("failed to read from '%v' server stderr: %v", langID, err)
			}
			return
		}
	}
}

type nop struct {
	io.Writer
	io.Reader
}

func (w nop) Close() error {
	if log.IsLevelEnabled(log.DebugLevel) {
		buf := make([]byte, 1<<16)
		n := runtime.Stack(buf, true)
		var pipe string
		if w.Writer != nil {
			pipe = "stdin"
		} else {
			pipe = "stdout"
		}
		log.Warningf("Close called on lsp server's %s pipe: \n%s", pipe, buf[:n])
	}
	return nil
}

func (h *lspEditorHandler) startLanguageServer(
	langID string, cmd interface{},
) (execServer, error) {
	ctx := context.Background()
	ctx, cancelFn := context.WithTimeout(ctx, h.connectTimeout)
	defer cancelFn()

	pid, err := h.parseCmd(cmd)
	if err != nil {
		err = fmt.Errorf("failed to parse language %s command: %v", langID, err)
		return execServer{}, err
	}

	stdin, stdout, stderr, err := h.getPipes(pid)
	if err != nil {
		err = fmt.Errorf("failed to create net.Conn for '%s': %v", langID, err)
		return execServer{}, err
	}

	log.Debugf("Starting lsp server '%s' with cmd: %#v", langID, pid)
	err = h.exec.Start(pid)
	if err != nil {
		err = fmt.Errorf("failed to start exec for '%s': %v", langID, err)
		return execServer{}, err
	}

	// helps debug
	if log.IsLevelEnabled(log.DebugLevel) {
		go logStderr(langID, stderr)
	}

	stdout = nop{Reader: stdout}
	stdin = nop{Writer: stdin}
	server, err := initializeConnection(h, stdout, stdin)
	if err != nil {
		err = fmt.Errorf("failed to initialize LSP server for '%s': %v", langID, err)
		return execServer{}, err
	}

	srv := execServer{langID: langID, cmd: pid, srv: server}
	initRes, err := sendInitializeRequest(ctx, h.cwd, srv)
	if err != nil {
		return execServer{}, err
	}

	srv.caps = initRes.Capabilities

	log.Infof("connected to '%s' LSP server '%s' with version %s: ",
		langID, initRes.ServerInfo.Name, initRes.ServerInfo.Version)

	return srv, nil
}

func (h *lspEditorHandler) initLanguageServers(pconfig plugin.Config) error {
	cfg, err := pconfig.GetMap("exec")
	if err != nil {
		err = fmt.Errorf("Failed to get lsp servers 'exec' config: %v", err)
		return err
	}

	h.servers = make(map[string]execServer)
	h.serversCfg = cfg

	return nil
}

func getSemanticTypesAttr(pconfig plugin.Config) (map[string]term.Attributes, error) {
	ret := make(map[string]term.Attributes, len(defaultSemanticTypeAttr))
	for k, v := range defaultSemanticTypeAttr {
		ret[k] = v
	}

	colors, err := pconfig.GetConfig("syntax_highlighting")
	if err != nil {
		if err != plugin.ErrNotFound {
			err = fmt.Errorf("Error getting 'syntax_highlighting' from plugin config: %v", err)
			return nil, err
		}
		return ret, nil
	}

	for semanticType := range defaultSemanticTypeAttr {
		attr, err := plugin.GetAttributes(colors, semanticType)
		if err != nil {
			if err != plugin.ErrNotFound {
				err = fmt.Errorf("Error getting 'syntax_highlighting.%s' "+
					"from plugin config: %v", semanticType, err)
				return nil, err
			}
			continue
		}
		ret[semanticType] = attr
	}

	return ret, nil
}

func severityToString(s protocol.DiagnosticSeverity) string {
	switch s {
	case protocol.SeverityError:
		return "error"
	case protocol.SeverityWarning:
		return "warning"
	case protocol.SeverityInformation:
		return "info"
	case protocol.SeverityHint:
		return "hint"
	default:
		return ""
	}
}

func getDiagnosticAttr(pconfig plugin.Config) (
	map[protocol.DiagnosticSeverity]term.Attributes, error,
) {
	ret := make(map[protocol.DiagnosticSeverity]term.Attributes, len(defaultDiagnosticAttr))
	for k, v := range defaultDiagnosticAttr {
		ret[k] = v
	}

	colors, err := pconfig.GetConfig("diagnostics")
	if err != nil {
		if err != plugin.ErrNotFound {
			err = fmt.Errorf("Error getting 'diagnostics' from plugin config: %v", err)
			return nil, err
		}
		return ret, nil
	}

	for s := range defaultDiagnosticAttr {
		name := severityToString(s)
		attr, err := plugin.GetAttributes(colors, name)
		if err != nil {
			if err != plugin.ErrNotFound {
				err = fmt.Errorf("Error getting 'diagnostics.%s' "+
					"from plugin config: %v", name, err)
				return nil, err
			}
			continue
		}
		ret[s] = attr
	}

	return ret, nil
}

func convertRange(
	rng protocol.Range, cells [][]term.Cell, colmap protocol.ColumnMapper,
) (from, to term.Coordinates, ok bool) {
	spn, err := colmap.RangeSpan(rng)
	if err != nil {
		log.Errorf("lspEditorHandler.convertRange: failed to create rangespan for range: %#v->%#v: %v",
			rng.Start, rng.End, err)
		return
	}

	startLine := spn.Start().Line() - 1
	startChar := spn.Start().Column() - 1
	from, ok = cell.ConvertRuneCoordinates(cells, startLine, startChar)
	if !ok {
		log.Errorf("lspEditorHandler.convertRange: failed to convert lsp Start coordinates"+
			" to term From coordinates: rng: (y=%d,x=%d) -> spn: %#v -> from:%#v",
			rng.Start.Line, rng.Start.Character, spn.Start(), from)
		return
	}

	log.Tracef("lspEditorHandler.convertRange: convert lsp Start coordinates"+
		" to term From coordinates: rng: (y=%d,x=%d) -> spn: %#v -> from:%#v",
		rng.Start.Line, rng.Start.Character, spn.Start(), from)

	endLine := spn.End().Line() - 1
	endChar := spn.End().Column() - 1
	to, ok = cell.ConvertRuneCoordinates(cells, endLine, endChar)
	if !ok {
		log.Errorf("lspEditorHandler.convertRange: failed to convert lsp End coordinates to "+
			"term From coordinates: rng: (y=%d,x=%d) -> spn: %#v -> to:%#v",
			rng.End.Line, rng.End.Character, spn.End(), to)
		return
	}

	log.Tracef("lspEditorHandler.convertRange: convert lsp End coordinates to "+
		" to term To coordinates: rng: (y=%d,x=%d) -> spn: %#v -> to:%#v",
		rng.End.Line, rng.End.Character, spn.End(), to)

	return
}

func newLspHandler(
	ed text.Editor, grants []plugin.Grant,
	broker proto.MuxBroker, pconfig plugin.Config,
) (plugutil.CommandEventHandler, error) {
	ret := new(lspEditorHandler)
	ret.ed = ed
	ret.files = make(map[string]*file)
	ret.pendingDiagnostic = make(map[string][]protocol.Diagnostic)
	ret.pendingGoTo = make(map[string]protocol.Range)
	ret.evChan = make(chan text.Event, handleBackpressureEvs)

	var err error
	ret.semanticTypesAttr, err = getSemanticTypesAttr(pconfig)
	if err != nil {
		return nil, err
	}

	ret.diagnosticAttr, err = getDiagnosticAttr(pconfig)
	if err != nil {
		return nil, err
	}

	ret.semanticTokensListID, err = pconfig.GetString("semantic_tokens_list_id")
	if err != nil {
		if err != plugin.ErrNotFound {
			err = fmt.Errorf("failed to get 'semantic_tokens_list_id' from config: %v", err)
			return nil, err
		}
		ret.semanticTokensListID = defaultSemanticTokensListID
	}

	ret.diagnosticListID, err = pconfig.GetString("diagnostic_list_id")
	if err != nil {
		if err != plugin.ErrNotFound {
			err = fmt.Errorf("failed to get 'diagnostic_list_id' from config: %v", err)
			return nil, err
		}
		ret.diagnosticListID = defaultDiagnosticListID
	}

	ret.connectTimeout, err = plugin.GetDuration(pconfig,
		"connect_timeout", defaultConnectTimeout)
	if err != nil {
		return nil, err
	}

	ret.disconnectTimeout, err = plugin.GetDuration(pconfig,
		"disconnect_timeout", defaultDisconnectTimeout)
	if err != nil {
		return nil, err
	}

	ret.rpcTimeout, err = plugin.GetDuration(pconfig,
		"rpc_timeout", defaultRpcTimeout)
	if err != nil {
		return nil, err
	}

	ret.cwd, err = os.Getwd()
	if err != nil {
		return nil, err
	}

	log.Infof("Setting up lsp handler with cwd %q", ret.cwd)

	err = ret.initLanguageServers(pconfig)
	if err != nil {
		return nil, err
	}

	for _, g := range grants {
		switch g.Permission {
		case plugin.PermissionWorkspace:
			ret.exec, err = plugin.Workspace(g.Token, broker)
			if err != nil {
				return nil, err
			}
		case plugin.PermissionBrowserResourceOpener:
			ret.o, err = plugin.ResourceOpener(g.Token, broker)
			if err != nil {
				return nil, err
			}
		case plugin.PermissionBrowserWindowManager:
			ret.wm, err = plugin.WindowManager(g.Token, broker)
			if err != nil {
				return nil, err
			}
		case plugin.PermissionBrowserMessenger:
			ret.m, err = plugin.Messenger(g.Token, broker)
			if err != nil {
				return nil, err
			}
		}
	}

	go ret.handleEvents(ret.evChan)

	return ret, nil
}

func (h *lspEditorHandler) getServer(languageID string) (
	execServer, bool,
) {
	h.mu.Lock()
	proc, ok := h.servers[languageID]
	h.mu.Unlock()
	if ok {
		return proc, true
	}

	h.mu.Lock()
	cmd, ok := h.serversCfg[languageID]
	h.mu.Unlock()
	if !ok {
		// language server not configured for language
		return execServer{}, false
	}

	srv, err := h.startLanguageServer(languageID, cmd)
	if err != nil {
		log.Errorf("Failed to start language server for %q: %s", languageID, err)
		return execServer{}, false
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	h.servers[languageID] = srv

	return srv, true
}

func (h *lspEditorHandler) newFile(
	handler text.Handler, uri workspace.URI, content string,
) *file {
	h.mu.Lock()
	defer h.mu.Unlock()

	spanURI := workspaceURIToSpan(uri)
	languageID := filepath.Ext(uri.Path())

	f := &file{
		_version: firstFileVersion,
		handler:  handler,
		docID: protocol.TextDocumentIdentifier{
			URI: protocol.URIFromSpanURI(spanURI),
		},
		uri:        uri,
		_cells:     cell.StringToCells(content),
		languageID: languageID,
	}

	h.files[uri.String()] = f
	return f
}

func (h *lspEditorHandler) removePendingGoTo(uri workspace.URI) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.pendingGoTo, uri.String())
}

func (h *lspEditorHandler) addPendingGoTo(
	uri workspace.URI, rs protocol.Range,
) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.pendingGoTo[uri.String()] = rs
}

func (h *lspEditorHandler) addPendingDiagnostics(
	uri workspace.URI, ds []protocol.Diagnostic,
) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.pendingDiagnostic[uri.String()] = ds
}

func (h *lspEditorHandler) dispatchPendingDiagnostics(
	ctx context.Context, uri workspace.URI,
) {
	h.mu.Lock()
	ds, ok := h.pendingDiagnostic[uri.String()]
	delete(h.pendingDiagnostic, uri.String())
	h.mu.Unlock()
	if !ok {
		return
	}

	h.handleDiagnostics(ctx, uri, ds, firstFileVersion)
}

func getColumnMapper(uri span.URI, buf *cell.Buffer) protocol.ColumnMapper {
	content := []byte(buf.String())
	tc := span.NewContentConverter(uri.Filename(), content)
	return protocol.ColumnMapper{
		URI:       uri,
		Content:   content,
		Converter: tc,
	}
}

func (h *lspEditorHandler) handleGoTo(f *file, rs protocol.Range) {
	cells := h.getCells(f)
	buf := cell.CellsToBuffer(cells)
	spanURI := workspaceURIToSpan(f.uri)
	colmap := getColumnMapper(spanURI, buf)
	pos, _, ok := convertRange(rs, cells, colmap)
	if !ok {
		return
	}

	err := h.ed.SetCursor(f.handler, pos)
	if err != nil {
		log.Errorf("lspEditorHandler.SetCursor(%s): %v", f.uri, err)
	}
}

func (h *lspEditorHandler) dispatchPendingGoTo(
	ctx context.Context, srv execServer, f *file,
) {
	uri := f.uri

	h.mu.Lock()
	rs, ok := h.pendingGoTo[uri.String()]
	delete(h.pendingGoTo, uri.String())
	h.mu.Unlock()
	if !ok {
		return
	}

	h.handleGoTo(f, rs)
}

func (h *lspEditorHandler) getFile(uri workspace.URI) (*file, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()

	f, ok := h.files[uri.String()]
	return f, ok
}

func parseLocationData(
	uri span.URI, cells [][]term.Cell, content []byte, d []uint32,
	semanticTypes map[string]term.Attributes,
) (ret []text.Location) {
	tc := span.NewContentConverter(uri.Filename(), content)
	colmap := protocol.ColumnMapper{
		URI:       uri,
		Content:   content,
		Converter: tc,
	}

	lspLine := make([]uint32, len(d)/5)
	lspChar := make([]uint32, len(d)/5)
	var line, char uint32
	for i := 0; 5*i < len(d); i++ {
		lspLine[i] = line + d[5*i+0]
		if d[5*i+0] > 0 {
			char = 0
		}
		lspChar[i] = char + d[5*i+1]
		char = lspChar[i]
		line = lspLine[i]
	}

	// second, convert to gopls coordinates
	for i := 0; 5*i < len(d); i++ {
		pr := protocol.Range{
			Start: protocol.Position{
				Line:      uint32(lspLine[i]),
				Character: uint32(lspChar[i]),
			},
			End: protocol.Position{
				Line:      uint32(lspLine[i]),
				Character: uint32(lspChar[i] + d[5*i+2]),
			},
		}
		from, to, ok := convertRange(pr, cells, colmap)
		if !ok {
			continue
		}

		// mods:   lsp.SemMods(int(d[5*i+4])),
		semType := lsp.SemType(int(d[5*i+3]))
		attr, ok := semanticTypes[semType]
		if !ok {
			log.Warnf("could not map semantic type %s; skipping token", semType)
			continue
		}

		loc := text.Location{From: from, To: to, Attr: attr}
		ret = append(ret, loc)
	}
	return ret
}

func (h *lspEditorHandler) newSemanticTokensCtx() (ctx context.Context) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.cancelTokensReq != nil {
		h.cancelTokensReq()
		h.cancelTokensReq = nil
	}
	ctx, h.cancelTokensReq = context.WithCancel(context.Background())
	return ctx
}

// https://microsoft.github.io/language-server-protocol/specifications/specification-current/#textDocument_semanticTokens
func (h *lspEditorHandler) semanticTokensFull(
	ctx context.Context, srv execServer,
	f *file, cells [][]term.Cell, content string,
) {
	// NOTE: gopls does not pass semanticTokensProvider
	if srv.caps.SemanticTokensProvider == nil && srv.langID != ".go" {
		log.Debugf("lspEditorHandler.Server.SemanticTokensFull(%s): server does not support semantic tokens", f.uri)
		return
	}
	version := h.getVersion(f)
	p2 := protocol.SemanticTokensParams{TextDocument: f.docID}
	resp, err := srv.srv.SemanticTokensFull(ctx, &p2)
	if err != nil || resp == nil {
		log.Errorf("lspEditorHandler.Server.SemanticTokensFull(%s): %v", f.uri, err)
		return
	}
	if h.getVersion(f) != version {
		log.Debugf("lspEditorHandler.Server.SemanticTokensFull(%s): stale result", f.uri)
		return
	}

	spanURI := workspaceURIToSpan(f.uri)
	locations := parseLocationData(spanURI, cells, []byte(content), resp.Data,
		h.semanticTypesAttr)
	if log.IsLevelEnabled(log.TraceLevel) {
		log.Tracef("lspEditorHandler.Server.SemanticTokensFull(%s): OK: %v: locations: %v",
			f.uri, resp.Data, locations)
	}
	err = h.ed.SetLocationList(f.handler, h.semanticTokensListID, text.LocationSlice(locations))
	if err != nil {
		log.Errorf("lspEditorHandler.SetLocationList(%s): %v", f.uri, err)
		return
	}
}

// https://microsoft.github.io/language-server-protocol/specifications/specification-current/#range
func makeProtocolRange(
	oldCells [][]term.Cell, from, to term.Coordinates,
) protocol.Range {
	from, to = cell.SortFromTo(from, to)
	starty, startx, ok := cell.ConvertTermCoordinates(oldCells, from)
	if !ok {
		panic("coordinates out of sync")
	}
	endy, endx, ok := cell.ConvertTermCoordinates(oldCells, to)
	if !ok {
		panic("coordinates out of sync")
	}

	return protocol.Range{
		Start: protocol.Position{
			Line:      uint32(starty),
			Character: uint32(startx),
		},
		End: protocol.Position{
			Line:      uint32(endy),
			Character: uint32(endx),
		},
	}
}

func (h *lspEditorHandler) callServerDidChange(
	ctx context.Context, srv execServer,
	f *file, evts []protocol.TextDocumentContentChangeEvent,
) error {
	params := protocol.DidChangeTextDocumentParams{
		TextDocument: protocol.VersionedTextDocumentIdentifier{
			Version:                h.getVersion(f),
			TextDocumentIdentifier: f.docID,
		},
		ContentChanges: evts,
	}
	err := srv.srv.DidChange(ctx, &params)
	if err != nil {
		log.Errorf("lspEditorHandler.Server.DidChange(%s): %v", f.uri, err)
	}
	return err
}

func (h *lspEditorHandler) getVersion(f *file) int32 {
	h.mu.Lock()
	defer h.mu.Unlock()
	return f._version
}

func (h *lspEditorHandler) incrementVersion(f *file) int32 {
	h.mu.Lock()
	defer h.mu.Unlock()
	f._version++
	return f._version
}

// TODO client is expected to support both incremental and full synchronization
// based on h.protocol server capabilities. Right now we are assuming incremental.
func (h *lspEditorHandler) pushFullEdit(
	ctx context.Context, srv execServer, f *file, content string,
) {
	version := h.incrementVersion(f)

	evts := []protocol.TextDocumentContentChangeEvent{{Text: content}}
	err := h.callServerDidChange(ctx, srv, f, evts)
	if err != nil {
		log.Errorf("failed to send file update: file=%v, length=%v, version=%v, err=%s",
			f.uri, len(content), version, err)
	} else {
		log.Tracef("sent full file update: file=%v, length=%v, version=%v",
			f.uri, len(content), version)
	}
}

func (h *lspEditorHandler) sendIncrementalEdit(
	ctx context.Context, srv execServer, f *file, newCells,
	oldCells [][]term.Cell, content string, from, to term.Coordinates,
) (protocol.Range, error) {
	version := h.incrementVersion(f)

	// https://microsoft.github.io/language-server-protocol/specification#textDocument_didChange
	rng := makeProtocolRange(oldCells, from, to)
	evts := []protocol.TextDocumentContentChangeEvent{{Text: content, Range: &rng}}

	log.Tracef("sending incremental file update: file=%v, length=%v, version=%v,"+
		" rangeStart: %#v, rangeEnd: %#v, from=%#v, to=%#v: content='%s'",
		f.uri, len(content), version, rng.Start, rng.End, from, to, content)

	err := h.callServerDidChange(ctx, srv, f, evts)
	if err != nil {
		log.Errorf("lspEditorHandler.Server.DidChange: %v", err)
		return rng, err
	}

	return rng, nil
}

func (h *lspEditorHandler) setCells(f *file, cells [][]term.Cell) {
	h.mu.Lock()
	defer h.mu.Unlock()

	f._cells = cells
}

func (h *lspEditorHandler) getCells(f *file) (cells [][]term.Cell) {
	h.mu.Lock()
	defer h.mu.Unlock()

	return f._cells
}

func (h *lspEditorHandler) handleFileFlush(ev text.Event) {
	ctx := context.Background()
	ctx, cancelFn := context.WithTimeout(ctx, h.rpcTimeout)
	defer cancelFn()

	// content last EOL is trimmed by the buffer's unix file reader.
	// lsp expects the last EOL
	ev.Content += "\n"

	f, ok := h.getFile(ev.URI)
	if !ok {
		f = h.newFile(ev.Resource, ev.URI, ev.Content)
	}

	srv, ok := h.getServer(f.languageID)
	if !ok {
		return
	}

	h.pushFullEdit(ctx, srv, f, ev.Content)
	h.setCells(f, cell.StringToCells(ev.Content))
	ctx = h.newSemanticTokensCtx()
	go h.semanticTokensFull(ctx, srv, f, h.getCells(f), ev.Content)
}

func (h *lspEditorHandler) handleFileEdit(ev text.Event) {
	ctx := context.Background()
	ctx, cancelFn := context.WithTimeout(ctx, h.rpcTimeout)
	defer cancelFn()
	f, ok := h.getFile(ev.URI)
	if !ok {
		log.Warnf("lspEditorHandler: Received insert/delete event for an unknown file: %#v", ev)
		return
	}

	srv, ok := h.getServer(f.languageID)
	if !ok {
		return
	}

	// text.Editor requires clients to re-send locations on every update.
	// unfortunately it seems that the LSP spec is a bit confusing regarding
	// what to do when there are updates to the buffer but changes do not affect diagnostics.
	// Certain LSP servers (rls, clangd, tsserver) re-send the diagnostics
	// after every update. Some other LSP servers do not follow this behavior so
	// we need to send the last known diagnostics and hope that if the locations are incorrect,
	// the LSP server will overwrite them:
	// See discussion https://github.com/microsoft/language-server-protocol/issues/1217
	// Fix I submitted to gopls and was rejected https://go-review.googlesource.com/c/tools/+/298853
	ds := h.getDiagnostics(f)
	if len(ds) != 0 {
		h.setDiagnosticsLocationList(ctx, f, ds)
	}

	oldCells := h.getCells(f)
	buf := cell.CellsToBuffer(oldCells)
	buf.Edit(ev.Start, ev.End, ev.Content)
	newCells := buf.RawCells()
	_, err := h.sendIncrementalEdit(ctx, srv, f, newCells, oldCells,
		ev.Content, ev.Start, ev.End)
	h.setCells(f, newCells)
	if err != nil {
		return
	}

	ctx = h.newSemanticTokensCtx()
	go h.semanticTokensFull(ctx, srv, f, newCells, buf.String())
}

func (h *lspEditorHandler) handleFileOpen(ev text.Event) {
	ctx := context.Background()
	ctx, cancelFn := context.WithTimeout(ctx, h.rpcTimeout)
	defer cancelFn()

	// content last EOL is trimmed by the buffer's unix file reader.
	// lsp expects the last EOL
	ev.Content += "\n"

	f := h.newFile(ev.Resource, ev.URI, ev.Content)
	srv, ok := h.getServer(f.languageID)
	if !ok {
		log.Warnf("could not connect to lsp server for %s: "+
			"configuration not found or process not running", f.languageID)
		return
	}

	spanURI := workspaceURIToSpan(f.uri)
	p := protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI:        protocol.URIFromSpanURI(spanURI),
			LanguageID: f.languageID,
			Version:    h.getVersion(f),
			Text:       ev.Content,
		},
	}

	if err := srv.srv.DidOpen(ctx, &p); err != nil {
		log.Errorf("lspEditorHandler.Server.DidOpen(%s, %s): %v",
			f.uri, f.languageID, err)
		return
	}
	log.Tracef("lspEditorHandler.Server.DidOpen(%s, %s)", f.uri, f.languageID)

	h.dispatchPendingDiagnostics(ctx, f.uri)
	h.dispatchPendingGoTo(ctx, srv, f)
	ctx = h.newSemanticTokensCtx()
	go h.semanticTokensFull(ctx, srv, f, h.getCells(f), ev.Content)
}

func (h *lspEditorHandler) removeFile(resource workspace.URI) (*file, bool) {
	f, ok := h.getFile(resource)
	if !ok {
		return nil, false
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	delete(h.files, f.uri.String())

	return f, true
}

func (h *lspEditorHandler) sendDidClose(
	ctx context.Context, srv execServer, uri workspace.URI,
) {
	spanURI := workspaceURIToSpan(uri)
	p := protocol.DidCloseTextDocumentParams{
		TextDocument: protocol.TextDocumentIdentifier{
			URI: protocol.URIFromSpanURI(spanURI),
		},
	}

	if err := srv.srv.DidClose(ctx, &p); err != nil {
		log.Errorf("lspEditorHandler.Server.DidClose(%s): %v", uri, err)
		return
	}
	log.Tracef("lspEditorHandler.Server.DidClose(%s)", uri)
}

func (h *lspEditorHandler) handleFileClose(ev text.Event) {
	ctx := context.Background()
	ctx, cancelFn := context.WithTimeout(ctx, h.rpcTimeout)
	defer cancelFn()
	f, ok := h.removeFile(ev.URI)
	if !ok {
		log.Warnf("lspEditorHandler: Received close event for an unknown file: %#v", ev)
		return
	}
	srv, ok := h.getServer(f.languageID)
	if !ok {
		return
	}

	h.sendDidClose(ctx, srv, f.uri)
	h.addPendingDiagnostics(f.uri, h.getDiagnostics(f))
}

func (h *lspEditorHandler) parseDiagnostics(
	f *file, d []protocol.Diagnostic,
) []text.Location {
	cells := h.getCells(f)
	buf := cell.CellsToBuffer(cells)
	spanURI := workspaceURIToSpan(f.uri)
	colmap := getColumnMapper(spanURI, buf)

	locs := make([]text.Location, 0, len(d))
	for _, d := range d {
		from, to, ok := convertRange(d.Range, cells, colmap)
		if !ok {
			continue
		}

		msg := fmt.Sprintf("%s: %s", d.Source, d.Message)
		attr, ok := h.diagnosticAttr[d.Severity]
		if !ok {
			log.Warnf("unknown diagnostic severity %v; skipping diagnostic", d.Severity)
			continue
		}

		loc := text.Location{Message: msg, From: from, To: to, Attr: attr}
		locs = append(locs, loc)
	}

	return locs
}

func (h *lspEditorHandler) setDiagnostics(f *file, ds []protocol.Diagnostic) {
	h.mu.Lock()
	defer h.mu.Unlock()

	f._diagnostics = ds
}

func (h *lspEditorHandler) getDiagnostics(f *file) (ds []protocol.Diagnostic) {
	h.mu.Lock()
	defer h.mu.Unlock()

	return f._diagnostics
}

func (h *lspEditorHandler) handleDiagnostics(
	ctx context.Context, file workspace.URI,
	ds []protocol.Diagnostic, version int32,
) {
	f, ok := h.getFile(file)
	if !ok {
		h.addPendingDiagnostics(file, ds)
		log.Tracef("lspEditorHandler: Received diagnostic for a unopened file: %#v", file)
		return
	}

	h.setDiagnostics(f, ds)

	if version != h.getVersion(f) {
		log.Debugf("lspEditorHandler: Received diagnostic for "+
			"outdated version of file '%s': %#v", f.uri, version)
		// it seems that tsserver does not send any version info
		// return
	}

	h.setDiagnosticsLocationList(ctx, f, ds)
}

func (h *lspEditorHandler) setDiagnosticsLocationList(
	ctx context.Context, f *file, ds []protocol.Diagnostic,
) {
	locs := h.parseDiagnostics(f, ds)
	err := h.ed.SetLocationList(f.handler, h.diagnosticListID, text.LocationSlice(locs))
	if err != nil {
		log.Errorf("lspEditorHandler.SetLocationList(%s): %v", f.uri, err)
		return
	}
}

func spanURIToWorkspace(u span.URI) (workspace.URI, error) {
	file, err := workspace.ParseURI(string(u))
	if err != nil {
		log.Errorf("failed to convert span URI to workspace URI %s: %s", u, err)
		return workspace.URI{}, err
	}
	return file, err
}

func workspaceURIToSpan(u workspace.URI) span.URI {
	// the workspace is always local for the language server
	// if URI is a remote uri, then the language server is executed
	// in the remote host as well.
	return span.URI(fmt.Sprintf("file://%s", u.Path()))
}

func (h *lspEditorHandler) HandleDiagnostics(
	ctx context.Context, p *protocol.PublishDiagnosticsParams,

) {
	var start time.Time
	if log.IsLevelEnabled(log.TraceLevel) {
		start = time.Now()
		log.Tracef("lspEditorHandler.HandleDiagnostics(%#v)", p.URI)
	}
	file, err := spanURIToWorkspace(p.URI.SpanURI())
	if err != nil {
		return
	}

	h.handleDiagnostics(ctx, file, p.Diagnostics, p.Version)

	if log.IsLevelEnabled(log.TraceLevel) {
		log.Tracef("lspEditorHandler.HandleDiagnostics(%#v) in %s", p.URI, time.Since(start))
	}
}

func (h *lspEditorHandler) goToLocation(win browser.Window, l protocol.Location) {
	uri, err := spanURIToWorkspace(l.URI.SpanURI())
	if err != nil {
		return
	}
	f, alreadyOpen := h.getFile(uri)
	if !alreadyOpen {
		h.addPendingGoTo(uri, l.Range)
	}

	buf, err := h.o.Open(uri)
	if err != nil {
		h.m.SetMessage("Open: %v", err)
		log.Errorf("lspEditorHandler.Open(%s): %v", uri, err)
		h.removePendingGoTo(uri)
		return
	}

	if alreadyOpen {
		h.handleGoTo(f, l.Range)
	}

	err = win.SetContent(buf)
	if err != nil && err != browser.ErrTabNotFree {
		log.Errorf("error SetContent: %v", err)
		return
	}
}

func (h *lspEditorHandler) getFilePosition(cursor term.Coordinates, uri workspace.URI) (
	f *file, pos protocol.Position, ok bool,
) {
	f, ok = h.getFile(uri)
	if !ok {
		log.Warnf("lspEditorHandler: Received hover request for an unknown file: %#v", uri)
		return
	}

	line, column, ok := cell.ConvertTermCoordinates(h.getCells(f), cursor)
	if !ok {
		log.Warnf("lspEditorHandler: Received hover request for an oob position: %#v", cursor)
		return
	}

	ok = true
	pos = protocol.Position{
		Line:      uint32(line),
		Character: uint32(column),
	}

	return
}

func (h *lspEditorHandler) handleGoToDefinition(
	cursor term.Coordinates, ed text.Handler, uri workspace.URI,
) {
	f, pos, ok := h.getFilePosition(cursor, uri)
	if !ok {
		return
	}

	p := protocol.DefinitionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: f.docID,
			Position:     pos,
		},
	}

	srv, ok := h.getServer(f.languageID)
	if !ok {
		return
	}

	ctx := context.Background()
	ctx, cancelFn := context.WithTimeout(ctx, h.rpcTimeout)
	defer cancelFn()

	locs, err := srv.srv.Definition(ctx, &p)
	if err != nil {
		log.Errorf("lspEditorHandler.Server.Definition(%s, %s): %v",
			f.uri, f.languageID, err)
		return
	}

	log.Tracef("lspEditorHandler.Server.Definition(%s, %s): %#v",
		f.uri, f.languageID, locs)

	if len(locs) == 0 {
		return
	}

	win, err := h.wm.Focus()
	if err != nil {
		log.Errorf("lspEditorHandler.Focus(): %v", err)
		return
	}

	for _, l := range locs {
		h.goToLocation(win, l)
	}
}

func findBestFloatingWindowPosition(cursorAtWindow term.Coordinates, width, height int) (
	term.Coordinates, int, int,
) {
	maxColumns := width
	width = int(math.Min(float64(maxColumns)+3, maxHoverColumns))
	height = height + 3 // frame + less bar

	at := cursorAtWindow
	at.Y -= height - 1

	if at.Y < 0 {
		at.Y = cursorAtWindow.Y + 2
	}

	return at, width, height
}

func (h *lspEditorHandler) handleHover(
	cursorAtScroll, cursorAtWindow term.Coordinates,
	ed text.Handler, uri workspace.URI,
) {
	f, pos, ok := h.getFilePosition(cursorAtScroll, uri)
	if !ok {
		return
	}

	p := protocol.HoverParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: f.docID,
			Position:     pos,
		},
	}

	srv, ok := h.getServer(f.languageID)
	if !ok {
		return
	}

	ctx := context.Background()
	ctx, cancelFn := context.WithTimeout(ctx, h.rpcTimeout)
	defer cancelFn()

	hover, err := srv.srv.Hover(ctx, &p)
	if err != nil || hover == nil {
		log.Errorf("lspEditorHandler.Server.Hover(%s, %s): %v", f.uri, f.languageID, err)
		return
	}

	log.Tracef("lspEditorHandler.Server.Hover(%s, %s): %#v", f.uri, f.languageID, hover)

	less := handler.NewLess(handler.DefaultLessConfig())
	less.Buffer().WriteString(hover.Contents.Value)
	bh := browser.NopHandler(less)

	width, height := less.Buffer().MaxColumns(), less.Buffer().Rows()
	at, width, height := findBestFloatingWindowPosition(cursorAtWindow, width, height)
	_, err = h.wm.Floating(bh, at, width, height)
	if err != nil {
		log.Errorf("lspEditorHandler.SplitHorizontalAbove(%s): %v", f.uri, err)
	}
}

func makeWorkspaceFolder(in string) protocol.WorkspaceFolder {
	return protocol.WorkspaceFolder{
		URI:  string(protocol.URIFromPath(in)),
		Name: in,
	}
}

func (h *lspEditorHandler) handleChangedWorkspace(
	uri workspace.URI, added []string, removed []string,
) {
	languageID := filepath.Ext(uri.Path())
	srv, ok := h.getServer(languageID)
	if !ok {
		return
	}

	/*cfg := caps.InnerServerCapabilities.Workspace.WorkspaceFolders
	if !cfg.Supported {
		h.m.SetMessage("LSP server does not support changing workspaces")
		log.Tracef("lspEditorHandler.Server.DidChangeWorkspaceFolders(%#v): not supported", caps)
		return
	}*/

	ctx := context.Background()
	ctx, cancelFn := context.WithTimeout(ctx, h.rpcTimeout)
	defer cancelFn()

	var remove []protocol.WorkspaceFolder
	for _, folder := range removed {
		remove = append(remove, makeWorkspaceFolder(folder))
	}

	var add []protocol.WorkspaceFolder
	for _, folder := range added {
		add = append(add, makeWorkspaceFolder(folder))
	}

	req := protocol.DidChangeWorkspaceFoldersParams{
		Event: protocol.WorkspaceFoldersChangeEvent{Added: add, Removed: remove},
	}

	log.Tracef("lspEditorHandler.Server.DidChangeWorkspaceFolders(%#v)", req)

	err := srv.srv.DidChangeWorkspaceFolders(ctx, &req)
	if err != nil {
		h.m.SetMessage("DidChangeWorkspaceFolders: %v", err)
		log.Errorf("lspEditorHandler.Server.DidChangeWorkspaceFolders(%s): %v", uri, err)
	}
}

func (h *lspEditorHandler) handleAddWorkspace(uri workspace.URI, args []string) {
	log.Tracef("lspEditorHandler.handleAddWorkspace(%v)", args)
	h.handleChangedWorkspace(uri, args, nil)
}

func (h *lspEditorHandler) handleRemoveWorkspace(uri workspace.URI, args []string) {
	log.Tracef("lspEditorHandler.handleRemoveWorkspace(%v)", args)
	h.handleChangedWorkspace(uri, nil, args)
}

func (h *lspEditorHandler) browseLocations(
	win browser.Window, locs []protocol.Location,
) {
	const locID = "highlight_loc"
	var (
		longestLocation int
		bottom, top     browser.Window
		done            bool
	)
	cfg := search.ListConfig{
		Algo:          search.FuzzyMatch,
		Interrupt:     term.Interrupt,
		CaseSensitive: false,
	}
	list := search.NewList(cfg)
	textToLocation := make(map[string]protocol.Location)
	buf := cell.NewBuffer()
	ed := vi.Editor()
	edh, _ := ed.Edit(workspace.URI{}, buf)

	for _, l := range locs {
		uri := l.URI.SpanURI()
		filename := uri.Filename()

		relative, err := filepath.Rel(h.cwd, filename)
		if err == nil && len(relative) < len(filename) {
			filename = relative
		}
		text := fmt.Sprintf("%s:%#v", filename, l.Range)
		list.PushSync([]byte(text))
		textToLocation[text] = l
		if len(text) > longestLocation {
			longestLocation = len(text)
		}
	}

	closeWin := func(win browser.Window) func() {
		return func() {
			h.mu.Lock()
			shouldClose := done && win != nil
			closeWin := win
			h.mu.Unlock()
			if shouldClose {
				closeWin.Close()
			}
		}
	}

	renderFile := func(l protocol.Location) {
		uri := l.URI.SpanURI()
		data, err := ioutil.ReadFile(uri.Filename())
		if err != nil {
			log.Errorf("lspEditorHandler.ReadFile(): %v", err)
			return
		}
		content := string(data)

		_ = ed.SetCursor(edh, term.Coordinates{})
		buf.Reset()
		buf.WriteString(content)

		cells := buf.RawCells()
		colmap := getColumnMapper(uri, buf)
		from, to, ok := convertRange(l.Range, cells, colmap)
		if !ok {
			log.Debugf("lspEditorHandler.convertRange(): %v", ok)
			return
		}

		attrs := term.Attributes{Bg: term.AttrReverse, Fg: term.AttrReverse}
		loc := text.Location{From: from, To: to, Attr: attrs}
		ed.SetLocationList(edh, locID, text.LocationSlice([]text.Location{loc}))
		ed.MoveToPrevLocation(edh, locID)
	}

	sh := search.Handler(list, func(text string) {
		h.mu.Lock()
		done = true
		h.mu.Unlock()

		// liberate all tabs
		closeWin(top)()
		closeWin(bottom)()

		h.goToLocation(win, textToLocation[text])
	})

	// wrap to detect when focus has changed
	// and re-render window.
	bh := handler.Wrap(sh, func(ev term.Event) (bool, bool) {
		before, _ := list.Focus()
		exit, handle := sh.Handle(ev)
		list.Wait()
		after, _ := list.Focus()
		afterStr := string(after)
		if exit {
			done = true
		}
		if string(before) != afterStr {
			renderFile(textToLocation[afterStr])
		}
		return exit, handle
	})

	eh := handler.Wrap(handler.Sync(&h.mu, edh), func(ev term.Event) (bool, bool) {
		h.mu.Lock()
		defer h.mu.Unlock()

		if ev.Type == term.EventResize {
			list.Wait()
			focus, _ := list.Focus()
			renderFile(textToLocation[string(focus)])
			return false, true
		}

		return edh.Handle(ev)
	})

	bhtop := browser.FuncHandler(eh, closeWin(bottom))
	top, err := h.wm.Split(browser.OrientationBottom, bhtop)
	if err != nil {
		log.Errorf("lspEditorHandler.SplitHorizontalBelow(): %v", err)
		return
	}

	bhbottom := browser.FuncHandler(bh, closeWin(top))
	bottom, err = h.wm.Split(browser.OrientationBottom, bhbottom)
	if err != nil {
		log.Errorf("lspEditorHandler.SplitHorizontalBelow(): %v", err)
		return
	}
}

func (h *lspEditorHandler) handleReferences(
	cursorAtScroll, cursorAtWindow term.Coordinates,
	ed text.Handler, uri workspace.URI,
) {
	win, err := h.wm.Focus()
	if err != nil {
		log.Errorf("lspEditorHandler.Focus(): %v", err)
		return
	}

	f, pos, ok := h.getFilePosition(cursorAtScroll, uri)
	if !ok {
		return
	}

	p := protocol.ReferenceParams{
		Context: protocol.ReferenceContext{IncludeDeclaration: true},
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: f.docID,
			Position:     pos,
		},
	}

	srv, ok := h.getServer(f.languageID)
	if !ok {
		return
	}

	ctx := context.Background()
	ctx, cancelFn := context.WithTimeout(ctx, h.rpcTimeout)
	defer cancelFn()

	locs, err := srv.srv.References(ctx, &p)
	if err != nil {
		log.Errorf("lspEditorHandler.Server.References(%s, %s): %v", f.uri, f.languageID, err)
		return
	}

	log.Tracef("lspEditorHandler.Server.References(%s, %s): %#v", f.uri, f.languageID, locs)

	if len(locs) == 0 {
		h.m.SetMessage("No references found")
		return
	}

	h.browseLocations(win, locs)
}

func toJSONEdits(edits []protocol.TextEdit) string {
	m, _ := json.Marshal(edits)
	return string(m)
}

func (h *lspEditorHandler) format(
	ctx context.Context, f *file, srv execServer, builder *editBuilder,
) {
	p := protocol.DocumentFormattingParams{
		TextDocument: f.docID,
		Options: protocol.FormattingOptions{
			// TODO expose tabsize with Open event
			TabSize:                4,
			InsertSpaces:           false,
			TrimTrailingWhitespace: true,
			InsertFinalNewline:     false,
			TrimFinalNewlines:      false,
		},
	}

	edits, err := srv.srv.Formatting(ctx, &p)
	if err != nil {
		log.Errorf("lspEditorHandler.Server.Formatting(%s): %v", f.uri, err)
		return
	}

	// json marshaling is expensive
	if log.IsLevelEnabled(log.TraceLevel) {
		log.Tracef("lspEditorHandler.Server.Formatting(%s): %v", f.uri, toJSONEdits(edits))
	}

	builder.applyEdits(edits)
}

func (h *lspEditorHandler) organizeImports(
	ctx context.Context, f *file, srv execServer, builder *editBuilder,
) {
	p := protocol.CodeActionParams{
		TextDocument: f.docID,
		Context: protocol.CodeActionContext{
			Only: []protocol.CodeActionKind{protocol.Source, protocol.SourceOrganizeImports},
		},
	}
	codeActions, err := srv.srv.CodeAction(ctx, &p)
	if err != nil {
		log.Errorf("lspEditorHandler.Server.CodeAction(%s): %v", f.uri, err)
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	for _, ca := range codeActions {
		switch ca.Kind {
		case protocol.Source, protocol.SourceOrganizeImports:
			log.Tracef("lspEditorHandler.Server.CodeAction(%s): %v", f.uri, ca.Edit)
			builder.applyWorkspaceEdit(ca.Edit)
		}
	}
}

func (h *lspEditorHandler) handleFormat(ed text.Handler, uri workspace.URI, imports bool) {
	ctx := context.Background()
	ctx, cancelFn := context.WithTimeout(ctx, h.rpcTimeout)
	defer cancelFn()

	f, ok := h.getFile(uri)
	if !ok {
		log.Errorf("lspEditorHandler: Received format event for an unknown file: %#v", uri)
		return
	}

	srv, ok := h.getServer(f.languageID)
	if !ok {
		return
	}
	w := h.ed.CellEditor(ed)

	// this cells are used to map edit ranges to term.Coordinates
	// but discarded because only Insert/Delete events should
	// apply changes to the local file copy.
	cells := h.getCells(f)

	var b editBuilder
	b.init(f, w, cells)

	if imports {
		h.organizeImports(ctx, f, srv, &b)
	} else {
		h.format(ctx, f, srv, &b)
	}
}

func (h *lspEditorHandler) HandleCommand(
	ctx context.Context, cmd text.Command,
) (exit bool) {
	if cmd.Resource == nil {
		return
	}

	switch cmd.Name {
	case commandNextDiagnostic:
		err := h.ed.MoveToNextLocation(cmd.Resource, h.diagnosticListID)
		if err != nil {
			log.Errorf("lspEditorHandler.MoveToNextLocation(%s): %v", cmd.Name, err)
		}
	case commandPrevDiagnostic:
		err := h.ed.MoveToPrevLocation(cmd.Resource, h.diagnosticListID)
		if err != nil {
			log.Errorf("lspEditorHandler.MoveToNextLocation(%s): %v", cmd.Name, err)
		}
	case commandHover:
		h.handleHover(cmd.Cursor.Content, cmd.Cursor.Window, cmd.Resource, cmd.URI)
	case commandGoToDef:
		h.handleGoToDefinition(cmd.Cursor.Content, cmd.Resource, cmd.URI)
	case commandReferences:
		h.handleReferences(cmd.Cursor.Content, cmd.Cursor.Window, cmd.Resource, cmd.URI)
	case commandAddWorkspace:
		h.handleAddWorkspace(cmd.URI, cmd.Args)
	case commandRemoveWorkspace:
		h.handleRemoveWorkspace(cmd.URI, cmd.Args)
	case commandFormat:
		h.handleFormat(cmd.Resource, cmd.URI, false)
	case commandOrganizeImports:
		h.handleFormat(cmd.Resource, cmd.URI, true)
	}

	return false
}

func (h *lspEditorHandler) handleEvents(ch chan text.Event) {
	for ev := range ch {
		var start time.Time
		if log.IsLevelEnabled(log.TraceLevel) {
			start = time.Now()
			log.Tracef("lspEditorHandler.Handle(%#v)", ev)
		}

		switch ev.Type {
		case text.EventTypeOpen:
			h.handleFileOpen(ev)
		case text.EventTypeClose:
			h.handleFileClose(ev)
		case text.EventTypeFlush:
			h.handleFileFlush(ev)
		case text.EventTypeEdit:
			h.handleFileEdit(ev)
		}

		if log.IsLevelEnabled(log.TraceLevel) {
			log.Tracef("lspEditorHandler.Handle(%#v) in %s", ev, time.Since(start))
		}
	}
}

func (h *lspEditorHandler) Handle(
	ctx context.Context, ev text.Event,
) (exit bool) {
	h.mu.Lock()
	exit = h.exit
	h.mu.Unlock()

	if exit {
		return
	}

	h.evChan <- ev
	return
}

func (h *lspEditorHandler) Close() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	defer close(h.evChan)
	h.exit = true

	var errs []string
	for _, server := range h.servers {
		ctx, cancel := context.WithTimeout(context.Background(), h.disconnectTimeout)

		h.mu.Unlock()
		err := server.srv.Shutdown(ctx)
		h.mu.Lock()

		cancel()
		if err != nil {
			errs = append(errs, err.Error())
		}
		err = h.exec.Signal(server.cmd, syscall.SIGTERM)
		if err != nil {
			errs = append(errs, err.Error())
		}
	}

	if len(errs) != 0 {
		return errors.New(strings.Join(errs, "; "))
	}

	return nil
}
