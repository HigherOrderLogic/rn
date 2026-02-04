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

	"github.com/unstablebuild/rune-go-sdk/api/textapi"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"unstable.build/go-tui/cell"
	"unstable.build/go-tui/component"
	"github.com/unstablebuild/rune-go-sdk/term"
)

// ScrollSubscriber returns a component.ScrollSubscriber which forwarsd Scroll events to evHandler
func ScrollSubscriber(
	resource workspaceapi.URI, h Handler, evHandler EventHandler,
) component.ScrollSubscriber {
	return scrollSubscriber{uri: resource, h: h, eh: evHandler}
}

// CellSubscriber returns a cell.Subscriber which forwards Insert/Delete events to evHandler
func CellSubscriber(
	uri workspaceapi.URI, h Handler, evHandler EventHandler,
) cell.Subscriber {
	return &cellSubscriber{uri: uri, h: h, eh: evHandler}
}

type cellSubscriber struct {
	uri workspaceapi.URI
	h   Handler
	eh  EventHandler

	onWillEditStr   string
	onWillEditStart term.Coordinates
	onWillEditEnd   term.Coordinates
}

func (s *cellSubscriber) OnWillEdit(
	ctx context.Context, start, end term.Coordinates, str string,
) {
	s.onWillEditStr = str
	s.onWillEditStart = start
	s.onWillEditEnd = end
}

func (s *cellSubscriber) OnDidEdit(
	ctx context.Context, from, to term.Coordinates, old string,
) {
	s.eh.Handle(ctx, textapi.Event{
		Type:     textapi.EventTypeEdit,
		Resource: s.h,
		URI:      s.uri,
		From:     from,
		To:       to,
		Start:    s.onWillEditStart,
		End:      s.onWillEditEnd,
		Content:  s.onWillEditStr,
	})
}

type scrollSubscriber struct {
	uri workspaceapi.URI
	h   Handler
	eh  EventHandler
}

func (s scrollSubscriber) OnWillSeek(from term.Coordinates) {
	/* no-op */
}

func (s scrollSubscriber) OnWillHide(start, end int) {
}

func (s scrollSubscriber) OnWillVisible(start int) {
}

func (s scrollSubscriber) OnDidSeek(from, to term.Coordinates) {
	s.eh.Handle(context.Background(), textapi.Event{
		Type:     textapi.EventTypeScroll,
		Resource: s.h,
		URI:      s.uri,
		Start:    to,
		From:     from,
	})
}

func (s scrollSubscriber) OnDidHide(
	start, end int,
) {
	s.eh.Handle(context.Background(), textapi.Event{
		Type:     textapi.EventTypeHidden,
		Resource: s.h,
		URI:      s.uri,
		Start:    term.Coordinates{Y: start},
		End:      term.Coordinates{Y: end},
	})
}

func (s scrollSubscriber) OnDidVisible(
	start int,
) {
	s.eh.Handle(context.Background(), textapi.Event{
		Type:     textapi.EventTypeVisible,
		Resource: s.h,
		URI:      s.uri,
		Start:    term.Coordinates{Y: start},
	})
}
