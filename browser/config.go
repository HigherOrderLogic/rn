package browser

import (
	"github.com/ernestrc/go-tui/component"
	"github.com/ernestrc/go-tui/handler"
	"github.com/ernestrc/go-tui/term"
	log "github.com/sirupsen/logrus"
)

// DefaultConfig returns the default Config.
func DefaultConfig() Config {
	return Config{
		Logger:              nil,
		MessageBarAttr:      term.Attributes{Bg: term.ColorRed, Fg: term.ColorWhite},
		FocusTabAttr:        term.Attributes{Fg: term.ColorWhite},
		NonFocusTabAttr:     term.Attributes{Fg: term.ColorRed},
		WallpaperAttr:       term.Attributes{Fg: term.ColorRed | term.AttrBold},
		FrameUnionCharSet:   component.DefaultFrameUnionCharSet(),
		WindowManagerConfig: handler.DefaultWindowManagerConfig(),
		PromptConfig: PromptConfig{
			Width:         50,
			Height:        14,
			TextAttr:      term.Attributes{},
			HighlightAttr: term.Attributes{Bg: term.ColorRed, Fg: term.ColorWhite},
		},
	}
}

// PromptConfig holds configuration for the browser's Prompt component.
type PromptConfig struct {
	Width, Height int
	TextAttr      term.Attributes
	HighlightAttr term.Attributes
}

// Config holds configuration for an browser.Component.
type Config struct {
	Logger *log.Logger

	Wallpaper               string
	WallpaperAttr           term.Attributes
	WallpaperBackgroundAttr term.Attributes

	MessageBarAttr term.Attributes

	FocusTabAttr    term.Attributes
	NonFocusTabAttr term.Attributes

	PromptConfig

	component.FrameUnionCharSet
	handler.WindowManagerConfig
}
