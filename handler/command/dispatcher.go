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

package command

import "github.com/unstablebuild/rune-go-sdk/component"

// Dispatcher abstracts the ability to dispatch commands.
type Dispatcher interface {
	Dispatch(cmd string, args ...string) bool
	// Preview allows implementations to provide a dynamic substitute
	// for the command manual via the returned tui.Component, and
	// if there are mutable effects, these can be reversed via the returned
	// cancel function. The final boolean is to indicate that either
	// tui.Component or the returned cancel function are non nil.
	Preview(cmd string, args ...string) (component.Responsive, func(), bool)
}

// FuncDispatcher returns a Dispatcher that calls fn every time Dispatch is called.
// It ignores calls to Preview.
func FuncDispatcher(fn func(string, ...string) bool) Dispatcher {
	return fnDispatcher{fn: fn}
}

// FuncDispatcherWithPreview returns a Dispatcher that calls fn every time Dispatch is called.
// It ignores calls to Preview.
func FuncDispatcherWithPreview(
	fn func(string, ...string) bool,
	preview func(string, ...string) (component.Responsive, func(), bool),
) Dispatcher {
	return fnDispatcher{fn: fn, preview: preview}
}

type fnDispatcher struct {
	fn      func(string, ...string) bool
	preview func(string, ...string) (component.Responsive, func(), bool)
}

func (d fnDispatcher) Dispatch(cmd string, args ...string) bool {
	return d.fn(cmd, args...)
}

func (d fnDispatcher) Preview(cmd string, args ...string) (component.Responsive, func(), bool) {
	if d.preview == nil {
		return nil, nil, false
	}
	return d.preview(cmd, args...)
}
