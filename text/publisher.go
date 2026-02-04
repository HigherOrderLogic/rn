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

package text

import (
	"context"

	log "github.com/sirupsen/logrus"
	"github.com/unstablebuild/blue/logging"
	"github.com/unstablebuild/rune-go-sdk/api/textapi"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"unstable.build/go-tui/cell"
	"github.com/unstablebuild/rune-go-sdk/term"
)

// Publisher implements pub/sub functionality for Editor implementations.
type Publisher struct {
	subs map[textapi.EventType][]EventHandler
}

type cursorPublisher struct {
	parent *Publisher
	uri    workspaceapi.URI
	buf    *cell.Buffer
	cursor *Cursor
	Handler
}

// NewPublisher allocates storage for a new Publisher and initializes it.
func NewPublisher() *Publisher {
	ret := new(Publisher)
	ret.Init()
	return ret
}

// Init initializes this Publisher.
func (p *Publisher) Init() {
	p.subs = make(map[textapi.EventType][]EventHandler)
}

// PublishEdit publishes EventTypeOpen and EventTypeFocus events to subscribers
// and wraps root with a Handler that dispatches EventTypeCursor events.
// It also subscribes to scroll changes to dispatch EventTypeScroll, and
// subscribes to buffer updates to dispatch EventTypeEdit.
func (p *Publisher) PublishEdit(
	resource workspaceapi.URI, buf *cell.Buffer, root Handler, cursor *Cursor,
) Handler {
	h := &cursorPublisher{
		buf:     buf,
		parent:  p,
		uri:     resource,
		cursor:  cursor,
		Handler: root,
	}

	ctx := context.Background()

	p.dispatchEvent(ctx, textapi.Event{
		Type:     textapi.EventTypeOpen,
		URI:      resource,
		Resource: h,
		Content:  buf.String(),
	})

	// if simple is the final tui.Handler, then the underlying handler
	// is always in focus. Consumers of this Editor should not
	// delegate SubscribeEditor to this handler if there's some other
	// focus mechanism in place.
	p.dispatchEvent(ctx, textapi.Event{
		Type:     textapi.EventTypeFocus,
		URI:      resource,
		Resource: h,
	})

	bsub := CellSubscriber(resource, h, p)

	// NOTE: should probably find a way to unsubscribe, since
	// a cell.Buffer can outlive this Publisher
	buf.Subscribe(bsub)

	csub := ScrollSubscriber(resource, h, p)
	cursor.SubscribeScroll(csub)

	return h
}

// SubscribeEvents subsribes sub to ev.
func (p *Publisher) SubscribeEvents(evs []textapi.EventType, sub EventHandler) {
	for _, ev := range evs {
		if _, ok := p.subs[ev]; !ok {
			p.subs[ev] = []EventHandler{sub}
		} else {
			p.subs[ev] = append(p.subs[ev], sub)
		}
	}
	p.log(log.TraceLevel, "subscribe subscriber sub=%p to evs %+v: "+
		"subscribers=%+v", sub, evs, p.subs)
}

// UnsubscribeEvents unsubscribes sub from all events.
func (p *Publisher) UnsubscribeEvents(sub EventHandler) (ret bool) {
	final := make(map[textapi.EventType][]EventHandler)
	for ev, subs := range p.subs {
		final[ev] = make([]EventHandler, 0, len(subs))
		for _, s := range subs {
			if s != sub {
				final[ev] = append(final[ev], s)
			} else {
				ret = true
			}
		}
	}
	p.subs = final
	p.log(log.TraceLevel, "unsubscribing subscriber sub=%p: "+
		"unsubscribed called. subscribers=%+v", sub, p.subs)
	return
}

func (p *Publisher) dispatchEvent(ctx context.Context, ev textapi.Event) {
	subs, ok := p.subs[ev.Type]
	if !ok {
		return
	}

	// call Handle in batch, as it might be an rpc, and removing from
	// edSubsribers after each call could introduce race conditions.
	exits := make([]bool, len(subs))
	for i, h := range subs {
		exits[i] = h.Handle(ctx, ev)
		p.log(log.TraceLevel, "dispatched to subscriber sub=%p, "+
			"event type %d: exit=%t", h, ev.Type, exits[i])
	}

	p.subs[ev.Type] = make([]EventHandler, 0, len(subs))
	for i, sub := range subs {
		if !exits[i] {
			p.subs[ev.Type] = append(p.subs[ev.Type], sub)
		}
	}
}

// Handle handles ev by dispatching to subscribers.
func (p *Publisher) Handle(ctx context.Context, ev textapi.Event) bool {
	p.dispatchEvent(ctx, ev)
	return false
}

// RecordCursorChange records a cursor change between this function call and
// dispatchEvent being called. If there is a cursor position change,
// then an EventTypeCursor is dispatched to subscribers.
func (p *Publisher) RecordCursorChange(h Handler) (dispatchEvent func()) {
	handler := h.(*cursorPublisher)
	cursor0 := handler.cursor.Coordinates()
	cursorAtScroll0 := handler.cursor.CursorAtScroll()
	selection0 := handler.cursor.Selection()

	return func() {
		cursor1 := handler.cursor.Coordinates()
		cursorAtScroll1 := handler.cursor.CursorAtScroll()
		selection1 := handler.cursor.Selection()

		if cursor0 != cursor1 || cursorAtScroll0 != cursorAtScroll1 {
			p.dispatchEvent(context.Background(), textapi.Event{
				Type:     textapi.EventTypeCursor,
				URI:      handler.uri,
				Resource: h,
				Start:    cursor1,
				From:     cursorAtScroll1,
			})
		}

		selectionFrom, _ := handler.cursor.SelectionFrom()
		if selection0 != selection1 {
			// select coordinates are right exclusive, but cursor is not
			cursorAtScroll1.X++
			p.dispatchEvent(context.Background(), textapi.Event{
				Type:     textapi.EventTypeSelection,
				URI:      handler.uri,
				Resource: h,
				Start:    selectionFrom,
				End:      cursorAtScroll1,
				Content:  selection1,
			})
		}

		p.log(log.TraceLevel, "record cursor change for %p: cursor before: %+v, "+
			"cursor after: %+v, cursorAtScroll before: %+v, cursorAtScroll after: %+v, "+
			"selection before: %s, selection after: %s",
			h, cursor0, cursor1, cursorAtScroll0, cursorAtScroll1,
			selection0, selection1)
	}
}

// Handle dispatchs cursor events if cursor has changed after
// underlying handler has processed ev.
func (p *cursorPublisher) Handle(ev term.Event) (bool, bool) {
	dispatch := p.parent.RecordCursorChange(p)
	defer dispatch()

	return p.Handler.Handle(ev)
}

// helper for internal tests
func (p *cursorPublisher) CursorReference() *Cursor {
	return p.cursor
}

func (p *cursorPublisher) SetCursorAtScroll(pos term.Coordinates) bool {
	dispatch := p.parent.RecordCursorChange(p)
	defer dispatch()

	return p.Handler.SetCursorAtScroll(pos)
}

func (p *cursorPublisher) MoveToNextLocation(ID string) bool {
	dispatch := p.parent.RecordCursorChange(p)
	defer dispatch()

	return p.Handler.MoveToNextLocation(ID)
}

func (p *cursorPublisher) MoveToPrevLocation(ID string) bool {
	dispatch := p.parent.RecordCursorChange(p)
	defer dispatch()

	return p.Handler.MoveToPrevLocation(ID)
}

func (p *Publisher) log(level log.Level, msg string, args ...any) {
	if !log.IsLevelEnabled(level) {
		return
	}
	log.WithFields(log.Fields{
		logging.KeyClass: "text.Publisher",
	}).Logf(level, msg, args...)
}
