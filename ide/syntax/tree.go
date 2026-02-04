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
	"bytes"
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
	"github.com/unstablebuild/rune-go-sdk/api/browserapi"
	"github.com/unstablebuild/rune-go-sdk/api/textapi"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"github.com/unstablebuild/rune-go-sdk/term"
	"unstable.build/go-tui/cell"
	"unstable.build/go-tui/debug"
	"unstable.build/go-tui/workspace"
)

const (
	// ParserFilename is the filename of the parser shared object.
	ParserFilename = "tree-sitter.so"

	// HighlightsFilename is the name given to the highlights query file
	HighlightsFilename = "highlights.scm"

	// IndentsFilename is the name given to the indents query file.
	IndentsFilename = "indents.scm"

	// FoldsFilename is the name given to the folds query file.
	FoldsFilename = "folds.scm"

	// LocalsFilename is the name given to the a query file
	// used to define scopes, definitions and references.
	LocalsFilename = "locals.scm"
)

// Opener abstract reading files and it's required to open custom query files.
type Opener interface {
	OpenFile(path string, flag int, perm os.FileMode) (workspaceapi.File, error)
}

// WithTree installs a tree parser into the given buffer via cell.Buffer.WithEditor,
// and wraps the given FlusherCloser to provide re-parse on reload and flush.
// A cell.Buffer's View method can be used to retrieve this Tree in other contexts.
func WithTree(
	ctx context.Context,
	n browserapi.Notifications, interrupter term.Interrupter,
	pkg PkgManager, loc LocationSetter,
	uri workspaceapi.URI, buf *cell.Buffer,
	fc workspace.FlusherCloser,
	opener Opener,
	config Config,
) *Tree {
	if config.ScheduleNextTick == nil {
		panic("invalid config")
	}
	ret := new(Tree)
	ret.config = config
	ret.buf = buf
	ret.uri = uri
	ret.opener = opener
	// NOTE: this shouldn't be removed as the buffer's view
	// is how we share this tree's capabilities with
	// other parts of the codebase via interface assertion.
	ret.cview = ret.buf.WithView(ret)
	ret.buf.Subscribe(ret)
	ret.n = n
	ret.interrupter = interrupter
	ret.pkg = pkg
	ret.loc = loc
	ret.fc = fc
	ret.statesubs = make(map[chan State]struct{})
	ret.waitingReady = make(chan struct{})

	go debug.CapturePanicReport(func() {
		files, err := ret.downloadFiles(ctx)
		if err != nil {
			config.ScheduleNextTick(func() {
				// set current state either way, so unblocking
				// waiting goroutines can stream the first state.
				defer close(ret.waitingReady)
				ret.mu.Lock()
				defer ret.mu.Unlock()
				ret.currState = State{Closed: ret.closed, ParserError: err.Error()}
			})
			return
		}
		config.ScheduleNextTick(func() {
			defer close(ret.waitingReady)
			err := ret.initParserFromFiles(ctx, files)
			ret.mu.Lock()
			defer ret.mu.Unlock()
			if err != nil {
				ret.currState = State{Closed: ret.closed, ParserError: err.Error()}
			} else {
				ret.currState = State{
					Closed:     ret.closed,
					LangID:     files.langID,
					Highlights: ret.highlights != nil,
					Folds:      ret.folds != nil,
					Indents:    ret.indents != nil,
				}
			}
		})
	})
	return ret
}

// Tree represents a file's syntax tree powered by tree-sitter.
type Tree struct {
	n           browserapi.Notifications
	interrupter term.Interrupter
	pkg         PkgManager
	loc         LocationSetter
	config      Config
	fc          workspace.FlusherCloser
	uri         workspaceapi.URI
	opener      Opener
	buf         *cell.Buffer
	cview       cell.View

	ready      bool
	closed     bool
	lib        uintptr
	mu         sync.Mutex
	cells      [][]term.Cell
	content    []byte
	contentBuf bytes.Buffer
	parser     *tree_sitter.Parser
	tree       *tree_sitter.Tree
	highlights *tree_sitter.Query
	indents    *tree_sitter.Query
	folds      *tree_sitter.Query
	locals     *tree_sitter.Query
	statesubs  map[chan State]struct{}
	currState  State

	onWillEditStart term.Coordinates
	onWillEditEnd   term.Coordinates
	onWillEditStr   string
	waitingReady    chan struct{}
}

// IndentationAt returns the indentation that should correspond to a node placed
// at the given line.
func (t *Tree) IndentationAt(line int) (int, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if !t.ready || t.closed || t.tree == nil || t.indents == nil {
		return 0, false
	}

	if !t.config.Autoindent {
		return 0, false
	}

	if line >= t.buf.Rows() || line < 0 {
		return 0, false
	}
	ret := t.getIndentation(uint(line))
	return ret, ret >= 0
}

// State returns an iterator that eventually, when the Tree is ready
// streams the current state of the tree, every time it's altered.
func (t *Tree) State() iterator.Iterator[State] {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed {
		return iterator.FromSlice([]State{t.currState})
	}

	if !t.ready {
		return newWaitStateIterator(t, t.waitingReady)
	}

	return newReadyStateIterator(t)
}

// Folds returns all the folds captured by the parser.
func (t *Tree) Folds() (iterator.Iterator[term.Range], bool) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed {
		return nil, false
	}

	if !t.ready {
		return newFoldsIterator(false, t, t.waitingReady), true
	}

	if t.folds == nil || t.tree == nil {
		return nil, false
	}

	return iterator.FromSlice(t.getFolds(false)), true
}

// FoldsFrom returns all the folds captured after the given position.
func (t *Tree) FoldsFrom(pos term.Coordinates) (iterator.Iterator[term.Range], bool) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed {
		return nil, false
	}

	if !t.ready {
		return newFoldsFromIterator(pos, t, t.waitingReady), true
	}

	if t.folds == nil || t.tree == nil {
		return nil, false
	}

	return iterator.FromSlice(t.getFoldsFrom(pos)), true
}

// InitialFolds returns the folds captured by the parser that should be folded
// when file is initialized.
func (t *Tree) InitialFolds() (iterator.Iterator[term.Range], bool) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed {
		return nil, false
	}

	if !t.ready {
		return newFoldsIterator(true, t, t.waitingReady), true
	}

	if t.folds == nil || t.tree == nil {
		return nil, false
	}

	return iterator.FromSlice(t.getFolds(true)), true
}

// Close closes all resources associated with this Tree.
func (t *Tree) Close() (ret error) {
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
	if t.tree != nil {
		t.tree.Close()
	}
	t.parser.Close()
	if t.highlights != nil {
		t.highlights.Close()
	}
	if t.folds != nil {
		t.folds.Close()
	}
	if t.locals != nil {
		t.locals.Close()
	}
	if t.indents != nil {
		t.indents.Close()
	}
	if err := purego.Dlclose(t.lib); err != nil {
		ret = multierror.Append(ret, err)
	}
	for ch := range t.statesubs {
		close(ch)
	}
	clear(t.statesubs)
	t.currState.Closed = true
	return
}

type internalTree = Tree

func (t *internalTree) Rows() int {
	return t.cview.Rows()
}

func (t *internalTree) Columns(row int) int {
	return t.cview.Columns(row)
}

func (t *internalTree) Cell(at term.Coordinates) (term.Cell, bool) {
	return t.cview.Cell(at)
}

func (t *internalTree) RawCells() [][]term.Cell {
	return t.cview.RawCells()
}

func (t *internalTree) String() string {
	return t.cview.String()
}

func (t *internalTree) OnWillEdit(ctx context.Context, start, end term.Coordinates, str string) {
	t.onWillEditStart = start
	t.onWillEditEnd = end
	t.onWillEditStr = str
}

func (t *internalTree) OnDidEdit(ctx context.Context, from, to term.Coordinates, old string) {
	t.incrementalParse(t.onWillEditStart, t.onWillEditEnd, from, to, t.onWillEditStr)
}

func (t *internalTree) Flush() error {
	ret := t.fc.Flush()
	t.reparse()
	return ret
}

func (t *internalTree) Reload() error {
	ret := t.fc.Reload()
	t.reparse()
	return ret
}

func (t *internalTree) ForceFlush() error {
	ret := t.fc.ForceFlush()
	t.reparse()
	return ret
}

func (t *internalTree) LastFlush() time.Time {
	return t.fc.LastFlush()
}

type files struct {
	langID string
	files  []string
}

func (t *Tree) downloadFiles(ctx context.Context) (files, error) {
	filename := t.uri.Name()
	id, err := LanguageForFile(filename)
	if err != nil {
		t.log(log.DebugLevel, "aborting syntax parsing: %s", err)
		return files{}, err
	}
	t.log(log.DebugLevel, "found language for file %s: %s", filename, id)
	iter, err := t.pkg.LibDir(ctx, id)
	if err != nil {
		if errors.Is(err, document.ErrNotFound) {
			msg := fmt.Sprintf("package for language "+
				"%q does not exist or it's not installed.", id)
			t.log(log.DebugLevel, "aborting syntax parsing: %s", msg)
			return files{}, errors.New(msg)
		}
		t.log(log.ErrorLevel, "aborting syntax parsing: %v", err)
		t.notifyNotAvail(id)
		return files{}, err
	}
	defer iter.Close()
	allFiles, err := iterator.ToSlice(ctx, iter)
	if err != nil {
		msg := fmt.Sprintf("fetch language %q package: %v", id, err)
		t.log(log.ErrorLevel, "%s", msg)
		t.notifyNotAvail(id)
		return files{}, errors.New(msg)
	}
	return files{files: allFiles, langID: id}, nil
}

func (t *Tree) initParserFromFiles(ctx context.Context, f files) error {
	var langFile, highlightsFile, indentsFile, foldsFile, localsFile string
	for _, file := range f.files {
		switch filepath.Base(file) {
		case ParserFilename:
			langFile = file
		case HighlightsFilename:
			highlightsFile = file
		case IndentsFilename:
			indentsFile = file
		case FoldsFilename:
			foldsFile = file
		case LocalsFilename:
			localsFile = file
		}
	}
	if langFile == "" {
		msg := fmt.Sprintf("parser file not found in language %q package", f.langID)
		t.log(log.InfoLevel, "language not available: %s", msg)
		t.notifyNotAvail(f.langID)
		return errors.New(msg)
	}
	err := t.initParser(ctx, f.langID, langFile,
		highlightsFile, indentsFile, foldsFile, localsFile)
	if err != nil {
		t.log(log.ErrorLevel, "initialize language %s: %v", f.langID, err)
		t.notifyNotAvail(f.langID)
		return err
	}
	t.log(log.DebugLevel, "successfully initialized parser")
	return nil
}

func (t *Tree) initParser(
	ctx context.Context, langID,
	langfile, highlightsfile, indentsFile, foldsFile, localsFile string,
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

	if highlightsfile != "" {
		if err := t.initHighlights(language, highlightsfile); err != nil {
			_ = purego.Dlclose(t.lib)
			t.parser.Close()
			return fmt.Errorf("initialize highlights: %w", err)
		}
	} else {
		t.log(log.InfoLevel, "highlights file not found, some features will be disabled")
	}

	if indentsFile != "" {
		if err := t.initIndents(language, indentsFile); err != nil {
			t.log(log.ErrorLevel, "initialize indents: %v", err)
		}
	} else {
		t.log(log.InfoLevel, "indents file not found, some features will be disabled")
	}

	if foldsFile != "" {
		if err := t.initFolds(language, foldsFile); err != nil {
			t.log(log.ErrorLevel, "initialize folds: %v", err)
		}
	} else {
		t.log(log.InfoLevel, "folds file not found, some features will be disabled")
	}

	if localsFile != "" {
		if err := t.initLocals(language, localsFile); err != nil {
			t.log(log.ErrorLevel, "initialize locals: %v", err)
		}
	} else {
		t.log(log.InfoLevel, "locals file not found, some features will be disabled")
	}

	// build the syntax tree
	t.persistCells()
	t.parseTree(nil, "initial parse error")

	// set ready to true, even if tree is nil
	t.ready = true

	if t.tree == nil {
		t.log(log.ErrorLevel, "parsing failed: nil tree")
		return nil
	}

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

func (t *Tree) initHighlights(
	language *tree_sitter.Language, highlightsfile string,
) error {
	// for files coming from LibDir, we can use
	// os.ReadFile since packages are installed on the local fs
	// and paths are always absolute.
	data, err := os.ReadFile(highlightsfile)
	if err != nil {
		return fmt.Errorf("read highlights file: %v", err)
	}

	// compile the highlights query for this language.
	highlights, qerr := tree_sitter.NewQuery(language, string(data))
	if qerr != nil {
		return fmt.Errorf("compile query: %v", qerr)
	}
	t.highlights = highlights
	t.log(log.DebugLevel, "highlights initialized")
	return nil
}

func (t *Tree) initIndents(
	language *tree_sitter.Language, indentsFile string,
) error {
	data, err := os.ReadFile(indentsFile)
	if err != nil {
		return fmt.Errorf("read indents file: %v", err)
	}

	indents, qerr := tree_sitter.NewQuery(language, string(data))
	if qerr != nil {
		return fmt.Errorf("compile query: %v", qerr)
	}

	t.indents = indents
	t.log(log.DebugLevel, "indents initialized")
	return nil
}

func (t *Tree) initFolds(
	language *tree_sitter.Language, foldsFile string,
) error {
	data, err := os.ReadFile(foldsFile)
	if err != nil {
		return fmt.Errorf("read folds file: %v", err)
	}

	folds, qerr := tree_sitter.NewQuery(language, string(data))
	if qerr != nil {
		return fmt.Errorf("compile query: %v", qerr)
	}

	t.folds = folds
	t.log(log.DebugLevel, "folds initialized")
	return nil
}

func (t *Tree) initLocals(
	language *tree_sitter.Language, localsFile string,
) error {
	data, err := os.ReadFile(localsFile)
	if err != nil {
		return fmt.Errorf("read locals file: %v", err)
	}

	locals, qerr := tree_sitter.NewQuery(language, string(data))
	if qerr != nil {
		return fmt.Errorf("compile locals query: %v", qerr)
	}

	t.locals = locals
	t.log(log.DebugLevel, "locals initialized")
	return nil
}

func (t *Tree) notifyNotAvail(ext string) {
	_, _ = t.n.Notify(browserapi.LevelWarn,
		"syntax tree parser for language (%q) is not available", ext)
	if err := t.interrupter.Interrupt(context.Background()); err != nil {
		t.log(log.WarnLevel, "interrupt: %v", err)
	}
}

func (t *Tree) reparse() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.ready || t.closed {
		return
	}
	t.doReparse()
}

func (t *Tree) doReparse() {
	t.log(log.TraceLevel, "reparsing tree after flush")
	if t.tree != nil {
		t.tree.Close() // dealloc previous tree
	}
	t.persistCells()
	t.parseTree(nil, "re-parse error")
	t.streamState()
	if t.tree == nil {
		t.log(log.ErrorLevel, "parse failed: nil tree")
		return
	}
	if err := t.highlight(); err != nil {
		t.log(log.ErrorLevel, "highlight: %v", err)
	}
}

func (t *Tree) incrementalParse(start, end, from, to term.Coordinates, content string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.ready || t.tree == nil || t.closed {
		t.log(log.TraceLevel, "incremental parse aborted: ready: %t, nil tree: %t, closed: %t",
			t.ready, t.tree == nil, t.closed)
		return
	}
	// use old cells to convert coordinates
	newCells := t.buf.RawCells()
	edit, ok := editToTreesitterEdit(t.cells, newCells, start, end, from, to, content)
	if !ok {
		t.log(log.WarnLevel, "convert edit to tree-sitter coordinates failed, "+
			"re-parsing enabled: %t", t.config.ReparseOnErrors)
		if t.config.ReparseOnErrors {
			t.doReparse()
		}
		return
	}
	t.log(log.TraceLevel, "converted edit(start=%v,end=%v,from=%v,to=%v)  "+
		"into tree sitter edit: %+v", start, end, from, to, edit)

	t.tree.Edit(&edit)

	// get new cells to re-parse
	t.persistCells()
	t.parseTree(t.tree, "incremental parse error")
	t.streamState()
	if t.tree == nil {
		t.log(log.DebugLevel, "incremental parsing failed: "+
			"nil tree, re-parse on errors: %t", t.config.ReparseOnErrors)
		if t.config.ReparseOnErrors {
			t.doReparse()
		}
		return
	}
	if t.currState.ParserError != "" {
		t.log(log.DebugLevel, "%s, re-parse on errors: %t",
			t.currState.ParserError, t.config.ReparseOnErrors)
		if t.config.ReparseOnErrors {
			t.doReparse()
			return
		}
		// best effort continue
	}
	if err := t.highlight(); err != nil {
		t.log(log.ErrorLevel, "highlight: %v", err)
	}
}

func (t *Tree) persistCells() {
	t.cells = term.CopyCells(t.cells, t.buf.RawCells())
	t.contentBuf.Reset()
	cell.CellsToBytesBuffer(&t.contentBuf, t.cells)
	t.content = t.contentBuf.Bytes()

}

func (t *Tree) highlight() error {
	if t.highlights == nil {
		return nil
	}
	highlights := t.getHighlights(t.cells, t.content)
	ll := textapi.LocationSlice(highlights)
	t.loc.SetLocationList(ll)

	t.log(log.TraceLevel, "set %d highlights", len(highlights))

	return nil
}

func (t *Tree) query(queryFile string, captureNames ...string) (iterator.Iterator[Match], error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed {
		return nil, errors.New("tree is closed")
	}

	if !t.ready {
		return newNodesIterator(t, t.waitingReady, queryFile, captureNames...), nil
	}

	if t.tree == nil {
		return nil, errors.New("tree could not be parsed")
	}

	data, err := t.runQuery(queryFile, captureNames)
	if err != nil {
		return nil, err
	}

	return iterator.FromSlice(data), nil
}

func (t *Tree) getHighlights(cells [][]term.Cell, content []byte) []textapi.Location {
	root := t.tree.RootNode()

	cur := tree_sitter.NewQueryCursor()
	defer cur.Close()

	captureNames := t.highlights.CaptureNames()
	matches := cur.Matches(t.highlights, root, content)
	var locations []textapi.Location
	for {
		m, ok := matches.Next()
		if !ok {
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
	return locations
}

func (t *Tree) streamState() {
	for ch := range t.statesubs {
		select {
		case ch <- t.currState:
		default:
		}
	}
}

func (t *Tree) parseTree(prev *tree_sitter.Tree, errorMsg string) {
	var hasError bool
	opts := tree_sitter.ParseOptions{
		ProgressCallback: func(state tree_sitter.ParseState) bool {
			hasError = state.HasError
			return false
		},
	}
	t.tree = t.parser.ParseWithOptions(func(i int, _ tree_sitter.Point) []byte {
		if i < len(t.content) {
			return t.content[i:]
		}
		return []byte{}
	}, prev, &opts)
	if hasError || (t.tree != nil &&
		t.tree.RootNode().HasError() && t.config.StrictErrors) {
		t.currState.ParserError = errorMsg
	} else {
		t.currState.ParserError = ""
	}
	t.currState.Progress = 1
}

func (t *Tree) log(level log.Level, msg string, args ...any) {
	if !log.IsLevelEnabled(level) {
		return
	}
	log.WithFields(log.Fields{
		logging.KeyClass: "syntax.tree",
		"uri":            t.uri,
	}).Logf(level, msg, args...)
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

func editToTreesitterEdit(
	before, after [][]term.Cell, start, end, from, to term.Coordinates, content string,
) (tree_sitter.InputEdit, bool) {
	startByte, sok := cell.ConvertCoordinatesToByteOffset(before, start)
	oldEndByte, eok := cell.ConvertCoordinatesToByteOffset(before, end)

	y, x, spok := cell.ConvertCoordinatesToRunePos(before, start)
	startPos := tree_sitter.Point{Row: uint(y), Column: uint(x)}
	y, x, epok := cell.ConvertCoordinatesToRunePos(before, end)
	oldEndPos := tree_sitter.Point{Row: uint(y), Column: uint(x)}
	y, x, tpok := cell.ConvertCoordinatesToRunePos(after, to)
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
