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

package idetutorial_test

import (
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/unstablebuild/rune-go-sdk/term"
	"github.com/unstablebuild/rune-go-sdk/tui"

	"unstable.build/go-tui/component/shader"
	"unstable.build/go-tui/ide/idetutorial"
)

type fakeRoot struct {
	got       []term.Event
	cursorOn  bool
	cursorAt  term.Coordinates
	selection string
	handled   bool
	exit      bool
}

func (f *fakeRoot) Resize(_, _ int) {}

func (f *fakeRoot) Draw(w term.Writer) {
	w.SetCell(term.Coordinates{X: 0, Y: 0}, term.Cell{Ch: 'R'})
}

func (f *fakeRoot) Handle(ev term.Event) (bool, bool) {
	f.got = append(f.got, ev)
	return f.exit, f.handled
}

func (f *fakeRoot) Cursor() (term.Coordinates, term.CursorStyle, bool) {
	return f.cursorAt, term.CursorStyleDefault, f.cursorOn
}

func (f *fakeRoot) Selection() (string, bool) {
	if f.selection == "" {
		return "", false
	}
	return f.selection, true
}

type fakeTutorial struct {
	handleExit    bool
	handleHandled bool
	handleCount   int
	completed     bool

	shader  idetutorial.Shader
	hasShdr bool

	cursor      bool
	selection   string
	componentAt func(term.Coordinates) (tui.Handler, bool)

	resetCount int
	stopCount  int

	observed []string

	observedEvents []string

	observeExit bool
}

func (t *fakeTutorial) Resize(_, _ int) {}

func (t *fakeTutorial) Draw(w term.Writer) {
	w.SetCell(term.Coordinates{X: 1, Y: 0}, term.Cell{Ch: 'T'})
}

func (t *fakeTutorial) Handle(_ term.Event) (bool, bool) {
	t.handleCount++
	return t.handleExit, t.handleHandled
}

func (t *fakeTutorial) Cursor() (term.Coordinates, term.CursorStyle, bool) {
	return term.Coordinates{X: 9, Y: 9}, term.CursorStyleDefault, t.cursor
}

func (t *fakeTutorial) ComponentAt(pos term.Coordinates) (tui.Handler, bool) {
	if t.componentAt == nil {
		return nil, false
	}
	return t.componentAt(pos)
}

func (t *fakeTutorial) Selection() (string, bool) {
	if t.selection == "" {
		return "", false
	}
	return t.selection, true
}

func (t *fakeTutorial) Reset() { t.resetCount++ }

func (t *fakeTutorial) Stop() { t.stopCount++ }

func (t *fakeTutorial) Completed() bool { return t.completed }

func (t *fakeTutorial) ObserveCommand(
	typed, _ string, _ []string, _ error,
) bool {
	t.observed = append(t.observed, typed)
	return t.observeExit
}

func (t *fakeTutorial) ObserveEvent(eventType, _ string) bool {
	t.observedEvents = append(t.observedEvents, eventType)
	return t.observeExit
}

func (t *fakeTutorial) Shader() (idetutorial.Shader, bool) {
	return t.shader, t.hasShdr
}

func (t *fakeTutorial) SetDefaultAttributes(_ term.Attributes) {}

type trackingShader struct {
	calls atomic.Int64
	tag   term.Attributes
}

func (s *trackingShader) Shade(_, _ int, cells [][]term.Cell) {
	s.calls.Add(1)
	if len(cells) > 0 && len(cells[0]) > 0 {
		cells[0][0].SetAttributes(s.tag)
	}
}

type taggingShader struct {
	tag term.Attributes
}

func (s *taggingShader) Shade(_, _ int, cells [][]term.Cell) {
	for y := range cells {
		for x := range cells[y] {
			if cells[y][x].Ch != 0 {
				cells[y][x].SetAttributes(s.tag)
			}
		}
	}
}

func newSpec(sh shader.Shader) idetutorial.Shader {
	return idetutorial.Shader{
		Shader:   sh,
		Width:    10,
		Height:   3,
		FPS:      30,
		Duration: time.Hour,
	}
}

func TestHandlerDrawOrderRootFirstTutorialOnTop(t *testing.T) {
	t.Parallel()
	root := &fakeRoot{}
	tut := &fakeTutorial{}
	h := idetutorial.New(root, tut, nil, nil)
	h.Resize(10, 3)

	w := term.NewStringWriter(10, 3)
	h.Draw(w)
	_ = w.Flush()

	lines := strings.Split(w.String(), "\n")
	assert.Truef(t, strings.HasPrefix(lines[0], "RT"),
		"want root then overlay (got %q)", lines[0])
}

func TestHandlerHandleTutorialClaimsEvent(t *testing.T) {
	t.Parallel()
	root := &fakeRoot{handled: true}
	tut := &fakeTutorial{handleHandled: true}
	h := idetutorial.New(root, tut, nil, nil)
	h.Resize(10, 3)

	exit, handled := h.Handle(term.Event{Type: term.EventKey, Ch: 'x'})
	assert.False(t, exit)
	assert.True(t, handled)
	assert.Equal(t, 1, tut.handleCount)
	assert.Empty(t, root.got,
		"root must not see events the tutorial reported handled")
}

func TestHandlerCompleted(t *testing.T) {
	t.Parallel()
	tut := &fakeTutorial{completed: true}
	h := idetutorial.New(&fakeRoot{}, tut, nil, nil)

	assert.True(t, h.Completed())
}

func TestHandlerHandleFallsThroughWhenTutorialDeclines(t *testing.T) {
	t.Parallel()
	root := &fakeRoot{handled: true}
	tut := &fakeTutorial{handleHandled: false}
	h := idetutorial.New(root, tut, nil, nil)
	h.Resize(10, 3)

	exit, handled := h.Handle(term.Event{Type: term.EventKey, Ch: 'x'})
	assert.False(t, exit)
	assert.True(t, handled, "fall-through to root should report handled")
	assert.Len(t, root.got, 1)
}

func TestHandlerHandlePropagatesExit(t *testing.T) {
	t.Parallel()

	t.Run("tutorial finish is reported by Finished", func(t *testing.T) {
		t.Parallel()
		root := &fakeRoot{}
		tut := &fakeTutorial{handleExit: true, handleHandled: true}
		h := idetutorial.New(root, tut, nil, nil)
		h.Resize(10, 3)

		exit, handled := h.Handle(term.Event{Type: term.EventKey, Ch: 'q'})
		assert.False(t, exit, "exit is the root's, not the tutorial's")
		assert.True(t, handled)
		assert.True(t, h.Finished())
	})

	t.Run("root exit is propagated", func(t *testing.T) {
		t.Parallel()
		root := &fakeRoot{exit: true, handled: true}
		tut := &fakeTutorial{handleHandled: false}
		h := idetutorial.New(root, tut, nil, nil)
		h.Resize(10, 3)

		exit, handled := h.Handle(term.Event{Type: term.EventKey, Ch: 'q'})
		assert.True(t, exit, "the root's exit must reach the caller")
		assert.True(t, handled)
		assert.False(t, h.Finished())
	})

	t.Run("observers set Finished", func(t *testing.T) {
		t.Parallel()
		h := idetutorial.New(&fakeRoot{}, &fakeTutorial{observeExit: true}, nil, nil)
		assert.True(t, h.ObserveCommand("quit", "quit", nil, nil))
		assert.True(t, h.Finished())

		h = idetutorial.New(&fakeRoot{}, &fakeTutorial{observeExit: true}, nil, nil)
		assert.True(t, h.ObserveEvent("open", "file:///x"))
		assert.True(t, h.Finished())
	})

	t.Run("Reset clears Finished", func(t *testing.T) {
		t.Parallel()
		tut := &fakeTutorial{handleExit: true, handleHandled: true}
		h := idetutorial.New(&fakeRoot{}, tut, nil, nil)
		h.Handle(term.Event{Type: term.EventKey, Ch: 'q'})
		assert.True(t, h.Finished())
		h.Reset()
		assert.False(t, h.Finished())
	})
}

func TestHandlerCursor(t *testing.T) {
	t.Parallel()
	// coversRow1 mimics an overlay box spanning X 0..9 on row Y=1.
	// coversRow1 mimics an overlay box spanning X 0..9 on row Y=1.
	coversRow1 := func(tut *fakeTutorial) func(term.Coordinates) (tui.Handler, bool) {
		return func(pos term.Coordinates) (tui.Handler, bool) {
			if pos.Y == 1 && pos.X < 10 {
				return tut, true
			}
			return nil, false
		}
	}

	t.Run("falls through to root when tutorial is passive", func(t *testing.T) {
		t.Parallel()
		root := &fakeRoot{cursorOn: true, cursorAt: term.Coordinates{X: 3, Y: 1}}
		tut := &fakeTutorial{}
		h := idetutorial.New(root, tut, nil, nil)

		c, _, show := h.Cursor()
		assert.True(t, show)
		assert.Equal(t, term.Coordinates{X: 3, Y: 1}, c)
	})

	t.Run("hides root cursor covered by the overlay", func(t *testing.T) {
		t.Parallel()
		root := &fakeRoot{cursorOn: true, cursorAt: term.Coordinates{X: 3, Y: 1}}
		tut := &fakeTutorial{}
		tut.componentAt = coversRow1(tut)
		h := idetutorial.New(root, tut, nil, nil)

		_, _, show := h.Cursor()
		assert.False(t, show,
			"root cursor must not bleed through an occluding tutorial overlay")
	})

	t.Run("shows root cursor outside the overlay", func(t *testing.T) {
		t.Parallel()
		root := &fakeRoot{cursorOn: true, cursorAt: term.Coordinates{X: 3, Y: 5}}
		tut := &fakeTutorial{}
		tut.componentAt = coversRow1(tut)
		h := idetutorial.New(root, tut, nil, nil)

		c, _, show := h.Cursor()
		assert.True(t, show,
			"root cursor outside the overlay must stay visible")
		assert.Equal(t, term.Coordinates{X: 3, Y: 5}, c)
	})

	t.Run("prefers the tutorial's own cursor", func(t *testing.T) {
		t.Parallel()
		root := &fakeRoot{cursorOn: true, cursorAt: term.Coordinates{X: 3, Y: 1}}
		tut := &fakeTutorial{cursor: true}
		tut.componentAt = coversRow1(tut)
		h := idetutorial.New(root, tut, nil, nil)

		c, _, show := h.Cursor()
		assert.True(t, show)
		assert.Equal(t, term.Coordinates{X: 9, Y: 9}, c)
	})
}

func TestHandlerSelectionAlwaysFromRoot(t *testing.T) {
	t.Parallel()
	root := &fakeRoot{selection: "root-sel"}
	tut := &fakeTutorial{selection: "tut-sel"}
	h := idetutorial.New(root, tut, nil, nil)

	sel, ok := h.Selection()
	assert.True(t, ok)
	assert.Equal(t, "root-sel", sel)
}

func TestHandlerInstallsShaderWhenTutorialDeclaresOne(t *testing.T) {
	t.Parallel()
	root := &fakeRoot{}
	track := &trackingShader{}
	tut := &fakeTutorial{
		hasShdr: true,
		shader:  newSpec(track),
	}
	h := idetutorial.New(root, tut, nil, nil)
	h.Resize(10, 3)
	_, _ = h.Handle(term.Event{Type: term.EventKey, Ch: 'x'})

	w := term.NewStringWriter(10, 3)
	h.Draw(w)
	_ = w.Flush()

	assert.Greater(t, track.calls.Load(), int64(0),
		"tracking shader should have been called during Draw")
}

func TestHandlerTearsDownShaderWhenTutorialStopsDeclaringOne(t *testing.T) {
	t.Parallel()
	root := &fakeRoot{}
	track := &trackingShader{}
	tut := &fakeTutorial{
		hasShdr: true,
		shader:  newSpec(track),
	}
	h := idetutorial.New(root, tut, nil, nil)
	h.Resize(10, 3)
	_, _ = h.Handle(term.Event{Type: term.EventKey, Ch: 'x'})

	w := term.NewStringWriter(10, 3)
	h.Draw(w)
	_ = w.Flush()
	require := assert.New(t)
	require.Greater(track.calls.Load(), int64(0))

	tut.hasShdr = false
	_, _ = h.Handle(term.Event{Type: term.EventKey, Ch: 'x'})

	w2 := term.NewStringWriter(10, 3)
	h.Draw(w2)
	_ = w2.Flush()
	before := track.calls.Load()

	w3 := term.NewStringWriter(10, 3)
	h.Draw(w3)
	_ = w3.Flush()

	require.Equal(before, track.calls.Load(),
		"closed shader.Component must not invoke Shade on further Draws")
}

func TestHandlerSwapsShaderWhenSpecChanges(t *testing.T) {
	t.Parallel()
	root := &fakeRoot{}
	first := &trackingShader{}
	second := &trackingShader{}
	tut := &fakeTutorial{
		hasShdr: true,
		shader:  newSpec(first),
	}
	h := idetutorial.New(root, tut, nil, nil)
	h.Resize(10, 3)
	_, _ = h.Handle(term.Event{Type: term.EventKey, Ch: 'x'})

	h.Draw(term.NewStringWriter(10, 3))
	assert.Greater(t, first.calls.Load(), int64(0))
	firstBefore := first.calls.Load()

	tut.shader = newSpec(second)
	_, _ = h.Handle(term.Event{Type: term.EventKey, Ch: 'x'})

	h.Draw(term.NewStringWriter(10, 3))
	assert.Greater(t, second.calls.Load(), int64(0),
		"new shader should drive frames after swap")

	h.Draw(term.NewStringWriter(10, 3))
	assert.Equal(t, firstBefore, first.calls.Load(),
		"closed shader.Component (old spec) must not see further frames")
}

func TestHandlerResetTearsDownShaderAndForwardsToTutorial(t *testing.T) {
	t.Parallel()
	root := &fakeRoot{}
	track := &trackingShader{}
	tut := &fakeTutorial{
		hasShdr: true,
		shader:  newSpec(track),
	}
	h := idetutorial.New(root, tut, nil, nil)
	h.Resize(10, 3)
	h.Reset()
	h.Draw(term.NewStringWriter(10, 3))
	require := assert.New(t)
	require.Greater(track.calls.Load(), int64(0))
	require.Equal(1, tut.resetCount)

	h.Reset()
	require.Equal(2, tut.resetCount)

	before := track.calls.Load()
	h.Draw(term.NewStringWriter(10, 3))
	require.Greater(track.calls.Load(), before,
		"Reset followed by Draw should install a fresh shader.Component")
}

func TestHandlerObserveCommandForwardsToTutorial(t *testing.T) {
	t.Parallel()
	root := &fakeRoot{}
	tut := &fakeTutorial{}
	h := idetutorial.New(root, tut, nil, nil)

	_ = h.ObserveCommand("e", "edit", []string{"f.go"}, nil)
	assert.Equal(t, []string{"e"}, tut.observed)
}

func TestHandlerObserveEventForwardsToTutorial(t *testing.T) {
	t.Parallel()
	root := &fakeRoot{}
	tut := &fakeTutorial{}
	h := idetutorial.New(root, tut, nil, nil)

	_ = h.ObserveEvent("open", "file:///f.go")
	assert.Equal(t, []string{"open"}, tut.observedEvents)
}

// TestHandlerCloseForwardsStopToTutorial asserts that Handler.Close
// calls Stop on the wrapped tutorial so background goroutines and
// installed prompts are torn down on overlay teardown.
func TestHandlerCloseForwardsStopToTutorial(t *testing.T) {
	t.Parallel()
	root := &fakeRoot{}
	tut := &fakeTutorial{}
	h := idetutorial.New(root, tut, nil, nil)

	require := assert.New(t)
	require.Equal(0, tut.stopCount)
	require.NoError(h.Close())
	require.Equal(1, tut.stopCount)
	require.NoError(h.Close(),
		"Handler.Close must be idempotent")
	require.Equal(2, tut.stopCount,
		"each Close call must forward Stop so repeated teardown is safe")
}

// TestHandlerShaderWrapsRootAndTutorialOverlay asserts that the
// installed shader modulates both root and tutorial-overlay cells.
//
// Regression: shader previously wrapped only the IDE root, so the
// tutorial overlay over-painted cells and hid the effect.
func TestHandlerShaderWrapsRootAndTutorialOverlay(t *testing.T) {
	t.Parallel()
	sentinel := term.Attributes{Attrs: term.AttrBold}
	root := &fakeRoot{}
	tut := &fakeTutorial{
		hasShdr: true,
		shader:  newSpec(&taggingShader{tag: sentinel}),
	}
	h := idetutorial.New(root, tut, nil, nil)
	h.Resize(10, 3)
	_, _ = h.Handle(term.Event{Type: term.EventKey, Ch: 'x'})

	w := term.NewStringWriter(10, 3)
	h.Draw(w)

	cells := w.Cells()
	assert.Equal(t, 'R', cells[0].Ch,
		"root must paint its 'R' marker")
	assert.Equal(t, 'T', cells[1].Ch,
		"tutorial must paint its 'T' marker on top")
	assert.Equal(t, sentinel, cells[0].Attributes(),
		"root cell must carry the shader's sentinel attribute")
	assert.Equal(t, sentinel, cells[1].Attributes(),
		"tutorial-drawn cell must also carry the shader's "+
			"sentinel — the shader must wrap (root + overlay), "+
			"not just root, so a pulse over the floating_window "+
			"prompt actually modulates the overlay's own cells")
}

// TestHandlerShaderUnderOuterShaderComponentReachesWriter asserts that an
// inner tutorial shader's transformation reaches the final writer when
// stacked under an outer shader.Component running shader.Nop.
func TestHandlerShaderUnderOuterShaderComponentReachesWriter(t *testing.T) {
	t.Parallel()
	sentinel := term.Attributes{Attrs: term.AttrBold}
	root := &fakeRoot{}
	tut := &fakeTutorial{
		hasShdr: true,
		shader:  newSpec(&taggingShader{tag: sentinel}),
	}
	h := idetutorial.New(root, tut, nil, nil)
	h.Resize(10, 3)
	_, _ = h.Handle(term.Event{Type: term.EventKey, Ch: 'x'})

	outer := shader.New(h, shader.Nop(), term.NopInterrupter(),
		30, 100*time.Millisecond)
	defer outer.Close()
	outer.Resize(10, 3)

	w := term.NewStringWriter(10, 3)
	outer.Draw(w)

	cells := w.Cells()
	assert.Equal(t, 'R', cells[0].Ch,
		"root marker must reach the writer through the stacked shaders")
	assert.Equal(t, 'T', cells[1].Ch,
		"tutorial marker must reach the writer through the stacked shaders")
	assert.Equal(t, sentinel, cells[0].Attributes(),
		"root cell sentinel must survive the outer Nop shader")
	assert.Equal(t, sentinel, cells[1].Attributes(),
		"tutorial cell sentinel must survive the outer Nop shader")
}
