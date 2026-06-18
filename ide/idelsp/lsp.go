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
	"strings"
	"sync"

	"github.com/unstablebuild/rune-go-sdk/api/semanticapi"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"unstable.build/go-tui/debug"
)

const (
	// InitializeOptionsLanguageID is the property that LSP clients must pass
	// to Initialize in order for it to recognize for what language the
	// it needs to initialize an LSP server.
	InitializeOptionsLanguageID = "langID"
	// InitializeOptionsLanguageCommand is the property that LSP clients must pass
	// to Initialize in order for it to know what program to look for when
	// initializing an LSP server for the given language. This can be an absolute path
	// or a name which will be searched in the user's PATH.
	InitializeOptionsLanguageCommand = "command"
	// InitializeOptionsAlternateCommands is an optional property mapping
	// individual LSP methods (e.g. "textDocument/formatting") to an
	// alternate command string that should serve those methods. When
	// present, the manager spawns one extra server per distinct
	// alternate command and routes the listed methods to it, while the
	// default command from InitializeOptionsLanguageCommand serves the
	// rest. This enables composing several single-purpose servers (such
	// as ty + ruff for Python) behind one language id.
	InitializeOptionsAlternateCommands = "alternate_commands"
)

// Initialize initializes an LSP server. The incoming InitializeParams.InitializeOptions
// json object, must have two extra properties set: `langID` and `command`, which
// determine the language identifier and the command (absolute path of the LSP
// executable + args) used to run the server.
func (m *Manager) Initialize(ctx context.Context, params semanticapi.InitializeParams) (
	ret semanticapi.InitializeResult, err error,
) {
	params.WorkDoneToken = m.tokenFor(params.WorkDoneToken)
	if params.RootURI != m.rootURI {
		err = errors.New("initializing LSP server for the wrong workspace: " +
			"uris don't match")
		return
	}
	var initialOptions map[string]any
	err = json.Unmarshal(params.InitializeOptions, &initialOptions)
	if err != nil {
		err = fmt.Errorf("decode initialize options: "+
			"decode language ID: %v", err)
		return
	}
	idAny, ok := initialOptions[InitializeOptionsLanguageID]
	if !ok {
		err = fmt.Errorf("decode initialize options: "+
			"decode language ID: '%s' not found",
			InitializeOptionsLanguageID)
		return
	}
	id, ok := idAny.(string)
	if !ok {
		err = fmt.Errorf("decode initialize options: "+
			"decode language ID: '%s' should be a string",
			InitializeOptionsLanguageID)
		return
	}
	cmdAny, ok := initialOptions[InitializeOptionsLanguageCommand]
	if !ok {
		err = fmt.Errorf("decode initialize options: "+
			"decode language command: '%s' not found",
			InitializeOptionsLanguageCommand)
		return
	}
	cmdAndArgs, ok := cmdAny.(string)
	if !ok {
		err = fmt.Errorf("decode initialize options: "+
			"decode language command: '%s' should be a string",
			InitializeOptionsLanguageCommand)
		return
	}
	argv := strings.Split(cmdAndArgs, " ")
	if len(argv) == 0 {
		err = fmt.Errorf("decode initialize options: "+
			"decode language command: '%s' is an empty string",
			InitializeOptionsLanguageCommand)
		return
	}
	alternates, err := parseAlternateCommands(initialOptions)
	if err != nil {
		err = fmt.Errorf("decode initialize options: %w", err)
		return
	}
	m.mu.Lock()
	_, ok = m.servers[id]
	m.mu.Unlock()
	if ok {
		err = errors.New("language server for " +
			"this language already initialized")
		return
	}

	// delete these options so the lsp server doesn't choke on them
	delete(initialOptions, InitializeOptionsLanguageID)
	delete(initialOptions, InitializeOptionsLanguageCommand)
	delete(initialOptions, InitializeOptionsAlternateCommands)

	params.InitializeOptions, err = json.Marshal(initialOptions)
	if err != nil {
		err = fmt.Errorf("re-marshal initialize options: %w", err)
		return
	}
	cfg := langConfig{id: id, command: argv[0], args: argv[1:]}
	if len(alternates) == 0 {
		var srv *langServer
		srv, err = m.initializeServer(ctx, cfg, params)
		if err != nil {
			err = fmt.Errorf("initialize server: %w", err)
			return
		}
		return srv.initResult(), nil
	}
	var srv server
	srv, err = m.initializeMultiServer(ctx, cfg, alternates, params)
	if err != nil {
		err = fmt.Errorf("initialize multi server: %w", err)
		return
	}
	return srv.initResult(), nil
}

// parseAlternateCommands extracts the optional alternate_commands map
// from raw initialize options. It returns nil when the key is absent,
// and an error when present with the wrong type. Each value maps an
// LSP method to the command string that should serve it.
func parseAlternateCommands(opts map[string]any) (map[string]string, error) {
	raw, ok := opts[InitializeOptionsAlternateCommands]
	if !ok {
		return nil, nil
	}
	m, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("'%s' should be a map of LSP method to command",
			InitializeOptionsAlternateCommands)
	}
	ret := make(map[string]string, len(m))
	for method, v := range m {
		cmd, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("'%s.%s' should be a command string",
				InitializeOptionsAlternateCommands, method)
		}
		ret[method] = cmd
	}
	return ret, nil
}

// Initialized is a no-op; servers are initialized lazily.
func (m *Manager) Initialized(ctx context.Context) error {
	return nil
}

// Shutdown shuts down all active servers.
func (m *Manager) Shutdown(ctx context.Context) error {
	servers := m.allServers()
	var errs []error
	for _, srv := range servers {
		err := srv.call(ctx, "shutdown", nil, nil)
		if err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// Exit sends exit to all active servers.
func (m *Manager) Exit(ctx context.Context) error {
	servers := m.allServers()
	var errs []error
	for _, srv := range servers {
		err := srv.notify(ctx, "exit", nil)
		if err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// DidOpen forwards the notification to the owning server.
func (m *Manager) DidOpen(
	ctx context.Context,
	params semanticapi.DidOpenTextDocumentParams,
) error {
	srv, err := m.serverForURI(params.TextDocument.URI)
	if err != nil {
		return err
	}
	return srv.notify(
		ctx, "textDocument/didOpen", params,
	)
}

// DidChange forwards the notification to the owning
// server.
func (m *Manager) DidChange(
	ctx context.Context,
	params semanticapi.DidChangeTextDocumentParams,
) error {
	srv, err := m.serverForURI(params.TextDocument.URI)
	if err != nil {
		return err
	}
	err = srv.notify(
		ctx, "textDocument/didChange", params,
	)
	if err == nil {
		m.callback.FileDidChange(params.TextDocument.URI, params.TextDocument.Version, true, false)
	}
	return err
}

// DidClose forwards the notification to the owning server.
func (m *Manager) DidClose(
	ctx context.Context,
	params semanticapi.DidCloseTextDocumentParams,
) error {
	srv, err := m.serverForURI(params.TextDocument.URI)
	if err != nil {
		return err
	}
	return srv.notify(
		ctx, "textDocument/didClose", params,
	)
}

// DidSave forwards the notification to the owning server.
func (m *Manager) DidSave(
	ctx context.Context,
	params semanticapi.DidSaveTextDocumentParams,
) error {
	srv, err := m.serverForURI(params.TextDocument.URI)
	if err != nil {
		return err
	}
	return srv.notify(
		ctx, "textDocument/didSave", params,
	)
}

// WillSave forwards the notification to the owning server.
func (m *Manager) WillSave(
	ctx context.Context,
	params semanticapi.WillSaveTextDocumentParams,
) error {
	srv, err := m.serverForURI(params.TextDocument.URI)
	if err != nil {
		return err
	}
	return srv.notify(
		ctx, "textDocument/willSave", params,
	)
}

// Completion routes to the owning server.
func (m *Manager) Completion(
	ctx context.Context,
	params semanticapi.CompletionParams,
) (semanticapi.CompletionResult, error) {
	params.WorkDoneToken = m.tokenFor(params.WorkDoneToken)
	srv, err := m.serverForURI(params.TextDocument.URI)
	if err != nil {
		return semanticapi.CompletionResult{}, err
	}
	var result lspCompletionList
	err = srv.call(ctx, "textDocument/completion", params, &result)
	if err != nil {
		return semanticapi.CompletionResult{}, err
	}
	ret := semanticapi.CompletionResult{
		IsIncomplete: result.IsIncomplete,
	}
	for _, item := range result.Items {
		ret.Items = append(
			ret.Items,
			lspCompletionItemToSemantic(item),
		)
	}
	return ret, nil
}

// Hover routes to the owning server.
func (m *Manager) Hover(
	ctx context.Context,
	params semanticapi.HoverParams,
) (*semanticapi.Hover, error) {
	params.WorkDoneToken = m.tokenFor(params.WorkDoneToken)
	srv, err := m.serverForURI(params.TextDocument.URI)
	if err != nil {
		return nil, err
	}
	var raw json.RawMessage
	err = srv.call(ctx, "textDocument/hover", params, &raw)
	if err != nil {
		return nil, err
	}
	if isNull(raw) {
		return nil, nil
	}
	var result semanticapi.Hover
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// SignatureHelp routes to the owning server.
func (m *Manager) SignatureHelp(
	ctx context.Context,
	params semanticapi.SignatureHelpParams,
) (*semanticapi.SignatureHelp, error) {
	params.WorkDoneToken = m.tokenFor(params.WorkDoneToken)
	srv, err := m.serverForURI(params.TextDocument.URI)
	if err != nil {
		return nil, err
	}
	var raw json.RawMessage
	err = srv.call(ctx, "textDocument/signatureHelp", params, &raw)
	if err != nil {
		return nil, err
	}
	if isNull(raw) {
		return nil, nil
	}
	var result lspSignatureHelp
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, err
	}
	return lspSignatureHelpToSemantic(&result), nil
}

// Definition routes to the owning server.
func (m *Manager) Definition(
	ctx context.Context,
	params semanticapi.DefinitionParams,
) (semanticapi.LocationResult, error) {
	params.WorkDoneToken = m.tokenFor(params.WorkDoneToken)
	return m.locationRequest(
		ctx, params.TextDocument.URI,
		"textDocument/definition", params,
	)
}

// Declaration routes to the owning server.
func (m *Manager) Declaration(
	ctx context.Context,
	params semanticapi.DeclarationParams,
) (semanticapi.LocationResult, error) {
	params.WorkDoneToken = m.tokenFor(params.WorkDoneToken)
	return m.locationRequest(
		ctx, params.TextDocument.URI,
		"textDocument/declaration", params,
	)
}

// TypeDefinition routes to the owning server.
func (m *Manager) TypeDefinition(
	ctx context.Context,
	params semanticapi.TypeDefinitionParams,
) (semanticapi.LocationResult, error) {
	params.WorkDoneToken = m.tokenFor(params.WorkDoneToken)
	return m.locationRequest(
		ctx, params.TextDocument.URI,
		"textDocument/typeDefinition", params,
	)
}

// Implementation routes to the owning server.
func (m *Manager) Implementation(
	ctx context.Context,
	params semanticapi.ImplementationParams,
) (semanticapi.LocationResult, error) {
	params.WorkDoneToken = m.tokenFor(params.WorkDoneToken)
	return m.locationRequest(
		ctx, params.TextDocument.URI,
		"textDocument/implementation", params,
	)
}

// References routes to the owning server.
func (m *Manager) References(
	ctx context.Context,
	params semanticapi.ReferenceParams,
) ([]semanticapi.Location, error) {
	params.WorkDoneToken = m.tokenFor(params.WorkDoneToken)
	srv, err := m.serverForURI(params.TextDocument.URI)
	if err != nil {
		return nil, err
	}
	var result []semanticapi.Location
	err = srv.call(ctx, "textDocument/references", params, &result)
	if err != nil {
		return nil, err
	}
	return result, nil
}

// DocumentHighlight routes to the owning server.
func (m *Manager) DocumentHighlight(
	ctx context.Context,
	params semanticapi.DocumentHighlightParams,
) ([]semanticapi.DocumentHighlight, error) {
	params.WorkDoneToken = m.tokenFor(params.WorkDoneToken)
	srv, err := m.serverForURI(params.TextDocument.URI)
	if err != nil {
		return nil, err
	}
	var result []semanticapi.DocumentHighlight
	err = srv.call(ctx, "textDocument/documentHighlight", params, &result)
	if err != nil {
		return nil, err
	}
	return result, nil
}

// DocumentSymbol routes to the owning server.
func (m *Manager) DocumentSymbol(
	ctx context.Context,
	params semanticapi.DocumentSymbolParams,
) (semanticapi.DocumentSymbolResult, error) {
	params.WorkDoneToken = m.tokenFor(params.WorkDoneToken)
	srv, err := m.serverForURI(params.TextDocument.URI)
	if err != nil {
		return semanticapi.DocumentSymbolResult{}, err
	}
	var raw json.RawMessage
	err = srv.call(ctx, "textDocument/documentSymbol", params, &raw)
	if err != nil {
		return semanticapi.DocumentSymbolResult{}, err
	}
	if isNull(raw) {
		return semanticapi.DocumentSymbolResult{}, nil
	}
	var docSymbols []semanticapi.DocumentSymbol
	if err := json.Unmarshal(raw, &docSymbols); err == nil && len(docSymbols) != 0 &&
		docSymbols[0].Range != (semanticapi.Range{}) {
		return semanticapi.DocumentSymbolResult{
			DocumentSymbols: docSymbols,
		}, nil
	}
	var symInfos []semanticapi.SymbolInformation
	if err := json.Unmarshal(raw, &symInfos); err != nil {
		return semanticapi.DocumentSymbolResult{}, err
	}
	return semanticapi.DocumentSymbolResult{
		SymbolInformation: symInfos,
	}, nil
}

// CodeAction routes to the owning server.
func (m *Manager) CodeAction(
	ctx context.Context,
	params semanticapi.CodeActionParams,
) ([]semanticapi.CodeActionResult, error) {
	params.WorkDoneToken = m.tokenFor(params.WorkDoneToken)
	srv, err := m.serverForURI(params.TextDocument.URI)
	if err != nil {
		return nil, err
	}
	var result []lspCodeAction
	err = srv.call(ctx, "textDocument/codeAction", params, &result)
	if err != nil {
		return nil, err
	}
	ret := make([]semanticapi.CodeActionResult, len(result))
	for i, a := range result {
		action := semanticapi.CodeAction{
			Title: a.Title,
			Kind:  semanticapi.CodeActionKind(a.Kind),
			Edit:  lspWorkspaceEditToSemantic(a.Edit),
		}
		if a.Command != nil {
			cmd := &semanticapi.Command{
				Title:   a.Command.Title,
				Command: a.Command.Command,
			}
			cmd.Arguments = append(
				cmd.Arguments,
				a.Command.Arguments...,
			)
			action.Command = cmd
		}
		ret[i] = semanticapi.CodeActionResult{
			CodeAction: &action,
		}
	}
	return ret, nil
}

// CodeLens routes to the owning server.
func (m *Manager) CodeLens(
	ctx context.Context,
	params semanticapi.CodeLensParams,
) ([]semanticapi.CodeLens, error) {
	params.WorkDoneToken = m.tokenFor(params.WorkDoneToken)
	srv, err := m.serverForURI(params.TextDocument.URI)
	if err != nil {
		return nil, err
	}
	var raw json.RawMessage
	err = srv.call(ctx, "textDocument/codeLens", params, &raw)
	if err != nil {
		return nil, err
	}
	if isNull(raw) {
		return nil, nil
	}
	var result []semanticapi.CodeLens
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// Formatting routes to the owning server.
func (m *Manager) Formatting(
	ctx context.Context,
	params semanticapi.DocumentFormattingParams,
) ([]semanticapi.TextEdit, error) {
	params.WorkDoneToken = m.tokenFor(params.WorkDoneToken)
	srv, err := m.serverForURI(params.TextDocument.URI)
	if err != nil {
		return nil, err
	}
	var raw json.RawMessage
	err = srv.call(ctx, "textDocument/formatting", params, &raw)
	if err != nil {
		return nil, err
	}
	if isNull(raw) {
		return nil, nil
	}
	var result []semanticapi.TextEdit
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// RangeFormatting routes to the owning server.
func (m *Manager) RangeFormatting(
	ctx context.Context,
	params semanticapi.DocumentRangeFormattingParams,
) ([]semanticapi.TextEdit, error) {
	params.WorkDoneToken = m.tokenFor(params.WorkDoneToken)
	srv, err := m.serverForURI(params.TextDocument.URI)
	if err != nil {
		return nil, err
	}
	var raw json.RawMessage
	err = srv.call(ctx, "textDocument/rangeFormatting", params, &raw)
	if err != nil {
		return nil, err
	}
	if isNull(raw) {
		return nil, nil
	}
	var result []semanticapi.TextEdit
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// Rename routes to the owning server.
func (m *Manager) Rename(
	ctx context.Context,
	params semanticapi.RenameParams,
) (*semanticapi.WorkspaceEdit, error) {
	params.WorkDoneToken = m.tokenFor(params.WorkDoneToken)
	srv, err := m.serverForURI(params.TextDocument.URI)
	if err != nil {
		return nil, err
	}
	var raw json.RawMessage
	err = srv.call(ctx, "textDocument/rename", params, &raw)
	if err != nil {
		return nil, err
	}
	if isNull(raw) {
		return nil, nil
	}
	var result lspWorkspaceEdit
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, err
	}
	return lspWorkspaceEditToSemantic(&result), nil
}

// PrepareRename routes to the owning server.
func (m *Manager) PrepareRename(
	ctx context.Context,
	params semanticapi.PrepareRenameParams,
) (*semanticapi.PrepareRenameResult, error) {
	params.WorkDoneToken = m.tokenFor(params.WorkDoneToken)
	srv, err := m.serverForURI(params.TextDocument.URI)
	if err != nil {
		return nil, err
	}
	var raw json.RawMessage
	err = srv.call(ctx, "textDocument/prepareRename", params, &raw)
	if err != nil {
		return nil, err
	}
	if isNull(raw) {
		return nil, nil
	}
	var result semanticapi.PrepareRenameResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// FoldingRange routes to the owning server.
func (m *Manager) FoldingRange(
	ctx context.Context,
	params semanticapi.FoldingRangeParams,
) ([]semanticapi.FoldingRange, error) {
	params.WorkDoneToken = m.tokenFor(params.WorkDoneToken)
	srv, err := m.serverForURI(params.TextDocument.URI)
	if err != nil {
		return nil, err
	}
	var result []semanticapi.FoldingRange
	err = srv.call(ctx, "textDocument/foldingRange", params, &result)
	if err != nil {
		return nil, err
	}
	return result, nil
}

// SelectionRange routes to the owning server.
func (m *Manager) SelectionRange(
	ctx context.Context,
	params semanticapi.SelectionRangeParams,
) ([]semanticapi.SelectionRange, error) {
	params.WorkDoneToken = m.tokenFor(params.WorkDoneToken)
	srv, err := m.serverForURI(params.TextDocument.URI)
	if err != nil {
		return nil, err
	}
	var result []semanticapi.SelectionRange
	err = srv.call(ctx, "textDocument/selectionRange", params, &result)
	if err != nil {
		return nil, err
	}
	return result, nil
}

// SemanticTokensFull routes to the owning server.
func (m *Manager) SemanticTokensFull(
	ctx context.Context,
	params semanticapi.SemanticTokensParams,
) (*semanticapi.SemanticTokens, error) {
	params.WorkDoneToken = m.tokenFor(params.WorkDoneToken)
	srv, err := m.serverForURI(params.TextDocument.URI)
	if err != nil {
		return nil, err
	}
	var raw json.RawMessage
	err = srv.call(ctx, "textDocument/semanticTokens/full", params, &raw)
	if err != nil {
		return nil, err
	}
	if isNull(raw) {
		return nil, nil
	}
	var result semanticapi.SemanticTokens
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// SemanticTokensRange routes to the owning server.
func (m *Manager) SemanticTokensRange(
	ctx context.Context,
	params semanticapi.SemanticTokensRangeParams,
) (*semanticapi.SemanticTokens, error) {
	params.WorkDoneToken = m.tokenFor(params.WorkDoneToken)
	srv, err := m.serverForURI(params.TextDocument.URI)
	if err != nil {
		return nil, err
	}
	var raw json.RawMessage
	err = srv.call(ctx, "textDocument/semanticTokens/range", params, &raw)
	if err != nil {
		return nil, err
	}
	if isNull(raw) {
		return nil, nil
	}
	var result semanticapi.SemanticTokens
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// SemanticTokensFullDelta routes to the owning server.
func (m *Manager) SemanticTokensFullDelta(
	ctx context.Context,
	params semanticapi.SemanticTokensDeltaParams,
) (*semanticapi.SemanticTokensDelta, error) {
	params.WorkDoneToken = m.tokenFor(params.WorkDoneToken)
	srv, err := m.serverForURI(params.TextDocument.URI)
	if err != nil {
		return nil, err
	}
	var raw json.RawMessage
	err = srv.call(ctx, "textDocument/semanticTokens/full/delta", params, &raw)
	if err != nil {
		return nil, err
	}
	if isNull(raw) {
		return nil, nil
	}
	var result semanticapi.SemanticTokensDelta
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// Diagnostic routes to the owning server.
func (m *Manager) Diagnostic(
	ctx context.Context,
	params semanticapi.DocumentDiagnosticParams,
) (semanticapi.DocumentDiagnosticReport, error) {
	params.WorkDoneToken = m.tokenFor(params.WorkDoneToken)

	err := m.callback.WaitFileProcessed(ctx, params.TextDocument.URI)
	if err != nil {
		return semanticapi.DocumentDiagnosticReport{}, err
	}

	srv, err := m.serverForURI(params.TextDocument.URI)
	if err != nil {
		return semanticapi.DocumentDiagnosticReport{}, err
	}
	var result semanticapi.DocumentDiagnosticReport
	err = srv.call(ctx, "textDocument/diagnostic", params, &result)
	if err != nil {
		return semanticapi.DocumentDiagnosticReport{}, err
	}
	return result, nil
}

// PrepareCallHierarchy routes to the owning server.
func (m *Manager) PrepareCallHierarchy(
	ctx context.Context,
	params semanticapi.CallHierarchyPrepareParams,
) ([]semanticapi.CallHierarchyItem, error) {
	params.WorkDoneToken = m.tokenFor(params.WorkDoneToken)
	srv, err := m.serverForURI(params.TextDocument.URI)
	if err != nil {
		return nil, err
	}
	var result []semanticapi.CallHierarchyItem
	err = srv.call(ctx, "textDocument/prepareCallHierarchy", params, &result)
	if err != nil {
		return nil, err
	}
	return result, nil
}

// CallHierarchyIncomingCalls routes via the item URI.
func (m *Manager) CallHierarchyIncomingCalls(
	ctx context.Context,
	params semanticapi.CallHierarchyIncomingCallsParams,
) ([]semanticapi.CallHierarchyIncomingCall, error) {
	params.WorkDoneToken = m.tokenFor(params.WorkDoneToken)
	srv, err := m.serverForURI(params.Item.URI)
	if err != nil {
		return nil, err
	}
	var result []semanticapi.CallHierarchyIncomingCall
	err = srv.call(ctx, "callHierarchy/incomingCalls", params, &result)
	if err != nil {
		return nil, err
	}
	return result, nil
}

// CallHierarchyOutgoingCalls routes via the item URI.
func (m *Manager) CallHierarchyOutgoingCalls(
	ctx context.Context,
	params semanticapi.CallHierarchyOutgoingCallsParams,
) ([]semanticapi.CallHierarchyOutgoingCall, error) {
	params.WorkDoneToken = m.tokenFor(params.WorkDoneToken)
	srv, err := m.serverForURI(params.Item.URI)
	if err != nil {
		return nil, err
	}
	var result []semanticapi.CallHierarchyOutgoingCall
	err = srv.call(ctx, "callHierarchy/outgoingCalls", params, &result)
	if err != nil {
		return nil, err
	}
	return result, nil
}

// CompletionResolve returns the item unchanged.
func (m *Manager) CompletionResolve(
	ctx context.Context,
	item semanticapi.CompletionItem,
) (semanticapi.CompletionItem, error) {
	return item, nil
}

// CodeLensResolve returns the lens unchanged.
func (m *Manager) CodeLensResolve(
	ctx context.Context,
	lens semanticapi.CodeLens,
) (semanticapi.CodeLens, error) {
	return lens, nil
}

// DocumentColor routes to the owning server.
func (m *Manager) DocumentColor(
	ctx context.Context,
	params semanticapi.DocumentColorParams,
) ([]semanticapi.ColorInformation, error) {
	params.WorkDoneToken = m.tokenFor(params.WorkDoneToken)
	srv, err := m.serverForURI(params.TextDocument.URI)
	if err != nil {
		return nil, nil
	}
	var result []semanticapi.ColorInformation
	err = srv.call(ctx, "textDocument/documentColor", params, &result)
	if err != nil {
		return nil, nil
	}
	return result, nil
}

// ColorPresentation routes to the owning server.
func (m *Manager) ColorPresentation(
	ctx context.Context,
	params semanticapi.ColorPresentationParams,
) ([]semanticapi.ColorPresentation, error) {
	params.WorkDoneToken = m.tokenFor(params.WorkDoneToken)
	srv, err := m.serverForURI(params.TextDocument.URI)
	if err != nil {
		return nil, nil
	}
	var result []semanticapi.ColorPresentation
	err = srv.call(ctx, "textDocument/colorPresentation", params, &result)
	if err != nil {
		return nil, nil
	}
	return result, nil
}

// DocumentLink routes to the owning server.
func (m *Manager) DocumentLink(
	ctx context.Context,
	params semanticapi.DocumentLinkParams,
) ([]semanticapi.DocumentLink, error) {
	params.WorkDoneToken = m.tokenFor(params.WorkDoneToken)
	srv, err := m.serverForURI(params.TextDocument.URI)
	if err != nil {
		return nil, nil
	}
	var raw json.RawMessage
	err = srv.call(ctx, "textDocument/documentLink", params, &raw)
	if err != nil {
		return nil, nil
	}
	if isNull(raw) {
		return nil, nil
	}
	var result []semanticapi.DocumentLink
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, nil
	}
	return result, nil
}

// DocumentLinkResolve returns the link unchanged.
func (m *Manager) DocumentLinkResolve(
	ctx context.Context,
	link semanticapi.DocumentLink,
) (semanticapi.DocumentLink, error) {
	return link, nil
}

// OnTypeFormatting routes to the owning server.
func (m *Manager) OnTypeFormatting(
	ctx context.Context,
	params semanticapi.DocumentOnTypeFormattingParams,
) ([]semanticapi.TextEdit, error) {
	srv, err := m.serverForURI(params.TextDocument.URI)
	if err != nil {
		return nil, nil
	}
	var result []semanticapi.TextEdit
	err = srv.call(ctx, "textDocument/onTypeFormatting", params, &result)
	if err != nil {
		return nil, nil
	}
	return result, nil
}

// LinkedEditingRange routes to the owning server.
func (m *Manager) LinkedEditingRange(
	ctx context.Context,
	params semanticapi.LinkedEditingRangeParams,
) (*semanticapi.LinkedEditingRanges, error) {
	params.WorkDoneToken = m.tokenFor(params.WorkDoneToken)
	srv, err := m.serverForURI(params.TextDocument.URI)
	if err != nil {
		return nil, nil
	}
	var raw json.RawMessage
	err = srv.call(ctx, "textDocument/linkedEditingRange", params, &raw)
	if err != nil {
		return nil, nil
	}
	if isNull(raw) {
		return nil, nil
	}
	var result semanticapi.LinkedEditingRanges
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, nil
	}
	return &result, nil
}

// Moniker routes to the owning server.
func (m *Manager) Moniker(
	ctx context.Context,
	params semanticapi.MonikerParams,
) ([]semanticapi.Moniker, error) {
	params.WorkDoneToken = m.tokenFor(params.WorkDoneToken)
	srv, err := m.serverForURI(
		params.TextDocument.URI,
	)
	if err != nil {
		return nil, nil
	}
	var result []semanticapi.Moniker
	err = srv.call(ctx, "textDocument/moniker", params, &result)
	if err != nil {
		return nil, nil
	}
	return result, nil
}

// WillSaveWaitUntil routes to the owning server.
func (m *Manager) WillSaveWaitUntil(
	ctx context.Context,
	params semanticapi.WillSaveTextDocumentParams,
) ([]semanticapi.TextEdit, error) {
	srv, err := m.serverForURI(params.TextDocument.URI)
	if err != nil {
		return nil, nil
	}
	var result []semanticapi.TextEdit
	err = srv.call(ctx, "textDocument/willSaveWaitUntil", params, &result)
	if err != nil {
		return nil, nil
	}
	return result, nil
}

// PrepareTypeHierarchy routes to the owning server.
func (m *Manager) PrepareTypeHierarchy(
	ctx context.Context,
	params semanticapi.TypeHierarchyPrepareParams,
) ([]semanticapi.TypeHierarchyItem, error) {
	params.WorkDoneToken = m.tokenFor(params.WorkDoneToken)
	srv, err := m.serverForURI(params.TextDocument.URI)
	if err != nil {
		return nil, nil
	}
	var result []semanticapi.TypeHierarchyItem
	err = srv.call(ctx, "textDocument/prepareTypeHierarchy", params, &result)
	if err != nil {
		return nil, nil
	}
	return result, nil
}

// TypeHierarchySupertypes routes via the item URI.
func (m *Manager) TypeHierarchySupertypes(
	ctx context.Context,
	params semanticapi.TypeHierarchySupertypesParams,
) ([]semanticapi.TypeHierarchyItem, error) {
	params.WorkDoneToken = m.tokenFor(params.WorkDoneToken)
	srv, err := m.serverForURI(params.Item.URI)
	if err != nil {
		return nil, nil
	}
	var result []semanticapi.TypeHierarchyItem
	err = srv.call(ctx, "typeHierarchy/supertypes", params, &result)
	if err != nil {
		return nil, nil
	}
	return result, nil
}

// TypeHierarchySubtypes routes via the item URI.
func (m *Manager) TypeHierarchySubtypes(
	ctx context.Context,
	params semanticapi.TypeHierarchySubtypesParams,
) ([]semanticapi.TypeHierarchyItem, error) {
	params.WorkDoneToken = m.tokenFor(params.WorkDoneToken)
	srv, err := m.serverForURI(params.Item.URI)
	if err != nil {
		return nil, nil
	}
	var result []semanticapi.TypeHierarchyItem
	err = srv.call(ctx, "typeHierarchy/subtypes", params, &result)
	if err != nil {
		return nil, nil
	}
	return result, nil
}

// InlayHint routes to the owning server.
func (m *Manager) InlayHint(
	ctx context.Context,
	params semanticapi.InlayHintParams,
) ([]semanticapi.InlayHint, error) {
	params.WorkDoneToken = m.tokenFor(params.WorkDoneToken)
	srv, err := m.serverForURI(params.TextDocument.URI)
	if err != nil {
		return nil, nil
	}
	var raw json.RawMessage
	err = srv.call(ctx, "textDocument/inlayHint", params, &raw)
	if err != nil {
		return nil, nil
	}
	if isNull(raw) {
		return nil, nil
	}
	var result []semanticapi.InlayHint
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, nil
	}
	return result, nil
}

// InlayHintResolve returns the hint unchanged.
func (m *Manager) InlayHintResolve(
	ctx context.Context,
	hint semanticapi.InlayHint,
) (semanticapi.InlayHint, error) {
	return hint, nil
}

// InlineValue routes to the owning server.
func (m *Manager) InlineValue(
	ctx context.Context,
	params semanticapi.InlineValueParams,
) ([]semanticapi.InlineValue, error) {
	params.WorkDoneToken = m.tokenFor(params.WorkDoneToken)
	srv, err := m.serverForURI(params.TextDocument.URI)
	if err != nil {
		return nil, nil
	}
	var result []semanticapi.InlineValue
	err = srv.call(ctx, "textDocument/inlineValue", params, &result)
	if err != nil {
		return nil, nil
	}
	return result, nil
}

// WorkspaceDiagnostic fans out to all servers and merges.
// A new workDoneToken is generated per server so that each
// can report progress independently via the CallbackHandler.
func (m *Manager) WorkspaceDiagnostic(
	ctx context.Context, params semanticapi.WorkspaceDiagnosticParams,
) (semanticapi.WorkspaceDiagnosticReport, error) {
	servers := m.allServers()
	if len(servers) == 0 {
		return semanticapi.WorkspaceDiagnosticReport{}, nil
	}
	type result struct {
		report semanticapi.WorkspaceDiagnosticReport
		err    error
	}
	results := make([]result, len(servers))
	var wg sync.WaitGroup
	wg.Add(len(servers))
	for i, srv := range servers {
		go debug.CapturePanicReport(func() {
			func(i int, srv server) {
				defer wg.Done()
				p := params
				token := semanticapi.NewWorkDoneToken()
				if err := m.callback.WorkDoneProgressCreate(
					ctx,
					semanticapi.WorkDoneProgressCreateParams{
						Token: *token,
					},
				); err != nil {
					m.log.Warn("workspace diagnostic: create progress token",
						"error", err,
					)
				}
				p.WorkDoneToken = token
				var report semanticapi.WorkspaceDiagnosticReport
				err := srv.call(
					ctx, "workspace/diagnostic",
					p, &report,
				)
				results[i] = result{report: report, err: err}
			}(i, srv)
		})
	}
	wg.Wait()

	var merged semanticapi.WorkspaceDiagnosticReport
	var errs []error
	for _, r := range results {
		if r.err != nil {
			errs = append(errs, r.err)
			continue
		}
		merged.Items = append(merged.Items, r.report.Items...)
	}
	return merged, errors.Join(errs...)
}

// WorkspaceSymbol fans out to all servers and merges.
func (m *Manager) WorkspaceSymbol(
	ctx context.Context, params semanticapi.WorkspaceSymbolParams,
) ([]semanticapi.SymbolInformation, error) {
	servers := m.allServers()
	if len(servers) == 0 {
		return nil, nil
	}
	type result struct {
		syms []semanticapi.SymbolInformation
		err  error
	}
	results := make([]result, len(servers))
	var wg sync.WaitGroup
	wg.Add(len(servers))
	for i, srv := range servers {
		go debug.CapturePanicReport(func() {
			func(i int, srv server) {
				defer wg.Done()
				p := params
				p.WorkDoneToken = m.tokenFor(p.WorkDoneToken)
				var syms []semanticapi.SymbolInformation
				err := srv.call(
					ctx, "workspace/symbol",
					p, &syms,
				)
				results[i] = result{syms: syms, err: err}
			}(i, srv)
		})
	}
	wg.Wait()

	var merged []semanticapi.SymbolInformation
	var errs []error
	for _, r := range results {
		if r.err != nil {
			errs = append(errs, r.err)
			continue
		}
		merged = append(merged, r.syms...)
	}
	return merged, errors.Join(errs...)
}

// ExecuteCommand broadcasts to all servers, returns
// first non-empty result.
func (m *Manager) ExecuteCommand(
	ctx context.Context,
	params semanticapi.ExecuteCommandParams,
) (string, error) {
	servers := m.allServers()
	var errs []error
	for _, srv := range servers {
		p := params
		p.WorkDoneToken = m.tokenFor(p.WorkDoneToken)
		var raw json.RawMessage
		err := srv.call(ctx, "workspace/executeCommand", p, &raw)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if raw != nil && string(raw) != "null" {
			return string(raw), nil
		}
	}
	return "", errors.Join(errs...)
}

// DidChangeConfiguration broadcasts to all servers.
func (m *Manager) DidChangeConfiguration(
	ctx context.Context,
	params semanticapi.DidChangeConfigurationParams,
) error {
	return m.broadcastNotify(
		ctx, workspaceapi.URI{},
		"workspace/didChangeConfiguration", params,
	)
}

// DidChangeWatchedFiles broadcasts to all servers.
func (m *Manager) DidChangeWatchedFiles(
	ctx context.Context,
	params semanticapi.DidChangeWatchedFilesParams,
) error {
	var hasDeletion bool
	for _, change := range params.Changes {
		m.fileDidChangeOOB(change.URI)
		if change.Type == semanticapi.FileChangeTypeDeleted {
			hasDeletion = true
		}
	}
	// A deletion (or rename, which is delivered as a Deleted+Created
	// pair) can force gopls to re-typecheck dependent packages
	// asynchronously and re-publish diagnostics for unrelated open
	// files. Mark all tracked URIs as pending so subsequent
	// WaitFileProcessed calls block until the next push arrives,
	// avoiding a stale snapshot from the LSP cache.
	if hasDeletion {
		m.callback.InvalidateAllPending()
	}
	return m.broadcastNotify(
		ctx, workspaceapi.URI{},
		"workspace/didChangeWatchedFiles", params,
	)
}

// DidChangeWorkspaceFolders broadcasts to all servers.
func (m *Manager) DidChangeWorkspaceFolders(
	ctx context.Context,
	params semanticapi.DidChangeWorkspaceFoldersParams,
) error {
	return m.broadcastNotify(
		ctx, workspaceapi.URI{},
		"workspace/didChangeWorkspaceFolders", params,
	)
}

// WorkDoneProgressCancel broadcasts to all servers.
func (m *Manager) WorkDoneProgressCancel(
	ctx context.Context,
	params semanticapi.WorkDoneProgressCancelParams,
) error {
	return m.broadcastNotify(
		ctx, workspaceapi.URI{},
		"window/workDoneProgress/cancel", params,
	)
}

// SetTrace broadcasts to all servers.
func (m *Manager) SetTrace(
	ctx context.Context,
	params semanticapi.SetTraceParams,
) error {
	return m.broadcastNotify(
		ctx, workspaceapi.URI{}, "$/setTrace", params,
	)
}

// DidCreateFiles broadcasts to all servers.
func (m *Manager) DidCreateFiles(
	ctx context.Context,
	params semanticapi.CreateFilesParams,
) error {
	return m.broadcastNotify(
		ctx,
		workspaceapi.URI{},
		"workspace/didCreateFiles", params,
	)
}

// DidRenameFiles broadcasts to all servers.
func (m *Manager) DidRenameFiles(
	ctx context.Context,
	params semanticapi.RenameFilesParams,
) error {
	return m.broadcastNotify(
		ctx, workspaceapi.URI{}, "workspace/didRenameFiles", params,
	)
}

// DidDeleteFiles broadcasts to all servers.
func (m *Manager) DidDeleteFiles(
	ctx context.Context,
	params semanticapi.DeleteFilesParams,
) error {
	return m.broadcastNotify(
		ctx, workspaceapi.URI{}, "workspace/didDeleteFiles", params,
	)
}

// WillCreateFiles fans out and merges workspace edits.
func (m *Manager) WillCreateFiles(
	ctx context.Context,
	params semanticapi.CreateFilesParams,
) (*semanticapi.WorkspaceEdit, error) {
	return m.fanOutWorkspaceEdit(
		ctx, "workspace/willCreateFiles", params,
	)
}

// WillRenameFiles fans out and merges workspace edits.
func (m *Manager) WillRenameFiles(
	ctx context.Context,
	params semanticapi.RenameFilesParams,
) (*semanticapi.WorkspaceEdit, error) {
	return m.fanOutWorkspaceEdit(
		ctx, "workspace/willRenameFiles", params,
	)
}

// WillDeleteFiles fans out and merges workspace edits.
func (m *Manager) WillDeleteFiles(
	ctx context.Context,
	params semanticapi.DeleteFilesParams,
) (*semanticapi.WorkspaceEdit, error) {
	return m.fanOutWorkspaceEdit(
		ctx, "workspace/willDeleteFiles", params,
	)
}

func (m *Manager) locationRequest(
	ctx context.Context, uri string,
	method string, params any,
) (semanticapi.LocationResult, error) {
	srv, err := m.serverForURI(uri)
	if err != nil {
		return semanticapi.LocationResult{}, err
	}
	var raw json.RawMessage
	err = srv.call(ctx, method, params, &raw)
	if err != nil {
		return semanticapi.LocationResult{}, err
	}
	if isNull(raw) {
		return semanticapi.LocationResult{}, nil
	}
	var locs []semanticapi.Location
	err = json.Unmarshal(raw, &locs)
	if err == nil && len(locs) != 0 &&
		locs[0] != (semanticapi.Location{}) {
		return semanticapi.LocationResult{Locations: locs}, nil
	}

	var single semanticapi.Location
	err2 := json.Unmarshal(raw, &single)
	if err2 == nil {
		return semanticapi.LocationResult{Location: &single}, err
	}

	var links []semanticapi.LocationLink
	err = json.Unmarshal(raw, &links)
	if err != nil {
		return semanticapi.LocationResult{},
			fmt.Errorf("could not decode location result in any shape: %v", err)
	}
	return semanticapi.LocationResult{LocationLinks: links}, nil
}

func (m *Manager) fanOutWorkspaceEdit(
	ctx context.Context, method string, params any,
) (*semanticapi.WorkspaceEdit, error) {
	servers := m.allServers()
	if len(servers) == 0 {
		return nil, nil
	}
	merged := &semanticapi.WorkspaceEdit{
		Changes: make(map[string][]semanticapi.TextEdit),
	}
	var errs []error
	for _, srv := range servers {
		var raw json.RawMessage
		err := srv.call(ctx, method, params, &raw)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if isNull(raw) {
			continue
		}
		var edit lspWorkspaceEdit
		if err := json.Unmarshal(
			raw, &edit,
		); err != nil {
			continue
		}
		se := lspWorkspaceEditToSemantic(&edit)
		if se == nil {
			continue
		}
		for uri, edits := range se.Changes {
			merged.Changes[uri] = append(
				merged.Changes[uri], edits...,
			)
		}
	}
	if len(merged.Changes) == 0 {
		return nil, errors.Join(errs...)
	}
	return merged, errors.Join(errs...)
}

func isNull(raw json.RawMessage) bool {
	return raw == nil || string(raw) == "null"
}
