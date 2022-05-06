package handler

import (
	"testing"

	"github.com/ernestrc/go-tui/component"
	"github.com/ernestrc/go-tui/term"
	"github.com/stretchr/testify/assert"
)

func TestPromptDefaults(t *testing.T) {
	t.Run("sets default callback", func(t *testing.T) {
		p := NewPrompt(PromptConfig{
			PromptConfig: component.PromptConfig{
				Message: "blah", Options: []string{"a"},
			}})
		exit, handled := p.Handle(term.Event{Key: term.KeyEnter})
		assert.True(t, exit)
		assert.True(t, handled)
	})
	t.Run("sets default highlight attrs", func(t *testing.T) {
		p := NewPrompt(PromptConfig{
			PromptConfig: component.PromptConfig{
				Message: "blah", Options: []string{"a"},
			}})
		attr := term.Attributes{Fg: term.AttrReverse, Bg: term.AttrReverse}
		assert.Equal(t, attr, p.cfg.HighlightAttr)
	})
}

func TestPromptHandle(t *testing.T) {
	opt0 := "Say what?"
	opt1 := "Yes"

	var calledI int
	var calledOpt string
	cfg := PromptConfig{
		PromptConfig: component.PromptConfig{
			Message: "What's for supper?",
			Options: []string{opt0, opt1},
			Frame:   component.FrameCharSetDefault(),
		},
		OptionCallback: func(i int, opt string) {
			calledI = i
			calledOpt = opt
		},
		OptionBindings: []term.KeyComb{
			{Ch: 'W'},
			{Ch: 'Y'},
		},
	}

	h := NewPrompt(cfg)

	resetStub := func() {
		calledI = -1
		calledOpt = "-1"
		h.Init(cfg)
	}
	resetStub()

	t.Run("enter after init calls first option", func(t *testing.T) {
		defer resetStub()

		exit, handled := h.Handle(term.Event{Key: term.KeyEnter})
		assert.True(t, exit)
		assert.True(t, handled)

		assert.Equal(t, 0, calledI)
		assert.Equal(t, opt0, calledOpt)
	})

	t.Run("arrow key right allow for moving right", func(t *testing.T) {
		defer resetStub()

		exit, handled := h.Handle(term.Event{Key: term.KeyArrowRight})
		assert.False(t, exit)
		assert.True(t, handled)

		exit, handled = h.Handle(term.Event{Key: term.KeyArrowRight})
		assert.False(t, exit)
		assert.False(t, handled)

		exit, handled = h.Handle(term.Event{Key: term.KeyEnter})
		assert.True(t, exit)
		assert.True(t, handled)

		assert.Equal(t, 1, calledI)
		assert.Equal(t, opt1, calledOpt)
	})

	t.Run("arrow key left allow for moving left", func(t *testing.T) {
		defer resetStub()

		exit, handled := h.Handle(term.Event{Key: term.KeyArrowRight})
		assert.False(t, exit)
		assert.True(t, handled)

		exit, handled = h.Handle(term.Event{Key: term.KeyArrowLeft})
		assert.False(t, exit)
		assert.True(t, handled)

		exit, handled = h.Handle(term.Event{Key: term.KeyArrowLeft})
		assert.False(t, exit)
		assert.False(t, handled)

		exit, handled = h.Handle(term.Event{Key: term.KeyEnter})
		assert.True(t, exit)
		assert.True(t, handled)

		assert.Equal(t, 0, calledI)
		assert.Equal(t, opt0, calledOpt)
	})

	t.Run("valid auto key binding no conflict", func(t *testing.T) {
		defer resetStub()

		exit, handled := h.Handle(term.Event{Ch: 'Y'})
		assert.True(t, exit)
		assert.True(t, handled)

		assert.Equal(t, 1, calledI)
		assert.Equal(t, opt1, calledOpt)
	})

	t.Run("invalid key binding", func(t *testing.T) {
		defer resetStub()

		exit, handled := h.Handle(term.Event{Ch: ' '})
		assert.False(t, exit)
		assert.False(t, handled)

		assert.Equal(t, -1, calledI)
		assert.Equal(t, "-1", calledOpt)
	})
}
