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

package texttest

import (
	"github.com/unstablebuild/rune-go-sdk/api/textapi"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"github.com/unstablebuild/rune-go-sdk/handler"
	"github.com/unstablebuild/rune-go-sdk/term"
	"unstable.build/go-tui/cell"
	"unstable.build/go-tui/text"
)

var _ text.Handler = (*TestHandler)(nil)

// TestHandler is a handler used to test composite handlers. See
// handler.TestHandler for more details.
type TestHandler struct {
	handler.TestHandler
	URI workspaceapi.URI
}

// NewTestHandler allocates storage for a new TestHandler and initializes it.
func NewTestHandler() (t *TestHandler) {
	t = new(TestHandler)
	t.TestHandler.Ch = 'A'
	return t
}

// Resource satisfies text.Editor.
func (t *TestHandler) Resource() workspaceapi.URI {
	return t.URI
}

// SetWrap satisfies text.Editor.
func (t *TestHandler) SetWrap(wrap bool) {
}

// ShowCommandBar satisfies text.Handler.
func (t *TestHandler) ShowCommandBar(show bool) {
}

// SetCursorAtScroll satisfies text.Handler.
func (t *TestHandler) SetCursorAtScroll(term.Coordinates) bool {
	return false
}

// Close satisfies text.Handler.
func (t *TestHandler) Close() error {
	return nil
}

// LocationLists satisfies text.Handler.
func (h *TestHandler) LocationLists() []text.LocationSet {
	return nil
}

// SeekUp satisfies component.Scrollable.
func (t *TestHandler) SeekUp() bool {
	return false
}

// SeekDown satisfies component.Scrollable.
func (t *TestHandler) SeekDown() bool {
	return false
}

// SeekOffset satisfies component.Scrollable.
func (t *TestHandler) SeekOffset() int {
	return 0
}

// MaxSeekOffset satisfies component.Scrollable.
func (t *TestHandler) MaxSeekOffset() int {
	return 0
}

// SetLocationList satisfies text.Handler.
func (t *TestHandler) SetLocationList(
	pri textapi.LocationPriority, ID string, loc text.LocationList,
) {
}

// MoveToNextLocation satisfies text.Handler.
func (t *TestHandler) MoveToNextLocation(ID string) bool {
	return false
}

// MoveToPrevLocation satisfies text.Handler.
func (t *TestHandler) MoveToPrevLocation(ID string) bool {
	return false
}

// CellView satisfies text.Handler.
func (t *TestHandler) CellView() cell.View {
	return nil
}

// CellEditor satisfies text.Handler.
func (t *TestHandler) CellEditor() cell.Editor {
	return nil
}

// SetDefaultAttributes satisfies text.Handler.
func (t *TestHandler) SetDefaultAttributes(attr term.Attributes) {
}

// CursorAtScroll satisfies text.Handler.
func (t *TestHandler) CursorAtScroll() term.Coordinates {
	return term.Coordinates{}
}
