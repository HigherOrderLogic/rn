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

package ide

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/unstablebuild/rune-go-sdk/api/config"
	"github.com/unstablebuild/rune-go-sdk/api/schemeapi"
	"github.com/unstablebuild/rune-go-sdk/api/textapi"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"github.com/unstablebuild/rune-go-sdk/term"
	"unstable.build/go-tui/ide/vctrl"
	"unstable.build/go-tui/term/vte"
	"unstable.build/go-tui/text"
	"unstable.build/go-tui/text/texttest"
	"unstable.build/go-tui/workspace"
)

func TestEventDispatching(t *testing.T) {
	t.Run("empty event does not panic", func(t *testing.T) {
		var mu sync.Mutex

		x := newExForEventTesting(t)
		fsev := testEventInfo{}

		assert.NotPanics(t, func() {
			dispatchFilesystemEvent(x, &mu, vctrl.NopMatcher(false), fsev)
		})
	})

	t.Run("dispatches", func(t *testing.T) {
		suite := []struct {
			in  schemeapi.Event
			out textapi.EventType
		}{
			{schemeapi.Create, textapi.EventTypeCreate},
			{schemeapi.Remove, textapi.EventTypeRemove},
			{schemeapi.Write, textapi.EventTypeChange},
			{schemeapi.Rename, textapi.EventTypeRename},
		}
		for _, test := range suite {
			desc := fmt.Sprintf("%s event when %s event is received", test.out, test.in)
			t.Run(desc, func(t *testing.T) {
				var mu sync.Mutex
				testURI, err := workspaceapi.ParseURI("file:///a")
				require.NoError(t, err)

				x := newExForEventTesting(t)
				fsev := testEventInfo{e: test.in, u: testURI}

				var called int
				x.comp.SubscribeEvents([]textapi.EventType{test.out},
					textapi.FuncEventHandler(func(ctx context.Context, ev textapi.Event) bool {
						called++
						assert.Equal(t, testURI, ev.URI)
						return false
					}))

				dispatchFilesystemEvent(x, &mu, vctrl.NopMatcher(false), fsev)
				assert.Equal(t, 1, called)
			})
		}
	})

	t.Run("ignores if matcher matches file", func(t *testing.T) {
		suite := []struct {
			in  schemeapi.Event
			out textapi.EventType
		}{
			{schemeapi.Create, textapi.EventTypeCreate},
			{schemeapi.Remove, textapi.EventTypeRemove},
			{schemeapi.Write, textapi.EventTypeChange},
			{schemeapi.Rename, textapi.EventTypeRename},
		}
		for _, test := range suite {
			desc := fmt.Sprintf("when %s event is received", test.in)
			t.Run(desc, func(t *testing.T) {
				var mu sync.Mutex
				testURI, err := workspaceapi.ParseURI("file:///a")
				require.NoError(t, err)

				ignores := vctrl.NopMatcher(true)

				x := newExForEventTesting(t)
				fsev := testEventInfo{e: test.in, u: testURI}

				var called int
				x.comp.SubscribeEvents([]textapi.EventType{test.out},
					textapi.FuncEventHandler(func(ctx context.Context, ev textapi.Event) bool {
						called++
						return false
					}))

				dispatchFilesystemEvent(x, &mu, ignores, fsev)
				assert.Equal(t, 0, called)
			})
		}
	})

	t.Run("ignores if user just flushed file", func(t *testing.T) {
		suite := []struct {
			in    schemeapi.Event
			dirty bool
		}{
			{schemeapi.Create, true},
			{schemeapi.Remove, true},
			{schemeapi.Write, true},
			{schemeapi.Rename, true},

			{schemeapi.Create, false},
			{schemeapi.Remove, false},
			{schemeapi.Write, false},
			{schemeapi.Rename, false},
		}
		for _, test := range suite {
			desc := fmt.Sprintf("when %s event is received, dirty=%t", test.in, test.dirty)
			t.Run(desc, func(t *testing.T) {
				var mu sync.Mutex

				ignores := vctrl.NopMatcher(true)

				x := newExForEventTesting(t)
				testURI, err := x.workspace.URI("a")
				require.NoError(t, err)

				require.NoError(t, x.editFiles(testURI.Path()))
				ed, err := x.comp.Editor(testURI)
				require.NoError(t, err)
				// flush below clears dirty property but it's
				// still useful to make sure that it's integrated correctly
				if test.dirty {
					ctx := context.Background()
					_, _, _ = ed.CellEditor().
						Edit(ctx, term.Coordinates{}, term.Coordinates{}, "ABC")
				}
				win, err := x.comp.Focus()
				require.NoError(t, err)
				require.NoError(t, x.comp.Flush(win))

				res, ok := x.comp.Resource(testURI)
				require.True(t, ok)

				flush, err := x.comp.LastFlush(res)
				require.NoError(t, err)

				fsev := testEventInfo{e: test.in, u: testURI}
				dispatchFilesystemEvent(x, &mu, ignores, fsev)

				assertNoPrompt(t, x)

				lastFlush, err := x.comp.LastFlush(res)
				require.NoError(t, err)
				assert.Equal(t, flush, lastFlush)
			})
		}
	})

	t.Run("is goroutine safe", func(t *testing.T) {
		var mu sync.Mutex
		ignores := vctrl.NopMatcher(false)

		x := newExForEventTesting(t)
		testURI, err := x.workspace.URI("a")
		require.NoError(t, err)

		createOpenWriteFile(t, x, testURI, "abc")

		fsev := testEventInfo{e: schemeapi.Write, u: testURI}

		n := 1000
		var wg sync.WaitGroup
		wg.Add(n)
		for i := 0; i < n; i++ {
			go func() {
				defer wg.Done()
				dispatchFilesystemEvent(x, &mu, ignores, fsev)
			}()
		}
		for i := 0; i < n; i++ {
			_ = os.WriteFile(testURI.Path(), []byte(strconv.Itoa(i)), 0666)
			mu.Lock()
			_, ok := x.comp.Resource(testURI)
			assert.True(t, ok)
			mu.Unlock()
		}
		wg.Wait()
	})

	t.Run("write triggers reload file if open, pre-created, non-dirty", func(t *testing.T) {
		var mu sync.Mutex
		ignores := vctrl.NopMatcher(false)

		x := newExForEventTesting(t)
		testURI, err := x.workspace.URI("a")
		require.NoError(t, err)

		createOpenWriteFile(t, x, testURI, "abc")

		fsev := testEventInfo{e: schemeapi.Write, u: testURI}
		dispatchFilesystemEvent(x, &mu, ignores, fsev)

		assertBufferContent(t, x, testURI, "abc")
	})

	t.Run("write triggers reload file if open, uncreated, non-dirty", func(t *testing.T) {
		var mu sync.Mutex
		ignores := vctrl.NopMatcher(false)

		x := newExForEventTesting(t)
		testURI, err := x.workspace.URI("a")
		require.NoError(t, err)

		openWriteUncreatedFile(t, x, testURI, "abc")

		fsev := testEventInfo{e: schemeapi.Write, u: testURI}
		dispatchFilesystemEvent(x, &mu, ignores, fsev)

		assertBufferContent(t, x, testURI, "abc")
	})

	t.Run("write triggers prompt if open, dirty file changes, user discards", func(t *testing.T) {
		var mu sync.Mutex
		ignores := vctrl.NopMatcher(false)

		x := newExForEventTesting(t)
		testURI, err := x.workspace.URI("a")
		require.NoError(t, err)

		createOpenWriteFile(t, x, testURI, "abc")

		editBuffer(t, x, testURI, "ABC")

		fsev := testEventInfo{e: schemeapi.Write, u: testURI}
		dispatchFilesystemEvent(x, &mu, ignores, fsev)

		userPromptDiscards(t, x)

		assertBufferContent(t, x, testURI, "abc")

		assertNoPrompt(t, x)
	})

	t.Run("write triggers prompt if open, dirty file changes, user overwrites", func(t *testing.T) {
		var mu sync.Mutex
		ignores := vctrl.NopMatcher(false)

		x := newExForEventTesting(t)
		testURI, err := x.workspace.URI("a")
		require.NoError(t, err)

		createOpenWriteFile(t, x, testURI, "abc")
		editBuffer(t, x, testURI, "ABC")

		fsev := testEventInfo{e: schemeapi.Write, u: testURI}
		dispatchFilesystemEvent(x, &mu, ignores, fsev)

		userPromptOverwrites(t, x)

		assertFileContent(t, x, testURI, "ABC")
		assertBufferContent(t, x, testURI, "ABC")

		assertNoPrompt(t, x)
	})

	t.Run("create triggers reload file if open, uncreated, non-dirty", func(t *testing.T) {
		var mu sync.Mutex
		ignores := vctrl.NopMatcher(false)

		x := newExForEventTesting(t)
		testURI, err := x.workspace.URI("a")
		require.NoError(t, err)

		openWriteUncreatedFile(t, x, testURI, "abc")

		fsev := testEventInfo{e: schemeapi.Create, u: testURI}
		dispatchFilesystemEvent(x, &mu, ignores, fsev)

		assertBufferContent(t, x, testURI, "abc")
	})

	t.Run("create triggers reload file if open, pre-created, non-dirty", func(t *testing.T) {
		var mu sync.Mutex
		ignores := vctrl.NopMatcher(false)

		x := newExForEventTesting(t)
		testURI, err := x.workspace.URI("a")
		require.NoError(t, err)

		createOpenWriteFile(t, x, testURI, "abc")

		fsev := testEventInfo{e: schemeapi.Create, u: testURI}
		dispatchFilesystemEvent(x, &mu, ignores, fsev)

		assertBufferContent(t, x, testURI, "abc")
		assertNoPrompt(t, x)
	})

	t.Run("create triggers prompt if open, dirty file changes; user discards", func(t *testing.T) {
		var mu sync.Mutex
		ignores := vctrl.NopMatcher(false)

		x := newExForEventTesting(t)
		testURI, err := x.workspace.URI("a")
		require.NoError(t, err)

		createOpenWriteFile(t, x, testURI, "abc")

		editBuffer(t, x, testURI, "ABC")

		fsev := testEventInfo{e: schemeapi.Create, u: testURI}
		dispatchFilesystemEvent(x, &mu, ignores, fsev)

		userPromptDiscards(t, x)

		assertBufferContent(t, x, testURI, "abc")
		assertNoPrompt(t, x)
	})

	t.Run("create triggers prompt if open, dirty file changes; user overwrites", func(t *testing.T) {
		var mu sync.Mutex
		ignores := vctrl.NopMatcher(false)

		x := newExForEventTesting(t)
		testURI, err := x.workspace.URI("a")
		require.NoError(t, err)

		createOpenWriteFile(t, x, testURI, "abc")

		editBuffer(t, x, testURI, "ABC")

		fsev := testEventInfo{e: schemeapi.Create, u: testURI}
		dispatchFilesystemEvent(x, &mu, ignores, fsev)

		userPromptOverwrites(t, x)

		assertFileContent(t, x, testURI, "ABC")
		assertBufferContent(t, x, testURI, "ABC")
		assertNoPrompt(t, x)
	})

	t.Run("remove triggers nothing if open, non-dirty file", func(t *testing.T) {
		var mu sync.Mutex
		ignores := vctrl.NopMatcher(false)

		x := newExForEventTesting(t)
		testURI, err := x.workspace.URI("a")
		require.NoError(t, err)

		require.NoError(t, x.editFiles(testURI.String()))

		fsev := testEventInfo{e: schemeapi.Remove, u: testURI}
		dispatchFilesystemEvent(x, &mu, ignores, fsev)

		assertNoPrompt(t, x)
		assertTabNotRemoved(t, x, testURI)
	})

	t.Run("remove triggers prompt if open, dirty file; user discards, removes tab", func(t *testing.T) {
		var mu sync.Mutex
		ignores := vctrl.NopMatcher(false)

		x := newExForEventTesting(t)
		testURI, err := x.workspace.URI("a")
		require.NoError(t, err)

		createOpenRemoveFile(t, x, testURI, "")

		editBuffer(t, x, testURI, "ABC")

		dirty, ok := x.comp.IsDirty(testURI)
		require.True(t, ok)
		require.True(t, dirty)

		fsev := testEventInfo{e: schemeapi.Remove, u: testURI}
		dispatchFilesystemEvent(x, &mu, ignores, fsev)

		userPromptDiscards(t, x)

		assertTabRemoved(t, x, testURI)
		assertNoPrompt(t, x)
	})

	t.Run("remove triggers prompt if open, dirty file; user overwrites", func(t *testing.T) {
		var mu sync.Mutex
		ignores := vctrl.NopMatcher(false)

		x := newExForEventTesting(t)
		testURI, err := x.workspace.URI("a")
		require.NoError(t, err)

		createOpenRemoveFile(t, x, testURI, "")

		editBuffer(t, x, testURI, "ABC")

		fsev := testEventInfo{e: schemeapi.Remove, u: testURI}
		dispatchFilesystemEvent(x, &mu, ignores, fsev)

		userPromptOverwrites(t, x)

		assertTabNotRemoved(t, x, testURI)
		assertBufferContent(t, x, testURI, "ABC")
		assertNoPrompt(t, x)
	})

	t.Run("rename original file triggers remove tab if open, non-dirty", func(t *testing.T) {
		var mu sync.Mutex
		ignores := vctrl.NopMatcher(false)

		x := newExForEventTesting(t)
		testURI, err := x.workspace.URI("a")
		require.NoError(t, err)

		createOpenRemoveFile(t, x, testURI, "")

		fsev := testEventInfo{e: schemeapi.Rename, u: testURI}
		dispatchFilesystemEvent(x, &mu, ignores, fsev)

		assertTabRemoved(t, x, testURI)
		assertNoPrompt(t, x)
	})

	t.Run("rename target file triggers reload tab if open, non-dirty", func(t *testing.T) {
		var mu sync.Mutex
		ignores := vctrl.NopMatcher(false)

		x := newExForEventTesting(t)
		testURI, err := x.workspace.URI("a")
		require.NoError(t, err)

		createOpenWriteFile(t, x, testURI, "abc")

		res, ok := x.comp.Resource(testURI)
		require.True(t, ok)

		flush, err := x.comp.LastFlush(res)
		require.NoError(t, err)

		fsev := testEventInfo{e: schemeapi.Rename, u: testURI}
		dispatchFilesystemEvent(x, &mu, ignores, fsev)

		lastFlush, err := x.comp.LastFlush(res)
		require.NoError(t, err)
		assert.NotEqual(t, flush, lastFlush)

		assertBufferContent(t, x, testURI, "abc")
		assertNoPrompt(t, x)
	})

	t.Run("rename target file opens prompt if open, dirty; user discards triggers reload", func(t *testing.T) {
		var mu sync.Mutex
		ignores := vctrl.NopMatcher(false)

		x := newExForEventTesting(t)
		testURI, err := x.workspace.URI("a")
		require.NoError(t, err)

		createOpenWriteFile(t, x, testURI, "abc")

		editBuffer(t, x, testURI, "ABC")

		fsev := testEventInfo{e: schemeapi.Rename, u: testURI}
		dispatchFilesystemEvent(x, &mu, ignores, fsev)

		userPromptDiscards(t, x)

		assertBufferContent(t, x, testURI, "abc")
		assertNoPrompt(t, x)
	})

	t.Run("rename original file opens prompt if open, dirty; user discards triggers remove tab", func(t *testing.T) {
		var mu sync.Mutex
		ignores := vctrl.NopMatcher(false)

		x := newExForEventTesting(t)
		testURI, err := x.workspace.URI("a")
		require.NoError(t, err)

		createOpenRemoveFile(t, x, testURI, "abc")

		editBuffer(t, x, testURI, "ABC")

		fsev := testEventInfo{e: schemeapi.Rename, u: testURI}
		dispatchFilesystemEvent(x, &mu, ignores, fsev)

		userPromptDiscards(t, x)

		assertTabRemoved(t, x, testURI)
		assertNoPrompt(t, x)
	})

	t.Run("rename target file opens prompt if open, dirty; user overwrites", func(t *testing.T) {
		var mu sync.Mutex
		ignores := vctrl.NopMatcher(false)

		x := newExForEventTesting(t)
		testURI, err := x.workspace.URI("a")
		require.NoError(t, err)

		createOpenWriteFile(t, x, testURI, "abc")

		editBuffer(t, x, testURI, "ABC")

		fsev := testEventInfo{e: schemeapi.Rename, u: testURI}
		dispatchFilesystemEvent(x, &mu, ignores, fsev)

		userPromptOverwrites(t, x)

		assertBufferContent(t, x, testURI, "ABC")
		assertNoPrompt(t, x)
	})
}

func assertFileContent(t *testing.T, x *ex, file workspaceapi.URI, content string) {
	t.Helper()
	data, err := os.ReadFile(file.Path())
	require.NoError(t, err)
	assert.Equal(t, content+"\n", string(data))
}

func assertBufferContent(t *testing.T, x *ex, file workspaceapi.URI, content string) {
	t.Helper()
	ed, err := x.comp.Editor(file)
	require.NoError(t, err)
	cells := ed.CellView().RawCells()
	assert.Equal(t, content, term.CellsToString(cells))
}

func assertTabRemoved(t *testing.T, x *ex, file workspaceapi.URI) {
	t.Helper()
	_, err := x.comp.Editor(file)
	require.Error(t, err, "tab was not removed")
}

func assertTabNotRemoved(t *testing.T, x *ex, file workspaceapi.URI) {
	_, err := x.comp.Editor(file)
	require.NoError(t, err, "tab was removed")
}

func assertNoPrompt(t *testing.T, x *ex) {
	exit, handled := x.Handle(term.Event{Type: term.EventKey, Ch: 'o'})
	assert.False(t, exit)
	require.False(t, handled)
}

func newExForEventTesting(t *testing.T) *ex {
	ctx := context.Background()
	opts := []text.Option{
		text.WithCommandKey(testCommandKey),
	}

	tempDir, err := os.MkdirTemp("", "")
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = os.RemoveAll(tempDir)
	})
	uri, err := workspaceapi.ParseURI(filepath.Join("file://", tempDir))
	require.NoError(t, err)

	fileScheme, err := workspace.NewFileScheme(ctx, config.NopConfig(), uri)
	require.NoError(t, err)

	workspace := workspace.NewSchemeWorkspace(uri, fileScheme)

	e := newExForTestingTerminal(t, workspace, texttest.NopEditor(),
		vte.DefaultConfig(), nopPublishEvent, opts...)

	t.Cleanup(func() {
		require.NoError(t, e.Close())
		require.NoError(t, fileScheme.Close())
	})

	return e.ex
}

type testEventInfo struct {
	e schemeapi.Event
	u workspaceapi.URI
	d bool
}

func (t testEventInfo) Event() schemeapi.Event {
	return t.e
}

func (t testEventInfo) URI() workspaceapi.URI {
	return t.u
}

func (t testEventInfo) IsDir() (bool, error) {
	return t.d, nil
}

// createOpenWriteFile touches a file on disk, opens it for editing (empty) then
// changes the contents on disk to content.
func createOpenWriteFile(t *testing.T, x *ex, file workspaceapi.URI, content string) {
	require.NoError(t, os.WriteFile(file.Path(), []byte(""), 0666))
	require.NoError(t, x.editFiles(file.String()))
	require.NoError(t, os.WriteFile(file.Path(), []byte(content), 0666))
}

// createOpenRemoveFile writes content to a file on disk, then opens it for editing
// (non empty, with content), then removes it from disk.
func createOpenRemoveFile(t *testing.T, x *ex, file workspaceapi.URI, content string) {
	require.NoError(t, os.WriteFile(file.Path(), []byte(content), 0666))
	require.NoError(t, x.editFiles(file.String()))
	require.NoError(t, os.Remove(file.Path()))
}

// openWriteUncreatedFile open an empty, uncreated file for editing, then changes
// the contents on disk to content.
func openWriteUncreatedFile(t *testing.T, x *ex, file workspaceapi.URI, content string) {
	require.NoError(t, x.editFiles(file.String()))
	require.NoError(t, os.WriteFile(file.Path(), []byte("abc"), 0666))
}

func userPromptDiscards(t *testing.T, x *ex) {
	keyCombs := []rune{'d', 'y'}
	for _, key := range keyCombs {
		exit, handled := x.Handle(term.Event{Type: term.EventKey, Ch: key})
		assert.False(t, exit)
		assert.True(t, handled)
	}
}

func userPromptOverwrites(t *testing.T, x *ex) {
	keyCombs := []rune{'o'}
	for _, key := range keyCombs {
		exit, handled := x.Handle(term.Event{Type: term.EventKey, Ch: key})
		assert.False(t, exit)
		assert.True(t, handled)
	}
}

func editBuffer(t *testing.T, x *ex, file workspaceapi.URI, content string) {
	h, err := x.comp.Editor(file)
	require.NoError(t, err)

	ctx := context.Background()
	_, _, _ = h.CellEditor().
		Edit(ctx, term.Coordinates{}, term.Coordinates{}, "ABC")

	dirty, ok := x.comp.IsDirty(file)
	require.True(t, ok)
	require.True(t, dirty)
}
