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

package extutil

import (
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"unstable.build/go-tui/component"
	"github.com/unstablebuild/rune-go-sdk/term"
	"unstable.build/go-tui/text"
)

// TrackedResource holds the state of a resource, replicated
// via ResourceTracker.
type TrackedResource struct {
	cursor *text.Cursor
	uri    workspaceapi.URI

	// Scroll is a mirror of the monitored resource's scroll.
	// Clients MUST NOT use any methods that update
	// the state of this scroll.
	component.Scroll

	// Metadata can be used by users to store data along
	// a TrackedResource.
	Metadata any
}

// URI returns this TrackedResource's resource URI.
func (t *TrackedResource) URI() workspaceapi.URI {
	return t.uri
}

// Cursor returns the cursor coordinates of this TrackedResource.
// Using this requires ResourceTracker to be subscribed
// to EventTypeCursor events.
func (t *TrackedResource) Cursor() term.Coordinates {
	if t.cursor == nil {
		return term.Coordinates{}
	}
	return t.cursor.CursorAtScroll()
}

// WindowCoordinates translates the given content
// position into window coordinates, given this TrackedResource's
// offset (and wraps offset given width, height in wrap mode).
// Using this requires ResourceTracker to be subscribed
// to EventTypeCursor, EventTypeScroll and EventTypeFocus events.
// The second returned value is used to indicate that the given
// position is inside a hidden block (false), or not (true).
func (t *TrackedResource) WindowCoordinates(pos term.Coordinates) (term.Coordinates, bool) {
	if t.Scroll.Width() == 0 && t.Scroll.Wrap {
		return term.CoordinatesDiff(pos, t.Scroll.Offset()), true
	}
	return t.Scroll.ScrollToWindowCoordinates(pos)
}

// ContentCoordinates translates the given window coordinates
// into content coordinates, given this TrackedResource's
// offset (and wraps offset given width, height in wrap mode).
// Using this requires ResourceTracker to be subscribed
// to EventTypeCursor, EventTypeScroll and EventTypeFocus events.
func (t *TrackedResource) ContentCoordinates(pos term.Coordinates) term.Coordinates {
	return t.Scroll.WindowToScrollCoordinates(pos)
}
