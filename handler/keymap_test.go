package handler

import (
	"testing"

	"github.com/ernestrc/go-tui"
	"github.com/ernestrc/go-tui/term"
	testutil "github.com/ernestrc/go-tui/util/test"
)

func TestKeyMappedLessHandle(t *testing.T) {
	cases := getLessHandleTestFlow([19]term.Event{
		term.Event{},
		term.Event{Ch: 'k', Type: term.EventKey},
		term.Event{Ch: 'U', Type: term.EventKey},
		term.Event{Ch: '%', Type: term.EventKey},
		term.Event{Ch: 'l', Type: term.EventKey},
		term.Event{Key: term.KeyArrowRight, Type: term.EventKey},
		term.Event{Key: term.KeyArrowLeft, Type: term.EventKey},
		term.Event{Ch: 'G', Type: term.EventKey},
		term.Event{Ch: 'g', Type: term.EventKey},
		term.Event{Ch: '\\', Type: term.EventKey},
		term.Event{Ch: 'X', Type: term.EventKey},
		term.Event{Key: term.KeyBackspace, Type: term.EventKey},
		term.Event{Ch: 'X', Type: term.EventKey},
		term.Event{Ch: 'X', Type: term.EventKey},
		term.Event{Key: term.KeyEnter, Type: term.EventKey},
		term.Event{Ch: 'g', Type: term.EventKey},
		term.Event{Ch: 'N', Type: term.EventKey, Mod: term.ModAlt},
		term.Event{Ch: 'n', Type: term.EventKey, Mod: term.ModAlt},
		term.Event{},
	})
	less1, writer3 := setup(t, nil, 8, 4)
	testutil.TestHandler(t, WithMapping(less1, map[term.KeyComb]term.KeyComb{
		term.KeyComb{Ch: 'k'}:                    term.KeyComb{Ch: 'k'},
		term.KeyComb{Ch: 'U'}:                    term.KeyComb{Ch: 'j'},
		term.KeyComb{Ch: '%'}:                    term.KeyComb{Ch: 'h'},
		term.KeyComb{Ch: '\\'}:                   term.KeyComb{Ch: '/'},
		term.KeyComb{Key: term.KeyArrowRight}:    term.KeyComb{Ch: '$'},
		term.KeyComb{Key: term.KeyArrowLeft}:     term.KeyComb{Ch: '0'},
		term.KeyComb{Key: 'N', Mod: term.ModAlt}: term.KeyComb{Ch: 'N'},
		term.KeyComb{Key: 'n', Mod: term.ModAlt}: term.KeyComb{Ch: 'n'},
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
	handler := &TestHandler{}
	cursor, _ := handler.Cursor()
	kmCursor, _ := WithMapping(handler, nil).Cursor()
	if cursor != kmCursor {
		t.Errorf("did not proxy Cursor correctly")
	}
}
