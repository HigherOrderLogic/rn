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

package starlarktutorial

import (
	"fmt"
	"strings"

	"github.com/unstablebuild/rune-go-sdk/component"
	"github.com/unstablebuild/rune-go-sdk/term"

	"unstable.build/go-tui/component/markdown"
	"unstable.build/go-tui/handler/command"
	"unstable.build/go-tui/ide/idetutorial"
)

const floatingWindowBodyPad = 2

// drawBanner paints lines top-left into w within (width, height).
// Out-of-range writes are dropped silently.
func drawBanner(w term.Writer, width, height int, lines []string) {
	if width <= 0 || height <= 0 {
		return
	}
	for y, line := range lines {
		if y >= height {
			return
		}
		for x, r := range line {
			if x >= width {
				break
			}
			w.SetCell(term.Coordinates{X: x, Y: y}, term.Cell{Ch: r})
		}
	}
}

// hintBoxTopOffset is the row offset from the top of the screen
// where the wait_key/wait_command hint window is anchored.
const hintBoxTopOffset = 1

// hintBoxBodyPad mirrors floating_window's body horizontal padding so
// the markdown content has the same breathing room.
const hintBoxBodyPad = 2

// hintBoxWidthRatio sizes the hint window relative to the screen
// (60% of the screen width, matching the floating_window heuristic).
const hintBoxWidthRatio = 6

// commandPromptTopFraction mirrors the command prompt's vertical
// anchor: the IDE opens the prompt at Y = 0.2 * height (see
// (*ex).newCommandPrompt). The wait hint window caps its height to end
// above that row so the prompt the user is asked to open stays visible
// beneath it.
const commandPromptTopFraction = 0.2

// hintBoxMinInnerH keeps the hint window tall enough to show its
// wrapped instruction line even on short screens, where the
// prompt-aware cap (0.2*height) would otherwise squeeze it below
// readability.
const hintBoxMinInnerH = 4

// hintBoxMaxHeightSlack lets the hint window extend a few rows past the
// prompt-aware cap before truncating its body, so longer command
// manuals are not cut off mid-sentence.
const hintBoxMaxHeightSlack = 3

// drawHintBox renders a framed, top-centered hint window whose body
// is the markdown-parsed body string. attrs supplies the frame and
// background attributes; the box is regular foreground/background,
// not inverse video. When title is non-empty a status-bar style title
// row (matching floating_window) replaces the top frame edge. Empty
// body or undersized geometry skips the draw silently.
func drawHintBox(
	w term.Writer, width, height int, title string, stepNum int, body string,
	fcs component.FrameCharSet, attrs term.Attributes,
) {
	if width <= 4 || height <= 4 || body == "" {
		return
	}
	if fcs == (component.FrameCharSet{}) {
		fcs = component.FrameCharSetDefault()
	}
	mdCfg := markdown.DefaultConfig()
	mdCfg.HeaderPrefix = false
	md, err := markdown.NewWithConfig(body, mdCfg)
	if err != nil {
		return
	}
	innerW := (width * hintBoxWidthRatio) / 10
	innerW = max(innerW, 20)
	innerW = min(innerW, width-2)
	if innerW < 8 {
		return
	}
	contentW := innerW - 2
	mdContentW := max(0, contentW-hintBoxBodyPad)
	md.Resize(mdContentW, height)
	contentH := md.Height(mdContentW)
	hasTitle := title != ""
	titleH := 0
	if hasTitle {
		titleH = 1
	}
	innerH := contentH + 2 + titleH
	innerH = max(innerH, 3)
	// Keep the window above the command prompt (opened at
	// 0.2*height) so the user can still see the prompt they are asked
	// to use.
	promptTopY := int(float64(height) * commandPromptTopFraction)
	maxInnerH := promptTopY - hintBoxTopOffset + hintBoxMaxHeightSlack
	maxInnerH = max(maxInnerH, hintBoxMinInnerH)
	innerH = min(innerH, maxInnerH)
	x0 := (width - innerW) / 2
	x0 = max(x0, 0)
	y0 := hintBoxTopOffset
	drawFrameOutline(w, x0, y0, innerW, innerH, fcs, attrs, !hasTitle)
	clearInside(w, x0, y0, innerW, innerH, attrs)
	if hasTitle {
		drawTitleBar(w, x0, y0, innerW, title, stepNum, attrs)
	}
	bodyH := innerH - 2 - titleH
	if bodyH <= 0 || contentW <= 0 {
		return
	}
	bodySpan := component.NewSpan(md, component.SpanConfig{
		PadHorizontal:    hintBoxBodyPad,
		ContentAlignment: component.AlignmentHorizontallyCentered,
	})
	bodySpan.Resize(contentW, bodyH)
	bodySpan.Draw(&component.VirtualWriter{
		Writer: w,
		Offset: term.Coordinates{X: x0 + 1, Y: y0 + 1 + titleH},
		Width:  contentW,
		Height: bodyH,
	})
}

// buildWaitCommandHint composes the markdown body for the
// wait_command hint window. The body always opens with a one-line
// prompt — "Waiting for you to open the command prompt `<cmd>` and
// try the `<command>` command:" — followed by the command's
// markdown-rendered manual when lookup returns one. When the
// runtime has swapped in an on_error message (request.text is non-
// empty), that message wins and is rendered verbatim after the
// prompt opener so the user sees the recovery hint.
func buildWaitCommandHint(
	r *request, cmdKey string, lookup CommandManualLookup,
	keyForCommand func(cmd string, args []string) string,
) string {
	if r == nil {
		return ""
	}
	cmdName := r.command
	var b strings.Builder
	fmt.Fprintf(&b,
		"Waiting for you to open the command prompt `%s` and try the `%s` command:\n\n",
		cmdKey, cmdName)
	if boundKey := waitCommandBoundKey(cmdName, keyForCommand); boundKey != "" {
		fmt.Fprintf(&b,
			"Or you can press `%s` to run it.\n\n", boundKey)
	}
	if r.text != "" {
		// on_error: append the (already expanded) recovery hint
		// verbatim — it is markdown produced by the author and
		// should be rendered as-is.
		b.WriteString(expandCmdTemplate(r.text, cmdKey))
		b.WriteString("\n")
		return b.String()
	}
	if lookup != nil {
		if man, ok := lookup(cmdName); ok {
			b.WriteString(renderCommandManual(man))
			return b.String()
		}
	}
	return b.String()
}

// waitCommandBoundKey resolves the pretty key spec bound to the
// awaited command (the first token of cmd is the command name and the
// rest are arguments). Returns "" when no resolver is wired or the
// command has no binding.
func waitCommandBoundKey(
	cmd string, keyForCommand func(name string, args []string) string,
) string {
	if keyForCommand == nil {
		return ""
	}
	fields := strings.Fields(cmd)
	if len(fields) == 0 {
		return ""
	}
	return keyForCommand(fields[0], fields[1:])
}

// buildWaitShellHint composes the markdown body for a wait_shell hint
// window: it asks the user to run the expected command inside Rune's
// console. A swapped-in on_error message (request.text
// non-empty) wins and is rendered verbatim after the prompt opener.
func buildWaitShellHint(r *request, cmdKey string) string {
	if r == nil {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b,
		"Waiting for you to run `%s` in Rune's console:\n\n",
		strings.Join(r.shellArgs, " "))
	if r.text != "" {
		b.WriteString(expandCmdTemplate(r.text, cmdKey))
		b.WriteString("\n")
	}
	return b.String()
}

// renderCommandManual returns a compact, markdown summary of man
// suitable for embedding in the wait_command hint window. The
// layout intentionally mirrors what handler/command renders in its
// own manual overlay so the user sees the same shape they would in
// the live prompt.
func renderCommandManual(man command.Manual) string {
	var b strings.Builder
	if man.Synopsis != "" {
		fmt.Fprintf(&b, "**Usage:** `%s %s`\n\n", man.Name, man.Synopsis)
	} else {
		fmt.Fprintf(&b, "**Usage:** `%s`\n\n", man.Name)
	}
	if len(man.AliasOf) > 0 {
		if len(man.AliasOf) == 1 {
			fmt.Fprintf(&b, "Alias of `%s`.\n\n", man.AliasOf[0])
		} else {
			b.WriteString("Alias of the following sequence of commands:\n\n")
			for _, c := range man.AliasOf {
				fmt.Fprintf(&b, "- `%s`\n", c)
			}
			b.WriteString("\n")
		}
	}
	if man.Summary != "" {
		b.WriteString(man.Summary)
		b.WriteString("\n")
	}
	if len(man.Commands) > 0 {
		b.WriteString("\n**Subcommands:**\n\n")
		for _, sub := range man.Commands {
			if sub.Summary != "" {
				fmt.Fprintf(&b, "- `%s` — %s\n", sub.Name, sub.Summary)
			} else {
				fmt.Fprintf(&b, "- `%s`\n", sub.Name)
			}
		}
	}
	return b.String()
}

// drawFloatingWindow paints the framed floating_window for r using
// the cached markdown component and body span. Out-of-range writes
// are dropped silently.
func drawFloatingWindow(
	w term.Writer, width, height int, r *request,
	fcs component.FrameCharSet, attrs term.Attributes,
) {
	if r == nil || r.md == nil || r.body == nil {
		return
	}
	layout, ok := floatingWindowLayout(
		width, height, r.title, r.md, fcs, r.align, r.offset)
	if !ok {
		return
	}
	hasTitle := r.title != ""
	drawFrameOutline(w, layout.x, layout.y, layout.innerW, layout.innerH,
		layout.fcs, attrs, !hasTitle)
	clearInside(w, layout.x, layout.y, layout.innerW, layout.innerH, attrs)
	if hasTitle {
		drawTitleBar(w, layout.x, layout.y, layout.innerW,
			r.title, r.stepNum, attrs)
	}
	contentY := layout.y + layout.titleH
	if !hasTitle {
		contentY = layout.y + 1
	}
	contentY += layout.topPad
	if layout.mdH > 0 {
		r.body.Resize(layout.contentW, layout.mdH)
		r.body.Draw(&component.VirtualWriter{
			Writer: w,
			Offset: term.Coordinates{X: layout.x + 1, Y: contentY},
			Width:  layout.contentW,
			Height: layout.mdH,
		})
	}
}

type floatingWindowLayoutResult struct {
	x, y, innerW, innerH int
	contentW, titleH     int
	topPad               int
	mdH                  int
	hintX, hintY, hintW  int
	fcs                  component.FrameCharSet
}

func floatingWindowLayout(
	width, height int, title string, md *markdown.Component,
	fcs component.FrameCharSet,
	alignment component.Alignment, offset term.Coordinates,
) (floatingWindowLayoutResult, bool) {
	var out floatingWindowLayoutResult
	if width <= 4 || height <= 4 || md == nil {
		return out, false
	}
	if fcs == (component.FrameCharSet{}) {
		fcs = component.FrameCharSetDefault()
	}
	innerW := (width * 6) / 10
	innerW = max(innerW, 20)
	innerW = min(innerW, width-2)
	if innerW < 4 {
		return out, false
	}
	contentW := innerW - 2
	mdContentW := max(0, contentW-floatingWindowBodyPad)
	md.Resize(mdContentW, height)
	contentH := md.Height(mdContentW)
	titleH := 0
	hasTitle := title != ""
	if hasTitle {
		// Title bar replaces the top frame edge: it consumes the
		// first row of the inner frame, but does not add a content
		// row above the body.
		titleH = 1
	}
	topEdge := 1
	if hasTitle {
		// No separate top edge when the title bar occupies row 0.
		topEdge = 0
	}
	// A title bar butts directly against the body, so add one blank
	// content row beneath it for breathing room. Untitled windows
	// already get that gap from their top frame edge.
	topPad := 0
	if hasTitle {
		topPad = 1
	}
	innerH := contentH + titleH + topEdge + topPad + 1
	innerH = min(innerH, height-2)
	innerH = max(innerH, 3)
	x, y := floatingWindowAnchor(width, height, innerW, innerH, alignment, offset)
	mdH := innerH - titleH - topEdge - topPad - 1
	mdH = max(mdH, 0)
	hintY := y + innerH - 3
	hintInsetX := floatingWindowBodyPad / 2
	out = floatingWindowLayoutResult{
		x:        x,
		y:        y,
		innerW:   innerW,
		innerH:   innerH,
		contentW: contentW,
		titleH:   titleH,
		topPad:   topPad,
		mdH:      mdH,
		hintX:    x + 1 + hintInsetX,
		hintY:    hintY,
		hintW:    mdContentW,
		fcs:      fcs,
	}
	return out, true
}

// floatingWindowAnchor returns the top-left (x, y) the framed window
// should occupy. Zero alignment maps to centered placement.
func floatingWindowAnchor(
	width, height, innerW, innerH int,
	alignment component.Alignment, offset term.Coordinates,
) (x, y int) {
	if alignment == 0 {
		alignment = component.AlignmentCentered
	}
	switch {
	case alignment&component.AlignmentLeft != 0:
		x = offset.X
	case alignment&component.AlignmentRight != 0:
		x = width - innerW - offset.X
	default:
		x = (width-innerW)/2 + offset.X
	}
	switch {
	case alignment&component.AlignmentTop != 0:
		y = offset.Y
	case alignment&component.AlignmentBottom != 0:
		y = height - innerH - offset.Y
	default:
		y = (height-innerH)/2 + offset.Y
	}
	x = max(x, 0)
	x = min(x, max(0, width-innerW))
	y = max(y, 0)
	y = min(y, max(0, height-innerH))
	return x, y
}

func drawFrame(
	w term.Writer,
	x, y, innerW, innerH int,
	fcs component.FrameCharSet, attrs term.Attributes,
) {
	if innerW < 2 || innerH < 2 {
		return
	}
	drawFrameOutline(w, x, y, innerW, innerH, fcs, attrs, true)
	clearInside(w, x, y, innerW, innerH, attrs)
}

// drawFrameOutline paints the sides and bottom of a framed window
// at (x, y) with the given inner dimensions. When includeTop is
// true it also paints the top edge (horizontal run + corner glyphs);
// callers that overlay a status-bar style title omit the top edge.
func drawFrameOutline(
	w term.Writer,
	x, y, innerW, innerH int,
	fcs component.FrameCharSet, attrs term.Attributes,
	includeTop bool,
) {
	if innerW < 2 || innerH < 2 {
		return
	}
	if includeTop {
		for col := range innerW {
			w.SetCell(term.Coordinates{X: x + col, Y: y},
				term.Cell{Ch: fcs.HorizontalTop, Attributes: attrs})
		}
	}
	for col := range innerW {
		w.SetCell(term.Coordinates{X: x + col, Y: y + innerH - 1},
			term.Cell{Ch: fcs.HorizontalBottom, Attributes: attrs})
	}
	for row := range innerH {
		if row == 0 && !includeTop {
			continue
		}
		w.SetCell(term.Coordinates{X: x, Y: y + row},
			term.Cell{Ch: fcs.VerticalLeft, Attributes: attrs})
		w.SetCell(term.Coordinates{X: x + innerW - 1, Y: y + row},
			term.Cell{Ch: fcs.VerticalRight, Attributes: attrs})
	}
	if includeTop {
		w.SetCell(term.Coordinates{X: x, Y: y},
			term.Cell{Ch: fcs.TopLeft, Attributes: attrs})
		w.SetCell(term.Coordinates{X: x + innerW - 1, Y: y},
			term.Cell{Ch: fcs.TopRight, Attributes: attrs})
	}
	w.SetCell(term.Coordinates{X: x, Y: y + innerH - 1},
		term.Cell{Ch: fcs.BottomLeft, Attributes: attrs})
	w.SetCell(term.Coordinates{X: x + innerW - 1, Y: y + innerH - 1},
		term.Cell{Ch: fcs.BottomRight, Attributes: attrs})
}

// clearInside fills the inner cells of a framed window with spaces
// using attrs. The outermost ring (top/bottom rows and left/right
// columns) is preserved for the frame edges or title bar.
func clearInside(
	w term.Writer,
	x, y, innerW, innerH int,
	attrs term.Attributes,
) {
	if innerW < 2 || innerH < 2 {
		return
	}
	for row := 1; row < innerH-1; row++ {
		for col := 1; col < innerW-1; col++ {
			w.SetCell(term.Coordinates{X: x + col, Y: y + row},
				term.Cell{Ch: ' ', Attributes: attrs})
		}
	}
}

// drawTitleBar paints a single-row status-bar style title across the
// top row of a framed window: the title text left-aligned and "Step N"
// right-aligned, both with the AttrReverse attribute merged into
// baseAttrs. The bar replaces the top frame edge (including the
// corner cells). When the available width cannot fit both zones,
// the right zone is dropped before the left zone is truncated with
// an ellipsis.
func drawTitleBar(
	w term.Writer,
	x, y, innerW int,
	title string, stepNum int,
	baseAttrs term.Attributes,
) {
	if innerW <= 0 {
		return
	}
	barAttrs := baseAttrs
	barAttrs.Attrs |= term.AttrReverse
	for col := range innerW {
		w.SetCell(term.Coordinates{X: x + col, Y: y},
			term.Cell{Ch: ' ', Attributes: barAttrs})
	}
	right := ""
	if stepNum > 0 {
		right = fmt.Sprintf("Step %d", stepNum)
	}
	// Reserve one cell of padding on each side; require at least one
	// space between the two zones. When the title and the right zone
	// cannot both fit, drop the right zone first; only when the title
	// alone still overflows do we truncate it with an ellipsis. This
	// matches the plan's spec: drop right, then truncate left.
	leftPad := 1
	rightPad := 1
	leftRunes := []rune(title)
	rightCells := []rune(right)
	required := leftPad + len(leftRunes) + 1 + len(rightCells) + rightPad
	if required > innerW {
		rightCells = nil
	}
	leftBudget := innerW - leftPad - rightPad
	if len(rightCells) > 0 {
		leftBudget = innerW - leftPad - rightPad - 1 - len(rightCells)
	}
	if leftBudget < 0 {
		leftBudget = 0
	}
	if len(leftRunes) > leftBudget {
		if leftBudget >= 1 {
			leftRunes = append(leftRunes[:leftBudget-1], '…')
		} else {
			leftRunes = nil
		}
	}
	for i, r := range leftRunes {
		w.SetCell(term.Coordinates{X: x + leftPad + i, Y: y},
			term.Cell{Ch: r, Attributes: barAttrs})
	}
	if len(rightCells) > 0 {
		startX := x + innerW - rightPad - len(rightCells)
		for i, r := range rightCells {
			w.SetCell(term.Coordinates{X: startX + i, Y: y},
				term.Cell{Ch: r, Attributes: barAttrs})
		}
	}
}

const (
	// promptMinInnerW and promptMinInnerH are the minimum inner
	// dimensions of the framed choice/confirm prompt overlay. The
	// prompt's ideal Dimensions are still consulted; these floors
	// just keep the overlay from collapsing to a cramped box when
	// the message is short.
	promptMinInnerW = 40
	promptMinInnerH = 7
)

// drawPromptOverlay renders the active reqConfirm/reqChoice prompt
// inside a framed window centered on the screen. The prompt's ideal
// dimensions (Floating.Dimensions) drive the inner size, with a
// minimum floor (promptMinInnerW × promptMinInnerH) so a short
// message does not produce a cramped overlay. The host supplies
// the frame and centered positioning.
func drawPromptOverlay(
	w term.Writer, width, height int, r *request,
	fcs component.FrameCharSet, attrs term.Attributes,
) {
	if r == nil || r.prompt == nil || r.promptVirtual == nil {
		return
	}
	if width <= 4 || height <= 4 {
		return
	}
	if fcs == (component.FrameCharSet{}) {
		fcs = component.FrameCharSetDefault()
	}
	pw, ph := r.prompt.Dimensions()
	innerW := pw + 4
	innerH := ph + 2
	innerW = max(innerW, promptMinInnerW)
	innerH = max(innerH, promptMinInnerH)
	innerW = min(innerW, width-2)
	innerH = min(innerH, height-2)
	innerW = max(innerW, 6)
	innerH = max(innerH, 3)
	x, y := floatingWindowAnchor(width, height, innerW, innerH,
		component.AlignmentCentered, term.Coordinates{})
	drawFrame(w, x, y, innerW, innerH, fcs, attrs)
	contentW := innerW - 4
	contentH := innerH - 2
	if contentW <= 0 || contentH <= 0 {
		return
	}
	r.promptVirtual.Move(term.Coordinates{X: x + 2, Y: y + 1})
	r.promptVirtual.Resize(contentW, contentH)
	r.promptVirtual.Draw(w)
}

// stageFloatingWindowShader rebuilds r.shaderSpec from a fresh
// (width, height) using the floating-window layout. The pulse is not
// armed (hasShader stays false); the TUI loop arms it after a stray
// keystroke.
func stageFloatingWindowShader(
	r *request, width, height int,
	fcs component.FrameCharSet, defAttr term.Attributes,
) {
	layout, ok := floatingWindowLayout(
		width, height, r.title, r.md, fcs, r.align, r.offset)
	if !ok {
		r.shaderSpec = idetutorial.Shader{}
		return
	}
	r.shaderSpec = idetutorial.Shader{
		Shader: buildHintPulse(defAttr,
			layout.hintX, layout.hintY, layout.hintW),
		Offset:   term.Coordinates{X: layout.hintX, Y: layout.hintY},
		Width:    layout.hintW,
		Height:   1,
		FPS:      hintFPS,
		Duration: hintDuration,
	}
}

// expandCmdTemplate replaces every <cmd> token in s with the
// already-prettified command-key display string.
func expandCmdTemplate(s, cmdKey string) string {
	return strings.ReplaceAll(s, "<cmd>", cmdKey)
}

// prettyKeySpecs maps a rendered key spec to a friendlier form for
// tutorial copy. It is display-only and never affects key matching.
var prettyKeySpecs = map[string]string{"<shift-;>": ":"}

// PrettyKeySpec rewrites a rendered key spec to its tutorial-friendly
// form (e.g. "<shift-;>" becomes ":"). Unmapped specs pass through
// unchanged. Used only for display in tutorial copy.
func PrettyKeySpec(s string) string {
	if p, ok := prettyKeySpecs[s]; ok {
		return p
	}
	return s
}
