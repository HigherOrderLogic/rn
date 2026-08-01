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

package idetutorial

import (
	"time"

	"github.com/unstablebuild/rune-go-sdk/term"
	"github.com/unstablebuild/rune-go-sdk/tui"

	"unstable.build/go-tui/browser"
	"unstable.build/go-tui/component/shader"
)

// Shader is a declarative description of the background shader a
// [Tutorial] wants installed over the IDE root. Shader values compare
// by all fields so a change is detected by value equality.
type Shader struct {
	// Shader is the per-frame transform applied to the buffered
	// root cells.
	Shader shader.Shader
	// Offset is the top-left corner of the shaded area. The shader
	// itself is responsible for clipping when needed.
	Offset term.Coordinates
	// Width and Height bound the shaded area. The handler does not
	// crop the writer.
	Width, Height int
	// FPS is the redraw cadence passed to [shader.New].
	FPS int
	// Duration is the maximum lifetime of the shader, passed to
	// [shader.New].
	Duration time.Duration
}

// Handler composes an IDE root [tui.Handler] with a [Tutorial] overlay
// and owns the lifecycle of any [shader.Component] the tutorial
// requests. Draw paints root then tutorial; when a shader is active
// both are drawn through it. Handle dispatches to the tutorial first
// and falls through to the root when unhandled. Selection always comes
// from the root; the root's cursor is hidden while it sits under the
// tutorial's overlay so it does not bleed through.
type Handler struct {
	root        tui.Handler
	tut         Tutorial
	overlay     *OverlayBrowser
	interrupter term.Interrupter

	width, height int

	shaderComp *shader.Component
	shaderSpec Shader
	shaderOn   bool

	finished bool

	composite *rootTutorialComposite
}

// New returns a Handler that wraps root with tut. overlay hosts the
// tutorial's floating windows above root; if nil, a fresh default
// overlay browser is substituted. interrupter is passed to
// [shader.New] when the tutorial requests a background shader; if
// nil, [term.NopInterrupter] is substituted.
func New(
	root tui.Handler, tut Tutorial, overlay *OverlayBrowser,
	interrupter term.Interrupter,
) *Handler {
	if interrupter == nil {
		interrupter = term.NopInterrupter()
	}
	if overlay == nil {
		overlay = NewOverlayBrowser(browser.NewComponent(browser.DefaultConfig()))
	}
	return &Handler{
		root:        root,
		tut:         tut,
		overlay:     overlay,
		interrupter: interrupter,
		composite: &rootTutorialComposite{
			root: root, tut: tut, overlay: overlay,
		},
	}
}

// Resize sets the handler's dimensions and forwards to root and the
// tutorial. When a shader is active the shader.Component's Resize
// recursively resizes its child composite, so the direct Resizes are
// skipped to avoid resizing root and tutorial twice.
func (h *Handler) Resize(width, height int) {
	h.width, h.height = width, height
	if h.shaderComp != nil {
		h.shaderComp.Resize(width, height)
		return
	}
	h.root.Resize(width, height)
	h.tut.Resize(width, height)
	h.overlay.Resize(width, height)
}

// Draw paints the root, the tutorial overlay, and any overlay browser
// windows. When a shader is active all are drawn through the wrapping
// [shader.Component].
func (h *Handler) Draw(w term.Writer) {
	if h.shaderComp != nil {
		h.shaderComp.Draw(w)
		return
	}
	h.root.Draw(w)
	h.tut.Draw(w)
	h.overlay.Draw(w)
}

// Handle routes mouse events over an overlay browser window to the
// browser so window-bar interactions (drag, close, maximize) and
// content scrolling work. Every other event goes to the tutorial
// first; unhandled events fall through to the root. exit is the
// root's exit value, per the [tui.Handler] contract; a tutorial that
// finished as a result of the event is reported by [Handler.Finished]
// instead. handled is true when any layer reported it. After dispatch
// the active background shader is reconciled against [Tutorial.Shader].
func (h *Handler) Handle(ev term.Event) (bool, bool) {
	if _, routed := h.overlay.HandleMouse(ev); routed {
		h.syncShader()
		return false, true
	}
	finished, handled := h.tut.Handle(ev)
	h.finished = h.finished || finished
	var exit bool
	if !handled {
		var rootHandled bool
		exit, rootHandled = h.root.Handle(ev)
		handled = handled || rootHandled
	}
	h.syncShader()
	return exit, handled
}

// Cursor prefers the tutorial's own cursor; the root's cursor is
// hidden when a tutorial overlay component or an overlay browser
// window covers it so it does not bleed through, and shown unchanged
// otherwise.
func (h *Handler) Cursor() (term.Coordinates, term.CursorStyle, bool) {
	if c, style, show := h.tut.Cursor(); show {
		return c, style, true
	}
	c, style, show := h.root.Cursor()
	if show {
		if _, covered := h.tut.ComponentAt(c); covered {
			return term.Coordinates{}, term.CursorStyleDefault, false
		}
		if h.overlay.Covers(c) {
			return term.Coordinates{}, term.CursorStyleDefault, false
		}
	}
	return c, style, show
}

// Selection returns the root's selection.
func (h *Handler) Selection() (string, bool) { return h.root.Selection() }

// ObserveCommand forwards to the underlying tutorial. Returns true
// when the tutorial finished as a result of the observation.
func (h *Handler) ObserveCommand(
	typed, resolved string, args []string, err error,
) bool {
	finished := h.tut.ObserveCommand(typed, resolved, args, err)
	h.finished = h.finished || finished
	return finished
}

// ObserveEvent forwards to the underlying tutorial. Returns true when
// the tutorial finished as a result of the observation.
func (h *Handler) ObserveEvent(eventType, uri string) bool {
	finished := h.tut.ObserveEvent(eventType, uri)
	h.finished = h.finished || finished
	return finished
}

// Reset forwards Reset to the underlying tutorial and reconciles any
// active shader against [Tutorial.Shader].
func (h *Handler) Reset() {
	h.finished = false
	h.tut.Reset()
	h.syncShader()
}

// Finished reports whether the wrapped tutorial ended during a
// previous Handle, ObserveCommand or ObserveEvent call.
func (h *Handler) Finished() bool { return h.finished }

// Completed reports whether the wrapped tutorial's most recent run returned
// normally.
func (h *Handler) Completed() bool { return h.tut.Completed() }

// Close tears down the active shader.Component. Safe to call when no
// shader is installed. Forwards Stop to the wrapped tutorial so any
// background work the tutorial owns is released, then closes any
// overlay browser windows left behind. Always returns nil.
func (h *Handler) Close() error {
	h.clearShader()
	h.tut.Stop()
	h.overlay.CloseAll()
	return nil
}

// SetDefaultAttributes forwards defAttr to the wrapped tutorial and
// reconciles any active shader so it is rebuilt with the new
// attributes on the next Draw.
func (h *Handler) SetDefaultAttributes(defAttr term.Attributes) {
	h.tut.SetDefaultAttributes(defAttr)
	h.syncShader()
}

func (h *Handler) syncShader() {
	spec, want := h.tut.Shader()
	if !want {
		h.clearShader()
		return
	}
	if h.shaderOn && spec == h.shaderSpec {
		return
	}
	h.clearShader()
	h.shaderComp = shader.New(
		h.composite, spec.Shader, h.interrupter,
		spec.FPS, spec.Duration,
	)
	if h.width > 0 && h.height > 0 {
		h.shaderComp.Resize(h.width, h.height)
	}
	h.shaderSpec = spec
	h.shaderOn = true
}

func (h *Handler) clearShader() {
	if h.shaderComp == nil {
		return
	}
	_ = h.shaderComp.Close()
	h.shaderComp = nil
	h.shaderSpec = Shader{}
	h.shaderOn = false
}

type rootTutorialComposite struct {
	root    tui.Handler
	tut     Tutorial
	overlay *OverlayBrowser
}

func (c *rootTutorialComposite) Resize(width, height int) {
	c.root.Resize(width, height)
	c.tut.Resize(width, height)
	c.overlay.Resize(width, height)
}

func (c *rootTutorialComposite) Draw(w term.Writer) {
	c.root.Draw(w)
	c.tut.Draw(w)
	c.overlay.Draw(w)
}
