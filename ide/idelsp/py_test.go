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
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/unstablebuild/rune-go-sdk/api/semanticapi"
	"github.com/unstablebuild/rune-go-sdk/iterator"
)

// pyPkgManager resolves both the ty and ruff binaries from a single
// language package, mirroring how the Python toolchain ships several
// executables in one lib dir. findBinary disambiguates by base name.
type pyPkgManager struct {
	bins []string
}

func (p *pyPkgManager) LibDir(
	_ context.Context, _ string,
) (iterator.Iterator[string], error) {
	return iterator.FromSlice(p.bins), nil
}

func findOnPath(t *testing.T, name string) string {
	t.Helper()
	bin, err := exec.LookPath(name)
	if err != nil {
		t.Skipf("%s not found, skipping python e2e test", name)
	}
	return bin
}

// setupPythonManager drives a real ty + ruff multi-server setup through
// the Manager: ty is the default child (hover/definition/symbols/etc.)
// and ruff serves formatting via alternate_commands. It self-skips when
// ty or ruff is not installed so CI without the Python toolchain stays
// green. It returns the manager and the opened main.py URI.
func setupPythonManager(
	t *testing.T, ctx context.Context, callback Callback,
) (*Manager, string) {
	t.Helper()
	if callback == nil {
		callback = &testCallback{}
	}
	tyBin := findOnPath(t, "ty")
	ruffBin := findOnPath(t, "ruff")

	tmpDir := setupTestWorkspace(t, filepath.Join("testdata", "py"))
	mainPath := filepath.Join(tmpDir, "main.py")
	mainContent, err := os.ReadFile(mainPath)
	require.NoError(t, err)
	mainURI := "file://" + mainPath

	uri := makeURI(t, "file://"+tmpDir)
	scheme := newTestScheme()

	mgr := New(uri, scheme, scheme,
		&pyPkgManager{bins: []string{tyBin, ruffBin}},
		nil, nil,
		Config{Callback: callback, MaxRetries: 1, NoInitializeServer: true})
	t.Cleanup(func() { _ = mgr.Close() })

	params := autoInitParams(uri.String())
	initOpts, err := json.Marshal(map[string]any{
		"langID":  "python",
		"command": "ty server",
		"alternate_commands": map[string]string{
			"textDocument/formatting":      "ruff server",
			"textDocument/rangeFormatting": "ruff server",
		},
	})
	require.NoError(t, err)
	params.InitializeOptions = initOpts

	_, err = mgr.Initialize(ctx, params)
	require.NoError(t, err)

	require.NoError(t, mgr.DidOpen(ctx, semanticapi.DidOpenTextDocumentParams{
		TextDocument: semanticapi.TextDocumentItem{
			URI:        mainURI,
			LanguageID: "python",
			Version:    0,
			Text:       string(mainContent),
		},
	}))

	return mgr, mainURI
}

// TestE2EPython exercises the same LSP API surface as the Go e2e suite
// against a real ty + ruff multi-server. Each case asserts the exact
// response so a routing or protocol regression fails loudly. The
// positions below are 0-based LSP coordinates into testdata/py/main.py:
//
//	line 17: import os
//	line 20: class Greeter:
//	line 23: def __init__(self, name: str) -> None:
//	line 26: def greet(self) -> str:
//	line 31: def add(a: int, b: int) -> int:
//	line 33: return a+b
//	line 38: g = Greeter("World")
//	line 39: result = add(1, 2)
func TestE2EPython(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	mgr, mainURI := setupPythonManager(t, ctx, nil)

	tests := []struct {
		name string
		fn   func(t *testing.T, mgr *Manager)
	}{
		{
			// ruff (alternate_commands child) serves formatting; the
			// edit reflows the body and fixes the a+b spacing.
			name: "Formatting routes to ruff",
			fn: func(t *testing.T, mgr *Manager) {
				edits, err := mgr.Formatting(ctx,
					semanticapi.DocumentFormattingParams{
						TextDocument: semanticapi.TextDocumentIdentifier{
							URI: mainURI,
						},
						Options: semanticapi.FormattingOptions{
							TabSize: 4, InsertSpaces: true,
						},
					},
				)
				require.NoError(t, err)
				require.Equal(t, []semanticapi.TextEdit{{
					Range: semanticapi.Range{
						Start: semanticapi.Position{Line: 33, Character: 0},
						End:   semanticapi.Position{Line: 44, Character: 10},
					},
					NewText: "    return a + b\n\n\ndef main() -> None:\n" +
						"    g = Greeter(\"World\")\n    print(g.greet())\n" +
						"    result = add(1, 2)\n    print(result)\n\n\n" +
						"if __name__ == \"__main__\":\n    main()\n",
				}}, edits)
			},
		},
		{
			// ty (default child) serves hover.
			name: "Hover routes to ty",
			fn: func(t *testing.T, mgr *Manager) {
				result, err := mgr.Hover(ctx, semanticapi.HoverParams{
					TextDocument: semanticapi.TextDocumentIdentifier{
						URI: mainURI,
					},
					Position: semanticapi.Position{Line: 31, Character: 4},
				})
				require.NoError(t, err)
				require.NotNil(t, result)
				assert.Equal(t, semanticapi.MarkupKindPlainText,
					result.Contents.Kind)
				assert.Equal(t,
					"def add(\n    a: int,\n    b: int\n) -> int\n"+
						"---------------------------------------------\n"+
						"add adds two integers.\n",
					result.Contents.Value)
				require.NotNil(t, result.Range)
				assert.Equal(t, semanticapi.Range{
					Start: semanticapi.Position{Line: 31, Character: 4},
					End:   semanticapi.Position{Line: 31, Character: 7},
				}, *result.Range)
			},
		},
		{
			name: "Definition routes to ty",
			fn: func(t *testing.T, mgr *Manager) {
				result, err := mgr.Definition(ctx,
					semanticapi.DefinitionParams{
						TextDocument: semanticapi.TextDocumentIdentifier{
							URI: mainURI,
						},
						Position: semanticapi.Position{
							Line: 39, Character: 13,
						},
					},
				)
				require.NoError(t, err)
				require.Equal(t, []semanticapi.Location{{
					URI: mainURI,
					Range: semanticapi.Range{
						Start: semanticapi.Position{Line: 31, Character: 4},
						End:   semanticapi.Position{Line: 31, Character: 7},
					},
				}}, result.Locations)
			},
		},
		{
			name: "References routes to ty",
			fn: func(t *testing.T, mgr *Manager) {
				locs, err := mgr.References(ctx,
					semanticapi.ReferenceParams{
						TextDocument: semanticapi.TextDocumentIdentifier{
							URI: mainURI,
						},
						Position: semanticapi.Position{
							Line: 31, Character: 4,
						},
						Context: semanticapi.ReferenceContext{
							IncludeDeclaration: true,
						},
					},
				)
				require.NoError(t, err)
				assert.ElementsMatch(t, []semanticapi.Location{
					{
						URI: mainURI,
						Range: semanticapi.Range{
							Start: semanticapi.Position{Line: 31, Character: 4},
							End:   semanticapi.Position{Line: 31, Character: 7},
						},
					},
					{
						URI: mainURI,
						Range: semanticapi.Range{
							Start: semanticapi.Position{Line: 39, Character: 13},
							End:   semanticapi.Position{Line: 39, Character: 16},
						},
					},
				}, locs)
			},
		},
		{
			name: "DocumentSymbol routes to ty",
			fn: func(t *testing.T, mgr *Manager) {
				result, err := mgr.DocumentSymbol(ctx,
					semanticapi.DocumentSymbolParams{
						TextDocument: semanticapi.TextDocumentIdentifier{
							URI: mainURI,
						},
					},
				)
				require.NoError(t, err)
				loc := func(sl, sc, el, ec uint32) semanticapi.Location {
					return semanticapi.Location{
						URI: mainURI,
						Range: semanticapi.Range{
							Start: semanticapi.Position{Line: sl, Character: sc},
							End:   semanticapi.Position{Line: el, Character: ec},
						},
					}
				}
				assert.Equal(t, []semanticapi.SymbolInformation{
					{Name: "Greeter", Kind: semanticapi.SymbolKindClass,
						Location: loc(20, 0, 28, 42)},
					{Name: "__init__", Kind: semanticapi.SymbolKindConstructor,
						Location: loc(23, 4, 24, 24)},
					{Name: "greet", Kind: semanticapi.SymbolKindMethod,
						Location: loc(26, 4, 28, 42)},
					{Name: "add", Kind: semanticapi.SymbolKindFunction,
						Location: loc(31, 0, 33, 14)},
					{Name: "main", Kind: semanticapi.SymbolKindFunction,
						Location: loc(36, 0, 40, 17)},
				}, result.SymbolInformation)
			},
		},
		{
			name: "WorkspaceSymbol routes to ty",
			fn: func(t *testing.T, mgr *Manager) {
				syms, err := mgr.WorkspaceSymbol(ctx,
					semanticapi.WorkspaceSymbolParams{Query: "add"},
				)
				require.NoError(t, err)
				require.Len(t, syms, 1)
				// ty resolves workspace symbols against its own index
				// root, so only assert the stable name/kind/range and
				// that the location is the package main.py.
				assert.Equal(t, "add", syms[0].Name)
				assert.Equal(t, semanticapi.SymbolKindFunction, syms[0].Kind)
				assert.True(t,
					strings.HasSuffix(syms[0].Location.URI, "/main.py"),
					"unexpected workspace symbol uri: %s",
					syms[0].Location.URI)
				assert.Equal(t, semanticapi.Range{
					Start: semanticapi.Position{Line: 31, Character: 0},
					End:   semanticapi.Position{Line: 33, Character: 14},
				}, syms[0].Location.Range)
			},
		},
		{
			name: "Completion routes to ty",
			fn: func(t *testing.T, mgr *Manager) {
				// Append a member-access expression so the completion
				// list is the deterministic set of Greeter members
				// followed by inherited object dunders.
				probe := "\n_probe = Greeter(\"x\").\n"
				orig, err := os.ReadFile(strings.TrimPrefix(mainURI, "file://"))
				require.NoError(t, err)
				require.NoError(t, mgr.DidChange(ctx,
					semanticapi.DidChangeTextDocumentParams{
						TextDocument: semanticapi.VersionedTextDocumentIdentifier{
							URI: mainURI, Version: 5,
						},
						ContentChanges: []semanticapi.TextDocumentContentChangeEvent{
							{Text: string(orig) + probe},
						},
					},
				))
				memberLine := uint32(len(strings.Split(string(orig), "\n")))
				result, err := mgr.Completion(ctx,
					semanticapi.CompletionParams{
						TextDocument: semanticapi.TextDocumentIdentifier{
							URI: mainURI,
						},
						Position: semanticapi.Position{
							Line: memberLine, Character: 22,
						},
					},
				)
				require.NoError(t, err)
				labels := make([]string, len(result.Items))
				for i, it := range result.Items {
					labels[i] = it.Label
				}
				assert.Equal(t, []string{
					"greet", "name",
					"__annotations__", "__class__", "__delattr__",
					"__dict__", "__dir__", "__doc__", "__eq__",
					"__format__", "__getattribute__", "__getstate__",
					"__hash__", "__init__", "__init_subclass__",
					"__module__", "__ne__", "__new__", "__reduce__",
					"__reduce_ex__", "__repr__", "__setattr__",
					"__sizeof__", "__str__", "__subclasshook__",
				}, labels)
				// restore original content for any later cases
				require.NoError(t, mgr.DidChange(ctx,
					semanticapi.DidChangeTextDocumentParams{
						TextDocument: semanticapi.VersionedTextDocumentIdentifier{
							URI: mainURI, Version: 6,
						},
						ContentChanges: []semanticapi.TextDocumentContentChangeEvent{
							{Text: string(orig)},
						},
					},
				))
			},
		},
		{
			name: "DocumentHighlight routes to ty",
			fn: func(t *testing.T, mgr *Manager) {
				highlights, err := mgr.DocumentHighlight(ctx,
					semanticapi.DocumentHighlightParams{
						TextDocument: semanticapi.TextDocumentIdentifier{
							URI: mainURI,
						},
						Position: semanticapi.Position{
							Line: 31, Character: 4,
						},
					},
				)
				require.NoError(t, err)
				assert.Equal(t, []semanticapi.DocumentHighlight{
					{
						Range: semanticapi.Range{
							Start: semanticapi.Position{Line: 31, Character: 4},
							End:   semanticapi.Position{Line: 31, Character: 7},
						},
						Kind: semanticapi.DocumentHighlightKindText,
					},
					{
						Range: semanticapi.Range{
							Start: semanticapi.Position{Line: 39, Character: 13},
							End:   semanticapi.Position{Line: 39, Character: 16},
						},
						Kind: semanticapi.DocumentHighlightKindRead,
					},
				}, highlights)
			},
		},
		{
			name: "FoldingRange routes to ty",
			fn: func(t *testing.T, mgr *Manager) {
				ranges, err := mgr.FoldingRange(ctx,
					semanticapi.FoldingRangeParams{
						TextDocument: semanticapi.TextDocumentIdentifier{
							URI: mainURI,
						},
					},
				)
				require.NoError(t, err)
				assert.Equal(t, []semanticapi.FoldingRange{
					{StartLine: 20, StartCharacter: 14, EndLine: 28, EndCharacter: 42},
					{StartLine: 23, StartCharacter: 42, EndLine: 24, EndCharacter: 24},
					{StartLine: 26, StartCharacter: 27, EndLine: 28, EndCharacter: 42},
					{StartLine: 31, StartCharacter: 31, EndLine: 33, EndCharacter: 14},
					{StartLine: 36, StartCharacter: 19, EndLine: 40, EndCharacter: 17},
					{StartLine: 43, StartCharacter: 26, EndLine: 44, EndCharacter: 10},
					{StartLine: 0, EndLine: 15, EndCharacter: 49,
						Kind: semanticapi.FoldingRangeKindComment},
				}, ranges)
			},
		},
		{
			// ty does not implement textDocument/prepareRename.
			name: "PrepareRename unsupported by ty",
			fn: func(t *testing.T, mgr *Manager) {
				result, err := mgr.PrepareRename(ctx,
					semanticapi.PrepareRenameParams{
						TextDocument: semanticapi.TextDocumentIdentifier{
							URI: mainURI,
						},
						Position: semanticapi.Position{
							Line: 31, Character: 4,
						},
					},
				)
				require.NoError(t, err)
				assert.Nil(t, result)
			},
		},
		{
			name: "Rename routes to ty",
			fn: func(t *testing.T, mgr *Manager) {
				edit, err := mgr.Rename(ctx, semanticapi.RenameParams{
					TextDocument: semanticapi.TextDocumentIdentifier{
						URI: mainURI,
					},
					Position: semanticapi.Position{Line: 31, Character: 4},
					NewName:  "addNums",
				})
				require.NoError(t, err)
				require.NotNil(t, edit)
				assert.Equal(t, map[string][]semanticapi.TextEdit{
					mainURI: {
						{
							Range: semanticapi.Range{
								Start: semanticapi.Position{Line: 31, Character: 4},
								End:   semanticapi.Position{Line: 31, Character: 7},
							},
							NewText: "addNums",
						},
						{
							Range: semanticapi.Range{
								Start: semanticapi.Position{Line: 39, Character: 13},
								End:   semanticapi.Position{Line: 39, Character: 16},
							},
							NewText: "addNums",
						},
					},
				}, edit.Changes)
			},
		},
		{
			name: "SelectionRange routes to ty",
			fn: func(t *testing.T, mgr *Manager) {
				ranges, err := mgr.SelectionRange(ctx,
					semanticapi.SelectionRangeParams{
						TextDocument: semanticapi.TextDocumentIdentifier{
							URI: mainURI,
						},
						Positions: []semanticapi.Position{
							{Line: 34, Character: 11},
						},
					},
				)
				require.NoError(t, err)
				require.Len(t, ranges, 1)
				assert.Equal(t, semanticapi.Range{
					Start: semanticapi.Position{Line: 0, Character: 0},
					End:   semanticapi.Position{Line: 44, Character: 10},
				}, ranges[0].Range)
			},
		},
		{
			name: "SignatureHelp routes to ty",
			fn: func(t *testing.T, mgr *Manager) {
				result, err := mgr.SignatureHelp(ctx,
					semanticapi.SignatureHelpParams{
						TextDocument: semanticapi.TextDocumentIdentifier{
							URI: mainURI,
						},
						Position: semanticapi.Position{
							Line: 39, Character: 17,
						},
					},
				)
				require.NoError(t, err)
				assert.Equal(t, []semanticapi.SignatureInformation{
					{
						Label: "(a: int, b: int) -> int",
						Parameters: []semanticapi.ParameterInformation{
							{Label: "a: int"},
							{Label: "b: int"},
						},
					},
				}, result.Signatures)
			},
		},
		{
			// ty resolves int to the vendored typeshed builtins stub;
			// the exact path is environment-specific so only its shape
			// is asserted.
			name: "TypeDefinition routes to ty",
			fn: func(t *testing.T, mgr *Manager) {
				result, err := mgr.TypeDefinition(ctx,
					semanticapi.TypeDefinitionParams{
						TextDocument: semanticapi.TextDocumentIdentifier{
							URI: mainURI,
						},
						Position: semanticapi.Position{
							Line: 38, Character: 4,
						},
					},
				)
				require.NoError(t, err)
				require.Len(t, result.Locations, 1)
				assert.True(t,
					strings.HasSuffix(result.Locations[0].URI, "builtins.pyi"),
					"unexpected type definition uri: %s",
					result.Locations[0].URI)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.fn(t, mgr)
		})
	}
}

// TestE2EPythonMergedDiagnostics asserts that diagnostics from ruff
// (the alternate child) reach the callback for the opened file. ty
// publishes an empty diagnostic set for this fixture, so the merged
// result is exactly ruff's F401 unused-import on `import os`.
func TestE2EPythonMergedDiagnostics(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var diagMu sync.Mutex
	byURI := map[string][]semanticapi.Diagnostic{}
	callback := &testCallback{
		onDiagnostics: func(p semanticapi.PublishDiagnosticsParams) {
			diagMu.Lock()
			defer diagMu.Unlock()
			if len(p.Diagnostics) > 0 {
				byURI[p.URI] = p.Diagnostics
			}
		},
	}

	_, mainURI := setupPythonManager(t, ctx, callback)

	var got semanticapi.Diagnostic
	require.Eventually(t, func() bool {
		diagMu.Lock()
		defer diagMu.Unlock()
		ds := byURI[mainURI]
		if len(ds) == 0 {
			return false
		}
		got = ds[0]
		return true
	}, 15*time.Second, 200*time.Millisecond,
		"expected ruff F401 diagnostic for unused import")

	assert.Equal(t, "F401", got.Code)
	assert.Equal(t, "Ruff", got.Source)
	assert.Equal(t, semanticapi.DiagnosticSeverityWarning, got.Severity)
	assert.Equal(t, "`os` imported but unused", got.Message)
	assert.Equal(t, semanticapi.Range{
		Start: semanticapi.Position{Line: 17, Character: 7},
		End:   semanticapi.Position{Line: 17, Character: 9},
	}, got.Range)
}
