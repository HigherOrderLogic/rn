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

package workspace

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	multierr "github.com/ernestrc/go-multierror"
	"github.com/unstablebuild/rune-go-sdk/api/schemeapi"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"github.com/unstablebuild/rune-go-sdk/term"
	"unstable.build/go-tui/cell"
	"unstable.build/go-tui/debug"
)

const (
	defaultFileMode os.FileMode = 0644
)

var _ FlusherCloser = (*file)(nil)

// file implements the sync (swap file) logic
type file struct {
	// used by Flush, Close and worker only
	// since async work goroutine only starts after
	// file has been fully initialized
	wg      sync.WaitGroup
	ch      chan struct{}
	mu      sync.Mutex
	content string
	scheme  schemeapi.Scheme

	buf             *cell.Buffer
	view            UnixFileView
	reloading       bool
	swapDir         string
	swapFileName    string
	fileName        string
	readOnly        bool
	infoModTime     time.Time
	swapInfoModTime time.Time
	orig, swap      workspaceapi.File
	delayedError    error
	unflushed       bool
	lastFlush       time.Time
	// asyncWG tracks in-flight Flush/ForceFlush/Reload goroutines so
	// Close can wait for them before tearing down resources.
	asyncWG sync.WaitGroup
	// flushing is true while an async Flush/ForceFlush/Reload is in
	// flight. Guarded by mu. Gates concurrent attempts via
	// ErrFlushInProgress, and makes OnDidEdit defer its swap write
	// until the async op completes.
	flushing bool
	// pendingEdits is set by OnDidEdit when an edit arrives while
	// flushing == true. The async goroutine consumes it on completion
	// to enqueue a catch-up swap write against the freshly opened
	// swap file.
	pendingEdits bool
}

func newFile(p schemeapi.Scheme, path string, buf *cell.Buffer, swapDir string, readOnly bool) (
	*file, error,
) {
	ret := new(file)
	ret.scheme = p

	err := ret.init(path, buf, swapDir, readOnly)
	if err != nil {
		return nil, err
	}
	return ret, nil
}

func newFileRecover(p schemeapi.Scheme, path, swapFilePath string, buf *cell.Buffer, force bool) (
	*file, error,
) {
	ret := new(file)
	ret.scheme = p

	err := ret.initRecover(path, swapFilePath, buf, force)
	if err != nil {
		return nil, err
	}
	return ret, err
}

func swapFileName(swapDir, filePath string) (string, string) {
	if swapDir == "" {
		swapDir = filepath.Dir(filePath)
	}
	return swapDir, path.Join(swapDir, fmt.Sprintf(".%s.swp", filepath.Base(filePath)))
}

func (f *file) initSwapFile(orig workspaceapi.File, origPerms os.FileMode) (workspaceapi.File, error) {
	swap, osErr := f.scheme.OpenFile(f.swapFileName, os.O_RDWR|os.O_CREATE|os.O_EXCL, origPerms)
	if osErr != nil {
		if os.IsExist(osErr) {
			return nil, workspaceapi.ErrFileAlreadyOpen
		}
		return nil, osErr
	}

	if orig == nil {
		return swap, nil
	}

	content, err := io.ReadAll(orig)
	if err != nil {
		_ = f.scheme.Remove(f.swapFileName)
		return nil, err
	}

	// not necessary for good scheme implementations, but we should
	// not trust that flags are interpreted correctly
	if err := swap.Truncate(0); err != nil {
		_ = f.scheme.Remove(f.swapFileName)
		return nil, err
	}

	if _, err := swap.Write(content); err != nil {
		_ = f.scheme.Remove(f.swapFileName)
		return nil, err
	}

	if err := swap.Sync(); err != nil {
		_ = f.scheme.Remove(f.swapFileName)
		return nil, err
	}

	if _, err := orig.Seek(0, 0); err != nil {
		_ = f.scheme.Remove(f.swapFileName)
		return nil, err
	}

	return swap, nil
}

func validateFileType(file workspaceapi.File) (os.FileInfo, error) {
	fileInfo, err := file.Stat()
	if err != nil {
		return nil, err
	}

	if fileInfo.IsDir() {
		return nil, workspaceapi.ErrFileIsNotRegular
	}

	mode := fileInfo.Mode()
	if mode.IsRegular() || mode&os.ModeSymlink != 0 {
		return fileInfo, nil
	}

	return nil, workspaceapi.ErrFileIsNotRegular
}

func (f *file) openFile(filePath string, flag int) (
	workspaceapi.File, os.FileInfo, error,
) {
	file, err := f.scheme.OpenFile(filePath, flag, 0666)
	if err != nil {
		return nil, nil, err
	}

	fileInfo, verr := validateFileType(file)
	if verr != nil {
		return nil, nil, verr
	}

	return file, fileInfo, nil
}

func (f *file) initFiles(filePath, swapDir string, readOnly bool) error {
	flag := os.O_RDWR
	if readOnly {
		flag = os.O_RDONLY
	}
	file, fileInfo, err := f.openFile(filePath, flag)
	switch {
	case os.IsNotExist(err):
		// create unless read-only mode
		if !readOnly {
			err = nil
		}
	case os.IsPermission(err):
		// delegate write error to Flush
		file, fileInfo, err = f.openFile(filePath, os.O_RDONLY)
		readOnly = true
	}
	if err != nil {
		return err
	}

	if !readOnly {
		err := f.initSwap(swapDir, filePath, file, fileInfo)
		if err != nil {
			return err
		}
	}

	f.orig = file
	if fileInfo != nil {
		f.infoModTime = fileInfo.ModTime()
	}
	f.fileName = filePath
	f.readOnly = readOnly

	return nil
}

func (f *file) initSwap(
	swapDir, filePath string, orig workspaceapi.File, fileInfo os.FileInfo,
) error {
	swapDir, swapFileName := swapFileName(swapDir, filePath)
	if f.swapFileName == "" {
		f.swapFileName = swapFileName
	}

	mode := os.FileMode(defaultFileMode)
	if fileInfo != nil {
		mode = fileInfo.Mode()
	}
	swap, err := f.initSwapFile(orig, mode)
	if err != nil {
		return err
	}
	// store swapInfo so we can check update times at Flush
	swapInfo, err := f.scheme.Stat(f.swapFileName)
	if err != nil {
		_ = f.scheme.Remove(f.swapFileName)
		return err
	}
	f.swap = swap
	f.swapInfoModTime = swapInfo.ModTime()
	f.swapDir = swapDir

	return nil
}

func (f *file) initBuffer(buf *cell.Buffer, file workspaceapi.File) (err error) {
	buf.Reset()
	view := NewUnixFileView(buf.View())

	// file could be not created yet, so initialize from the swap
	// in case some scheme implementations initialize files with
	// a template
	if file == nil {
		file = f.swap
	}
	if file != nil {
		_, err = buf.ReadFrom(file)
		if err != nil {
			return
		}

		defer func() {
			_, err = file.Seek(0, 0)
		}()
	}

	if !view.EndsWithEOL() {
		buf.WriteString("\n")
	}

	buf.Subscribe(f)
	buf.WithView(view)
	buf.ResetVersion()

	f.buf = buf
	f.view = view

	return
}

func (f *file) initRecover(filePath, swapFilePath string, buf *cell.Buffer, force bool) error {
	orig, info, err := f.openFile(filePath, os.O_RDWR)
	if os.IsNotExist(err) {
		err = nil
	}
	if err != nil {
		return err
	}
	swap, swapFileInfo, osErr := f.openFile(swapFilePath, os.O_RDWR)
	if osErr != nil {
		return osErr
	}

	f.orig = orig
	f.swap = swap
	if info != nil {
		f.infoModTime = info.ModTime()
	}
	f.swapInfoModTime = swapFileInfo.ModTime()
	f.swapFileName = swapFilePath
	f.swapDir = filepath.Dir(swapFilePath)
	f.fileName = filePath

	err = f.initBuffer(buf, f.swap)
	if err != nil {
		// do not remove swap if error is that swap is out of date
		// let use decide what to do with it
		if clerr := f.swap.Close(); clerr != nil {
			err = multierr.Append(err, clerr)
		}
		f.swap = nil
		if clerr := f.Close(); clerr != nil {
			err = multierr.Append(err, clerr)
		}
		return err
	}

	err = f.flush(force)
	if err != nil {
		buf.Reset()
		if clerr := f.swap.Close(); clerr != nil {
			err = multierr.Append(err, clerr)
		}
		f.swap = nil
		if clerr := f.Close(); clerr != nil {
			err = multierr.Append(err, clerr)
		}
		return err
	}

	f.setupCopySwapWorker()

	return nil
}

func (f *file) setupCopySwapWorker() {
	// a buffered channel of 1 guarantees that if worker
	// is busy and the call to copyFlushSwap is skipped
	// we are going to copyFlushSwap at least one final time
	f.ch = make(chan struct{}, 1)

	ch := f.ch
	go debug.CapturePanicReport(func() {
		for range ch {
			f.mu.Lock()
			str := f.content
			f.mu.Unlock()
			f.copyFlushSwapFile(str)
			f.wg.Done()
		}
	})
}

// init instantiates opens the file at filePath and initializes
// buf with the contents of it. If swapDir is "", then filePath directory is
// used as a swap directory
func (f *file) init(
	file string, buf *cell.Buffer, swapDir string, readOnly bool,
) error {
	err := f.initFiles(file, swapDir, readOnly)
	if err != nil {
		return err
	}

	err = f.initBuffer(buf, f.orig)
	if err != nil {
		if clerr := f.Close(); clerr != nil {
			err = multierr.Append(err, clerr)
		}
		return err
	}

	f.setupCopySwapWorker()

	return nil
}

func (f *file) delayCopySwapError(err error) {
	f.delayedError = fmt.Errorf("swap file error %s: %s", f.swapFileName, err)
}

// recoverFiles closes the cached orig/swap descriptors and re-opens
// them against the current scheme. It is used when a previous swap
// write failed and may have left us with stale file descriptors —
// for example after an ssh scheme reconnected following a transient
// network drop. The on-disk swap file is removed before re-opening
// because the in-memory buffer is the source of truth at this point
// (a subsequent copyFlushSwapFile will re-stage it).
//
// recoverFiles intentionally does not touch the buffer or set
// f.reloading: the swap-copy worker is quiescent because the caller
// (flush) is already running with f.wg drained.
//
// On error f.orig / f.swap are left nil and f.readOnly stays at the
// pre-recovery value. flush() is expected to retry recovery on the
// next call: see the swap == nil && !readOnly branch.
func (f *file) recoverFiles() error {
	prevReadOnly := f.readOnly
	if f.orig != nil {
		_ = f.orig.Close()
		f.orig = nil
	}
	if f.swap != nil {
		_ = f.swap.Close()
		f.swap = nil
	}
	// Best-effort removal of any leftover swap file from the dead
	// session. If the scheme is still flaky this may fail; that's
	// fine, initFiles → initSwap will surface a clearer error.
	if f.swapFileName != "" {
		_ = f.scheme.Remove(f.swapFileName)
	}
	err := f.initFiles(f.fileName, f.swapDir, prevReadOnly)
	if err != nil {
		// Don't leave the file flagged read-only just because a
		// transient recovery attempt failed; otherwise a later
		// successful recovery would still surface as
		// ErrFileIsNotWritable. readOnly is only meaningful once
		// initFiles has succeeded against a live scheme.
		f.readOnly = prevReadOnly
	}
	return err
}

func (f *file) copyFlushSwapFile(str string) (ok bool) {
	if f.swap == nil {
		return
	}

	f.unflushed = true
	err := f.swap.Truncate(0)
	if err != nil {
		f.delayCopySwapError(err)
		return
	}
	_, err = f.swap.Seek(0, 0)
	if err != nil {
		f.delayCopySwapError(err)
		return
	}

	// files must end in EOL
	if !strings.HasSuffix(str, "\n") {
		str += "\n"
	}

	if _, err = f.swap.Write([]byte(str)); err != nil {
		f.delayCopySwapError(err)
		return
	}

	if err := f.swap.Sync(); err != nil {
		f.delayCopySwapError(err)
		return
	}

	finfo, err := f.swap.Stat()
	if err != nil {
		f.delayCopySwapError(err)
		return
	}

	// we have to always use what the file is reporting as mod time
	// because we cannot use the host's clock or a skew on a remote
	// workspace would introduce all sorts of bugs
	f.swapInfoModTime = finfo.ModTime()
	ok = true
	return
}

func (f *file) OnWillEdit(ctx context.Context, start, end term.Coordinates, str string) {
	f.wg.Add(1)
}

func (f *file) OnDidEdit(ctx context.Context, from, to term.Coordinates, old string) {
	if f.reloading {
		f.wg.Done()
		return
	}
	// store the latest version of the buffer so the last
	// copyFlushSwap to run uses the up-to-date version.
	f.mu.Lock()
	f.content = f.buf.String()
	flushing := f.flushing
	if flushing {
		f.pendingEdits = true
	}
	f.mu.Unlock()
	if flushing {
		// async Flush/ForceFlush/Reload owns the swap file right now;
		// the goroutine will enqueue a catch-up swap write upon
		// completion.
		f.wg.Done()
		return
	}
	select {
	case f.ch <- struct{}{}:
	default:
		f.wg.Done()
	}
}

func (f *file) touchFile() (isExist bool) {
	var err error
	f.orig, err = f.scheme.OpenFile(f.fileName, os.O_RDWR|os.O_CREATE|os.O_EXCL, defaultFileMode)
	if err != nil {
		// file was not created when instantiating this file, but now
		// file seems to be there so file must be stale.
		if os.IsExist(err) {
			return true
		}
		// ignore other errors
	}
	return false
}

// Flush saves the contents of the buffer to disk asynchronously. If
// the file was modified by some other process, the returned channel
// will receive workspaceapi.ErrStaleData. See FlusherCloser for the
// full channel contract.
func (f *file) Flush(ctx context.Context) (<-chan error, error) {
	return f.startAsync(ctx, false, func() error { return f.flush(false) })
}

// ForceFlush forces saving the contents of the buffer to disk,
// overwriting any changes if the file was modified by another
// process. See Flush for the channel contract.
func (f *file) ForceFlush(ctx context.Context) (<-chan error, error) {
	return f.startAsync(ctx, false, func() error { return f.flush(true) })
}

// LastFlush returns the last time this file was flushed or a zero value time
// if this file has not been flushed yet.
func (f *file) LastFlush() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.lastFlush
}

// Reload reloads the contents of the buffer from disk asynchronously.
// See Flush for the channel contract.
func (f *file) Reload(ctx context.Context) (<-chan error, error) {
	return f.startAsync(ctx, true, f.reload)
}

// reload performs the synchronous reload work. It must be invoked
// from the async goroutine (via startAsync) so that f.reloading and
// the worker quiescing protocol hold.
func (f *file) reload() error {
	// stop worker and copy swap while we're reloading swap
	f.reloading = true
	defer func() {
		f.reloading = false
	}()

	f.wg.Wait()

	// re-init files, if any of these error, we either
	// don't care or it'll cause an error in initFiles
	if f.orig != nil {
		_ = f.orig.Close()
	}
	if f.swap != nil {
		_ = f.swap.Close()
		_ = f.scheme.Remove(f.swapFileName)
	}

	err := f.initFiles(f.fileName, f.swapDir, f.readOnly)
	if err != nil {
		return err
	}

	if f.orig == nil {
		return errors.New("cannot reload a file that doesn't exist on disk")
	}

	// read from file into buffer
	data, err := io.ReadAll(f.orig)
	if err != nil {
		_, _ = f.orig.Seek(0, 0) // avoid partially read file
		return fmt.Errorf("read from file: %w", err)
	}
	// ensure that data is erased regardless of view installed
	f.buf.Reset()
	f.buf.InsertString(term.Coordinates{}, string(data))

	if !f.view.EndsWithEOL() {
		f.buf.WriteString("\n")
	}

	_, err = f.orig.Seek(0, 0)
	if err != nil {
		return fmt.Errorf("seek: %w", err)
	}
	f.mu.Lock()
	f.lastFlush = f.infoModTime
	f.mu.Unlock()
	return nil
}

// startAsync runs work on a background goroutine and returns a
// buffered channel that will receive the result. Returns
// ErrFlushInProgress if another async op is in flight. If
// suppressKick is true (reload), the pendingEdits catch-up worker
// kick is skipped — reload intentionally resets the buffer to the
// on-disk contents and edits in flight are discarded by design.
func (f *file) startAsync(
	ctx context.Context, suppressKick bool, work func() error,
) (<-chan error, error) {
	f.mu.Lock()
	if f.flushing {
		f.mu.Unlock()
		return nil, ErrFlushInProgress
	}
	f.flushing = true
	f.pendingEdits = false
	f.mu.Unlock()

	ch := make(chan error, 1)
	f.asyncWG.Add(1)
	go debug.CapturePanicReport(func() {
		defer f.asyncWG.Done()

		err := work()

		f.mu.Lock()
		f.flushing = false
		kick := !suppressKick && f.pendingEdits && f.swap != nil
		f.pendingEdits = false
		f.mu.Unlock()

		if kick {
			f.wg.Add(1)
			select {
			case f.ch <- struct{}{}:
			default:
				f.wg.Done()
			}
		}

		// Honor ctx cancellation: deliver ctx.Err() instead of the real
		// result so callers see a uniform "cancelled" signal.
		if ctxErr := ctx.Err(); ctxErr != nil {
			ch <- ctxErr
		} else {
			ch <- err
		}
		close(ch)
	})
	return ch, nil
}

func (f *file) flush(force bool) error {
	f.wg.Wait()

	// If a previous recoverFiles call failed partway (e.g. the
	// scheme was still flaky when the user retried :write right
	// after a reconnect), f.swap may be nil even though the file
	// isn't actually read-only. Retry recovery once before
	// declaring the file unwritable so the next :write is not
	// stuck on a stale state. We skip this for genuinely
	// read-only files (readOnly == true) where initFiles
	// purposefully left swap nil. force flushes always proceed —
	// they handle f.swap == nil themselves further down.
	if f.swap == nil && !force && !f.readOnly && f.fileName != "" {
		if rerr := f.recoverFiles(); rerr != nil {
			return workspaceapi.ErrFileIsNotWritable
		}
		if f.swap == nil {
			return workspaceapi.ErrFileIsNotWritable
		}
	} else if f.swap == nil && !force {
		return workspaceapi.ErrFileIsNotWritable
	}

	err := f.delayedError
	if err != nil {
		f.delayedError = nil
		if !f.copyFlushSwapFile(f.buf.String()) {
			// Replay failed. The most likely cause is that the
			// underlying scheme dropped (e.g. ssh transport died,
			// scheme reconnected) and our cached file descriptors
			// are stale against the new session. Try recovering
			// the file handles and retry the copy once before
			// giving up. If recovery still fails we surface the
			// most recent error so the user can see what's wrong.
			// Capture the replay's swap-file error before recovery
			// (recovery clears f.delayedError on success) so we can
			// fall back to it when recovery itself fails — keeping
			// the user-facing "swap file error <name>: ..." surface.
			replayErr := f.delayedError
			if replayErr == nil {
				replayErr = err
			}
			if rerr := f.recoverFiles(); rerr != nil {
				return replayErr
			}
			f.delayedError = nil
			if !f.copyFlushSwapFile(f.buf.String()) {
				retErr := f.delayedError
				if retErr == nil {
					retErr = replayErr
				}
				return retErr
			}
		}
	}

	// used to override with symlink target if applicable
	origTarget := f.fileName

	var newFileInfo os.FileInfo

	// create file if it didn't exist before
	if f.orig == nil {
		isExist := f.touchFile()
		if isExist {
			return workspaceapi.ErrStaleData
		}
	} else if f.readOnly && force {
		// if it exists, but created readonly and want to force flush
		// overwrite f.orig with correct flags
		f.orig, newFileInfo, err = f.openFile(f.fileName, os.O_CREATE|os.O_RDWR)
		if err != nil {
			return err
		}
	} else {
		newFileInfo, err = f.scheme.Stat(f.fileName)
		if err != nil && force && os.IsNotExist(err) {
			// file was might have been removed, create it if in force mode
			f.orig, newFileInfo, err = f.openFile(f.fileName, os.O_CREATE|os.O_RDWR)
			if err != nil {
				return err
			}
		} else if err != nil {
			return err
		} else {
			if !force && (newFileInfo.ModTime().After(f.swapInfoModTime) ||
				newFileInfo.ModTime().After(f.infoModTime)) {
				return workspaceapi.ErrStaleData
			}

			newFileInfo, err = f.scheme.Lstat(f.fileName)
			if err != nil {
				return err
			}
			if newFileInfo.Mode()&os.ModeSymlink != 0 {
				origTarget, err = f.scheme.Readlink(origTarget)
				if err != nil {
					return err
				}
			}
		}
	}

	newSwapInfo, err := f.scheme.Stat(f.swapFileName)
	if (err != nil && force) || f.swap == nil {
		err := f.initSwap(f.swapDir, f.fileName, f.orig, newFileInfo)
		if err != nil {
			return err
		}
		// write happens in the default goroutine so there's no need to sync
		f.content = f.buf.String()
		f.copyFlushSwapFile(f.content)
	} else if err != nil {
		return err
	}

	if !force && newSwapInfo.ModTime().After(f.swapInfoModTime) {
		return workspaceapi.ErrStaleData
	}

	if f.orig != nil {
		_ = f.orig.Close()
	}
	_ = f.swap.Close()

	err = f.scheme.Rename(f.swapFileName, origTarget)
	if err != nil {
		return err
	}

	readOnly := f.readOnly
	if readOnly && force {
		readOnly = false
	}
	err = f.initFiles(f.fileName, f.swapDir, readOnly)
	if err != nil {
		return err
	}

	f.unflushed = false
	f.mu.Lock()
	f.lastFlush = f.infoModTime
	f.mu.Unlock()

	return nil
}

// Close should be called once when this structure is not to be used anymore.
func (f *file) Close() (ret error) {
	if f.fileName == "" {
		return errors.New("trying to Close an uninitialized file")
	}

	// wait for any in-flight async Flush/ForceFlush/Reload before
	// tearing down resources they may still be using
	f.asyncWG.Wait()

	f.fileName = ""

	// Wait for all async work to complete.
	// Calling goroutine should be the same goroutine
	// that calls OnDidEdit so no more work should be added
	f.wg.Wait()

	if f.ch != nil {
		close(f.ch)
	}

	if f.buf != nil {
		f.buf.Unsubscribe(f)
	}

	if f.swap != nil {
		if err := f.swap.Close(); err != nil {
			ret = multierr.Append(ret, err)
		}
		newSwapInfo, err := f.scheme.Stat(f.swapFileName)
		if err != nil {
			ret = multierr.Append(ret, err)
		} else {
			// do not remove a swap from another process, in case
			// another process took over ownership after we did
			if !newSwapInfo.ModTime().After(f.swapInfoModTime) {
				if err := f.scheme.Remove(f.swapFileName); err != nil {
					ret = multierr.Append(ret, err)
				}
			}
		}

		f.swap = nil
	}

	if f.orig != nil {
		if err := f.orig.Close(); err != nil {
			ret = multierr.Append(ret, err)
		}
		f.orig = nil
	}

	return ret
}
