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

package ideupgrade

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// fakePlatformOps is a recording PlatformOps used to drive runUpgrade
// from runupgrade_test.go. It implements darwin- and linux-style
// operations against real files in t.TempDir() so the test asserts on
// observable filesystem state in addition to the call sequence.
type fakePlatformOps struct {
	mu sync.Mutex

	// archiveContent is the bytes returned by Download.
	archiveContent []byte
	// dmgPayload, when non-empty, populates the mounted directory
	// with a single .app folder containing the listed files (relative
	// path -> file content).
	dmgPayload map[string]string
	// archiveLayout populates a tar.gz-equivalent extraction with the
	// given relative path -> file content map.
	archiveLayout map[string]string

	// Failure injection.
	verifyErr          error
	mountErr           error
	gatekeeperErrs     []error // consumed FIFO across AssessGatekeeper calls; nil entry = success.
	verifyCodesignErrs []error // consumed FIFO across VerifyCodesign calls; nil entry = success.
	dittoErr           error
	extractErr         error
	symlinkErr         error
	renameErr          map[string]error // keyed by destination path
	// freeSpace maps a path (typically opts.installRoot or
	// opts.cacheDir) to the free-space value the fake should report
	// for FreeSpace queries. Missing keys default to a huge value so
	// existing tests don't trip the pre-flight check.
	freeSpace map[string]uint64
	// renameOnce, when true, causes the renameErr injection to apply
	// only on the first call to a given destination — letting the
	// rollback rename (which targets the same path) proceed.
	renameOnce      bool
	renameDstFailed map[string]bool

	// Call recording.
	calls []string

	// Mount tracking so detach can clean up.
	mountedAt string

	dittoCalls   int
	renameCalls  int
	removedPaths []string
}

func (f *fakePlatformOps) record(s string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, s)
}

func (f *fakePlatformOps) callSeq() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, len(f.calls))
	copy(out, f.calls)
	return out
}

func (f *fakePlatformOps) Download(_ context.Context, url, dest string, progress func(int64, int64)) error {
	f.record("Download:" + url)
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(dest, f.archiveContent, 0o644); err != nil {
		return err
	}
	if progress != nil {
		n := int64(len(f.archiveContent))
		// Emit an intermediate sample then the boundary so tests can
		// observe live progress as well as completion.
		if n > 1 {
			progress(n/2, n)
		}
		progress(n, n)
	}
	return nil
}

func (f *fakePlatformOps) VerifySHA256(path, want string) error {
	f.record("VerifySHA256")
	if f.verifyErr != nil {
		return f.verifyErr
	}
	return verifySHA256(path, want)
}

func (f *fakePlatformOps) MountDMG(_ context.Context, _ string) (string, func() error, error) {
	f.record("MountDMG")
	if f.mountErr != nil {
		return "", nil, f.mountErr
	}
	dir, err := os.MkdirTemp("", "fake-mount-")
	if err != nil {
		return "", nil, err
	}
	app := filepath.Join(dir, "Rune.app")
	if err := os.MkdirAll(app, 0o755); err != nil {
		return "", nil, err
	}
	for rel, content := range f.dmgPayload {
		full := filepath.Join(app, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return "", nil, err
		}
		if err := os.WriteFile(full, []byte(content), 0o755); err != nil {
			return "", nil, err
		}
	}
	f.mountedAt = dir
	detach := func() error {
		f.record("Detach")
		return os.RemoveAll(dir)
	}
	return dir, detach, nil
}

func (f *fakePlatformOps) AssessGatekeeper(_ context.Context, _ string) error {
	f.record("AssessGatekeeper")
	return f.popErr(&f.gatekeeperErrs)
}

func (f *fakePlatformOps) VerifyCodesign(_ context.Context, _ string) error {
	f.record("VerifyCodesign")
	return f.popErr(&f.verifyCodesignErrs)
}

// popErr returns and removes the next error from a FIFO slice. When
// the slice is empty, the call succeeds. Lets tests differentiate
// pre-install vs post-install behavior for the same op.
func (f *fakePlatformOps) popErr(slot *[]error) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(*slot) == 0 {
		return nil
	}
	err := (*slot)[0]
	*slot = (*slot)[1:]
	return err
}

func (f *fakePlatformOps) Ditto(_ context.Context, src, dst string) error {
	f.record("Ditto")
	f.dittoCalls++
	if f.dittoErr != nil {
		return f.dittoErr
	}
	return copyDir(src, dst)
}

func (f *fakePlatformOps) ExtractTarGz(_ context.Context, _, destDir string) error {
	f.record("ExtractTarGz")
	if f.extractErr != nil {
		return f.extractErr
	}
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return err
	}
	for rel, content := range f.archiveLayout {
		full := filepath.Join(destDir, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(full, []byte(content), 0o755); err != nil {
			return err
		}
	}
	return nil
}

func (f *fakePlatformOps) Symlink(target, linkPath string) error {
	f.record("Symlink")
	if f.symlinkErr != nil {
		return f.symlinkErr
	}
	return replaceSymlink(target, linkPath)
}

func (f *fakePlatformOps) RenameAtomic(src, dst string) error {
	f.record(fmt.Sprintf("Rename:%s->%s", filepath.Base(src), filepath.Base(dst)))
	f.renameCalls++
	if e, ok := f.renameErr[dst]; ok {
		if f.renameOnce {
			if f.renameDstFailed == nil {
				f.renameDstFailed = map[string]bool{}
			}
			if !f.renameDstFailed[dst] {
				f.renameDstFailed[dst] = true
				return e
			}
			return os.Rename(src, dst)
		}
		return e
	}
	return os.Rename(src, dst)
}

func (f *fakePlatformOps) RemoveAll(path string) error {
	f.record("RemoveAll")
	f.removedPaths = append(f.removedPaths, path)
	return os.RemoveAll(path)
}

func (f *fakePlatformOps) FreeSpace(path string) (uint64, error) {
	f.record("FreeSpace")
	if v, ok := f.freeSpace[path]; ok {
		return v, nil
	}
	// Default: plenty of room. Tests that want the check to fire
	// must inject an explicit small value for the relevant path.
	return 1 << 40, nil
}

// copyDir recursively copies src into dst. Used by the fake Ditto so
// the destination tree exists for downstream assertions.
func copyDir(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return errors.New("copyDir: src is not a directory")
	}
	if err := os.MkdirAll(dst, info.Mode()); err != nil {
		return err
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, e := range entries {
		s := filepath.Join(src, e.Name())
		d := filepath.Join(dst, e.Name())
		if e.IsDir() {
			if err := copyDir(s, d); err != nil {
				return err
			}
			continue
		}
		data, err := os.ReadFile(s)
		if err != nil {
			return err
		}
		mode := os.FileMode(0o644)
		if fi, err := e.Info(); err == nil {
			mode = fi.Mode()
		}
		if err := os.WriteFile(d, data, mode); err != nil {
			return err
		}
	}
	return nil
}
