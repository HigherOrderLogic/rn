package vi

import (
	"unstable.build/go-tui/term"
	"unstable.build/go-tui/text/clipboard"
)

// viConfig holds configuration for Vi.
type viConfig struct {
	attr            term.Attributes
	resAttr         term.Attributes
	barAttr         term.Attributes
	clipboard       clipboard.Register
	defaultRegister string
	debug           bool
	wrap            bool
	barHidden       bool
	skipNulls       bool
}

// Option represents a Vi handler configuration option.
type Option func(*viConfig)

// WithResAttr sets the search result cell attributes to be rendered.
func WithResAttr(attr term.Attributes) Option {
	return func(cfg *viConfig) {
		cfg.resAttr = attr
	}
}

// WithBarAttr sets the command bar cell attributes to be rendered.
func WithBarAttr(attr term.Attributes) Option {
	return func(cfg *viConfig) {
		cfg.barAttr = attr
	}
}

// WithBarHidden sets the command bar to be hidden.
func WithBarHidden(hide bool) Option {
	return func(cfg *viConfig) {
		cfg.barHidden = hide
	}
}

// WithAttr sets the default cell attributes to be rendered.
func WithAttr(attr term.Attributes) Option {
	return func(cfg *viConfig) {
		cfg.attr = attr
	}
}

// WithClipboard sets the editor.Clipboard implementation to use.
func WithClipboard(clip clipboard.Register) Option {
	return func(cfg *viConfig) {
		cfg.clipboard = clip
	}
}

// WithDebug disables cursor position correction to aid with cursor debugging.
func WithDebug(debug bool) Option {
	return func(cfg *viConfig) {
		cfg.debug = debug
	}
}

// WithWrap enables or disables word wrapping mode.
func WithWrap(wrap bool) Option {
	return func(cfg *viConfig) {
		cfg.wrap = wrap
	}
}

// WithAutoSkipNullCells determines whether vi should automatically
// shift the cursor on top a null cell (no content) in
// normal, yank, search, g and delete modes. Default is on.
//
// This behaviour is overriden to off if WithDebug Option is used.
func WithAutoSkipNullCells(skip bool) Option {
	return func(cfg *viConfig) {
		cfg.skipNulls = skip
	}
}
