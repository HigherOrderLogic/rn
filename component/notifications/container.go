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

package notifications

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/unstablebuild/tcell/v3"
	"unstable.build/go-tui"
	"unstable.build/go-tui/component"
	"unstable.build/go-tui/term"
)

// Level represents the notification level.
type Level uint8

const (
	// LevelError signals an unexpected error.
	LevelError Level = iota
	// LevelWarn signals an expected error.
	LevelWarn
	// LevelInfo signals an informational message.
	LevelInfo
	// LevelSuccess signals a message of success.
	LevelSuccess
)

// Config configures Container.
type Config struct {
	// AutoClose the minimum time to automatically close a notification.
	// If the message is longer than usual, the time will be increased proportionately.
	AutoClose time.Duration
	// ProgressBar switches an auto-close progress bar on or off.
	ProgressBar bool
	// Width determines the width of each notification in columns.
	Width int
	// Attributes of the notification text.
	Attributes term.Attributes
	// BackgroundAttributes of the notification text.
	BackgroundAttributes term.Attributes
	// FrameCharSet of the notification frame if Frame is enabled.
	// This is optional. If not set, component.FrameCharSetDefault is used.
	FrameCharSet component.FrameCharSet
	// Interrupt is needed to asynchronously update the UI. This is optional.
	Interrupter term.Interrupter

	// ColorInfo determines the style of the progress bar for info notifications.
	// By default, the default color along with a bold foreground is used.
	ColorInfo term.Attributes
	// ColorSuccess determines the style of the progress bar for success notifications.
	// By default, tcell.ColorGreen is used.
	ColorSuccess term.Attributes
	// ColorWarning determines the style of the progress bar for warning notifications.
	// By default, tcell.ColorYellow is used.
	ColorWarning term.Attributes
	// ColorError determines the style of the progress bar for error notifications.
	// By default, tcell.ColorRed is used.
	ColorError term.Attributes
}

// Container renders an inner tui.Component and overlays any notifications that were
// posted via Notify.
type Container struct {
	cfg   Config
	inner tui.Component

	ctx       context.Context
	cancelCtx func()

	mu    sync.RWMutex
	list  component.ResponsiveList
	vlist component.Virtual

	notifications map[string]*notificationTicket
}

// New allocates storage for a new instance of Container and initializes it with
// inner. See Containerfor more details
func New(inner tui.Component, cfg Config) *Container {
	ret := new(Container)
	ret.Init(inner, cfg)
	return ret
}

// Init initializes this Container with inner and cfg. It panics if cfg.Width <= 0.
func (n *Container) Init(inner tui.Component, cfg Config) {
	if cfg.Width <= 0 {
		panic(fmt.Sprintf("notifications.Container with invalid width: %v", cfg.Width))
	}
	if cfg.AutoClose == 0 {
		panic(fmt.Sprintf("notifications.Container with invalid close timeout: %v", cfg.AutoClose))
	}
	if cfg.FrameCharSet == (component.FrameCharSet{}) {
		cfg.FrameCharSet = component.FrameCharSetDefault()
	}
	if cfg.ColorInfo == (term.Attributes{}) {
		cfg.ColorInfo.Attrs = cfg.BackgroundAttributes.Attrs
		cfg.ColorInfo.Attrs |= tcell.AttrBold
	}
	if cfg.ColorSuccess == (term.Attributes{}) {
		cfg.ColorSuccess.Attrs = cfg.BackgroundAttributes.Attrs
		cfg.ColorSuccess.Attrs |= tcell.AttrBold
		cfg.ColorSuccess.Fg = tcell.ColorGreen
	}
	if cfg.ColorWarning == (term.Attributes{}) {
		cfg.ColorWarning.Attrs = cfg.BackgroundAttributes.Attrs
		cfg.ColorWarning.Attrs |= tcell.AttrBold
		cfg.ColorWarning.Fg = tcell.ColorYellow
	}
	if cfg.ColorError == (term.Attributes{}) {
		cfg.ColorError.Attrs = cfg.BackgroundAttributes.Attrs
		cfg.ColorError.Attrs |= tcell.AttrBold
		cfg.ColorError.Fg = tcell.ColorRed
	}
	n.inner = inner
	n.cfg = cfg
	n.list.Init()
	n.vlist.C = &n.list
	n.ctx, n.cancelCtx = context.WithCancel(context.Background())
	n.notifications = make(map[string]*notificationTicket)
}

// Draw satisfies tui.Component.
func (n *Container) Draw(w term.Writer) {
	n.inner.Draw(w)

	n.mu.RLock()
	defer n.mu.RUnlock()

	n.vlist.Draw(w)
}

// Resize satisfies tui.Component.
func (n *Container) Resize(width, height int) {
	n.inner.Resize(width, height)

	n.mu.Lock()
	defer n.mu.Unlock()

	effectiveWidth := n.cfg.Width
	offset := width - n.cfg.Width
	if offset < 0 {
		offset = 0
		effectiveWidth = width
	}

	n.vlist.Move(term.Coordinates{X: offset, Y: 0})
	n.vlist.Resize(effectiveWidth, height)
}

// Notify creates a new notification with the given msg and level.
func (n *Container) Notify(level Level, msg string) {
	const baselineChars = len("this is a simple notification.")
	duration := n.cfg.AutoClose
	if len(msg) > baselineChars {
		duration *= time.Duration(float64(len(msg)) / float64(baselineChars))
		duration = time.Duration(math.Min(float64(2*n.cfg.AutoClose), float64(duration)))
		duration = time.Duration(math.Max(float64(n.cfg.AutoClose), float64(duration)))
	}

	ctx, cancel := context.WithTimeout(n.ctx, duration)
	comp := newNotification(level, msg, n.cfg, duration, cancel)

	n.mu.Lock()
	defer n.mu.Unlock()

	// clear out current notification, based on message equality, if it exists
	if ticket, exists := n.notifications[msg]; exists {
		ticket.cancelCtx()
		n.list.Remove(&ticket.el)
		delete(n.notifications, msg)
	}

	// add a new notification
	el := n.list.PushFront(comp)
	n.notifications[msg] = &notificationTicket{el: el, cancelCtx: cancel}

	n.startAutoClose(ctx, cancel, el, msg)
}

// CloseAll closes all open notifications.
func (n *Container) CloseAll() {
	n.mu.Lock()
	defer n.mu.Unlock()

	// clear out current notification, based on message equality, if it exists
	for _, ticket := range n.notifications {
		n.closeNotification(ticket.el)
	}
	n.notifications = make(map[string]*notificationTicket)
}

// PauseAll pauses auto-close progress on all open notifications.
func (n *Container) PauseAll() {
	n.mu.Lock()
	defer n.mu.Unlock()
	// clear out current notification, based on message equality, if it exists
	for _, ticket := range n.notifications {
		n.pauseNotification(ticket.el)
	}
}

// ResumeAll resumes auto-close progress on all open notifications.
func (n *Container) ResumeAll() {
	n.mu.Lock()
	defer n.mu.Unlock()

	n.resumeAll()
}
func (n *Container) resumeAll() {
	for msg, ticket := range n.notifications {
		n.resumeNotification(ticket, msg)
	}
}

// Handle handles the given term.Event. It ignores anything
// other than mouse events. It handles mouse events by pausing
// notifications on hover.
func (n *Container) Handle(ev term.Event) (exit, handled bool) {
	if ev.Type != term.EventMouse || ev.Mod != 0 {
		return
	}

	pos := n.vlist.Position()
	height := n.vlist.Height()
	width := n.vlist.Width()
	if ev.MouseX < pos.X || ev.MouseY < pos.Y ||
		ev.MouseX >= pos.X+width || ev.MouseY >= pos.Y+height {
		n.ResumeAll() // hover outside should resume all
		return
	}

	n.mu.Lock()
	defer n.mu.Unlock()

	el, ok := n.list.ElementAt(term.Coordinates{X: ev.MouseX - pos.X, Y: ev.MouseY - pos.Y})
	if !ok {
		n.resumeAll()
		return
	}

	switch ev.Key {
	case term.MouseLeft:
		n.closeNotification(el)
	default:
		n.pauseNotification(el)
	}

	handled = true
	return
}

// Close cancels all pending notifications and closes this
// notification container for good. Future calls to Notify will
// not produce new notifications.
func (n *Container) Close() error {
	n.cancelCtx()
	return nil
}

func (n *Container) closeNotification(el component.ListNode) {
	el.Value().(*notificationComp).cancel()
	n.list.Remove(&el)
}

func (n *Container) pauseNotification(el component.ListNode) {
	comp := el.Value().(*notificationComp)
	if comp.pausedAt != (time.Time{}) {
		return // resume is idempotent
	}
	// this prevents element from being removed
	// and cancels fps interrupt goroutine.
	comp.pausedAt = time.Now()
	comp.cancel()

}

func (n *Container) resumeNotification(t *notificationTicket, msg string) {
	comp := t.el.Value().(*notificationComp)
	if comp.pausedAt == (time.Time{}) {
		return // resume is idempotent
	}
	remaining := comp.end.Sub(comp.pausedAt)
	newEnd := time.Now().Add(remaining)

	ctx, cancelCtx := context.WithTimeout(n.ctx, remaining)

	comp.end = newEnd
	comp.pausedAt = time.Time{}
	comp.cancel = cancelCtx

	n.startAutoClose(ctx, cancelCtx, t.el, msg)
}

func (n *Container) startAutoClose(
	ctx context.Context, cancel func(),
	el component.ListNode, msg string,
) {
	go func() {
		defer cancel()

		<-ctx.Done()
		if errors.Is(ctx.Err(), context.Canceled) {
			return
		}

		n.mu.Lock()
		// no need to ensure that while we were trying to acquire a lock, no one
		// removed it already as Remove is itempotent. See Go std's list.List.
		n.list.Remove(&el)
		delete(n.notifications, msg)
		n.mu.Unlock()

		if n.cfg.Interrupter != nil {
			_ = n.cfg.Interrupter.Interrupt(ctx)
		}
	}()

	if !n.cfg.ProgressBar {
		return
	}

	if n.cfg.Interrupter != nil {
		go term.InterruptAt(ctx, n.cfg.Interrupter, 10)
	}
}

type notificationTicket struct {
	cancelCtx func()
	el        component.ListNode
}
