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

package syntax

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/unstablebuild/rune-go-sdk/term"
	"github.com/unstablebuild/tcell/v3"
)

func TestCaptureNameAttributesSupportsMarkupAndTextCaptures(t *testing.T) {
	attrs := DefaultConfig().CaptureNamesAttributes

	assert.Equal(t, tcell.ColorYellow, captureNameAttributes(attrs, "markup.heading").Fg)
	assert.Equal(t, tcell.ColorYellow, captureNameAttributes(attrs, "markup.heading.1").Fg)
	assert.Equal(t, tcell.ColorFuchsia, captureNameAttributes(attrs, "markup.raw.block").Fg)
	assert.Equal(t, tcell.ColorFuchsia, captureNameAttributes(attrs, "markup.link.url").Fg)
	assert.Equal(t, tcell.ColorYellow, captureNameAttributes(attrs, "markup.list.checked").Fg)
	assert.Equal(t, tcell.ColorBlue, captureNameAttributes(attrs, "markup.quote").Fg)
	assert.Equal(t, tcell.ColorYellow, captureNameAttributes(attrs, "text.title").Fg)
	assert.Equal(t, tcell.ColorFuchsia, captureNameAttributes(attrs, "text.uri").Fg)
}

func TestCaptureNameAttributesUserConfigOverridesFallback(t *testing.T) {
	attrs := DefaultConfig().CaptureNamesAttributes
	attrs["markup.heading"] = term.Attributes{Fg: tcell.ColorGreen}
	attrs["markup.heading.2"] = term.Attributes{Fg: tcell.ColorRed}

	assert.Equal(t, tcell.ColorRed, captureNameAttributes(attrs, "markup.heading.2").Fg)
	assert.Equal(t, tcell.ColorGreen, captureNameAttributes(attrs, "markup.heading.3").Fg)
}

func TestCaptureNameAttributesUnknownCaptureReturnsZeroAttributes(t *testing.T) {
	assert.Equal(t, term.Attributes{}, captureNameAttributes(nil, "unknown.capture"))
}
