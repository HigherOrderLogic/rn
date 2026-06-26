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

// Package idetutorial implements the interactive tutorial overlay used
// by the IDE.
package idetutorial

import (
	"github.com/unstablebuild/rune-go-sdk/term"
	"github.com/unstablebuild/rune-go-sdk/tui"
)

// Tutorial is the in-process state machine driving a single tutorial run.
// Implementations draw an overlay on top of the IDE root via direct
// [term.Writer] writes and return exit=true from Handle when the tutorial
// finishes or is dismissed. Reset zeroes runtime progress so the same
// instance can be dispatched again; host services and parsed content are
// retained.
type Tutorial interface {
	tui.Handler
	Reset()

	// Stop tears down any background work the tutorial owns (the
	// Starlark goroutine, an installed Prompt overlay, etc.) without
	// implying that the tutorial will be re-run. Stop must be safe
	// to call multiple times and on a tutorial that never ran.
	Stop()

	// ObserveCommand reports a dispatched IDE command to the tutorial.
	// typed is the user-typed name (possibly an alias), resolved is the
	// alias-expanded target, args are positional arguments, and err is
	// the dispatch result. When err is non-nil the tutorial state must
	// not advance. Returns exit=true when the tutorial finishes as a
	// result of this observation.
	ObserveCommand(typed, resolved string, args []string, err error) (exit bool)

	// ObserveEvent reports an observed editor event to the tutorial.
	// eventType is the lowercase event-type name (e.g. "open") and uri
	// is the affected document URI. Returns exit=true when the tutorial
	// finishes as a result of this observation.
	ObserveEvent(eventType, uri string) (exit bool)

	// Shader returns the background shader the tutorial wants installed
	// over the IDE root, and ok=true when one is desired. ok=false means
	// no shader should be installed. Returning a different Shader value
	// triggers a tear-down and rebuild of the underlying shader
	// component by value equality.
	Shader() (Shader, bool)

	// SetDefaultAttributes updates the tutorial's view of the current
	// default terminal attributes so per-step shaders read live theme
	// colors. Implementations must restage any active step's shader spec
	// so the next Draw rebuilds it with the new attributes.
	SetDefaultAttributes(defAttr term.Attributes)
}
