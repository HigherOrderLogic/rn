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

package text

import (
	"context"
	"fmt"
	"maps"
	"path/filepath"
	"time"

	"github.com/unstablebuild/rune-go-sdk/api/browserapi"
	"github.com/unstablebuild/rune-go-sdk/api/storageapi"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"github.com/unstablebuild/rune-go-sdk/clipboard"
	"github.com/unstablebuild/rune-go-sdk/component"
	"github.com/unstablebuild/rune-go-sdk/iterator"
	"github.com/unstablebuild/rune-go-sdk/term"
	"github.com/unstablebuild/tcell/v3"
	"unstable.build/go-tui/browser"
	"unstable.build/go-tui/cell"
	"unstable.build/go-tui/component/markdown"
	"unstable.build/go-tui/handler"
	"unstable.build/go-tui/handler/command"
	"unstable.build/go-tui/ide/idelsp/languages"
	"unstable.build/go-tui/ide/syntax"
)

// CommandOverlayConfig holds configuration for the
// command's interface.
type CommandOverlayConfig struct {
	MatchedTextAttr  term.Attributes
	FocusElementAttr term.Attributes
	ElementAttr      term.Attributes
	ManualAttr       term.Attributes
	ShowManualAfter  time.Duration
	ShowProgressHint bool
}

// OpenRouter abstracts rerouting file opens to another editor/browser context.
// It is consulted when an open fails with workspaceapi.ErrFileAlreadyOpen,
// before the recovery prompt is shown.
type OpenRouter interface {
	RouteOpen(uri workspaceapi.URI, readOnly bool) (browserapi.Handler, bool, error)
}

type nopOpenRouter struct{}

func (nopOpenRouter) RouteOpen(
	workspaceapi.URI, bool,
) (browserapi.Handler, bool, error) {
	return nil, false, nil
}

// Config holds configuration for an editor.Component.
type Config struct {
	Tabspaces               int
	CommandEvent            term.KeyComb
	CommandMaxHistory       int
	ShellMaxHistory         int
	CommandHistoryKey       term.KeyComb
	CommandKeyBindings      map[term.KeyComb][][]string
	CommandSequenceBindings map[handler.Sequence][][]string
	CommandAliases          map[string]CommandAlias
	SequencerTimeout        time.Duration
	DirtyTabAttr            term.Attributes
	Icons                   IconSet
	Syntax                  syntax.Config
	PkgManager              syntax.PkgManager
	Markdown                markdown.Config
	Clipboard               clipboard.Register
	OpenRouter              OpenRouter
	Comments                CommentConfig
	// ScheduleNextTick is used by async flush completion to run
	// UI-mutating callbacks (notably the EventTypeFlush dispatch and
	// the dirty-attribute reset) on the event-loop goroutine instead
	// of on the awaiter goroutine that delivered the result. When
	// nil at NewComponent time, NewComponent panics — embedders must
	// wire it. Production: ide.ex.init forwards
	// emulatorConfig.ScheduleNextTick. Tests must install their own
	// scheduler (a queueing or lock-serialising one for fixtures that
	// also drive UI mutations from another goroutine).
	ScheduleNextTick func(func()) bool
	// FileExplorerIndentAttr selects the attributes applied to the
	// indent guide rune drawn at the start of every depth level in
	// the file explorer. Defaults to tcell.ColorGray when zero.
	FileExplorerIndentAttr term.Attributes
	// FileExplorerIconAttr selects the attributes applied to the
	// per-row icon glyph (directory, default file, or per-extension
	// override) drawn after the indent guides in the file explorer.
	// Defaults to tcell.ColorGray when zero.
	FileExplorerIconAttr term.Attributes

	EventPublisher func(term.Event) bool

	CommandOverlay CommandOverlayConfig
	browser.Config
}

// CommentBlock configures one block comment delimiter pair.
type CommentBlock struct {
	Start string
	End   string
}

// CommentSpec configures language-specific comment delimiters.
// Line lists line-comment prefixes and Block lists block comment delimiter
// pairs. The first configured token(s) are used for insertion while all
// configured tokens are considered for removal.
type CommentSpec struct {
	Line  []string
	Block []CommentBlock
}

// CommentConfig maps language IDs to their comment delimiters.
type CommentConfig map[string]CommentSpec

const (
	// IndentRuneTab is a tab rune used for indenting.
	IndentRuneTab rune = '\t'
	// IndentRuneSpace is a space rune used for indenting.
	IndentRuneSpace rune = ' '
)

// IndentConfig maps language IDs to the indent material used by editors.
// Supported values are IndentRuneTab and IndentRuneSpace.
type IndentConfig map[string]rune

// ForLanguage returns the indent rune for a language ID.
func (c IndentConfig) ForLanguage(lang string) (rune, bool) {
	if c == nil {
		return 0, false
	}
	r, ok := c[lang]
	return r, ok
}

// IndentConfigForURI returns the indent rune, the number of spaces per
// indent level, and whether the configuration could be resolved for the
// given URI.
//
// Resolution order:
//  1. If the language cannot be determined from the URI, return
//     (0, defaultTabspaces, false).
//  2. If the buffer's existing indentation is conclusive (only tabs or
//     only spaces found at the start of indented lines), prefer the
//     detected rune and, for space-indented buffers, the smallest
//     detected leading-space run as the indent width.
//  3. Otherwise fall back to the configured indent rune for the language
//     and defaultTabspaces for the width.
//  4. If no configuration is present either, return
//     (0, defaultTabspaces, false).
//
// The returned width is always defaultTabspaces for tab-indented material
// and when detection does not yield a conclusive space width.
func IndentConfigForURI(
	uri workspaceapi.URI, buf *cell.Buffer, indents IndentConfig, defaultTabspaces int,
) (rune, int, bool) {
	lang, err := languages.LanguageForFile(filepath.Base(uri.Path()))
	if err != nil {
		return 0, defaultTabspaces, false
	}
	if r, n, ok := detectIndentConfig(buf); ok {
		if r == IndentRuneSpace && n > 0 {
			return r, n, true
		}
		return r, defaultTabspaces, true
	}
	if r, ok := indents.ForLanguage(lang); ok {
		return r, defaultTabspaces, true
	}
	return 0, defaultTabspaces, false
}

// detectIndentScanLimit is the maximum number of non-empty rows inspected
// by detectIndentConfig before giving up.
const detectIndentScanLimit = 500

// detectIndentConfig inspects the leading runes of the given buffer's rows
// and reports whether the file is consistently indented with tabs or
// spaces. For space-indented buffers it also returns the smallest observed
// leading-space run (a reasonable proxy for one indent level). It returns
// (0, 0, false) when the buffer is nil, empty, or uses a mix of both styles
// (inconclusive).
//
// A row counts as tab-indented when its first cell is '\t'. A row counts
// as space-indented when its first two cells are both ' ' (single leading
// spaces are ignored to avoid false positives on non-indent content such
// as Markdown paragraphs). All other rows are ignored.
func detectIndentConfig(buf *cell.Buffer) (rune, int, bool) {
	if buf == nil {
		return 0, 0, false
	}
	rows := buf.RawCells()
	tabs, spaces, scanned, minSpaces := 0, 0, 0, 0
	for _, row := range rows {
		if len(row) == 0 {
			continue
		}
		scanned++
		if scanned > detectIndentScanLimit {
			break
		}
		switch row[0].Ch {
		case IndentRuneTab:
			tabs++
		case IndentRuneSpace:
			n := 0
			for n < len(row) && row[n].Ch == IndentRuneSpace {
				n++
			}
			if n >= 2 {
				spaces++
				if minSpaces == 0 || n < minSpaces {
					minSpaces = n
				}
			}
		}
	}
	switch {
	case tabs > 0 && spaces == 0:
		return IndentRuneTab, 0, true
	case spaces > 0 && tabs == 0:
		return IndentRuneSpace, minSpaces, true
	}
	return 0, 0, false
}

// ForLanguage returns the comment spec for a language ID.
func (c CommentConfig) ForLanguage(lang string) (CommentSpec, bool) {
	if c == nil {
		return CommentSpec{}, false
	}
	spec, ok := c[lang]
	return spec, ok
}

// CommentSpecForURI returns the comment spec for the given URI if its language
// can be determined and a spec is configured.
func CommentSpecForURI(uri workspaceapi.URI, comments CommentConfig) (CommentSpec, bool) {
	lang, err := languages.LanguageForFile(filepath.Base(uri.Path()))
	if err != nil {
		return CommentSpec{}, false
	}
	return comments.ForLanguage(lang)
}

// HasLine reports whether a line-comment prefix is configured.
func (c CommentSpec) HasLine() bool {
	return len(c.Line) != 0 && c.Line[0] != ""
}

// HasBlock reports whether at least one block comment pair is configured.
func (c CommentSpec) HasBlock() bool {
	return len(c.Block) != 0 && c.Block[0].Start != "" && c.Block[0].End != ""
}

// IconSet is used to render icons next to file names in tabs.
type IconSet struct {
	Extensions map[string]rune
	Directory  rune
	// OpenDirectory is used by the file explorer for directories that
	// are currently expanded; collapsed directories use Directory.
	OpenDirectory rune
	Default       rune
	Terminal      rune
	Shell         rune
}

// CommandAlias is a command to command alias, along with completion configuration.
type CommandAlias struct {
	Name     string
	Commands []string
	// Completers is the ordered list of completer factories that produce
	// suggestions for this alias. When the alias is matched, results from
	// each completer are concatenated in list order with duplicates filtered
	// out, so that earlier completers take priority over later ones. An empty
	// or nil slice falls back to the prompt's implicit history-based fallback.
	Completers []func(*Component) command.Completer
}

// DefaultCommandOverlayConfig returns the default Config's CommandOverlayConfig.
func DefaultCommandOverlayConfig() (cfg CommandOverlayConfig) {
	// NOTE: cannot use handler/command config: dependency cycle
	cfg.MatchedTextAttr = term.Attributes{Fg: tcell.ColorRed}
	cfg.FocusElementAttr = term.Attributes{Fg: tcell.ColorRed, Attrs: tcell.AttrBold | tcell.AttrUnderline}
	cfg.ElementAttr = term.Attributes{}
	cfg.ManualAttr = term.Attributes{}
	cfg.ShowManualAfter = 1 * time.Second
	cfg.ShowProgressHint = true
	return
}

// DefaultConfig returns the default Config.
func DefaultConfig() Config {
	cfg := Config{
		Tabspaces:               4,
		CommandEvent:            term.KeyComb{Ch: ':'},
		CommandMaxHistory:       2000,
		ShellMaxHistory:         2000,
		CommandHistoryKey:       term.KeyComb{Mod: term.ModMeta, Ch: 'r'},
		Config:                  browser.DefaultConfig(),
		DirtyTabAttr:            term.Attributes{Attrs: tcell.AttrBold},
		CommandKeyBindings:      make(map[term.KeyComb][][]string),
		CommandSequenceBindings: make(map[handler.Sequence][][]string),
		CommandAliases:          make(map[string]CommandAlias),
		SequencerTimeout:        400 * time.Millisecond,
		CommandOverlay:          DefaultCommandOverlayConfig(),
		EventPublisher:          func(term.Event) bool { return false },
		Syntax:                  syntax.DefaultConfig(),
		PkgManager:              nopPkgManager{},
		Markdown:                markdown.DefaultConfig(),
		Clipboard:               clipboard.NewInMemory(),
		OpenRouter:              nopOpenRouter{},
		Comments:                CommentConfig{},
		FileExplorerIndentAttr:  term.Attributes{Fg: tcell.ColorGray},
		FileExplorerIconAttr:    term.Attributes{Fg: tcell.ColorGray},
		Icons: IconSet{
			Extensions:    map[string]rune{},
			Directory:     '',
			OpenDirectory: '',
			Default:       'o',
			Terminal:      '$',
			Shell:         '',
		},
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

// WithSyntaxConfig returns an Option that sets syntax configuration.
func WithSyntaxConfig(syntax syntax.Config) Option {
	return func(cfg *Config) {
		cfg.Syntax = syntax
	}
}

// WithMarkdownConfig returns an Option that sets markdown configuration.
func WithMarkdownConfig(markdown markdown.Config) Option {
	return func(cfg *Config) {
		cfg.Markdown = markdown
	}
}

// WithClipboard returns an Option that sets the clipboard.
func WithClipboard(clip clipboard.Register) Option {
	return func(cfg *Config) {
		cfg.Clipboard = clip
	}
}

// WithComments returns an Option that sets language-specific comment config.
func WithComments(comments CommentConfig) Option {
	return func(cfg *Config) {
		cfg.Comments = maps.Clone(comments)
	}
}

// WithPackageManager returns an Option that sets the package manager
// for the syntax tree parser.
func WithPackageManager(pkg syntax.PkgManager) Option {
	return func(cfg *Config) {
		cfg.PkgManager = pkg
	}
}

// WithCommandKey returns an Option that defines what key triggers the editor's
// command mode.
func WithCommandKey(event term.KeyComb) Option {
	return func(cfg *Config) {
		cfg.CommandEvent = event
	}
}

// WithCommandHistoryKey returns an Option that defines what key toggles
// the command prompt between the command list and the command history.
func WithCommandHistoryKey(event term.KeyComb) Option {
	return func(cfg *Config) {
		cfg.CommandHistoryKey = event
	}
}

// WithTabBarOffset defines the x offset for
// rendering the tab bar.
func WithTabBarOffset(offset int) Option {
	return func(cfg *Config) {
		cfg.TabBarOffset = offset
	}
}

// WithTabBarHeight defines the height of the tab bar.
func WithTabBarHeight(height int) Option {
	return func(cfg *Config) {
		cfg.TabBarHeight = height
	}
}

// WithTabNameSeparator defines an alternate separator
// of tab names. By default it's two spaces.
func WithTabNameSeparator(sep string) Option {
	return func(cfg *Config) {
		cfg.TabNameSeparator = sep
	}
}

// WithWallpaper sets the starting buffer default text wallpaper.
func WithWallpaper(wallpaper browser.Wallpaper) Option {
	return func(cfg *Config) {
		cfg.Wallpaper = wallpaper
	}
}

// WithFrameUnion defines whether the frames should be unioned or not.
// By default is true if the configuration given to WithWindowManagerConfig
// sets Frame to true.
func WithFrameUnion(frameUnion bool) Option {
	return func(cfg *Config) {
		cfg.Config.FrameUnion = frameUnion
	}
}

// WithTabsClickCallback sets a callback to be called every time the top tabs bar
// is clicked.
func WithTabsClickCallback(fn func(int) bool) Option {
	return func(cfg *Config) {
		cfg.Config.OnTabsClick = fn
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

// WithNotifications returns an Option that configures
// a Component's notifications.
func WithNotifications(n browser.Notifications) Option {
	return func(cfg *Config) {
		cfg.Notifications = n
	}
}

// WithFocusTabAttr returns an Option that configures the attributes of a
// Components's tab in focus.
func WithFocusTabAttr(attr, iconAttr term.Attributes) Option {
	return func(cfg *Config) {
		cfg.FocusTabAttr = attr
		cfg.FocusTabIconAttr = iconAttr
	}
}

// WithNonFocusTabAttr returns an Option that configures the attributes of a
// Components's tabs that are not in focus.
func WithNonFocusTabAttr(attr, iconAttr term.Attributes) Option {
	return func(cfg *Config) {
		cfg.NonFocusTabAttr = attr
		cfg.NonFocusTabIconAttr = iconAttr
	}
}

// WithFocusTabHighlightAttr returns an Option that configures the attributes
// of the highlight rune drawn on top of the focused tab.
func WithFocusTabHighlightAttr(attr term.Attributes) Option {
	return func(cfg *Config) {
		cfg.FocusTabHighlightAttr = attr
	}
}

// WithFocusTabHighlightChar returns an Option that configures the rune drawn
// on top of the focused tab.
func WithFocusTabHighlightChar(r rune) Option {
	return func(cfg *Config) {
		cfg.FocusTabHighlightChar = r
	}
}

// WithCommandKeyBinding maps key to issue cmd.
func WithCommandKeyBinding(key term.KeyComb, cmdAndArgs [][]string) Option {
	return func(cfg *Config) {
		sum := term.KeyComb{Mod: key.Mod, Ch: key.Ch, Key: key.Key}
		cfg.CommandKeyBindings[sum] = cmdAndArgs
	}
}

// WithCommandMaxHistory sets the max command history to store for searching back through it.
func WithCommandMaxHistory(max int) Option {
	return func(cfg *Config) {
		cfg.CommandMaxHistory = max
	}
}

// WithShellMaxHistory sets the max history to retain for the companion shell.
func WithShellMaxHistory(max int) Option {
	return func(cfg *Config) {
		cfg.ShellMaxHistory = max
	}
}

// WithCommandSequenceBinding configures an editor to trigger
// cmd when key sequence is pressed.
func WithCommandSequenceBinding(sequence handler.Sequence, cmdsAndArgs [][]string) Option {
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
		cfg.CommandSequenceBindings[seq] = cmdsAndArgs
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

// Deprecated: this should not be used other than in tests.
func WithFloatingNoMaxSize(val bool) Option { //nolint:revive
	return func(cfg *Config) {
		cfg.NoMaxSize = val
	}
}

// WithCommandAliases defines command aliases.
func WithCommandAliases(aliases map[string]CommandAlias) Option {
	return func(cfg *Config) {
		cfg.CommandAliases = aliases
	}
}

// WithPromptConfig sets the Components's prompt properties.
func WithPromptConfig(c browser.PromptConfig) Option {
	return func(cfg *Config) {
		cfg.Config.PromptConfig = c
	}
}

// WithIconSet sets the Components's file icons.
func WithIconSet(icons IconSet) Option {
	return func(cfg *Config) {
		cfg.Icons = icons
	}
}

// WithFileExplorerIndentAttr sets the attributes used to render the
// indent guide rune of every depth level in the file explorer.
func WithFileExplorerIndentAttr(attr term.Attributes) Option {
	return func(cfg *Config) {
		cfg.FileExplorerIndentAttr = attr
	}
}

// WithFileExplorerIconAttr sets the attributes used to render the
// per-row icon glyph in the file explorer.
func WithFileExplorerIconAttr(attr term.Attributes) Option {
	return func(cfg *Config) {
		cfg.FileExplorerIconAttr = attr
	}
}

// WithEventPublisher sets the Component's event publisher
func WithEventPublisher(f func(term.Event) bool) Option {
	return func(cfg *Config) {
		cfg.EventPublisher = f
	}
}

// WithOpenRouter sets the Component's open router.
func WithOpenRouter(r OpenRouter) Option {
	return func(cfg *Config) {
		cfg.OpenRouter = r
	}
}

// ValidateCommandAliases validates that the given command aliases configuration
// doesn't contain any self-referencing aliases.
func ValidateCommandAliases(aliases map[string]CommandAlias) error {
	for alias := range aliases {
		if isErr := exploreAlias(aliases, alias, make(map[string]struct{})); isErr {
			return fmt.Errorf("alias cycle detected: '%s'", alias)
		}
	}
	return nil
}

func exploreAlias(
	aliases map[string]CommandAlias, exploringAlias string,
	origins map[string]struct{},
) bool {
	origins[exploringAlias] = struct{}{}
	for _, target := range aliases[exploringAlias].Commands {
		if _, seenInPath := origins[target]; seenInPath {
			return true
		}
		if _, isAlias := aliases[target]; !isAlias {
			continue
		}
		copyOrigins := make(map[string]struct{}, len(origins)+1)
		maps.Copy(copyOrigins, origins)
		if isErr := exploreAlias(aliases, target, copyOrigins); isErr {
			return true
		}
	}
	return false
}

type nopPkgManager struct {
}

func (t nopPkgManager) LibDir(ctx context.Context, id string) (
	iterator.Iterator[string], error,
) {
	return nil, storageapi.ErrNotFound
}
