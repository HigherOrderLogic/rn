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

package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/unstablebuild/rune-go-sdk/component"
	"github.com/unstablebuild/rune-go-sdk/term"

	"unstable.build/go-tui/browser"
	"unstable.build/go-tui/ide/idetutorial/starlarktutorial"
)

// TestBasicsTutorialParses asserts that the embedded basics.star
// tutorial parses through starlarktutorial.New, registers a
// callable entry, and reports the expected id/title/version.
func TestBasicsTutorialParses(t *testing.T) {
	t.Parallel()

	require.NotEmpty(t, basicsTutorial,
		"basicsTutorial embed must not be empty")
	tut, err := starlarktutorial.New(
		"basics", basicsTutorial,
		nil,
		nil,
		nil,
		nil,
		term.Attributes{},
		component.FrameCharSet{},
		browser.PromptConfig{},
		nil,
		nil,
		term.KeyComb{Ch: ':'},
		"modeless",
		nil,
		nil,
	)
	require.NoError(t, err)
	require.NotNil(t, tut)

	assert.Equal(t, "basics", tut.ID())
	assert.Equal(t, "Rune basics", tut.Title())
	assert.Equal(t, "27", tut.Version())
}

// TestBasicsTutorialParsesModalMode asserts the embedded basics
// tutorial also parses under modal editor mode, exercising the
// modal-only branches (e.g. the modal-surfaces step).
func TestBasicsTutorialParsesModalMode(t *testing.T) {
	t.Parallel()

	tut, err := starlarktutorial.New(
		"basics", basicsTutorial,
		nil,
		nil,
		nil,
		nil,
		term.Attributes{},
		component.FrameCharSet{},
		browser.PromptConfig{},
		nil,
		nil,
		term.KeyComb{Ch: ':'},
		"modal",
		nil,
		nil,
	)
	require.NoError(t, err)
	require.NotNil(t, tut)
	assert.Equal(t, "27", tut.Version())
}

// TestEmbeddedTutorialOptionsRegistersBasics asserts that the embedded
// tutorial option list registers at least one tutorial.
func TestEmbeddedTutorialOptionsRegistersBasics(t *testing.T) {
	t.Parallel()
	opts := embeddedTutorialOptions()
	require.NotEmpty(t, opts,
		"embeddedTutorialOptions must register at least basics")
}
