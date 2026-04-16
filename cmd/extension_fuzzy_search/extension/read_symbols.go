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
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sync"

	"github.com/ernestrc/go-multierror"
	log "github.com/sirupsen/logrus"
	sitter "github.com/tree-sitter/go-tree-sitter"
	"github.com/unstablebuild/blue/iterator"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"github.com/unstablebuild/rune-go-sdk/term"
	"unstable.build/go-tui/cell"
	"unstable.build/go-tui/debug"
	"unstable.build/go-tui/extension"
	"unstable.build/go-tui/ide/idelsp/languages"
)

var (
	defaultWorkers  = runtime.NumCPU()
	errInvalidQuery = errors.New("invalid query for language")
)

type match struct {
	CaptureName string
	LineString  string
}

func readSymbols(
	ctx context.Context, w workspaceapi.FileSystem,
	dataDir string, uri workspaceapi.URI, paths iterator.Iterator[string],
	queryFile string, query string,
) (iterator.Iterator[match], error) {
	files := make(chan string)
	results := make(chan match)
	closeWaitCh := make(chan struct{})
	ctx, cancel := context.WithCancel(ctx)

	var wg sync.WaitGroup
	wg.Add(defaultWorkers)
	errors := make([]error, defaultWorkers)
	validErrors := make([]map[string]*expectedError, defaultWorkers)
	for i := range defaultWorkers {
		validErrors[i] = make(map[string]*expectedError)
		go func(err *error, missingLanguage map[string]*expectedError) {
			defer wg.Done()
			readSymbolsWorker(ctx, w, dataDir, uri, queryFile, results, files, err,
				missingLanguage, query)
		}(&errors[i], validErrors[i])
	}

	it := &listSymbolsIterator{ctx: ctx, ch: results}
	it.cancel = cancel
	it.closeWaitCh = closeWaitCh

	var itErr error
	go debug.CapturePanicReport(func() {
		defer close(closeWaitCh)
		defer close(results)
		defer paths.Close()

	loop:
		for {
			file, ok := paths.Next(ctx)
			if !ok {
				break
			}
			select {
			case files <- file:
			case <-ctx.Done():
				break loop
			}
		}
		if err := paths.Err(); err != nil {
			itErr = err
		}
		close(files)
		wg.Wait()

		it.mu.Lock()
		defer it.mu.Unlock()

		it.err = itErr
		for _, err := range errors {
			if err != nil {
				it.err = multierror.Append(it.err, err)
			}
		}
		missingLanguage, invalidQuery := mergeValidErrorsMap(validErrors)
		if len(missingLanguage) != 0 {
			log.Debugf("Missing language parser for the following file extensions: %#v",
				missingLanguage)
		}
		if len(invalidQuery) != 0 {
			log.Debugf("Invalid query for the following file extensions: %#v",
				invalidQuery)
		}
	})
	return it, nil
}

func mergeValidErrorsMap(m []map[string]*expectedError) (
	missingLanguage map[string]int,
	invalidQuery map[string]int,
) {
	missingLanguage = make(map[string]int)
	invalidQuery = make(map[string]int)
	for _, mm := range m {
		for k, v := range mm {
			if v.missingLanguage != 0 {
				if _, ok := missingLanguage[k]; !ok {
					missingLanguage[k] = 0
				}
				missingLanguage[k] += v.missingLanguage
			}
			if v.invalidQuery != 0 {
				if _, ok := invalidQuery[k]; !ok {
					invalidQuery[k] = 0
				}
				invalidQuery[k] += v.invalidQuery
			}
		}
	}
	return
}

func readFileSymbols(
	ctx context.Context,
	parser *parser,
	w workspaceapi.FileSystem,
	filename string, results chan match,
) error {
	file, err := w.OpenFile(filename, os.O_RDONLY, 0)
	if err != nil {
		return fmt.Errorf("open file: %v", err)
	}

	r := bufio.NewReader(file)
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}

	buf := new(cell.Buffer)
	buf.Init()
	_, _ = buf.ReadFrom(bytes.NewReader(data))

	tree := parser.parser.Parse(data, nil)
	if tree == nil {
		return errors.New("failed to parse data")
	}
	defer tree.Close()

	cur := sitter.NewQueryCursor()
	defer cur.Close()

	root := tree.RootNode()
	captureNames := parser.query.CaptureNames()
	content := []byte(buf.String())
	matches := cur.Matches(parser.query, root, content)
	var retErr error
	for {
		m, ok := matches.Next()
		if !ok {
			break
		}
		for _, cap := range m.Captures {
			name := captureNames[cap.Index]
			rng := cap.Node.Range()
			from, to, err := convertRangeToCoordinates(buf.RawCells(), rng)
			if err != nil {
				log.Tracef("convert query: %v", err)
				continue
			}
			if int(cap.Index) >= len(captureNames) {
				log.Tracef("index %d does not belong capture names %v",
					cap.Index, captureNames)
				continue
			}
			result, err := makeSymbolItem(from, to, filename, buf, name)
			if err != nil {
				retErr = multierror.Append(retErr, err)
				continue
			}
			select {
			case results <- result:
			case <-ctx.Done():
				return retErr
			}
		}
	}

	return retErr
}

func convertRangeToCoordinates(cells [][]term.Cell, n sitter.Range) (
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

func makeSymbolItem(
	from, to term.Coordinates, filename string, buf *cell.Buffer,
	captureName string,
) (match, error) {
	to.Y = from.Y
	to.X = buf.Columns(from.Y)
	cells, _, _ := buf.Select(from, to)
	textToDisplay := term.CellsToString(cells)
	lineStr := fmt.Sprintf("%s:%d: %s", filename, from.Y+1, textToDisplay)
	return match{CaptureName: captureName, LineString: lineStr}, nil
}

func readSymbolsWorker(
	ctx context.Context, w workspaceapi.FileSystem, dataDir string,
	uri workspaceapi.URI, queryFile string, results chan match, files chan string,
	err *error, expectedErrors map[string]*expectedError,
	query string,
) {
	pkg := extension.NewPkgManager(dataDir, uri, w)
	parsers := make(map[string]*parser)
	for {
		select {
		case <-ctx.Done():
			return
		case path, ok := <-files:
			if !ok {
				return
			}
			langID, lerr := languages.LanguageForFile(path)
			if lerr != nil {
				continue
			}
			parser, ok := parsers[langID]
			if !ok {
				var err error
				parser, err = newParser(ctx, langID, pkg, queryFile, query)
				if err != nil {
					if errors.Is(err, extension.ErrNotInstalled) {
						ext := filepath.Ext(path)
						if _, ok := expectedErrors[ext]; !ok {
							expectedErrors[ext] = &expectedError{}
						}
						expectedErrors[ext].missingLanguage++
					} else if errors.Is(err, errInvalidQuery) {
						ext := filepath.Ext(path)
						if _, ok := expectedErrors[ext]; !ok {
							expectedErrors[ext] = &expectedError{}
						}
						expectedErrors[ext].invalidQuery++
					} else {
						log.Errorf("new parser for language %q: %v", langID, err)
					}
					continue
				}
				defer parser.Close()
				parsers[langID] = parser
			}

			readErr := readFileSymbols(ctx, parser, w, path, results)
			if readErr != nil {
				*err = multierror.Append(*err, readErr)
			}
		}
	}
}

type listSymbolsIterator struct {
	mu          sync.Mutex
	err         error
	ctx         context.Context
	ch          chan match
	cancel      func()
	closeWaitCh chan struct{}
}

func (l *listSymbolsIterator) Next(ctx context.Context) (match, bool) {
	select {
	case <-ctx.Done():
		l.mu.Lock()
		defer l.mu.Unlock()
		l.err = multierror.Append(l.err, ctx.Err())
		return match{}, false
	case <-l.ctx.Done():
		l.mu.Lock()
		defer l.mu.Unlock()
		l.err = multierror.Append(l.err, l.ctx.Err())
		return match{}, false
	case path, ok := <-l.ch:
		return path, ok
	}
}

func (l *listSymbolsIterator) Err() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.err == nil {
		return l.ctx.Err()
	}
	// avoid data races onto l.err which is an instance of
	// *multierr.Error by creating a new multierr.Error
	err := multierror.Append(nil, l.err)
	if l.ctx.Err() == nil {
		return err
	}
	return multierror.Append(err, l.ctx.Err())
}

func (l *listSymbolsIterator) Close() error {
	l.cancel()
	<-l.closeWaitCh
	return nil
}

type expectedError struct {
	missingLanguage int
	invalidQuery    int
}
