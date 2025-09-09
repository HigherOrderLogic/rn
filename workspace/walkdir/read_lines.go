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
	"bufio"
	"context"
	"fmt"
	"os"
	"sync"

	multierr "github.com/ernestrc/go-multierror"
	"github.com/unstablebuild/blue/iterator"
	"unstable.build/go-tui/debug"
)

// ReadLines takes an iterator of file paths, i.e. return of ListFiles
// and returns an iterator of file lines, encoded
func ReadLines(ctx context.Context, w Reader, paths iterator.Iterator[string]) (
	iterator.Iterator[string], error,
) {
	files := make(chan string)
	lines := make(chan string)
	closeWaitCh := make(chan struct{})
	ctx, cancel := context.WithCancel(ctx)

	var wg sync.WaitGroup
	wg.Add(defaultWorkers)
	errors := make([]error, defaultWorkers)
	for i := 0; i < defaultWorkers; i++ {
		err := &errors[i]
		go debug.CapturePanicReport(func() {
			defer wg.Done()
			readFileWorker(ctx, w, lines, files, err)
		})
	}

	it := &listFilesIterator{
		ctx:    ctx,
		dataCh: lines,
	}
	it.cancel = cancel
	it.closeWaitCh = closeWaitCh

	var itErr error
	go debug.CapturePanicReport(func() {
		defer close(closeWaitCh)
		defer close(lines)
		defer paths.Close()

	loop:
		for {
			file, ok := paths.Next(ctx)
			if !ok {
				if err := paths.Err(); err != nil {
					itErr = err
				}
				break
			}
			select {
			case <-ctx.Done():
				break loop
			case files <- file:
			}
		}
		close(files)
		wg.Wait()

		it.mu.Lock()
		defer it.mu.Unlock()

		it.err = itErr
		for _, err := range errors {
			if err != nil {
				it.err = multierr.Append(it.err, err)
			}
		}
	})
	return it, nil
}

func readFile(w Reader, buffer []byte, file string, lines chan string) error {
	f, werr := w.Open(file, os.O_RDONLY, 0)
	if werr != nil {
		return werr.ToError()
	}
	r := bufio.NewScanner(f)
	r.Buffer(buffer, len(buffer))
	var i int
	for r.Scan() {
		i++
		lines <- fmt.Sprintf("%s:%d:%s", file, i, r.Text())
	}
	return r.Err()
}

func readFileWorker(
	ctx context.Context, w Reader,
	lines chan string, files chan string,
	err *error,
) {
	buffer := make([]byte, bufio.MaxScanTokenSize)
	for {
		select {
		case <-ctx.Done():
			return
		case path, ok := <-files:
			if !ok {
				return
			}
			readErr := readFile(w, buffer, path, lines)
			if readErr != nil {
				*err = multierr.Append(*err, readErr)
			}
		}
	}
}
