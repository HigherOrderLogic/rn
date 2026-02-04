// Unstable Build LLC ("COMPANY") CONFIDENTIAL
//
// Unpublished Copyright (c) 2017-2024 Unstable Build, All Rights Reserved.
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

package extension

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	multierr "github.com/ernestrc/go-multierror"
	log "github.com/sirupsen/logrus"
	"github.com/unstablebuild/golang-internal-tools/fakenet"
	"github.com/unstablebuild/golang-internal-tools/jsonrpc2"
	"github.com/unstablebuild/golang-internal-tools/lsp"
	"github.com/unstablebuild/golang-internal-tools/lsp/protocol"
	"github.com/unstablebuild/golang-internal-tools/lsp/source"
	"github.com/unstablebuild/golang-internal-tools/span"
	"github.com/unstablebuild/rune-go-sdk/api/browserapi"
	"github.com/unstablebuild/rune-go-sdk/api/config"
	"github.com/unstablebuild/rune-go-sdk/api/extensionapi"
	"github.com/unstablebuild/rune-go-sdk/api/textapi"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"github.com/unstablebuild/rune-go-sdk/component"
	"github.com/unstablebuild/rune-go-sdk/handler"
	"github.com/unstablebuild/rune-go-sdk/iterator"
	"github.com/unstablebuild/rune-go-sdk/term"
	"github.com/unstablebuild/tcell/v3"
	"unstable.build/go-tui/api/browserapi/browserext"
	configextension "unstable.build/go-tui/api/config/extension"
	"unstable.build/go-tui/api/workspaceapi/workspaceext"
	"unstable.build/go-tui/cell"
	"unstable.build/go-tui/extension"
	"unstable.build/go-tui/extension/extutil"
	thandler "unstable.build/go-tui/handler"
	"unstable.build/go-tui/handler/search"
	"unstable.build/go-tui/rpc"
	"unstable.build/go-tui/text/modeless"
	"unstable.build/go-tui/text/vi"
)

const (
	maxHoverColumns             = 90
	defaultRpcTimeout           = 10 * time.Second
	defaultConnectTimeout       = 10 * time.Second
	defaultDisconnectTimeout    = 300 * time.Millisecond
	firstFileVersion            = 1
	commandHover                = "lsphover"
	commandGoToDef              = "lspgotodef"
	commandFormat               = "lspformat"
	commandOrganizeImports      = "lspimports"
	commandReferences           = "lspref"
	referencesWindowWidth       = 50
	referencesWindowHeight      = 15
	defaultSemanticTokensListID = "_lspsyntax"
	defaultDiagnosticListID     = "lspdiagnostic"
)

// Grantee returns this extension's Grantee and the permissions required to run it.
func Grantee() (extension.Grantee, []extensionapi.Permission) {
	return extutil.NewEditorEventHandler(
		LSPHandlerCommands, newLspHandler,
		LSPHandlerEvents, LSPHandlerPermissions...)
}

var (
	// LSPHandlerCommands returns the commands that this extension is
	// interested in registering.
	LSPHandlerCommands = []textapi.CommandManual{
		{
			Name: commandHover,
			Summary: "Opens a tooltip with the documentation of the symbol whose name " +
				"is under the cursor provided by the LSP server configured for the file's programming language.",
		},
		{
			Name: commandGoToDef,
			Summary: "Moves the cursor to the definition of the symbol whose name " +
				"is under the cursor, opening a new file if the definition is in another file.",
		},
		{
			Name:    commandFormat,
			Summary: "Formats the whole file as per the LSP server's formatting capabilities.",
		},
		{
			Name: commandReferences,
			Summary: "Performs a usage lookup of the symbol whose name is under the cursor, using " +
				"the LSP server configured for the file's programming language. It opens two" +
				"tiled windows, one for preview and another one for fuzzy search over the " +
				"files that make use of the given symbol. Standard fuzzy search key bindings apply.",
		},
		{
			Name:    commandOrganizeImports,
			Summary: "Formats the imports section of the file as per the LSP server's formatting capabilities.",
		},
	}

	// LSPHandlerEvents returns the events that this extension is
	// interested in subscribing to.
	LSPHandlerEvents = []textapi.EventType{
		textapi.EventTypeClose,
		textapi.EventTypeFlush,
		textapi.EventTypeRemove,
		textapi.EventTypeRename,
		textapi.EventTypeChange,
		textapi.EventTypeCreate,
		textapi.EventTypeOpen,
		textapi.EventTypeEdit,
	}

	// LSPHandlerPermissions are the required permissions for this
	// extension to run.
	LSPHandlerPermissions = []extensionapi.Permission{
		extensionapi.Permission(extensionapi.PermissionBrowserWindowManager),
		extensionapi.Permission(extensionapi.PermissionBrowserResourceOpener),
		extensionapi.Permission(extensionapi.PermissionInterrupt),
		extensionapi.Permission(extensionapi.PermissionNotifications),
		extensionapi.Permission(extensionapi.PermissionFileSystem),
		extensionapi.Permission(extensionapi.PermissionExecute),
		extensionapi.PermissionConfig,
	}

	defaultDiagnosticAttr = map[protocol.DiagnosticSeverity]term.Attributes{
		protocol.SeverityError:       {Bg: tcell.ColorRed, Fg: tcell.ColorWhite},
		protocol.SeverityWarning:     {Bg: tcell.ColorYellow, Fg: tcell.ColorBlack},
		protocol.SeverityInformation: {Bg: tcell.ColorBlue, Fg: tcell.ColorWhite},
		protocol.SeverityHint:        {Bg: tcell.ColorGreen, Fg: tcell.ColorWhite},
	}
	defaultSemanticTypeAttr = map[string]tcell.Color{
		"namespace":     0,
		"type":          0,
		"class":         0,
		"enum":          0,
		"interface":     tcell.ColorYellow,
		"struct":        0,
		"typeParameter": 0,
		"parameter":     0,
		"variable":      0,
		"property":      0,
		"enumMember":    0,
		"event":         0,
		"method":        0,
		"function":      0,
		"member":        0,
		"macro":         0,
		"keyword":       tcell.ColorYellow,
		"modifier":      tcell.ColorYellow,
		"comment":       tcell.ColorBlue,
		"string":        tcell.ColorPurple,
		"number":        tcell.ColorRed,
		"regexp":        0,
		"operator":      0,
	}

	matcherString = map[source.SymbolMatcher]string{
		source.SymbolFuzzy:           "fuzzy",
		source.SymbolCaseSensitive:   "caseSensitive",
		source.SymbolCaseInsensitive: "caseInsensitive",
	}
	errNoServer            = errors.New("no LSP server found for language file language")
	defaultHoverWindowAttr = term.Attributes{}
	defaultRefWindowAttr   = term.Attributes{}
)

type file struct {
	uri        workspaceapi.URI
	languageID string
	handler    textapi.Handler
	docID      protocol.TextDocumentIdentifier

	// handler use getters
	_version     int32
	_cells       [][]term.Cell
	_diagnostics []protocol.Diagnostic
}

type execServer struct {
	langID  string
	pid     workspaceapi.Pid
	srv     protocol.Server
	caps    protocol.ServerCapabilities
	closers []io.Closer
}

type lspEditorHandler struct {
	mu        sync.Mutex
	ctx       context.Context
	cancelCtx func()

	ed   textapi.Editor
	wm   browserapi.WindowManager
	m    browserapi.Notifications
	o    browserapi.ResourceOpener
	exec workspaceapi.Executor
	fs   workspaceapi.FileSystem
	p    browserapi.EventPublisher

	tabspaces            int
	frame                bool
	semanticTypesAttr    map[string]tcell.Color
	diagnosticAttr       map[protocol.DiagnosticSeverity]term.Attributes
	enableSemanticTokens map[string]bool
	semanticTokensListID string
	diagnosticListID     string
	rpcTimeout           time.Duration
	connectTimeout       time.Duration
	disconnectTimeout    time.Duration
	hoverWindowAttr      term.Attributes
	refWindowAttr        term.Attributes
	refElementAttr       *term.Attributes
	refCountAttr         *term.Attributes
	refMatchedAttr       *term.Attributes
	refFocusElementAttr  *term.Attributes

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

func streamRPC(cc jsonrpc2.Conn, h *lspEditorHandler) {
	ch := lspClientHandler{h: h}
	ctx := context.Background()

	cc.Go(ctx,
		protocol.Handlers(protocol.ClientHandler(&ch, jsonrpc2.MethodNotFound)))
	<-cc.Done()
	err := cc.Err()

	h.mu.Lock()
	exit := h.exit
	h.mu.Unlock()
	if err != nil && !exit {
		log.Errorf("jsonrpc2 processing goroutine error: %v", err)
	}
}

func initializeConnection(
	h *lspEditorHandler, reader io.ReadCloser, writer io.WriteCloser,
) protocol.Server {
	conn := fakenet.NewConn("stdio", reader, writer)
	stream := jsonrpc2.NewHeaderStream(conn)
	cc := jsonrpc2.NewConn(stream)
	server := protocol.ServerDispatcher(cc)
	go streamRPC(cc, h)

	return server
}

func (h *lspEditorHandler) parseCmd(arg interface{}) (workspaceapi.Cmd, error) {
	str, ok := arg.(string)
	cmd := strings.Split(str, " ")
	if !ok || len(cmd) == 0 {
		return workspaceapi.Cmd{}, fmt.Errorf("invalid command: %v", arg)
	}

	return workspaceapi.Cmd{
		Path: cmd[0],
		Args: cmd[1:],
	}, nil
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
	/* enable for debugging
	if log.IsLevelEnabled(log.TraceLevel) {
		buf := make([]byte, 1<<16)
		n := runtime.Stack(buf, true)
		var pipe string
		if w.Writer != nil {
			pipe = "stdin"
		} else {
			pipe = "stdout"
		}
		log.Warningf("Close called on lsp server's %s pipe: \n%s", pipe, buf[:n])
	}*/
	return nil
}

func (h *lspEditorHandler) setPipes(
	cmd *workspaceapi.Cmd, srv *execServer,
) (stdout, stderr io.ReadCloser, stdin io.WriteCloser, err error) {
	var stdoutWrite, stderrWrite io.WriteCloser
	var stdinRead io.ReadCloser
	stdout, stdoutWrite, err = os.Pipe()
	if err != nil {
		return
	}
	stderr, stderrWrite, err = os.Pipe()
	if err != nil {
		return
	}
	stdinRead, stdin, err = os.Pipe()
	if err != nil {
		return
	}
	cmd.Stdout = stdoutWrite
	cmd.Stderr = stderrWrite
	cmd.Stdin = stdinRead
	srv.closers = []io.Closer{
		stdoutWrite, stderrWrite, stdinRead, stdout, stderr, stdin,
	}
	return
}

func (h *lspEditorHandler) startLanguageServer(
	langID string, cmdAndArgs interface{},
) (execServer, error) {
	ctx := context.Background()
	ctx, cancelFn := context.WithTimeout(ctx, h.connectTimeout)
	defer cancelFn()

	cmd, err := h.parseCmd(cmdAndArgs)
	if err != nil {
		return execServer{}, err
	}

	srv := execServer{langID: langID}

	stdout, stderr, stdin, err := h.setPipes(&cmd, &srv)
	if err != nil {
		return execServer{}, err
	}

	log.Debugf("starting %q LSP server with cmd %#v", langID, cmd)
	pid, err := h.exec.Start(h.ctx, cmd)
	log.Tracef("started %q LSP server with pid=%d: err=%v", langID, pid, err)
	if err != nil {
		err = fmt.Errorf("workspace.Start: %v", err)
		return execServer{}, err
	}

	// helps debug
	if log.IsLevelEnabled(log.DebugLevel) {
		go logStderr(langID, stderr)
	}

	stdout = nop{Reader: stdout}
	stdin = nop{Writer: stdin}
	server := initializeConnection(h, stdout, stdin)

	srv.srv = server
	srv.pid = pid
	initRes, err := sendInitializeRequest(ctx, h.cwd, srv)
	if err != nil {
		return execServer{}, err
	}

	srv.caps = initRes.Capabilities

	log.Infof("connected to '%s' LSP server '%s' with version %s: ",
		langID, initRes.ServerInfo.Name, initRes.ServerInfo.Version)

	return srv, nil
}

func (h *lspEditorHandler) initLanguageServers(pconfig config.Config) error {
	cfg, err := pconfig.GetMap("exec")
	if err != nil {
		err = fmt.Errorf("failed to get lsp servers 'exec' config: %v", err)
		return err
	}

	h.servers = make(map[string]execServer)
	h.serversCfg = cfg

	return nil
}

func getSemanticTypesAttr(pconfig config.Config) (map[string]tcell.Color, error) {
	ret := make(map[string]tcell.Color, len(defaultSemanticTypeAttr))
	for k, v := range defaultSemanticTypeAttr {
		ret[k] = v
	}

	colors, err := pconfig.GetConfig("syntax_highlighting")
	if err != nil {
		if err != config.ErrNotFound {
			err = fmt.Errorf("get 'syntax_highlighting' from extension config: %v", err)
			return nil, err
		}
		return ret, nil
	}

	for semanticType := range defaultSemanticTypeAttr {
		attr, err := colors.GetColor(semanticType)
		if err != nil {
			if err != config.ErrNotFound {
				err = fmt.Errorf("get 'syntax_highlighting.%s' "+
					"from extension config: %v", semanticType, err)
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

func getDiagnosticAttr(pconfig config.Config) (
	map[protocol.DiagnosticSeverity]term.Attributes, error,
) {
	ret := make(map[protocol.DiagnosticSeverity]term.Attributes, len(defaultDiagnosticAttr))
	for k, v := range defaultDiagnosticAttr {
		ret[k] = v
	}

	colors, err := pconfig.GetConfig("diagnostics")
	if err != nil {
		if err != config.ErrNotFound {
			err = fmt.Errorf("get 'diagnostics' from extension config: %v", err)
			return nil, err
		}
		return ret, nil
	}

	for s := range defaultDiagnosticAttr {
		name := severityToString(s)
		attr, err := config.GetAttributes(colors, name)
		if err != nil {
			if err != config.ErrNotFound {
				err = fmt.Errorf("get 'diagnostics.%s' "+
					"from extension config: %v", name, err)
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
	from, ok = cell.ConvertRunePosToCoordinates(cells, startLine, startChar)
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
	to, ok = cell.ConvertRunePosToCoordinates(cells, endLine, endChar)
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
	ctx context.Context, ed textapi.Editor, grants []extension.Grant,
	broker rpc.MuxBroker, pconfig config.Config,
) (extutil.CommandEventHandler, error) {
	ret := new(lspEditorHandler)
	ret.ed = ed
	ret.files = make(map[string]*file)
	ret.pendingDiagnostic = make(map[string][]protocol.Diagnostic)
	ret.pendingGoTo = make(map[string]protocol.Range)
	ret.ctx, ret.cancelCtx = context.WithCancel(ctx)

	var err error
	ret.semanticTypesAttr, err = getSemanticTypesAttr(pconfig)
	if err != nil {
		return nil, err
	}

	ret.diagnosticAttr, err = getDiagnosticAttr(pconfig)
	if err != nil {
		return nil, err
	}

	var configErr error
	ret.semanticTokensListID, err = pconfig.GetString("semantic_tokens_list_id")
	if err != nil {
		if err != config.ErrNotFound {
			configErr = multierr.Append(configErr,
				fmt.Errorf("failed to get 'semantic_tokens_list_id' from config: %v", err))
		}
		ret.semanticTokensListID = defaultSemanticTokensListID
	}

	// default is disabled for all
	ret.enableSemanticTokens = make(map[string]bool)

	var enableSemanticTokensIfc map[string]interface{}
	enableSemanticTokensIfc, err = pconfig.GetMap("enable_semantic_tokens")
	if err != nil {
		if err != config.ErrNotFound {
			configErr = multierr.Append(configErr,
				fmt.Errorf("failed to get 'enable_semantic_tokens' from config: %v", err))
		}
	} else {
		for k, v := range enableSemanticTokensIfc {
			b, ok := v.(bool)
			if !ok {
				configErr = multierr.Append(configErr,
					fmt.Errorf("failed to get 'enable_semantic_tokens' from config: "+
						"expected map of string to bool, found %q to be %v", k, v))
				continue
			}
			ret.enableSemanticTokens[k] = b
		}
	}

	ret.diagnosticListID, err = pconfig.GetString("diagnostic_list_id")
	if err != nil {
		if err != config.ErrNotFound {
			configErr = multierr.Append(configErr,
				fmt.Errorf("failed to get 'diagnostic_list_id' from config: %v", err))
		}
		ret.diagnosticListID = defaultDiagnosticListID
	}

	ret.connectTimeout, err = config.GetDuration(pconfig,
		"connect_timeout", defaultConnectTimeout)
	if err != nil {
		if err != config.ErrNotFound {
			configErr = multierr.Append(configErr, err)
		}
	}

	ret.disconnectTimeout, err = config.GetDuration(pconfig,
		"disconnect_timeout", defaultDisconnectTimeout)
	if err != nil {
		if err != config.ErrNotFound {
			configErr = multierr.Append(configErr, err)
		}
	}
	ret.hoverWindowAttr, err = config.GetAttributes(pconfig, "hover_window_attr")
	if err != nil {
		if err != config.ErrNotFound {
			configErr = multierr.Append(configErr, err)
		}
		ret.hoverWindowAttr = defaultHoverWindowAttr
	}

	references, err := pconfig.GetMap("references")
	if err != nil {
		if err != config.ErrNotFound {
			configErr = multierr.Append(configErr, err)
		}
	} else {
		cfg := config.MapConfig(references)
		refWindowAttr, err := config.GetAttributes(cfg, "window_attr")
		if err != nil {
			if err != config.ErrNotFound {
				configErr = multierr.Append(configErr, err)
			}
			ret.refWindowAttr = defaultRefWindowAttr
		} else {
			ret.refWindowAttr = refWindowAttr
		}

		refElementAttr, err := config.GetAttributes(cfg, "element_attr")
		if err != nil {
			if err != config.ErrNotFound {
				configErr = multierr.Append(configErr, err)
			}
			// default allocated for search list background
			ret.refElementAttr = new(term.Attributes)
		} else {
			ret.refElementAttr = &refElementAttr
		}
		refMatchedAttr, err := config.GetAttributes(cfg, "matched_text_attr")
		if err != nil {
			if err != config.ErrNotFound {
				configErr = multierr.Append(configErr, err)
			}
		} else {
			ret.refMatchedAttr = &refMatchedAttr
		}
		refCountAttr, err := config.GetAttributes(cfg, "count_attr")
		if err != nil {
			if err != config.ErrNotFound {
				configErr = multierr.Append(configErr, err)
			}
		} else {
			ret.refCountAttr = &refCountAttr
		}
		refFocusElementAttr, err := config.GetAttributes(cfg, "focus_element_attr")
		if err != nil {
			if err != config.ErrNotFound {
				configErr = multierr.Append(configErr, err)
			}
		} else {
			ret.refFocusElementAttr = &refFocusElementAttr
		}
	}

	ret.rpcTimeout, err = config.GetDuration(pconfig,
		"rpc_timeout", defaultRpcTimeout)
	if err != nil {
		if err != config.ErrNotFound {
			configErr = multierr.Append(configErr, err)
		}
	}
	if configErr != nil {
		log.Warnf("Errors loading config: %v", configErr)
	}

	err = ret.initLanguageServers(pconfig)
	if err != nil {
		return nil, err
	}

	for _, g := range grants {
		switch g.Permission {
		case extensionapi.PermissionFileSystem:
			ret.fs, err = workspaceext.FileSystem(ctx, g, broker)
			if err != nil {
				return nil, err
			}
			cwdURI, err := ret.fs.URI(".")
			if err != nil {
				return nil, err
			}
			ret.cwd = cwdURI.Path()
		case extensionapi.PermissionExecute:
			ret.exec, err = workspaceext.Executor(ctx, g, broker)
			if err != nil {
				return nil, err
			}
		case extensionapi.PermissionInterrupt:
			ret.p, err = browserext.EventPublisher(ctx, g, broker)
			if err != nil {
				return nil, err
			}
		case extensionapi.PermissionBrowserResourceOpener:
			ret.o, err = browserext.ResourceOpener(ctx, g, broker)
			if err != nil {
				return nil, err
			}
		case extensionapi.PermissionBrowserWindowManager:
			ret.wm, err = browserext.WindowManager(ctx, g, broker)
			if err != nil {
				return nil, err
			}
		case extensionapi.PermissionNotifications:
			ret.m, err = browserext.Notifications(ctx, g, broker)
			if err != nil {
				return nil, err
			}
		case extensionapi.PermissionConfig:
			config, err := configextension.FetchConfig(ctx, g, broker)
			if err != nil {
				return nil, err
			}
			ret.tabspaces, err = extutil.Tabspaces(config)
			if err != nil {
				log.Warnf("Could not get tabspaces from config: %s.. Using default of %d",
					err, ret.tabspaces)
			}
			ret.frame, err = extutil.WindowManagerFrame(config)
			if err != nil {
				log.Warnf("Could not get frame from config: %s.. Using default of %t",
					err, ret.frame)
			}
		}
	}

	log.Debugf("Initialized LSP handler with cwd %q", ret.cwd)

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
		log.Errorf("failed to start language server for %q: %s", languageID, err)
		return execServer{}, false
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	h.servers[languageID] = srv

	return srv, true
}

func (h *lspEditorHandler) newFile(
	handler textapi.Handler, uri workspaceapi.URI, content string,
) *file {

	spanURI := workspaceURIToSpan(uri)
	languageID := filepath.Ext(uri.Path())

	f := &file{
		_version: firstFileVersion,
		handler:  handler,
		docID: protocol.TextDocumentIdentifier{
			URI: protocol.URIFromSpanURI(spanURI),
		},
		uri:        uri,
		_cells:     term.StringToCells(content),
		languageID: languageID,
	}

	return f
}

func (h *lspEditorHandler) addNewFile(
	handler textapi.Handler, uri workspaceapi.URI, content string,
) *file {
	f := h.newFile(handler, uri, content)

	h.mu.Lock()
	defer h.mu.Unlock()

	h.files[uri.String()] = f

	return f
}

func (h *lspEditorHandler) removePendingGoTo(uri workspaceapi.URI) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.pendingGoTo, uri.String())
}

func (h *lspEditorHandler) addPendingGoTo(
	uri workspaceapi.URI, rs protocol.Range,
) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.pendingGoTo[uri.String()] = rs
}

func (h *lspEditorHandler) addPendingDiagnostics(
	uri workspaceapi.URI, ds []protocol.Diagnostic,
) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.pendingDiagnostic[uri.String()] = ds
}

func (h *lspEditorHandler) dispatchPendingDiagnostics(
	ctx context.Context, uri workspaceapi.URI,
) error {
	h.mu.Lock()
	ds, ok := h.pendingDiagnostic[uri.String()]
	delete(h.pendingDiagnostic, uri.String())
	h.mu.Unlock()
	if !ok {
		return nil
	}

	return h.handleDiagnostics(ctx, uri, ds, firstFileVersion)
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

func (h *lspEditorHandler) handleGoTo(f *file, rs protocol.Range) error {
	cells := h.getCells(f)
	buf := cell.CellsToBuffer(cells)
	spanURI := workspaceURIToSpan(f.uri)
	colmap := getColumnMapper(spanURI, buf)
	pos, _, ok := convertRange(rs, cells, colmap)
	if !ok {
		return fmt.Errorf("could not convert lsp range to coordinates")
	}

	err := h.ed.SetCursor(f.handler, pos)
	if err != nil {
		err = fmt.Errorf("ed.SetCursor(%s): %v", f.uri, err)
		return err
	}
	return nil
}

func (h *lspEditorHandler) dispatchPendingGoTo(
	ctx context.Context, srv execServer, f *file,
) error {
	uri := f.uri

	h.mu.Lock()
	rs, ok := h.pendingGoTo[uri.String()]
	delete(h.pendingGoTo, uri.String())
	h.mu.Unlock()
	if !ok {
		return nil
	}

	return h.handleGoTo(f, rs)
}

func (h *lspEditorHandler) getFile(uri workspaceapi.URI) (*file, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()

	f, ok := h.files[uri.String()]
	return f, ok
}

func parseLocationData(
	uri span.URI, cells [][]term.Cell, content []byte, d []uint32,
	semanticTypes map[string]tcell.Color,
) (ret []textapi.Location) {
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

		loc := textapi.Location{From: from, To: to, Attr: term.Attributes{Fg: attr}}
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
) error {
	ext := filepath.Ext(f.uri.Path())
	if enabled, ok := h.enableSemanticTokens[ext]; !ok || !enabled {
		log.Debugf("lspEditorHandler.Server.SemanticTokensFull(%s): disabled for file with extension %s",
			f.uri, ext)
		return nil
	}
	// NOTE: gopls does not pass semanticTokensProvider
	if srv.caps.SemanticTokensProvider == nil && srv.langID != ".go" {
		log.Debugf("lspEditorHandler.Server.SemanticTokensFull(%s): server does not support semantic tokens", f.uri)
		return nil
	}
	version := h.getVersion(f)
	p2 := protocol.SemanticTokensParams{TextDocument: f.docID}
	resp, err := srv.srv.SemanticTokensFull(ctx, &p2)
	if err != nil || resp == nil {
		err = fmt.Errorf("Server.SemanticTokensFull(%s): %v", f.uri, err)
		return err
	}
	if resp == nil {
		return errors.New("empty Server.SemanticTokensFull response")
	}
	if h.getVersion(f) != version {
		log.Warnf("lspEditorHandler.Server.SemanticTokensFull(%s): stale result", f.uri)
		return nil
	}

	spanURI := workspaceURIToSpan(f.uri)
	locations := parseLocationData(spanURI, cells, []byte(content), resp.Data,
		h.semanticTypesAttr)
	if log.IsLevelEnabled(log.TraceLevel) {
		log.Tracef("lspEditorHandler.Server.SemanticTokensFull(%s): OK: %v: locations: %v",
			f.uri, resp.Data, locations)
	}
	err = h.ed.SetLocationList(f.handler, textapi.LocationPriorityInfo,
		h.semanticTokensListID, textapi.LocationSlice(locations))
	if err != nil {
		err = fmt.Errorf("SetLocationList(%s): %v", f.uri, err)
		return err
	}
	return nil
}

// https://microsoft.github.io/language-server-protocol/specifications/specification-current/#range
func makeProtocolRange(
	oldCells [][]term.Cell, from, to term.Coordinates,
) protocol.Range {
	from, to = term.CoordinatesSort(from, to)
	starty, startx, ok := cell.ConvertCoordinatesToRunePos(oldCells, from)
	if !ok {
		panic("coordinates out of sync")
	}
	endy, endx, ok := cell.ConvertCoordinatesToRunePos(oldCells, to)
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
		err = fmt.Errorf("Server.DidChange(%s): %v", f.uri, err)
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
) error {
	version := h.incrementVersion(f)

	evts := []protocol.TextDocumentContentChangeEvent{{Text: content}}
	err := h.callServerDidChange(ctx, srv, f, evts)
	if err != nil {
		err = fmt.Errorf("failed to send file update: file=%v, length=%v, version=%v, err=%s",
			f.uri, len(content), version, err)
		return err
	}
	log.Tracef("sent full file update: file=%v, length=%v, version=%v",
		f.uri, len(content), version)
	return nil
}

func (h *lspEditorHandler) sendChangeWatchedFiles(
	ctx context.Context, srv execServer, uri workspaceapi.URI,
	t protocol.FileChangeType,
) error {
	spanURI := workspaceURIToSpan(uri)
	params := protocol.DidChangeWatchedFilesParams{
		Changes: []protocol.FileEvent{
			{URI: protocol.URIFromSpanURI(spanURI), Type: t},
		},
	}
	log.Tracef("lspEditorHandler.Server.DidChangeWatchedFiles(%s)", uri)
	err := srv.srv.DidChangeWatchedFiles(ctx, &params)
	if err != nil {
		err = fmt.Errorf("did change watched files(%s): %v", uri, err)
	}
	return err
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
		" rangeStart: %#v, rangeEnd: %#v, from=%#v, to=%#v",
		f.uri, len(content), version, rng.Start, rng.End, from, to)

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

func (h *lspEditorHandler) handleFileFlush(ev textapi.Event) error {
	ctx := context.Background()
	ctx, cancelFn := context.WithTimeout(ctx, h.rpcTimeout)
	defer cancelFn()

	// content last EOL is trimmed by the buffer's unix file reader.
	// lsp expects the last EOL
	ev.Content += "\n"

	f, ok := h.getFile(ev.URI)
	if !ok {
		// do not add file, as it might or might
		// not be an open file. If we couldn't find it
		// in the cache, then it's probably not an open file.
		f = h.addNewFile(ev.Resource, ev.URI, ev.Content)
	}

	srv, ok := h.getServer(f.languageID)
	if !ok {
		return errNoServer
	}

	err := h.pushFullEdit(ctx, srv, f, ev.Content)
	if err != nil {
		return err
	}
	h.setCells(f, term.StringToCells(ev.Content))
	ctx = h.newSemanticTokensCtx()
	return h.semanticTokensFull(ctx, srv, f, h.getCells(f), ev.Content)
}

func (h *lspEditorHandler) handleChangeWatchedFiles(
	ev textapi.Event, t protocol.FileChangeType,
) error {
	ctx := context.Background()
	ctx, cancelFn := context.WithTimeout(ctx, h.rpcTimeout)
	defer cancelFn()

	languageID := filepath.Ext(ev.URI.Path())

	srv, ok := h.getServer(languageID)
	if !ok {
		return errNoServer
	}

	return h.sendChangeWatchedFiles(ctx, srv, ev.URI, t)
}

func (h *lspEditorHandler) handleFileEdit(ev textapi.Event) error {
	ctx := context.Background()
	ctx, cancelFn := context.WithTimeout(ctx, h.rpcTimeout)
	defer cancelFn()
	f, ok := h.getFile(ev.URI)
	if !ok {
		err := fmt.Errorf("received insert/delete event for an unknown file: %#v", ev)
		return err
	}

	srv, ok := h.getServer(f.languageID)
	if !ok {
		return errNoServer
	}

	var ret error
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
		if err := h.setDiagnosticsLocationList(ctx, f, ds); err != nil {
			ret = multierr.Append(ret, err)
		}
	}

	oldCells := h.getCells(f)
	buf := cell.CellsToBuffer(oldCells)
	buf.Edit(ctx, ev.Start, ev.End, ev.Content)
	newCells := buf.RawCells()
	if _, err := h.sendIncrementalEdit(ctx, srv, f, newCells, oldCells,
		ev.Content, ev.Start, ev.End); err != nil {
		ret = multierr.Append(ret, err)
	}
	h.setCells(f, newCells)

	ctx = h.newSemanticTokensCtx()
	if err := h.semanticTokensFull(ctx, srv, f, newCells, buf.String()); err != nil {
		ret = multierr.Append(ret, err)
	}
	return ret
}

func (h *lspEditorHandler) handleFileOpen(ev textapi.Event) error {
	ctx := context.Background()
	ctx, cancelFn := context.WithTimeout(ctx, h.rpcTimeout)
	defer cancelFn()

	// content last EOL is trimmed by the buffer's unix file reader.
	// lsp expects the last EOL
	ev.Content += "\n"

	f := h.addNewFile(ev.Resource, ev.URI, ev.Content)
	srv, ok := h.getServer(f.languageID)
	if !ok {
		return errNoServer
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
		err = fmt.Errorf("Server.DidOpen(%s, %s): %v",
			f.uri, f.languageID, err)
		return err
	}
	log.Tracef("lspEditorHandler.Server.DidOpen(%s, %s)", f.uri, f.languageID)

	var ret error
	if err := h.dispatchPendingDiagnostics(ctx, f.uri); err != nil {
		ret = multierr.Append(ret, err)
	}
	if err := h.dispatchPendingGoTo(ctx, srv, f); err != nil {
		ret = multierr.Append(ret, err)
	}
	ctx = h.newSemanticTokensCtx()
	if err := h.semanticTokensFull(ctx, srv, f, h.getCells(f), ev.Content); err != nil {
		ret = multierr.Append(ret, err)
	}
	return ret
}

func (h *lspEditorHandler) removeFile(resource workspaceapi.URI) (*file, bool) {
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
	ctx context.Context, srv execServer, uri workspaceapi.URI,
) error {
	spanURI := workspaceURIToSpan(uri)
	p := protocol.DidCloseTextDocumentParams{
		TextDocument: protocol.TextDocumentIdentifier{
			URI: protocol.URIFromSpanURI(spanURI),
		},
	}

	if err := srv.srv.DidClose(ctx, &p); err != nil {
		err = fmt.Errorf("Server.DidClose(%s): %v", uri, err)
		return err
	}
	log.Tracef("lspEditorHandler.Server.DidClose(%s)", uri)
	return nil
}

func (h *lspEditorHandler) handleFileClose(ev textapi.Event) error {
	ctx := context.Background()
	ctx, cancelFn := context.WithTimeout(ctx, h.rpcTimeout)
	defer cancelFn()
	f, ok := h.removeFile(ev.URI)
	if !ok {
		err := fmt.Errorf("received close event for an unknown file: %#v", ev)
		return err
	}
	srv, ok := h.getServer(f.languageID)
	if !ok {
		return errNoServer
	}

	var ret error
	if err := h.sendDidClose(ctx, srv, f.uri); err != nil {
		ret = multierr.Append(ret, err)
	}
	h.addPendingDiagnostics(f.uri, h.getDiagnostics(f))
	return ret
}

func (h *lspEditorHandler) parseDiagnostics(
	f *file, d []protocol.Diagnostic,
) []textapi.Location {
	cells := h.getCells(f)
	buf := cell.CellsToBuffer(cells)
	spanURI := workspaceURIToSpan(f.uri)
	colmap := getColumnMapper(spanURI, buf)

	locs := make([]textapi.Location, 0, len(d))
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

		loc := textapi.Location{Message: msg, From: from, To: to, Attr: attr}
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
	ctx context.Context, file workspaceapi.URI,
	ds []protocol.Diagnostic, version int32,
) error {
	f, ok := h.getFile(file)
	if !ok {
		h.addPendingDiagnostics(file, ds)
		log.Tracef("lspEditorHandler: Received diagnostic for a unopened file: %#v", file)
		return nil
	}

	h.setDiagnostics(f, ds)

	if version != h.getVersion(f) {
		log.Debugf("lspEditorHandler: Received diagnostic for "+
			"outdated version of file '%s': %#v", f.uri, version)
		// it seems that tsserver does not send any version info
		// return
	}

	return h.setDiagnosticsLocationList(ctx, f, ds)
}

func (h *lspEditorHandler) setDiagnosticsLocationList(
	ctx context.Context, f *file, ds []protocol.Diagnostic,
) error {
	locs := h.parseDiagnostics(f, ds)
	// NOTE: should probably break down by location priority rather than
	// bundling all of them under Error.
	err := h.ed.SetLocationList(f.handler, textapi.LocationPriorityError,
		h.diagnosticListID, textapi.LocationSlice(locs))
	if err != nil {
		err = fmt.Errorf("SetLocationList(%s): %v", f.uri, err)
		return err
	}
	return err
}

func (h *lspEditorHandler) spanURIToWorkspace(u span.URI) (workspaceapi.URI, error) {
	localFile, err := workspaceapi.ParseURI(string(u))
	if err != nil {
		err = fmt.Errorf("ParseURI: convert LSP URI to workspaceapi.URI %s: %s", u, err)
		return workspaceapi.URI{}, err
	}
	// convert local LSP file URI to the current workspace's URI scheme
	// which could be remote or something else.
	workspaceFile, err := h.fs.URI(localFile.Path())
	if err != nil {
		return workspaceapi.URI{}, fmt.Errorf("workspaceapi.URI: %s", err)
	}
	return workspaceFile, err
}

func workspaceURIToSpan(u workspaceapi.URI) span.URI {
	// the workspace is always local for the language server
	// if URI is a remote uri, then the language server is executed
	// in the remote host as well.
	return span.URI(fmt.Sprintf("file://%s", u.Path()))
}

func (h *lspEditorHandler) HandleDiagnostics(
	ctx context.Context, p *protocol.PublishDiagnosticsParams,
) error {
	var start time.Time
	if log.IsLevelEnabled(log.TraceLevel) {
		start = time.Now()
		log.Tracef("lspEditorHandler.HandleDiagnostics(%#v)", p.URI)
	}
	file, err := h.spanURIToWorkspace(p.URI.SpanURI())
	if err != nil {
		return err
	}

	err = h.handleDiagnostics(ctx, file, p.Diagnostics, p.Version)

	if log.IsLevelEnabled(log.TraceLevel) {
		log.Tracef("lspEditorHandler.HandleDiagnostics(%#v) in %s", p.URI, time.Since(start))
	}

	return err
}

func (h *lspEditorHandler) goToLocation(win browserapi.Window, l protocol.Location) error {
	uri, err := h.spanURIToWorkspace(l.URI.SpanURI())
	if err != nil {
		return err
	}
	f, alreadyOpen := h.getFile(uri)
	if !alreadyOpen {
		h.addPendingGoTo(uri, l.Range)
	}

	buf, err := h.o.Open(uri)
	if err != nil {
		err = fmt.Errorf("browser.Open(%s): %v", uri, err)
		h.removePendingGoTo(uri)
		return err
	}

	if alreadyOpen {
		err := h.handleGoTo(f, l.Range)
		if err != nil {
			return err
		}
	}

	err = h.wm.SetWindowContent(win, buf)
	if err != nil && err != browserapi.ErrTabNotFree {
		err = fmt.Errorf("win.SetContent: %v", err)
		return err
	}
	return nil
}

func (h *lspEditorHandler) getFilePosition(cursor term.Coordinates, uri workspaceapi.URI) (
	f *file, pos protocol.Position, ok bool,
) {
	f, ok = h.getFile(uri)
	if !ok {
		return
	}

	line, column, ok := cell.ConvertCoordinatesToRunePos(h.getCells(f), cursor)
	if !ok {
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
	cursor term.Coordinates, ed textapi.Handler, uri workspaceapi.URI,
	win browserapi.Window,
) error {
	f, pos, ok := h.getFilePosition(cursor, uri)
	if !ok {
		return fmt.Errorf("resource with URI %q not found", uri)
	}

	p := protocol.DefinitionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: f.docID,
			Position:     pos,
		},
	}

	srv, ok := h.getServer(f.languageID)
	if !ok {
		return errNoServer
	}

	ctx := context.Background()
	ctx, cancelFn := context.WithTimeout(ctx, h.rpcTimeout)
	defer cancelFn()

	locs, err := srv.srv.Definition(ctx, &p)
	if err != nil {
		err = fmt.Errorf("Server.Definition(%s, %s): %v", f.uri, f.languageID, err)
		return err
	}

	log.Tracef("lspEditorHandler.Server.Definition(%s, %s): %#v",
		f.uri, f.languageID, locs)

	if len(locs) == 0 {
		err = errors.New("no definitions found for symbol at position")
		return err
	}

	for _, l := range locs {
		if gerr := h.goToLocation(win, l); gerr != nil {
			err = multierr.Append(err, gerr)
		}
	}
	return err
}

func (h *lspEditorHandler) findBestFloatingWindowPosition(
	cursorAtWindow term.Coordinates, height int,
) term.Coordinates {
	at := cursorAtWindow
	// first try to set it above cursor, otherwise below if it's too large
	at.Y -= height
	if at.Y < 0 {
		at.Y = cursorAtWindow.Y + 1
	}

	return at
}

func (h *lspEditorHandler) handleHover(
	cursorAtScroll, cursorAtWindow term.Coordinates,
	ed textapi.Handler, uri workspaceapi.URI,
) error {
	f, pos, ok := h.getFilePosition(cursorAtScroll, uri)
	if !ok {
		return fmt.Errorf("resource with URI %q not found", uri)
	}

	p := protocol.HoverParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: f.docID,
			Position:     pos,
		},
	}

	srv, ok := h.getServer(f.languageID)
	if !ok {
		return errNoServer
	}

	ctx := context.Background()
	ctx, cancelFn := context.WithTimeout(ctx, h.rpcTimeout)
	defer cancelFn()

	hover, err := srv.srv.Hover(ctx, &p)
	if err != nil || hover == nil {
		err = fmt.Errorf("Server.Hover(%s, %s): %v", f.uri, f.languageID, err)
		return err
	}

	log.Tracef("lspEditorHandler.Server.Hover(%s, %s): %#v", f.uri, f.languageID, hover)
	cfg := thandler.DefaultLessConfig()
	cfg.Attributes = h.hoverWindowAttr
	cfg.BarAttr = h.hoverWindowAttr
	less := thandler.NewLess(cfg)
	less.Buffer().WriteString(hover.Contents.Value)
	padx, pady := 1, 1
	if h.frame {
		pady += 2
		padx += 2
	}
	bh := browserapi.NopFloatingHandler(handler.PaddedFloating(
		thandler.FloatingBuffer(less, less.Buffer()), padx, pady))

	at := h.findBestFloatingWindowPosition(cursorAtWindow, less.Buffer().Rows())
	_, err = h.wm.Floating(bh, browserapi.FloatingConfig{Offset: at})
	if err != nil {
		err = fmt.Errorf("wm.Floating: %v", err)
		return err
	}
	return nil
}

func makeWorkspaceFolder(in string) protocol.WorkspaceFolder {
	return protocol.WorkspaceFolder{
		URI:  string(protocol.URIFromPath(in)),
		Name: in,
	}
}

func (h *lspEditorHandler) browseLocations(
	win browserapi.Window, locs []protocol.Location,
) error {
	const locID = "highlight_loc"
	var (
		longestLocation int
		bottom, top     browserapi.Window
		done            bool
	)
	cfg := search.ListConfig{
		Algo:             search.FuzzyMatch,
		Interrupter:      h.p,
		CaseSensitive:    false,
		MatchedTextAttr:  h.refMatchedAttr,
		ElementAttr:      h.refElementAttr,
		CountAttr:        h.refCountAttr,
		FocusElementAttr: h.refFocusElementAttr,
	}
	list := search.NewList(cfg)
	textToLocation := make(map[string]protocol.Location)
	buf := cell.NewBuffer()
	ed := vi.Editor(vi.WithAttr(h.refWindowAttr))
	edh, err := ed.Edit(workspaceapi.RandomURI("lsp"), buf, false, false)
	if err != nil {
		err = fmt.Errorf("ed.Edit: %s", err)
		return err
	}

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

	closeWin := func(win browserapi.Window) func() error {
		return func() error {
			h.mu.Lock()
			shouldClose := done && win != nil
			closeWin := win
			h.mu.Unlock()
			if shouldClose {
				return h.wm.CloseWindow(closeWin)
			}
			return nil
		}
	}

	renderFile := func(l protocol.Location) {
		uri := l.URI.SpanURI()
		// Open assumes path in current workspace
		f, oerr := h.fs.OpenFile(uri.Filename(), os.O_RDONLY, 0)
		if oerr != nil {
			log.Errorf("could not render preview file: Open: %v", oerr)
			return
		}
		defer f.Close()
		data, err := io.ReadAll(f)
		if err != nil {
			log.Errorf("could not render preview file: Read: %v", err)
			return
		}
		content := string(data)

		_ = edh.SetCursorAtScroll(term.Coordinates{})
		buf.Reset()
		buf.WriteString(content)

		cells := buf.RawCells()
		colmap := getColumnMapper(uri, buf)
		from, to, ok := convertRange(l.Range, cells, colmap)
		if !ok {
			log.Debugf("lspEditorHandler.convertRange: %v", ok)
			return
		}

		attrs := term.Attributes{Attrs: tcell.AttrReverse}
		loc := textapi.Location{From: from, To: to, Attr: attrs}
		edh.SetLocationList(textapi.LocationPriorityInfo,
			locID, textapi.LocationSlice([]textapi.Location{loc}))
		edh.MoveToPrevLocation(locID)
	}
	sed, _ := modeless.Editor(modeless.WithWrap(true)).
		Edit(workspaceapi.RandomURI("search"), list.Buffer(), false, false)
	sh := search.Handler(list, sed, func(text string) {
		h.mu.Lock()
		done = true
		h.mu.Unlock()

		// liberate all tabs
		_ = closeWin(top)()
		_ = closeWin(bottom)()

		err := h.goToLocation(win, textToLocation[text])
		if err != nil {
			_, _ = h.m.Notify(browserapi.LevelError, "go to location: %s", err)
		}
	})

	// wrap to detect when focus has changed
	// and re-render window.
	bh := handler.Wrap(sh, func(ev term.Event) (bool, bool) {
		before, _ := list.Focus()
		exit, handle := sh.Handle(ev)
		list.Wait()
		after, _ := list.Focus()
		afterStr := string(after.Data())
		if exit {
			done = true
		}
		if string(before.Data()) != afterStr {
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
			renderFile(textToLocation[string(focus.Data())])
			return false, true
		}

		return edh.Handle(ev)
	})

	bhtop := browserapi.FuncHandler(eh, closeWin(bottom))
	top, err = h.wm.Split(browserapi.OrientationBottom, win, bhtop)
	if err != nil {
		err = fmt.Errorf("wm.split: %v", err)
		return err
	}

	bhbottom := browserapi.FuncHandler(handler.WithComponent(bh,
		component.WithBackground(bh, term.Cell{Ch: ' ', Attributes: *h.refElementAttr}),
	), closeWin(top))
	bottom, err = h.wm.Split(browserapi.OrientationBottom, top, bhbottom)
	if err != nil {
		err = fmt.Errorf("wm.Split: %v", err)
		return err
	}
	return nil
}

func (h *lspEditorHandler) handleReferences(
	cursorAtScroll, cursorAtWindow term.Coordinates,
	ed textapi.Handler, uri workspaceapi.URI, win browserapi.Window,
) error {
	f, pos, ok := h.getFilePosition(cursorAtScroll, uri)
	if !ok {
		return fmt.Errorf("resource with URI %q not found", uri)
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
		return errNoServer
	}

	ctx := context.Background()
	ctx, cancelFn := context.WithTimeout(ctx, h.rpcTimeout)
	defer cancelFn()

	locs, err := srv.srv.References(ctx, &p)
	if err != nil {
		err = fmt.Errorf("Server.References(%s, %s): %v", f.uri, f.languageID, err)
		return err
	}

	log.Tracef("lspEditorHandler.Server.References(%s, %s): %#v", f.uri, f.languageID, locs)

	if len(locs) == 0 {
		err = errors.New("no references found for symbol at position")
		return err
	}

	return h.browseLocations(win, locs)
}

func toJSONEdits(edits []protocol.TextEdit) string {
	m, _ := json.Marshal(edits)
	return string(m)
}

func (h *lspEditorHandler) format(
	ctx context.Context, f *file, srv execServer, builder *editBuilder,
) error {
	p := protocol.DocumentFormattingParams{
		TextDocument: f.docID,
		Options: protocol.FormattingOptions{
			TabSize:                uint32(h.tabspaces),
			InsertSpaces:           false,
			TrimTrailingWhitespace: true,
			InsertFinalNewline:     false,
			TrimFinalNewlines:      false,
		},
	}

	edits, err := srv.srv.Formatting(ctx, &p)
	if err != nil {
		err = fmt.Errorf("Server.Formatting(%s): %v", f.uri, err)
		return err
	}

	// json marshaling is expensive
	if log.IsLevelEnabled(log.TraceLevel) {
		log.Tracef("lspEditorHandler.Server.Formatting(%s): %v", f.uri, toJSONEdits(edits))
	}

	return builder.applyEdits(edits)
}

func (h *lspEditorHandler) organizeImports(
	ctx context.Context, f *file, srv execServer, builder *editBuilder,
) error {
	p := protocol.CodeActionParams{
		TextDocument: f.docID,
		Context: protocol.CodeActionContext{
			Only: []protocol.CodeActionKind{protocol.Source, protocol.SourceOrganizeImports},
		},
	}
	codeActions, err := srv.srv.CodeAction(ctx, &p)
	if err != nil {
		err = fmt.Errorf("Server.CodeAction(%s): %v", f.uri, err)
		return err
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	var ret error
	for _, ca := range codeActions {
		switch ca.Kind {
		case protocol.Source, protocol.SourceOrganizeImports:
			log.Tracef("lspEditorHandler.Server.CodeAction(%s): %v", f.uri, ca.Edit)
			if err := builder.applyWorkspaceEdit(ca.Edit); err != nil {
				ret = multierr.Append(ret, err)
			}
		}
	}
	return ret
}

func (h *lspEditorHandler) handleFormat(ed textapi.Handler, uri workspaceapi.URI, imports bool) error {
	ctx := context.Background()
	ctx, cancelFn := context.WithTimeout(ctx, h.rpcTimeout)
	defer cancelFn()

	f, ok := h.getFile(uri)
	if !ok {
		err := fmt.Errorf("extraneous file %q", uri.String())
		return err
	}

	srv, ok := h.getServer(f.languageID)
	if !ok {
		return errNoServer
	}
	w := h.ed.CellEditor(ed)

	// this cells are used to map edit ranges to term.Coordinates
	// but discarded because only Insert/Delete events should
	// apply changes to the local file copy.
	cells := h.getCells(f)

	var b editBuilder
	b.init(h.tabspaces, f, w, cells)

	var err error
	if imports {
		err = h.organizeImports(ctx, f, srv, &b)
	} else {
		err = h.format(ctx, f, srv, &b)
	}
	return err
}

func (h *lspEditorHandler) Complete(ctx context.Context, name string, args []string) (
	iterator.Iterator[string], error,
) {
	return iterator.FromSlice[string](nil), nil
}

func (h *lspEditorHandler) HandleCommand(
	ctx context.Context, cmd textapi.Command,
) (err error) {
	if cmd.Resource == nil {
		return
	}

	switch cmd.Name {
	case commandHover:
		err = h.handleHover(cmd.Cursor.Content, cmd.Cursor.Window, cmd.Resource, cmd.URI)
	case commandGoToDef:
		err = h.handleGoToDefinition(cmd.Cursor.Content, cmd.Resource, cmd.URI, cmd.Window)
	case commandReferences:
		err = h.handleReferences(cmd.Cursor.Content, cmd.Cursor.Window, cmd.Resource, cmd.URI, cmd.Window)
	case commandFormat:
		err = h.handleFormat(cmd.Resource, cmd.URI, false)
	case commandOrganizeImports:
		err = h.handleFormat(cmd.Resource, cmd.URI, true)
	}

	return
}

func (h *lspEditorHandler) Handle(
	ctx context.Context, ev textapi.Event,
) (exit bool) {
	h.mu.Lock()
	exit = h.exit
	h.mu.Unlock()
	if exit {
		return
	}

	var start time.Time
	if log.IsLevelEnabled(log.TraceLevel) {
		start = time.Now()
		log.Tracef("lspEditorHandler.Handle(%v)", ev.Type)
	}

	var err error
	switch ev.Type {
	case textapi.EventTypeOpen:
		err = h.handleFileOpen(ev)
	case textapi.EventTypeClose:
		err = h.handleFileClose(ev)
	case textapi.EventTypeFlush:
		err = h.handleFileFlush(ev)
	case textapi.EventTypeRemove:
		err = h.handleChangeWatchedFiles(ev, protocol.Deleted)
	case textapi.EventTypeRename:
		err = h.handleChangeWatchedFiles(ev, protocol.Changed)
	case textapi.EventTypeCreate:
		err = h.handleChangeWatchedFiles(ev, protocol.Created)
	case textapi.EventTypeChange:
		err = h.handleChangeWatchedFiles(ev, protocol.Changed)
	case textapi.EventTypeEdit:
		err = h.handleFileEdit(ev)
	}

	if log.IsLevelEnabled(log.TraceLevel) {
		log.Tracef("lspEditorHandler.Handle(%v) in %s: %s", ev.Type, time.Since(start), err)
	}
	if err != nil && err != errNoServer {
		log.Errorf("failed to process file event %v: %v", ev.Type, err)
	}

	return
}

func (h *lspEditorHandler) Close() error {
	h.mu.Lock()
	h.exit = true
	h.mu.Unlock()

	log.Debugf("shutting down %d servers", len(h.servers))

	var wg sync.WaitGroup
	var i int
	errors := make([]error, len(h.servers))
	wg.Add(len(h.servers))
	for _, server := range h.servers {
		go func(ret *error, server execServer) {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), h.disconnectTimeout)
			defer cancel()
			log.Debugf("shutting down server %v for %s", server.pid, server.langID)
			err := server.srv.Shutdown(ctx)
			if err != nil {
				*ret = multierr.Append(*ret, fmt.Errorf("shutdown: %v", err))
			}
			log.Debugf("closing server pipes for process %v for %s", server.pid, server.langID)
			for _, closer := range server.closers {
				if err := closer.Close(); err != nil {
					*ret = multierr.Append(*ret, fmt.Errorf("pipe close: %v", err))
				}
			}
			log.Debugf("done cleaning resources for %v for %s", server.pid, server.langID)
		}(&errors[i], server)
		i++
	}
	wg.Wait()

	var ret error
	for _, err := range errors {
		if err != nil {
			ret = multierr.Append(ret, err)
		}
	}
	level := log.DebugLevel
	if ret != nil {
		level = log.ErrorLevel
	}

	log.WithFields(log.Fields{}).
		Logf(level, "shut down servers: %d: %v", len(h.servers), ret)

	h.cancelCtx()

	return ret
}
