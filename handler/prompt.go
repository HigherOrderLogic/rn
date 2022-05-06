package handler

import (
	"github.com/ernestrc/go-tui"
	"github.com/ernestrc/go-tui/component"
	"github.com/ernestrc/go-tui/term"
)

type PromptConfig struct {
	component.PromptConfig

	OptionBindings []term.KeyComb
	OptionCallback func(i int, option string)
	HighlightAttr  term.Attributes
	OptionAttr     term.Attributes
}

type Prompt struct {
	component.Prompt

	hi       int
	cfg      PromptConfig
	bindings map[term.KeyComb]int
}

// NewPrompt allocates storage for a new Prompt and initializes it.
// See Init for more details.
func NewPrompt(cfg PromptConfig) (f *Prompt) {
	f = new(Prompt)
	f.Init(cfg)
	return
}

// Init initializes this Prompt with the given PromptConfig.
// Note that if OptionBindings is defined, it should be of the same
// length as Options.
func (f *Prompt) Init(cfg PromptConfig) {
	f.Prompt.Init(cfg.PromptConfig)
	f.hi = 0 // allow for Init to be used as reset

	if len(cfg.OptionBindings) != 0 &&
		len(cfg.OptionBindings) != len(cfg.Options) {
		panic("invalid OptionBindings; length should match of Options")
	}
	if cfg.OptionCallback == nil {
		cfg.OptionCallback = func(i int, option string) {}
	}

	if cfg.HighlightAttr == (term.Attributes{}) {
		cfg.HighlightAttr = term.Attributes{
			Bg: cfg.OptionAttr.Bg | term.AttrReverse,
			Fg: cfg.OptionAttr.Fg | term.AttrReverse,
		}
	}

	f.cfg = cfg
	f.bindings = make(map[term.KeyComb]int)
	for i, ev := range f.cfg.OptionBindings {
		f.bindings[ev] = i
	}

	// Prompt guarantees that there's at least one option
	f.highlightOption()
}

func (f *Prompt) highlightOption() {
	for j := range f.cfg.Options {
		f.Prompt.SetOptionAttr(j, f.cfg.OptionAttr)
	}
	f.Prompt.SetOptionAttr(f.hi, f.cfg.HighlightAttr)
}

// Handle satisfies tui.Handler.
func (f *Prompt) Handle(ev term.Event) (exit, handled bool) {
	if i, ok := f.bindings[ev.KeyComb()]; ok {
		f.cfg.OptionCallback(i, f.cfg.Options[i])
		exit = true
		handled = true
		return
	}

	switch ev.Key {
	case term.KeyArrowLeft:
		if f.hi > 0 {
			f.hi--
			handled = true
			f.highlightOption()
		}
	case term.KeyArrowRight:
		if f.hi < len(f.cfg.Options)-1 {
			f.hi++
			handled = true
			f.highlightOption()
		}
	case term.KeyEnter:
		f.cfg.OptionCallback(f.hi, f.cfg.Options[f.hi])
		exit = true
		handled = true
	case term.KeyEsc:
		exit = true
		handled = true
	}
	return
}

// Cursor satisfies tui.Handler.
func (f *Prompt) Cursor() (pos term.Coordinates, show bool) {
	return
}

// Man satisfies tui.Component.
func (f *Prompt) Man() tui.Manual {
	panic("TODO")
}
