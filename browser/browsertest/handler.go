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

package browsertest

import (
	"github.com/unstablebuild/rune-go-sdk/handler"
	"unstable.build/go-tui/browser"
)

// TestHandler is a testing Handler.
type TestHandler struct {
	handler.TestHandler
	CloseCallback func() error
}

// NewTestHandler allocates storage for a new TestHandler and initializes it.
func NewTestHandler() *TestHandler {
	ret := new(TestHandler)
	ret.TestHandler = *handler.NewTestHandler()
	return ret
}

// Close calls t.Close.
func (t *TestHandler) Close() error {
	if t.CloseCallback != nil {
		return t.CloseCallback()
	}
	return nil
}

var _ browser.Floating = (*TestFloating)(nil)

// TestFloating is a testing Handler.
type TestFloating struct {
	handler.TestFloating
}

// NewTestFloating allocates storage for a new TestHandler and initializes it.
func NewTestFloating(width, height int) *TestFloating {
	ret := new(TestFloating)
	ret.TestFloating = *handler.NewTestFloating(width, height)
	return ret
}

// Close satisfies browser.Floating.
func (t *TestFloating) Close() error {
	return nil
}
