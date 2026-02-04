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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/unstablebuild/rune-go-sdk/handler"
	"github.com/unstablebuild/rune-go-sdk/term"
	"unstable.build/go-tui/handler/handlertest"
)

func TestKeyMappedLessHandle(t *testing.T) {
	cases := getLessHandleTestFlow([26]term.Event{
		{},
		{Ch: 'k', Type: term.EventKey},
		{Ch: 'U', Type: term.EventKey},
		{Ch: 'v', Type: term.EventKey},
		{Ch: '%', Type: term.EventKey},
		{Key: term.KeyArrowRight, Type: term.EventKey},
		{Key: term.KeyArrowLeft, Type: term.EventKey},
		{Ch: 'G', Type: term.EventKey},
		{Ch: 'g', Type: term.EventKey},
		{Ch: '\\', Type: term.EventKey},
		{Ch: 'X', Type: term.EventKey},
		{Key: term.KeyBackspace, Type: term.EventKey},
		{Ch: 'X', Type: term.EventKey},
		{Ch: 'X', Type: term.EventKey},
		{Key: term.KeyEnter, Type: term.EventKey},
		{Ch: 'g', Type: term.EventKey},
		{Ch: 'N', Type: term.EventKey, Mod: term.ModAlt},
		{Ch: 'n', Type: term.EventKey, Mod: term.ModAlt},
		{Ch: 'G', Type: term.EventKey},
		{Ch: '_', Type: term.EventKey},
		{Ch: 'X', Type: term.EventKey},
		{Ch: 'X', Type: term.EventKey},
		{Key: term.KeyEnter, Type: term.EventKey},
		{Ch: 'n', Type: term.EventKey, Mod: term.ModAlt},
		{Ch: 'n', Type: term.EventKey, Mod: term.ModAlt},
		{Ch: 'N', Type: term.EventKey, Mod: term.ModAlt},
	})
	less1, writer3 := setup(t, nil, 8, 4)
	handlertest.TestHandler(t, WithMapping(less1, map[term.KeyComb]term.KeyComb{
		{Ch: 'k'}:                   {Ch: 'k'},
		{Ch: 'U'}:                   {Ch: 'j'},
		{Ch: '%'}:                   {Ch: 'h'},
		{Ch: 'v'}:                   {Ch: 'l'},
		{Ch: '\\'}:                  {Ch: '/'},
		{Ch: '_'}:                   {Ch: '?'},
		{Key: term.KeyArrowRight}:   {Ch: '$'},
		{Key: term.KeyArrowLeft}:    {Ch: '0'},
		{Ch: 'N', Mod: term.ModAlt}: {Ch: 'N'},
		{Ch: 'n', Mod: term.ModAlt}: {Ch: 'n'},
	}), cases, writer3)
}

func TestKeyMappingCursor(t *testing.T) {
	handler := &handler.TestHandler{CursorStyle: term.CursorStyleBlinkingBlock}
	cursor, style, _ := handler.Cursor()
	kmCursor, kmStyle, _ := WithMapping(handler, nil).Cursor()
	assert.Equal(t, cursor, kmCursor)
	assert.Equal(t, style, kmStyle)
}
