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

package search

import (
	"time"

	fzf "github.com/junegunn/fzf/src/algo"
	"github.com/unstablebuild/tcell/v3"
	"github.com/unstablebuild/rune-go-sdk/term"
)

const (
	// interrupt periodically but not on every new chunk
	defaultInterruptEvery    = 200 * time.Millisecond
	defaultSetFileCountEvery = 1024 // chunks
)

// AlgoConfig is the algoritum to use to search through the input data.
type AlgoConfig uint

const (
	// FuzzyMatch instructs list to perform approximate string matching.
	FuzzyMatch AlgoConfig = iota
	// EqualMatch instructs list to perform equal string matching.
	EqualMatch
	// ContainsMatch instructs list to match strings that contain search term.
	ContainsMatch
)

// ListConfig is used to initialize a List.
type ListConfig struct {
	// Algorithm to use. See AlgoConfig.
	Algo AlgoConfig

	// function to use to force a redraw of the list.
	Interrupter term.Interrupter

	// Attributes to use to highlight matched text
	MatchedTextAttr *term.Attributes

	// Attributes to use on match and total count row
	CountAttr *term.Attributes

	// FocusElementAttr attributes to use for the element in focus.
	FocusElementAttr *term.Attributes

	// ElementAttr attributes to use for the list elements.
	ElementAttr *term.Attributes

	// CaseSensitive determines whether Algo is case sensitive.
	CaseSensitive bool

	// BottomSearchBar determines whether the search bar and therefore
	// the highest score matches should be at the top or at the bottom.
	BottomSearchBar bool

	interruptEvery    time.Duration
	setFileCountEvery int
}

func (c ListConfig) toInternal() listConfig {
	matchCountAttr := term.Attributes{
		Fg:    tcell.ColorRed,
		Attrs: tcell.AttrBold,
	}
	matchedTextAttr := term.Attributes{
		Fg: tcell.ColorRed,
	}
	searchBaseAttr := term.Attributes{}
	focusAttr := term.Attributes{
		Fg:    tcell.ColorRed,
		Attrs: tcell.AttrBold,
	}
	textAttr := term.Attributes{}
	if c.MatchedTextAttr != nil {
		matchedTextAttr = *c.MatchedTextAttr
	}
	if c.CountAttr != nil {
		matchCountAttr = *c.CountAttr
	}
	if c.FocusElementAttr != nil {
		focusAttr = *c.FocusElementAttr
	}
	if c.ElementAttr != nil {
		textAttr = *c.ElementAttr
	}

	interrupter := term.NopInterrupter()
	if c.Interrupter != nil {
		interrupter = c.Interrupter
	}

	algo := fzf.FuzzyMatchV2
	switch c.Algo {
	case EqualMatch:
		algo = fzf.EqualMatch
	case ContainsMatch:
		algo = containsMatch
	}

	if c.interruptEvery == 0 {
		c.interruptEvery = defaultInterruptEvery
	}

	if c.setFileCountEvery == 0 {
		c.setFileCountEvery = defaultSetFileCountEvery
	}

	return listConfig{
		algo:              algo,
		matchedTextAttr:   matchedTextAttr,
		matchCountAttr:    matchCountAttr,
		searchBaseAttr:    searchBaseAttr,
		textAttr:          textAttr,
		focusAttr:         focusAttr,
		interrupter:       interrupter,
		caseSensitive:     c.CaseSensitive,
		bottomSearchBar:   c.BottomSearchBar,
		interruptEvery:    c.interruptEvery,
		setFileCountEvery: c.setFileCountEvery,
	}
}

// internal representation of ListConfig
type listConfig struct {
	matchedTextAttr   term.Attributes
	matchCountAttr    term.Attributes
	searchBaseAttr    term.Attributes
	textAttr          term.Attributes
	focusAttr         term.Attributes
	algo              fzf.Algo
	interrupter       term.Interrupter
	caseSensitive     bool
	bottomSearchBar   bool
	interruptEvery    time.Duration
	setFileCountEvery int
}
