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

import (
	"time"

	"github.com/unstablebuild/rune-go-sdk/component"
	"github.com/unstablebuild/rune-go-sdk/term"
	"github.com/unstablebuild/tcell/v3"
)

// Config represents the configuration needed to initialize a Handler.
type Config struct {
	// HistoryKey is the key used to trigger scrolling through history.
	HistoryKey       term.KeyComb
	MatchedTextAttr  term.Attributes
	FocusElementAttr term.Attributes
	ElementAttr      term.Attributes
	// DocumentID is the key used to store data in the underlying document.Service.
	DocumentID string
	MaxHistory int

	// Sync makes auto-completion deterministic but very very slow.
	// It should only be used in tests.
	Sync bool

	// ShowManualAfter configures how long to sit idle until
	// command manual is displayed.
	ShowManualAfter time.Duration

	// ManualAttr is used to configure the style of the alternate manual window.
	ManualAttr term.Attributes

	// FrameCharSet is used to determine if a frame is to be used to separate manual from search list.
	FrameCharSet component.FrameCharSet
	// FrameAttr if a frame is to be used to separate manual from search list.
	FrameAttr term.Attributes
}

// DefaultConfig returns a sane configuration for initializing a Handler.
func DefaultConfig() Config {
	return Config{
		MaxHistory:       100,
		HistoryKey:       term.KeyComb{Ch: ':'},
		MatchedTextAttr:  term.Attributes{Fg: tcell.ColorRed},
		FocusElementAttr: term.Attributes{Attrs: tcell.AttrBold | tcell.AttrUnderline, Fg: tcell.ColorRed},
		ElementAttr:      term.Attributes{},
		DocumentID:       "command-history",
		ShowManualAfter:  1 * time.Second,
	}
}
