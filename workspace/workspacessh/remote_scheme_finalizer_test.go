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

package workspacessh

import (
	"context"
	"os"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/unstablebuild/rune-go-sdk/api/schemeapi"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
)

// fakeSDKFile stands in for workspacerpc.FileClient, which arms a
// finalizer that sends a close for its descriptor.
type fakeSDKFile struct {
	workspaceapi.File
	fd   uintptr
	name string
}

func (f *fakeSDKFile) Fd() uintptr  { return f.fd }
func (f *fakeSDKFile) Name() string { return f.name }
func (f *fakeSDKFile) Close() error { return nil }

type fakeSDKScheme struct {
	schemeapi.Scheme
	nextFd atomic.Uint64
	// closes records the descriptors whose finalizer fired.
	closes chan uintptr
}

func (s *fakeSDKScheme) newFile(name string) workspaceapi.File {
	ret := &fakeSDKFile{fd: uintptr(s.nextFd.Add(1)), name: name}
	// The finalizer must not capture ret, or it would stay reachable.
	fd, closes := ret.fd, s.closes
	runtime.SetFinalizer(ret, func(*fakeSDKFile) { closes <- fd })
	return ret
}

func (s *fakeSDKScheme) Open(filename string) (workspaceapi.File, error) {
	return s.newFile(filename), nil
}

func (s *fakeSDKScheme) Create(filename string) (workspaceapi.File, error) {
	return s.newFile(filename), nil
}

func (s *fakeSDKScheme) OpenFile(filename string, _ int, _ os.FileMode) (
	workspaceapi.File, error,
) {
	return s.newFile(filename), nil
}

func (s *fakeSDKScheme) TempFile(dir, prefix string) (workspaceapi.File, error) {
	return s.newFile(dir + "/" + prefix), nil
}

func (s *fakeSDKScheme) Chroot(string) (schemeapi.Scheme, error) { return s, nil }

func (s *fakeSDKScheme) Close() error { return nil }

func forceGC() {
	for range 3 {
		runtime.GC()
		time.Sleep(10 * time.Millisecond)
	}
}

func TestRemoteSchemeTracksOpenedFiles(t *testing.T) {
	uri, err := workspaceapi.ParseURI("ssh://unstable.build/home/ernie")
	require.NoError(t, err)

	tsuite := []struct {
		desc string
		open func(schemeapi.Scheme) (workspaceapi.File, error)
	}{
		{"Open", func(s schemeapi.Scheme) (workspaceapi.File, error) {
			return s.Open("a.txt")
		}},
		{"Create", func(s schemeapi.Scheme) (workspaceapi.File, error) {
			return s.Create("a.txt")
		}},
		{"OpenFile", func(s schemeapi.Scheme) (workspaceapi.File, error) {
			return s.OpenFile("a.txt", os.O_RDONLY, 0)
		}},
		{"TempFile", func(s schemeapi.Scheme) (workspaceapi.File, error) {
			return s.TempFile("tmp", "pre")
		}},
	}

	// The SDK finalizer is the control: it proves the fake file really
	// does become collectable, so the assertions below are not vacuous.
	t.Run("control: an untracked SDK file is closed by its finalizer", func(t *testing.T) {
		fake := &fakeSDKScheme{closes: make(chan uintptr, 8)}
		fd := func() uintptr {
			f, err := fake.Open("a.txt")
			require.NoError(t, err)
			return f.Fd()
		}()

		forceGC()
		select {
		case got := <-fake.closes:
			require.Equal(t, fd, got)
		default:
			t.Fatal("expected the SDK finalizer to fire")
		}
	})

	for _, tcase := range tsuite {
		for _, chrooted := range []bool{false, true} {
			desc := tcase.desc
			if chrooted {
				desc += " through a chroot"
			}
			t.Run(desc, func(t *testing.T) {
				fake := &fakeSDKScheme{closes: make(chan uintptr, 8)}
				scheme := newRemoteScheme(context.Background(), func(
					_ context.Context, _ workspaceapi.URI, _ func(error),
				) (schemeapi.Scheme, error) {
					return fake, nil
				}, uri)
				t.Cleanup(func() { require.NoError(t, scheme.Close()) })

				target := scheme
				if chrooted {
					target, err = scheme.Chroot(".git")
					require.NoError(t, err)
				}

				f, err := tcase.open(target)
				require.NoError(t, err)
				require.IsType(t, (*remoteFile)(nil), f)

				// Chrooted views must resolve through the parent
				// registry rather than a registry of their own.
				require.Same(t, f, scheme.NewFile(f.Fd(), f.Name()))
				require.Same(t, f, target.NewFile(f.Fd(), f.Name()))

				forceGC()
				select {
				case fd := <-fake.closes:
					t.Fatalf("leaked SDK finalizer sent a stale close for fd %d", fd)
				default:
				}
			})
		}
	}
}
