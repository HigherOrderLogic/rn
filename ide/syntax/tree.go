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
	"time"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ernestrc/go-multierror"
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
	"unstable.build/go-tui/debug"
	"unstable.build/go-tui/term"
	"unstable.build/go-tui/workspace"
)

const (
	parserFilename     = "tree-sitter.so"
	highlightsFilename = "highlights.scm"
)

// WithTree installs a tree parser into the given buffer via cell.Buffer.WithEditor,
// and wraps the given FlusherCloser to provide re-parse on reload and flush.
func WithTree(
	ctx context.Context,
	n browserapi.Notifications, interrupter term.Interrupter,
	pkg PkgManager, loc LocationSetter,
	uri workspaceapi.URI, buf *cell.Buffer,
	fc workspace.FlusherCloser,
	config Config,
) workspace.FlusherCloser {
	if config.ScheduleNextTick == nil {
		panic("invalid config")
	}
	ret := new(tree)
	ret.config = config
	ret.buf = buf
	ret.uri = uri
	ret.ced = ret.buf.WithEditor(ret)
	ret.n = n
	ret.interrupter = interrupter
	ret.pkg = pkg
	ret.loc = loc
	ret.fc = fc

	go debug.CapturePanicReport(func() {
		ret.downloadFiles(ctx)
	})
	return ret
}

func (t *tree) Edit(ctx context.Context, start, end term.Coordinates, str string) (
	from, to term.Coordinates, old string,
) {
	from, to, old = t.ced.Edit(ctx, start, end, str)
	t.edit(start, end, from, to, str)
	return
}

func (t *tree) Flush() error {
	ret := t.fc.Flush()
	t.reparse()
	return ret
}

func (t *tree) Reload() error {
	ret := t.fc.Reload()
	t.reparse()
	return ret
}

func (t *tree) ForceFlush() error {
	ret := t.fc.ForceFlush()
	t.reparse()
	return ret
}

func (t *tree) LastFlush() time.Time {
	return t.fc.LastFlush()
}

// tree represents a file's syntax tree powered by tree-sitter.
type tree struct {
	n           browserapi.Notifications
	interrupter term.Interrupter
	pkg         PkgManager
	loc         LocationSetter
	config      Config
	fc          workspace.FlusherCloser
	uri         workspaceapi.URI
	buf         *cell.Buffer
	ced         cell.Editor

	ready      bool
	closed     bool
	lib        uintptr
	mu         sync.RWMutex
	cells      [][]term.Cell
	content    []byte
	parser     *tree_sitter.Parser
	tree       *tree_sitter.Tree
	highlights *tree_sitter.Query
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
		t.initParserFromFiles(ctx, ext, id, allFiles)
	}) {
		t.log(log.ErrorLevel, "could not schedule language %q initialization through event-loop", id)
		t.notifyNotAvail(ext)
	}
}

func (t *tree) initParserFromFiles(
	ctx context.Context, ext, langID string, allFiles []string,
) {
	var langfile, highlightsfile string
	for _, file := range allFiles {
		if filepath.Base(file) == parserFilename {
			langfile = file
			if highlightsfile == "" {
				continue
			}
			err := t.initParser(ctx, langID, langfile, highlightsfile)
			if err != nil {
				t.log(log.ErrorLevel, "initialize language %s: %v", langID, err)
				t.notifyNotAvail(ext)
			} else {
				t.log(log.DebugLevel, "successfully initialized parser")
			}
			return
		}
		if filepath.Base(file) == highlightsFilename {
			highlightsfile = file
			if langfile == "" {
				continue
			}
			err := t.initParser(ctx, langID, langfile, highlightsfile)
			if err != nil {
				t.log(log.ErrorLevel, "initialize language %s: %v", langID, err)
				t.notifyNotAvail(ext)
			} else {
				t.log(log.DebugLevel, "successfully initialized parser")
			}
			return
		}
	}

	t.log(log.WarnLevel, "language %s not available: reason: "+
		"%q and %q files not found: files found: %v", langID,
		parserFilename, highlightsFilename, allFiles)
	t.notifyNotAvail(ext)
}

func (t *tree) initParser(
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
	_, _ = t.n.Notify(notifications.LevelWarn,
		"syntax tree parser for language (%q) is not available", ext)
	if err := t.interrupter.Interrupt(context.Background()); err != nil {
		t.log(log.WarnLevel, "interrupt: %v", err)
	}
}

func (t *tree) reparse() {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if !t.ready {
		return
	}
	t.doReparse()
}

func (t *tree) doReparse() {
	t.log(log.TraceLevel, "reparsing tree after flush")
	t.tree.Close() // dealloc previous tree
	t.persistCells()
	t.tree = t.parser.Parse(t.content, nil)
	if err := t.highlight(); err != nil {
		t.log(log.ErrorLevel, "highlight: %v", err)
	}
}

func (t *tree) edit(start, end, from, to term.Coordinates, content string) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if !t.ready {
		return
	}
	// use old cells to convert coordinates
	newCells := t.buf.RawCells()
	edit, ok := textapiEditTotreeSitterEdit(t.cells, newCells, start, end, from, to, content)
	if !ok {
		t.log(log.WarnLevel, "convert edit to tree-sitter coordinates failed, "+
			"re-parsing enabled: %t", t.config.ReparseOnErrors)
		if t.config.ReparseOnErrors {
			t.doReparse()
		}
		return
	}
	t.log(log.TraceLevel, "converted edit(start=%v,end=%v,from=%v,to=%v,content=%s)  "+
		"into tree sitter edit: %+v", start, end, from, to, content, edit)

	t.tree.Edit(&edit)

	// get new cells to re-parse
	t.persistCells()
	t.tree = t.parser.Parse(t.content, t.tree)
	if err := t.highlight(); err != nil {
		t.log(log.ErrorLevel, "highlight: %v", err)
	}
}

func (t *tree) persistCells() {
	t.cells = t.buf.RawCells()
	t.cells = cell.CloneCells(t.cells)
	t.content = []byte(cell.CellsToString(t.cells))
}

func (t *tree) highlight() error {
	highlights, err := t.getHighlights(t.cells, t.content)
	if err != nil {
		return err
	}

	ll := textapi.LocationSlice(highlights)
	if err := t.loc.SetLocationList(ll); err != nil {
		return fmt.Errorf("set location list: %w", err)
	}

	t.log(log.TraceLevel, "set %d highlights", len(highlights))

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
			/*t.log(log.TraceLevel, "mapped range %v into coords: %v, %v", rng, from, to)*/
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

func (t *tree) Close() (ret error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return nil
	}
	t.closed = true
	if err := t.fc.Close(); err != nil {
		ret = multierror.Append(ret, err)
	}
	if !t.ready {
		return nil
	}
	t.tree.Close()
	t.parser.Close()
	t.highlights.Close()
	if err := purego.Dlclose(t.lib); err != nil {
		ret = multierror.Append(ret, err)
	}
	return
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

func textapiEditTotreeSitterEdit(
	before, after [][]term.Cell, start, end, from, to term.Coordinates, content string,
) (tree_sitter.InputEdit, bool) {
	startByte, sok := cell.ConvertCoordinatesToByteOffset(before, start)
	oldEndByte, eok := cell.ConvertCoordinatesToByteOffset(before, end)

	x, y, spok := cell.ConvertCoordinatesToRunePos(before, start)
	startPos := tree_sitter.Point{Row: uint(y), Column: uint(x)}
	x, y, epok := cell.ConvertCoordinatesToRunePos(before, end)
	oldEndPos := tree_sitter.Point{Row: uint(y), Column: uint(x)}
	x, y, tpok := cell.ConvertCoordinatesToRunePos(after, to)
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
		NewEndByte:     uint(startByte + int(len([]byte(content)))),
		StartPosition:  startPos,
		OldEndPosition: oldEndPos,
		NewEndPosition: newEndPos,
	}, true
}
