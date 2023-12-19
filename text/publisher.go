package text

import (
	"context"

	textapi "unstable.build/go-tui/api/text"
	workspaceapi "unstable.build/go-tui/api/workspace"
	"unstable.build/go-tui/cell"
	"unstable.build/go-tui/term"
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

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

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

// Handler returns the underlying handler passed to PublishEdit.
func (p *Publisher) Handler(h Handler) Handler {
	return h.(*cursorPublisher).Handler
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
	return
}

func (p *Publisher) dispatchEvent(ctx context.Context, ev textapi.Event) {
	subs, ok := p.subs[ev.Type]
	if !ok {
		return
	}

	remain := make([]EventHandler, 0, len(subs))
	for _, sub := range subs {
		exit := sub.Handle(ctx, ev)
		if !exit {
			remain = append(remain, sub)
		}
	}
	p.subs[ev.Type] = remain
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
	cursor0, _, _ := handler.Handler.Cursor()
	cursorAtScroll0 := handler.cursor.CursorAtScroll()

	return func() {
		cursor1, _, _ := handler.Handler.Cursor()
		cursorAtScroll1 := handler.cursor.CursorAtScroll()

		if cursor0 != cursor1 || cursorAtScroll0 != cursorAtScroll1 {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			p.dispatchEvent(ctx, textapi.Event{
				Type:     textapi.EventTypeCursor,
				URI:      handler.uri,
				Resource: h,
				Start:    cursor1,
				From:     cursorAtScroll1,
			})
		}
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
