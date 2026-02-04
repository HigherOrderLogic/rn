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

package handler

import (
	"github.com/unstablebuild/rune-go-sdk/tui"
	"github.com/unstablebuild/rune-go-sdk/term"
)

type keyMappingHandler struct {
	tui.Component
	inner    tui.Handler
	mappings map[term.KeyComb]term.KeyComb
}

// WithMapping takes a handler and a set of event mappings to provide
// key and event mapping to override default handler event handler.
func WithMapping(
	inner tui.Handler, mappings map[term.KeyComb]term.KeyComb,
) tui.Handler {
	return keyMappingHandler{inner, inner, mappings}
}

// Handle finds a mapping and overwrites event or delegates the event to
// underlying handler.
func (k keyMappingHandler) Handle(ev term.Event) (bool, bool) {
	if ev.Type != term.EventKey {
		return k.inner.Handle(ev)
	}
	mapped, ok := k.mappings[ev.KeyComb()]
	if ok {
		ev = term.Event{
			Type: term.EventKey,
			Ch:   mapped.Ch,
			Mod:  mapped.Mod,
			Key:  mapped.Key,
			Raw:  ev.Raw,
		}
	}
	return k.inner.Handle(ev)
}

// Cursor delegates call to underlying handler.
func (k keyMappingHandler) Cursor() (term.Coordinates, term.CursorStyle, bool) {
	return k.inner.Cursor()
}

// Selection delegates call to underlying handler.
func (k keyMappingHandler) Selection() (string, bool) {
	return k.inner.Selection()
}
