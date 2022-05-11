package text

import (
	"time"

	"github.com/ernestrc/blue/datastore/document"
	"github.com/ernestrc/go-tui/browser"
	"github.com/ernestrc/go-tui/component"
	"github.com/ernestrc/go-tui/handler"
	"github.com/ernestrc/go-tui/term"
	"github.com/ernestrc/go-tui/workspace"
	log "github.com/sirupsen/logrus"
)

// CommandOverlayConfig holds configuration for the
// command's interface.
type CommandOverlayConfig struct {
	Frame            bool
	Width, Height    int
	MatchedTextAttr  term.Attributes
	CountAttr        term.Attributes
	FocusElementAttr term.Attributes
	ElementAttr      term.Attributes
}

// Config holds configuration for an editor.Component.
type Config struct {
	Tabspaces               int
	Filepaths               []workspace.URI
	RecoveryFilepath        workspace.URI
	CommandEvent            term.KeyComb
	CommandMaxHistory       int
	CommandKeyBindings      map[term.KeyComb]string
	CommandSequenceBindings map[handler.Sequence]string
	SequencerTimeout        time.Duration
	Storage                 document.Service
	DirtyTabAttr            term.Attributes

	SendInterrupt func()
	SendEventNone func()

	CommandOverlay CommandOverlayConfig
	browser.Config
}

// DefaultCommandOverlayConfig returns the default Config's CommandOverlayConfig.
func DefaultCommandOverlayConfig() (cfg CommandOverlayConfig) {
	cfg.Frame = true
	cfg.Width = 50
	cfg.Height = 14
	cfg.MatchedTextAttr = term.Attributes{Fg: term.ColorRed}
	cfg.CountAttr = term.Attributes{Fg: term.ColorRed | term.AttrBold}
	cfg.FocusElementAttr = term.Attributes{Fg: term.AttrBold | term.ColorRed}
	cfg.ElementAttr = term.Attributes{}
	return
}

// DefaultConfig returns the default Config.
func DefaultConfig() Config {
	cfg := Config{
		Tabspaces:               4,
		Filepaths:               nil,
		RecoveryFilepath:        workspace.URI{},
		CommandEvent:            term.KeyComb{Ch: ':'},
		CommandMaxHistory:       10,
		Config:                  browser.DefaultConfig(),
		Storage:                 document.NewInMemoryCache(),
		DirtyTabAttr:            term.Attributes{Fg: term.AttrBold},
		CommandKeyBindings:      make(map[term.KeyComb]string),
		CommandSequenceBindings: make(map[handler.Sequence]string),
		SequencerTimeout:        400 * time.Millisecond,
		CommandOverlay:          DefaultCommandOverlayConfig(),
		SendInterrupt:           term.Interrupt,
		SendEventNone:           term.SendNoneEvent,
	}
	return cfg
}

// Option represents a configuration option for Component.
type Option func(*Config)

// WithTabspaces sets the number of spaces used to render a tab.
func WithTabspaces(tabspaces int) Option {
	return func(cfg *Config) {
		cfg.Tabspaces = tabspaces
	}
}

// WithStorage updates the storage of an Editor to use svc.
func WithStorage(svc document.Service) Option {
	return func(cfg *Config) {
		cfg.Storage = svc
	}
}

// WithRecoveryFile indicates that an Editor is to be initialized
// from recovery file swapFilePath. This option overrides WithSwapDir because
// the swap directory of swapFilePath is used instead.
func WithRecoveryFile(swapFilePath workspace.URI) Option {
	return func(cfg *Config) {
		cfg.RecoveryFilepath = swapFilePath
	}
}

// WithFilepath returns an Option that sets the filepath of the file to open with
// a Editor handler.
func WithFile(file workspace.URI) Option {
	return func(cfg *Config) {
		cfg.Filepaths = append(cfg.Filepaths, file)
	}
}

// WithCommandKey returns an Option that defines what key triggers the editor's
// command mode.
func WithCommandKey(event term.KeyComb) Option {
	return func(cfg *Config) {
		cfg.CommandEvent = event
	}
}

// WithLogger sets a logger that the editor can use to log debugging data.
func WithLogger(l *log.Logger) Option {
	return func(cfg *Config) {
		cfg.Logger = l
	}
}

// WithWallpaper sets the starting buffer default text wallpaper.
func WithWallpaper(text string) Option {
	return func(cfg *Config) {
		cfg.Wallpaper = text
	}
}

// WithFrameUnionCharSet configures the characters used to draw the frame union
// between the browser tabs and the window manager.
func WithFrameUnionCharSet(cs component.FrameUnionCharSet) Option {
	return func(cfg *Config) {
		cfg.FrameUnionCharSet = cs
	}
}

// WithWindowManagerConfig returns an Option that defines
// the underlying's WindowManager initialization configuration.
// See handler.WindowManagerConfig for more info. If this option is not passed
// DefaultWindowManagerConfig is utilized.
func WithWindowManagerConfig(config handler.WindowManagerConfig) Option {
	return func(cfg *Config) {
		cfg.WindowManagerConfig = config
	}
}

// WithMessageBarAttr returns an Option that configures
// a Component's message bar attr.
func WithMessageBarAttr(attr term.Attributes) Option {
	return func(cfg *Config) {
		cfg.MessageBarAttr = attr
	}
}

// WithFocusTabAttr returns an Option that configures the attributes of a
// Components's tab in focus.
func WithFocusTabAttr(attr term.Attributes) Option {
	return func(cfg *Config) {
		cfg.FocusTabAttr = attr
	}
}

// WithNonFocusTabAttr returns an Option that configures the attributes of a
// Components's tabs that are not in focus.
func WithNonFocusTabAttr(attr term.Attributes) Option {
	return func(cfg *Config) {
		cfg.NonFocusTabAttr = attr
	}
}

// WithWallpaperAttr returns an Option that configures the attributes of the
// text passed to WithWallpaper.
func WithWallpaperAttr(attr term.Attributes) Option {
	return func(cfg *Config) {
		cfg.WallpaperAttr = attr
	}
}

// WithWallpaperBackgroundAttr returns an Option that configures the attributes of the
// padded background around text passed to WithWallpaper.
func WithWallpaperBackgroundAttr(attr term.Attributes) Option {
	return func(cfg *Config) {
		cfg.WallpaperBackgroundAttr = attr
	}
}

// WithCommandKeyBinding maps key to issue cmd.
func WithCommandKeyBinding(key term.KeyComb, cmd string) Option {
	return func(cfg *Config) {
		sum := term.KeyComb{Mod: key.Mod, Ch: key.Ch, Key: key.Key}
		cfg.CommandKeyBindings[sum] = cmd
	}
}

// WithCommandMaxHistory sets the max command history to store for searching back through it.
func WithCommandMaxHistory(max int) Option {
	return func(cfg *Config) {
		cfg.CommandMaxHistory = max
	}
}

// WithCommandSequenceBinding configures an editor to trigger
// cmd when key sequence is pressed.
func WithCommandSequenceBinding(sequence handler.Sequence, cmd string) Option {
	return func(cfg *Config) {
		seq := handler.Sequence{
			First: term.KeyComb{
				Mod: sequence.First.Mod,
				Ch:  sequence.First.Ch,
				Key: sequence.First.Key,
			},
			Last: term.KeyComb{
				Mod: sequence.Last.Mod,
				Ch:  sequence.Last.Ch,
				Key: sequence.Last.Key,
			},
		}
		cfg.CommandSequenceBindings[seq] = cmd
	}
}

// WithSequencerTimeout configures the time span during which two key events
// can be considered as a sequence.
func WithSequencerTimeout(t time.Duration) Option {
	return func(cfg *Config) {
		cfg.SequencerTimeout = t
	}
}

// WithDirtyTabAttr defines the attributes to use to indicate that
// the buffer in a tab has been modified but not flushed the changes to disk yet.
func WithDirtyTabAttr(attr term.Attributes) Option {
	return func(cfg *Config) {
		cfg.DirtyTabAttr = attr
	}
}

// WithCommandOverlayConfig defines the command overlay interface properties.
func WithCommandOverlayConfig(c CommandOverlayConfig) Option {
	return func(cfg *Config) {
		cfg.CommandOverlay = c
	}
}

// WithPromptConfig sets the Components's prompt properties.
func WithPromptConfig(c browser.PromptConfig) Option {
	return func(cfg *Config) {
		cfg.Config.PromptConfig = c
	}
}

// WithInterrupt sets the Component's interrupt function.
// By default this is set to term.Interrupt.
func WithInterrupt(fn func()) Option {
	return func(cfg *Config) {
		cfg.SendInterrupt = fn
	}
}

// WithSendNone sets the Component's send EventNone function.
// By default this is set to term.SendNoneEvent.
func WithSendNone(fn func()) Option {
	return func(cfg *Config) {
		cfg.SendEventNone = fn
	}
}
