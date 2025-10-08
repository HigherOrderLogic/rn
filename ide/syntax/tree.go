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

package syntax

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ernestrc/logd-go/logging"
	log "github.com/sirupsen/logrus"
	tree_sitter "github.com/tree-sitter/go-tree-sitter"
	"github.com/unstablebuild/blue/document"
	"github.com/unstablebuild/blue/iterator"
	"unstable.build/go-tui/api/browserapi"
	"unstable.build/go-tui/api/textapi"
	"unstable.build/go-tui/api/workspaceapi"
	"unstable.build/go-tui/cell"
	"unstable.build/go-tui/component/notifications"
	"unstable.build/go-tui/term"
	"unstable.build/go-tui/text"
)

const (
	parserFilename     = "tree-sitter.so"
	highlightsFilename = "highlights.scm"
)

// tree represents a file's syntax tree powered by tree-sitter.
type tree struct {
	notifications browserapi.Notifications
	interrupter   term.Interrupter
	config        Config
	uri           workspaceapi.URI
	view          textapi.CellView
	handler       text.Handler
	ed            text.Editor
	ready         bool
	closed        bool
	lib           uintptr
	pkg           PkgManager
	mu            sync.RWMutex
	cells         [][]term.Cell
	content       []byte
	parser        *tree_sitter.Parser
	tree          *tree_sitter.Tree
	highlights    *tree_sitter.Query
}

func (t *tree) downloadFiles(ctx context.Context) {
	ext := filepath.Ext(t.uri.String())
	if ext == "" {
		t.log(log.DebugLevel, "aborting syntax parsing: file does not have an extension")
		return
	}
	id, ok := extensionToLanguageID[ext]
	if !ok {
		id = ext[1:]
	}
	files, err := t.pkg.LibDir(ctx, id)
	if err != nil {
		if errors.Is(err, document.ErrNotFound) {
			t.log(log.DebugLevel, "aborting syntax parsing: package for language "+
				"%q does not exist or it's not installed.", id)
			return
		}
		t.log(log.ErrorLevel, "aborting syntax parsing: %v", err)
		t.notifyNotAvail(ext)
		return
	}
	defer files.Close()
	allFiles, err := iterator.ToSlice(ctx, files)
	if err != nil {
		t.log(log.ErrorLevel, "language %s not available: reason: %v", id, err)
		t.notifyNotAvail(ext)
		return
	}

	// access to text.Editor is unsynchronized,
	// schedule calls on the next tick iteration
	if !t.config.ScheduleNextTick(func() {
		t.initLanguageFromFiles(ctx, ext, id, allFiles)
	}) {
		t.log(log.ErrorLevel, "could not schedule language %q initialization through event-loop", id)
		t.notifyNotAvail(ext)
	}
}

func (t *tree) initLanguageFromFiles(
	ctx context.Context, ext, langID string, allFiles []string,
) {
	var langfile, highlightsfile string
	for _, file := range allFiles {
		if filepath.Base(file) == parserFilename {
			langfile = file
			if highlightsfile == "" {
				continue
			}
			err := t.initLanguage(ctx, langID, langfile, highlightsfile)
			if err != nil {
				t.log(log.ErrorLevel, "initialize language %s: %v", langID, err)
				t.notifyNotAvail(ext)
			}
			return
		}
		if filepath.Base(file) == highlightsFilename {
			highlightsfile = file
			if langfile == "" {
				continue
			}
			err := t.initLanguage(ctx, langID, langfile, highlightsfile)
			if err != nil {
				t.log(log.ErrorLevel, "initialize language %s: %v", langID, err)
				t.notifyNotAvail(ext)
			}
			return
		}
	}

	t.log(log.WarnLevel, "language %s not available: reason: "+
		"%q and %q files not found: files found: %v", langID,
		parserFilename, highlightsFilename, allFiles)
	t.notifyNotAvail(ext)
}

func (t *tree) initLanguage(
	ctx context.Context, langID, langfile, highlightsfile string,
) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed {
		return nil
	}

	lib, err := purego.Dlopen(langfile, purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		return fmt.Errorf("dlopen %q: %w", langfile, err)
	}
	t.lib = lib

	parserID := fmt.Sprintf("tree_sitter_%s", langID)
	t.log(log.TraceLevel, "loading parser %q", parserID)

	var lang func() uintptr
	sym, err := purego.Dlsym(lib, parserID)
	if err != nil {
		_ = purego.Dlclose(t.lib)
		return fmt.Errorf("load symbol %q: %w", parserID, err)
	}
	purego.RegisterFunc(&lang, sym)

	language := tree_sitter.NewLanguage(unsafe.Pointer(lang()))
	t.parser = tree_sitter.NewParser()
	if err := t.parser.SetLanguage(language); err != nil {
		_ = purego.Dlclose(t.lib)
		t.parser.Close()
		return fmt.Errorf("set parser language: %v", err)
	}

	data, err := os.ReadFile(highlightsfile)
	if err != nil {
		_ = purego.Dlclose(t.lib)
		t.parser.Close()
		return fmt.Errorf("read highlights file: %v", err)
	}

	// compile the highlights query for this language.
	highlights, qerr := tree_sitter.NewQuery(language, string(data))
	if qerr != nil {
		_ = purego.Dlclose(t.lib)
		t.parser.Close()
		return fmt.Errorf("compile query: %v", qerr)
	}

	// build the syntax tree
	t.persistCells()
	t.tree = t.parser.Parse(t.content, nil)
	t.highlights = highlights
	t.ready = true

	if err := t.highlight(); err != nil {
		t.log(log.ErrorLevel, "highlight: %v", err)
	}

	if err := t.interrupter.Interrupt(ctx); err != nil {
		t.log(log.WarnLevel, "interrupt: %v", err)
	}
	// do not return highlight or interrupt error so we don't
	// show initialize SO failure notification to user.
	return nil
}

func (t *tree) notifyNotAvail(ext string) {
	_, _ = t.notifications.Notify(notifications.LevelWarn,
		"syntax tree parser for language (%q) is not available", ext)
	if err := t.interrupter.Interrupt(context.Background()); err != nil {
		t.log(log.WarnLevel, "interrupt: %v", err)
	}
}

func (t *tree) flush() {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if !t.ready {
		return
	}
	t.doFlush()
}

func (t *tree) doFlush() {
	t.log(log.TraceLevel, "reparsing tree after flush")
	t.tree.Close() // dealloc previous tree
	t.persistCells()
	t.tree = t.parser.Parse(t.content, nil)
	if err := t.highlight(); err != nil {
		t.log(log.ErrorLevel, "highlight: %v", err)
	}
}

func (t *tree) edit(ev textapi.Event) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if !t.ready {
		return
	}
	// use old cells to convert coordinates
	newCells, _ := t.view.RawCells()
	edit, ok := textapiEditTotreeSitterEdit(t.cells, newCells, ev)
	if !ok {
		t.log(log.WarnLevel, "convert edit to tree-sitter coordinates failed, "+
			"re-parsing enabled: %t", t.config.ReparseOnErrors)
		if t.config.ReparseOnErrors {
			t.doFlush()
		}
		return
	}
	t.log(log.TraceLevel, "converted edit(start=%v,end=%v,from=%v,to=%v,content=%s)  "+
		"into tree sitter edit: %+v", ev.Start, ev.End, ev.From, ev.To, ev.Content, edit)

	t.tree.Edit(&edit)

	// get new cells to re-parse
	t.persistCells()
	t.tree = t.parser.Parse(t.content, t.tree)
	if err := t.highlight(); err != nil {
		t.log(log.ErrorLevel, "highlight: %v", err)
	}
}

func (t *tree) persistCells() {
	t.cells, _ = t.view.RawCells()
	t.cells = cell.CloneCells(t.cells)
	t.content = []byte(cell.CellsToString(t.cells))
}

func (t *tree) highlight() error {
	highlights, err := t.getHighlights(t.cells, t.content)
	if err != nil {
		return err
	}

	ll := textapi.LocationSlice(highlights)
	err = t.ed.SetLocationList(t.handler, textapi.LocationPriorityInfo, "stree", ll)
	if err != nil {
		return fmt.Errorf("set location list: %w", err)
	}

	return nil
}

func (t *tree) getHighlights(cells [][]term.Cell, content []byte) (
	[]textapi.Location, error,
) {
	root := t.tree.RootNode()

	cur := tree_sitter.NewQueryCursor()
	defer cur.Close()

	captureNames := t.highlights.CaptureNames()
	matches := cur.Matches(t.highlights, root, content)
	var locations []textapi.Location
	for {
		m := matches.Next()
		if m == nil {
			break
		}
		for _, cap := range m.Captures {
			rng := cap.Node.Range()
			from, to, err := convertRangeToCoordinates(cells, rng)
			if err != nil {
				t.log(log.WarnLevel, "get highlights: %v", err)
				continue
			}
			t.log(log.TraceLevel, "mapped range %v into coords: %v, %v", rng, from, to)
			if int(cap.Index) >= len(captureNames) {
				t.log(log.WarnLevel, "index %d does not belong capture names %v",
					cap.Index, captureNames)
				continue
			}
			name := captureNames[cap.Index]
			attr, ok := t.config.CaptureNamesAttributes[name]
			if !ok {
				attr = defaultCaptureNamesAttributes[name]
			} // if not ok, zero value attr works
			locations = append(locations, textapi.Location{
				Attr: attr,
				From: from,
				To:   to,
			})
		}
	}
	return locations, nil
}

func (t *tree) log(level log.Level, msg string, args ...any) {
	if !log.IsLevelEnabled(level) {
		return
	}
	log.WithFields(log.Fields{
		logging.KeyClass: "syntax.tree",
		"uri":            t.uri,
	}).Logf(level, msg, args...)
}

func (t *tree) Close() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return
	}
	t.closed = true
	if !t.ready {
		return
	}
	t.tree.Close()
	t.parser.Close()
	t.highlights.Close()
	_ = purego.Dlclose(t.lib)
}

func convertRangeToCoordinates(cells [][]term.Cell, n tree_sitter.Range) (
	from, to term.Coordinates, err error,
) {
	start, end := n.StartPoint, n.EndPoint
	from, ok := cell.ConvertRunePosToCoordinates(cells, int(start.Row), int(start.Column))
	if !ok {
		err = fmt.Errorf("convert points: failed to convert sitter 'start point "+
			" to term 'from' coordinates: point: %v", start)
		return
	}
	to, ok = cell.ConvertRunePosToCoordinates(cells, int(end.Row), int(end.Column))
	if !ok {
		err = fmt.Errorf("convert points: failed to convert sitter 'end' point "+
			" to term 'to' coordinates: point: %v", end)
		return
	}
	return
}

func textapiEditTotreeSitterEdit(before, after [][]term.Cell, ev textapi.Event) (tree_sitter.InputEdit, bool) {
	startByte, sok := cell.ConvertCoordinatesToByteOffset(before, ev.Start)
	oldEndByte, eok := cell.ConvertCoordinatesToByteOffset(before, ev.End)

	x, y, spok := cell.ConvertCoordinatesToRunePos(before, ev.Start)
	startPos := tree_sitter.Point{Row: uint(y), Column: uint(x)}
	x, y, epok := cell.ConvertCoordinatesToRunePos(before, ev.End)
	oldEndPos := tree_sitter.Point{Row: uint(y), Column: uint(x)}
	x, y, tpok := cell.ConvertCoordinatesToRunePos(after, ev.To)
	newEndPos := tree_sitter.Point{Row: uint(y), Column: uint(x)}

	if !sok || !eok || !spok || !epok || !tpok {
		/*logrus.Errorf("convert edit to tree-sitter coordinates failed: "+
		"sok=%t, eok=%t, spok=%t, epok=%t, tpok=%t",
		sok, eok, spok, epok, tpok) */
		return tree_sitter.InputEdit{}, false
	}

	return tree_sitter.InputEdit{
		StartByte:      uint(startByte),
		OldEndByte:     uint(oldEndByte),
		NewEndByte:     uint(startByte + int(len([]byte(ev.Content)))),
		StartPosition:  startPos,
		OldEndPosition: oldEndPos,
		NewEndPosition: newEndPos,
	}, true
}
