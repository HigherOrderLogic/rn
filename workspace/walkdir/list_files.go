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

package walkdir

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"

	multierr "github.com/ernestrc/go-multierror"
	"github.com/unstablebuild/blue/iterator"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"unstable.build/go-tui/debug"
)

// Reader abstracts the ability to read directory contents.
type Reader interface {
	URI(string) (workspaceapi.URI, error)
	OpenFile(path string, flag int, perm os.FileMode) (workspaceapi.File, error)
	Stat(path string) (os.FileInfo, error)
	ReadDir(name string) ([]os.DirEntry, error)
}

// ListFiles traverses the workspace directory and returns
// an iterator that returns all file paths under root. If root is
// a partial or full file name, it will be ignored and its base
// directory, will be used. If errors are encountered while reading
// the contents of directories those errors will be aggregated and
// reported by the iterator's Err method.
func ListFiles(
	ctx context.Context, w Reader, root string,
) (iterator.Iterator[string], error) {
	var wg sync.WaitGroup
	iterCh := make(chan string)
	workerCh := make(chan string)
	closeWaitCh := make(chan struct{})
	allErrors := make([]error, defaultWorkers)

	workspaceURI, err := w.URI(".")
	if err != nil {
		return nil, fmt.Errorf("URI: %v", err)
	}
	rootURI, err := w.URI(root)
	if err != nil {
		return nil, fmt.Errorf("URI: %v", err)
	}

	// get root as relative path to workspace
	root = workspaceapi.RelPath(workspaceURI, rootURI)

	iterator := &listFilesIterator{dataCh: iterCh}
	ctx, cancel := context.WithCancel(ctx)
	iterator.ctx = ctx
	iterator.cancel = cancel
	iterator.closeWaitCh = closeWaitCh

	for i := 0; i < defaultWorkers; i++ {
		go debug.CapturePanicReport(func() {
			traverseDirWorker(ctx, w, &wg, iterCh, workerCh,
				workspaceURI.Path(), &iterator.mu, &allErrors[i])
		})
	}

	for {
		finfo, err := w.Stat(root)
		if err == nil && finfo.IsDir() {
			break
		}
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		root = filepath.Dir(root)
	}

	wg.Add(1)
	workerCh <- root

	go debug.CapturePanicReport(func() {
		defer close(closeWaitCh)
		defer close(iterCh)
		defer close(workerCh)

		wg.Wait()
		iterator.mu.Lock()
		defer iterator.mu.Unlock()
		for _, err := range allErrors {
			if err != nil {
				iterator.err = multierr.Append(iterator.err, err)
			}
		}
	})

	return iterator, nil
}

var defaultWorkers = runtime.NumCPU() * 8

func traverseDirWorker(
	ctx context.Context, w Reader, wg *sync.WaitGroup,
	iterCh, workerCh chan string, cwd string, mu *sync.Mutex, err *error,
) {
	for {
		// do not use ctx here, as we could endup with an outstanding
		// counter on the wait group, which would leak a goroutine.
		path, ok := <-workerCh
		if !ok {
			return
		}
		dirErr := dirTraversal(ctx, w, cwd, path, wg, iterCh, workerCh)
		if dirErr != nil {
			mu.Lock()
			*err = multierr.Append(*err, dirErr)
			mu.Unlock()
		}
	}
}

func dirTraversal(
	ctx context.Context, w Reader, cwd, dirname string,
	wg *sync.WaitGroup, iterCh, workerCh chan string,
) error {
	defer wg.Done()
	absPath := dirname
	if !filepath.IsAbs(dirname) {
		absPath = filepath.Join(cwd, dirname)
	}

	dirNames, err := w.ReadDir(absPath)
	if err != nil {
		return err
	}

	var ret error
	for _, info := range dirNames {
		path := filepath.Join(dirname, info.Name())
		tpe := info.Type()
		if tpe.IsRegular() {
			// ensure dirTraversal returns
			select {
			case <-ctx.Done():
				return ctx.Err()
			case iterCh <- path:
				continue
			}
		}
		if !tpe.IsDir() {
			continue
		}

		wg.Add(1)
		select {
		// ensure dirTraversal returns
		case <-ctx.Done():
			wg.Done()
			return ctx.Err()
		case workerCh <- path:
		default:
			// the rest of workers are busy, keep going
			err := dirTraversal(ctx, w, cwd, path, wg, iterCh, workerCh)
			if err != nil {
				ret = multierr.Append(ret, err)
			}
		}
	}
	return ret
}

type listFilesIterator struct {
	mu          sync.Mutex
	err         error
	ctx         context.Context
	dataCh      chan string
	cancel      func()
	closeWaitCh chan struct{}
}

func (l *listFilesIterator) Next(ctx context.Context) (string, bool) {
	select {
	case <-ctx.Done():
		l.mu.Lock()
		defer l.mu.Unlock()
		l.err = multierr.Append(l.err, ctx.Err())
		return "", false
	case <-l.ctx.Done():
		l.mu.Lock()
		defer l.mu.Unlock()
		l.err = multierr.Append(l.err, l.ctx.Err())
		return "", false
	case path, ok := <-l.dataCh:
		return path, ok
	}
}

func (l *listFilesIterator) Close() error {
	l.cancel()
	<-l.closeWaitCh
	return nil
}

func (l *listFilesIterator) Err() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.err == nil {
		return l.ctx.Err()
	}
	// avoid data races onto l.err which is an instance of
	// *multierr.Error by creating a new multierr.Error
	err := multierr.Append(nil, l.err)
	if l.ctx.Err() == nil {
		return err
	}
	return multierr.Append(err, l.ctx.Err())
}
