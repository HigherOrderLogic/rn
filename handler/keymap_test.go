package handler

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"unstable.build/go-tui"
	"unstable.build/go-tui/term"
	testutil "unstable.build/go-tui/util/test"
)

func TestKeyMappedLessHandle(t *testing.T) {
	cases := getLessHandleTestFlow([19]term.Event{
		{},
		{Ch: 'k', Type: term.EventKey},
		{Ch: 'U', Type: term.EventKey},
		{Ch: '%', Type: term.EventKey},
		{Ch: 'l', Type: term.EventKey},
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
		{},
	})
	less1, writer3 := setup(t, nil, 8, 4)
	testutil.TestHandler(t, WithMapping(less1, map[term.KeyComb]term.KeyComb{
		{Ch: 'k'}:                    {Ch: 'k'},
		{Ch: 'U'}:                    {Ch: 'j'},
		{Ch: '%'}:                    {Ch: 'h'},
		{Ch: '\\'}:                   {Ch: '/'},
		{Key: term.KeyArrowRight}:    {Ch: '$'},
		{Key: term.KeyArrowLeft}:     {Ch: '0'},
		{Key: 'N', Mod: term.ModAlt}: {Ch: 'N'},
		{Key: 'n', Mod: term.ModAlt}: {Ch: 'n'},
	}), cases, writer3)
}

func TestKeyMappingMan(t *testing.T) {
	mySummary := "My Summary"
	myID := "myID"
	myDesc := "myDesc"
	kKey := term.KeyComb{Ch: 'k'}
	jKey := term.KeyComb{Ch: 'j'}

	var handler tui.Handler
	handler = &TestHandler{Manual: tui.Manual{
		Summary: mySummary,
		Keys: tui.KeyMap{
			kKey: {
				ID:          myID,
				Description: myDesc,
			},
			jKey: {
				ID:          "",
				Description: "",
			},
		},
	}}

	manualBefore := handler.Man()
	handler = WithMapping(handler, map[term.KeyComb]term.KeyComb{
		kKey: jKey,
	})

	manualAfter := handler.Man()

	if manualBefore.Keys[kKey] != manualAfter.Keys[jKey] ||
		len(manualBefore.Keys) != len(manualAfter.Keys) ||
		len(manualAfter.Keys) != 2 {
		t.Errorf("Manual mapping not correct")
	}
}

func TestKeyMappingCursor(t *testing.T) {
	handler := &TestHandler{CursorStyle: term.CursorStyleBlinkingBlock}
	cursor, style, _ := handler.Cursor()
	kmCursor, kmStyle, _ := WithMapping(handler, nil).Cursor()
	assert.Equal(t, cursor, kmCursor)
	assert.Equal(t, style, kmStyle)
}
