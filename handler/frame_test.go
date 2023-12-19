package handler

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"unstable.build/go-tui"
	"unstable.build/go-tui/term"
)

func TestFrameProxyMan(t *testing.T) {
	myManual := tui.Manual{
		Summary: "sup",
		Keys: tui.KeyMap{
			term.KeyComb{Ch: 'j'}: {
				ID:          "wow",
				Description: "now",
			},
		},
	}
	handler := &TestHandler{Manual: myManual}
	if !reflect.DeepEqual(handler.Man(), NewFrame(handler).Man()) {
		t.Errorf("did not proxy Man correctly")
	}
}

func TestFrameProxyCursor(t *testing.T) {
	handler := &TestHandler{
		CursorPos:   term.Coordinates{X: 1},
		CursorStyle: term.CursorStyleBlinkingBar,
	}
	proxy := NewFrame(handler)
	proxy.Resize(4, 4)
	offsetCursor, style, ok := handler.Cursor()
	require.True(t, ok)
	offsetCursor.X++
	offsetCursor.Y++

	newCursor, style, ok := proxy.Cursor()
	require.True(t, ok)
	assert.Equal(t, offsetCursor, newCursor)
	assert.Equal(t, term.CursorStyleBlinkingBar, style)
}
