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

package extension

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/rand"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"unstable.build/go-tui/cmd/rune-agent/agent"
	"unstable.build/go-tui/cmd/rune-agent/agent/agentools"
	"unstable.build/go-tui/cmd/rune-agent/agent/agentools/webfetch"
	"unstable.build/go-tui/cmd/rune-agent/agent/audit"
	"unstable.build/go-tui/cmd/rune-agent/agent/geminitools"
	"unstable.build/go-tui/cmd/rune-agent/agent/taskstore"
	"unstable.build/go-tui/cmd/rune-agent/configedit"

	"github.com/unstablebuild/rune-go-sdk/api/browserapi"
	"github.com/unstablebuild/rune-go-sdk/api/config"
	"github.com/unstablebuild/rune-go-sdk/api/extensionapi"
	"github.com/unstablebuild/rune-go-sdk/api/llmapi"
	"github.com/unstablebuild/rune-go-sdk/api/semanticapi"
	"github.com/unstablebuild/rune-go-sdk/api/storageapi"
	"github.com/unstablebuild/rune-go-sdk/api/syntaxapi"
	"github.com/unstablebuild/rune-go-sdk/api/textapi"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"github.com/unstablebuild/rune-go-sdk/clipboard"
	"github.com/unstablebuild/rune-go-sdk/component"
	"github.com/unstablebuild/rune-go-sdk/handler"
	"github.com/unstablebuild/rune-go-sdk/iterator"
	"github.com/unstablebuild/rune-go-sdk/term"
	"github.com/unstablebuild/rune-go-sdk/tui"
	"unstable.build/go-tui/cmd/rune-agent/agent/skills"
	"unstable.build/go-tui/cmd/rune-agent/agentshell"
	"unstable.build/go-tui/cmd/rune-agent/dialogue/dialoguemanager"
	"unstable.build/go-tui/cmd/rune-agent/dialogue/dialoguetui"
	"unstable.build/go-tui/cmd/rune-agent/hooks"
	"unstable.build/go-tui/cmd/rune-agent/llm/llmarg"

	runemcp "unstable.build/go-tui/cmd/rune-agent/mcp"
	"unstable.build/go-tui/cmd/rune-agent/memory"
	"unstable.build/go-tui/component/markdown"
	"unstable.build/go-tui/debug"
	"unstable.build/go-tui/extension/extutil"
	"unstable.build/go-tui/text"

	tconfig "unstable.build/go-tui/api/config"
)

const (
	commandQuery = "?"
	commandChat  = "agent"

	commandEffort    = "chateffort"
	commandMaxTokens = "chatmaxtokens"
	commandSkill     = "chatskill"
	commandModel     = "chatmodel"
	commandClear     = "chatclear"
	commandCompact   = "chatcompact"
	commandFork      = "chatfork"
	commandExport    = "chatexport"
	commandLog       = "chatlog"
)

// Router-side model aliases the agent resolves to for its own concepts.
// Each is managed through `models alias` and falls back to `default`
// when unset.
const (
	queryModelAlias   = "query"
	compactModelAlias = "compact"
	dreamModelAlias   = "dream"
)

var (
	defaultComponentCfg = dialoguetui.ComponentConfig{
		MessagesRowConfig: component.SpanConfig{
			PadHorizontal:    -80,
			PadVertical:      2,
			ContentAlignment: component.AlignmentCentered,
		},
		InputRowColumns: 10,
		InputBox: dialoguetui.InputBoxConfig{
			Placeholder: "Message your assistant...",
			PlaceholderConfig: component.StringResponsiveConfig{
				NoSplitWords: true,
				StringConfig: component.StringConfig{
					Alignment:            component.AlignmentLeft,
					Attributes:           term.Attributes{Fg: term.ColorGray},
					BackgroundAttributes: term.Attributes{},
				},
			},
			ContentAttr: term.Attributes{},
			FrameAttr:   term.Attributes{},
		},
		ReceiveMessageStringConfig: component.StringConfig{
			Alignment:            component.AlignmentLeft,
			Attributes:           term.Attributes{Fg: term.ColorSilver},
			BackgroundAttributes: term.Attributes{},
		},
		ReceiveMessageSpanConfig: component.SpanConfig{
			ContentAlignment: component.AlignmentLeft,
		},
		ReasoningStringConfig: component.StringConfig{
			Alignment:            component.AlignmentLeft,
			Attributes:           term.Attributes{Fg: term.GetColor("dimgray")},
			BackgroundAttributes: term.Attributes{},
		},
		ReasoningSpanConfig: component.SpanConfig{
			ContentAlignment: component.AlignmentLeft,
		},
		SendMessageStringConfig: component.StringConfig{
			Alignment:            component.AlignmentLeft,
			Attributes:           term.Attributes{},
			BackgroundAttributes: term.Attributes{},
		},
		SendMessageSpanConfig: component.SpanConfig{
			ContentAlignment: component.AlignmentLeft,
		},
		SendMessageBottomPad: 1,
		ToolCallStringConfig: component.StringConfig{
			Alignment:            component.AlignmentLeft,
			Attributes:           term.Attributes{Fg: term.ColorPurple},
			BackgroundAttributes: term.Attributes{},
		},
		ToolCallSpanConfig: component.SpanConfig{
			ContentAlignment: component.AlignmentLeft,
		},
		ToolCallArgsStringConfig: component.StringConfig{
			Alignment:            component.AlignmentLeft,
			Attributes:           term.Attributes{Fg: term.ColorGray},
			BackgroundAttributes: term.Attributes{},
		},
		ToolResultStringConfig: component.StringConfig{
			Alignment:            component.AlignmentLeft,
			Attributes:           term.Attributes{Fg: term.ColorGray},
			BackgroundAttributes: term.Attributes{},
		},
		ToolResultSpanConfig: component.SpanConfig{
			ContentAlignment: component.AlignmentLeft,
		},
		ErrorStringConfig: component.StringConfig{
			Alignment:            component.AlignmentLeft,
			Attributes:           term.Attributes{Fg: term.ColorRed},
			BackgroundAttributes: term.Attributes{},
		},
		ErrorSpanConfig: component.SpanConfig{
			PadVertical:      1,
			ContentAlignment: component.AlignmentLeft,
		},
		WarningStringConfig: component.StringConfig{
			Alignment:            component.AlignmentLeft,
			Attributes:           term.Attributes{Fg: term.ColorYellow},
			BackgroundAttributes: term.Attributes{},
		},
		WarningSpanConfig: component.SpanConfig{
			PadVertical:      1,
			ContentAlignment: component.AlignmentLeft,
		},
		CommandOutputSpanConfig: component.SpanConfig{
			PadVertical: 1,
		},
		ToolResultMaxLines: 5,
		StartCollapsed:     true,
		ReasoningAnnotationStringConfig: component.StringConfig{
			Alignment:  component.AlignmentLeft,
			Attributes: term.Attributes{Attrs: term.AttrDim},
		},
		MarkdownConfig: defaultMarkdownConfig(),
		PromptToolCallStringConfig: component.StringConfig{
			Alignment:            component.AlignmentLeft,
			Attributes:           term.Attributes{Fg: term.ColorAqua},
			BackgroundAttributes: term.Attributes{},
		},
		SelectionConfig: dialoguetui.SelectionConfig{
			TitleStringConfig: component.StringConfig{
				Alignment:  component.AlignmentLeft,
				Attributes: term.Attributes{Fg: term.ColorSilver},
			},
			OptionStringConfig: component.StringConfig{
				Alignment:  component.AlignmentLeft,
				Attributes: term.Attributes{Fg: term.ColorSilver},
			},
			CursorStringConfig: component.StringConfig{
				Alignment:  component.AlignmentLeft,
				Attributes: term.Attributes{Attrs: term.AttrReverse},
			},
			HeaderStringConfig: component.StringConfig{
				Alignment:  component.AlignmentLeft,
				Attributes: term.Attributes{Fg: term.ColorYellow},
			},
			DescStringConfig: component.StringConfig{
				Alignment:  component.AlignmentLeft,
				Attributes: term.Attributes{Fg: term.GetColor("dimgray")},
			},
		},
	}
)

func defaultMarkdownConfig() *markdown.Config {
	cfg := markdown.DefaultConfig()
	cfg.HeaderPrefix = false
	return &cfg
}

func newCommandEventHandler(
	ctx context.Context, ed textapi.Editor, w *extensionapi.Workspace,
	pconfig config.Config,
) (ret *aiEditorHandler, err error) {
	fs := w.FileSystem(ctx)
	executor := w.Executor(ctx)
	terminal := w.Terminal(ctx)
	cwd, err := fs.URI(".")
	if err != nil {
		return nil, fmt.Errorf("get workspace root: %w", err)
	}

	var toolsCfg agentools.Config
	if v, err := pconfig.GetInt("max_line_bytes"); err == nil {
		toolsCfg.MaxLineBytes = v
	} else if err != nil && !errors.Is(err, config.ErrNotFound) {
		slog.Warn("get 'max_line_bytes' from config", "error", err)
	}

	lsp := w.LSP(ctx)
	cfg := configedit.NewConfig(fs, cwd, pconfig)
	tools, tracker := agentools.DefaultTools(fs, executor, cwd, lsp, toolsCfg, cfg)

	fetcher := webfetch.NewHTTPFetcher(webfetch.DefaultConfig())
	tools = append(tools, agentools.NewWebFetch(fetcher))

	parser := w.Parser(ctx)
	tools = append(tools, agentools.LSPTools(lsp, fs, parser, cwd, tracker)...)
	tools = append(tools, agentools.SyntaxTools(parser, fs, cwd, tracker)...)

	mcpManager := runemcp.NewManager()
	mcpData, mcpReadErr := readWorkspaceFile(fs, filepath.Join(cwd.Path(), ".mcp.json"))
	if mcpReadErr == nil {
		mcpCfg, mcpParseErr := runemcp.LoadConfig(mcpData)
		if mcpParseErr != nil {
			slog.Warn("mcp: failed to parse .mcp.json", "error", mcpParseErr)
		} else {
			mcpTools := mcpManager.Connect(ctx, mcpCfg)
			tools = append(tools, mcpTools...)
		}
	} else if !errors.Is(mcpReadErr, os.ErrNotExist) {
		slog.Warn("mcp: failed to read .mcp.json", "error", mcpReadErr)
	}

	// Discover skills from config dirs.
	var skillDirs []string
	if dirs, err := pconfig.GetSlice("skills"); err == nil {
		for _, d := range dirs {
			if s, ok := d.(string); ok {
				skillDirs = append(skillDirs, s)
			}
		}
	}
	skillRegistry := skills.NewRegistry(fs, cwd, skillDirs, w.Notifications(ctx))
	if loaded := skillRegistry.List(); len(loaded) > 0 {
		slog.Info("loaded skills", "count", len(loaded))
	}
	tools = append(tools, agentools.NewSkillTool(skillRegistry, nil, nil))

	// Always register memory tools — the workspace may be bootstrapped
	// after the extension starts. Tools fail gracefully at runtime.
	memoryPath := filepath.Join(w.DataDir(ctx), "memory")
	tools = append(tools, agentools.MemoryTools(executor, memoryPath)...)

	db := w.Storage(ctx)
	sessionsDir := filepath.Join(w.DataDir(ctx), "sessions")
	dialogueStore := dialoguemanager.NewStore(db, sessionsDir)
	tools = append(tools, agentools.ConversationTools(dialogueStore, fs, sessionsDir)...)

	ret = new(aiEditorHandler)
	ret.defaultEffort = llmapi.ReasoningEffortHigh
	ret.ctx, ret.cancelCtx = context.WithCancel(context.Background())
	ret.ed = ed
	ret.config = cfg
	ret.mcpManager = mcpManager
	ret.baseTools = tools
	ret.toolRegistry = agent.NewRegistry(tools...)
	ret.executor = executor
	ret.sessionMgr = agentools.NewSessionManager(ret.ctx, executor, terminal)
	ret.toolRegistry.RegisterOverrides("openai",
		agentools.NewListDir(fs, cwd),
		agentools.NewWriteStdin(ret.sessionMgr),
	)
	ret.toolRegistry.RegisterOverrides("codex",
		agentools.NewListDir(fs, cwd),
		agentools.NewWriteStdin(ret.sessionMgr),
	)
	for _, provider := range []string{"openai", "codex"} {
		ret.toolRegistry.RegisterReplacement(provider, "search_content",
			agentools.NewGrepFiles(fs, cwd, tracker))
		ret.toolRegistry.RegisterReplacement(provider, "bash",
			agentools.NewExecCommand(ret.sessionMgr, cwd, cfg))
		ret.toolRegistry.RegisterExclusions(provider, "compact")
	}
	geminitools.Register(ret.toolRegistry)
	ret.systemPrompt = agent.DefaultSystemPrompt(cwd)

	// Discover and load project instruction files (e.g. AGENTS.md).
	agentsFile := agent.DefaultAgentsFile
	if v, err := pconfig.GetString("agents_file"); err == nil {
		agentsFile = v
	} else if err != nil && !errors.Is(err, config.ErrNotFound) {
		slog.Warn("get 'agents_file' from config", "error", err)
	}
	if agentsFile != "" {
		paths := agent.DiscoverAgentsFiles(fs, cwd.Path(), agentsFile)
		if section := agent.LoadAgentsFiles(fs, paths); section != "" {
			ret.projectInstructions = section
			slog.Info("loaded project instructions", "file", agentsFile, "count", len(paths))
		}
	}

	ret.skillRegistry = skillRegistry
	ret.plansDir = filepath.Join(w.DataDir(ctx), "plans")
	ret.memoryPath = memoryPath
	ret.cwd = cwd
	ret.fs = fs
	ret.exec = executor
	ret.gitID = resolveGitIdentity(ctx, executor, cwd)
	ret.lsp = lsp
	ret.parser = parser
	ret.memoryDataPath = filepath.Join(w.DataDir(ctx), "memory")
	// Pass the alias name through unresolved; the router resolves it per
	// request so a `models alias` change takes effect without a restart.
	ret.defaultModel = llmapi.DefaultModel
	if v, err := pconfig.GetInt("max_tool_output_bytes"); err == nil {
		ret.maxToolOutputBytes = v
	} else if err != nil && !errors.Is(err, config.ErrNotFound) {
		slog.Warn("get 'max_tool_output_bytes' from config", "error", err)
	}
	if v, err := pconfig.GetFloat("auto_compact_ratio"); err == nil {
		ret.autoCompactRatio = v
	} else if err != nil && !errors.Is(err, config.ErrNotFound) {
		slog.Warn("get 'auto_compact_ratio' from config", "error", err)
	}
	// The host-provided llmapi.Service (w.LLM(ctx)) owns provider auth,
	// llama.cpp, codex login, and custom_provider routing; rune-agent
	// no longer constructs its own registry.
	ret.llmSvc = w.LLM(ctx)

	// Hooks: parse extensions.rune-agent.config.hooks. Errors are
	// non-fatal: the extension still starts with an empty (no-op)
	// hook config. The single Runner constructed here is shared
	// across chat and query agents and propagated via agent.Config.
	noti := w.Notifications(ctx)
	var hooksCfg hooks.Config
	if hooksMap, hookErr := pconfig.GetMap("hooks"); hookErr == nil {
		parsed, parseErr := hooks.Parse(hooksMap)
		if parseErr != nil {
			_, _ = noti.Notify(browserapi.LevelWarn,
				"hooks: invalid config: %v", parseErr)
		} else {
			hooksCfg = parsed
		}
	} else if !errors.Is(hookErr, config.ErrNotFound) {
		_, _ = noti.Notify(browserapi.LevelWarn,
			"hooks: invalid config: %v", hookErr)
	}
	ret.hookRunner = hooks.NewRunner(hooksCfg, executor, noti, cwd.Path())

	ret.cfg = defaultComponentCfg
	ret.cfg.MarkdownConfig.Parser = w.Parser(ctx)
	backgroundAttr, err := config.GetAttributes(pconfig, "background_attr")
	if err != nil {
		if err != config.ErrNotFound {
			slog.Warn("get 'background_attr' from extension config", "error", err)
		}
	}
	ret.cfg.ReceiveMessageStringConfig.BackgroundAttributes = backgroundAttr
	ret.cfg.InputBox.PlaceholderConfig.BackgroundAttributes = backgroundAttr

	ret.cfg.InputBox.FrameAttr.Bg = backgroundAttr.Bg
	ret.cfg.InputBox.PlaceholderConfig.Bg = backgroundAttr.Bg
	ret.cfg.InputBox.ContentAttr.Bg = backgroundAttr.Bg

	ret.cfg.ReceiveMessageStringConfig.Bg = backgroundAttr.Bg

	sendMsgBackgroundAttr, err := config.GetAttributes(pconfig, "user_msg_background_attr")
	if err != nil {
		if err != config.ErrNotFound {
			slog.Warn("get 'user_msg_background_attr' from extension config", "error", err)
		}
		sendMsgBackgroundAttr = term.Attributes{Bg: term.ColorGray}
	}
	ret.cfg.SendMessageStringConfig.BackgroundAttributes = sendMsgBackgroundAttr
	ret.cfg.SendMessageStringConfig.Bg = sendMsgBackgroundAttr.Bg
	ret.cfg.ReasoningStringConfig.BackgroundAttributes = backgroundAttr
	ret.cfg.ReasoningStringConfig.Bg = backgroundAttr.Bg
	ret.cfg.ToolCallStringConfig.BackgroundAttributes = backgroundAttr
	ret.cfg.ToolCallStringConfig.Bg = backgroundAttr.Bg
	ret.cfg.ToolCallArgsStringConfig.BackgroundAttributes = backgroundAttr
	ret.cfg.ToolCallArgsStringConfig.Bg = backgroundAttr.Bg
	ret.cfg.ToolResultStringConfig.BackgroundAttributes = backgroundAttr
	ret.cfg.ToolResultStringConfig.Bg = backgroundAttr.Bg
	ret.cfg.ReasoningAnnotationStringConfig.BackgroundAttributes = backgroundAttr
	ret.cfg.ReasoningAnnotationStringConfig.Bg = backgroundAttr.Bg
	ret.cfg.ErrorStringConfig.BackgroundAttributes = backgroundAttr
	ret.cfg.ErrorStringConfig.Bg = backgroundAttr.Bg
	ret.cfg.WarningStringConfig.BackgroundAttributes = backgroundAttr
	ret.cfg.WarningStringConfig.Bg = backgroundAttr.Bg
	ret.cfg.PromptToolCallStringConfig.BackgroundAttributes = backgroundAttr
	ret.cfg.PromptToolCallStringConfig.Bg = backgroundAttr.Bg
	ret.cfg.SelectionConfig.TitleStringConfig.BackgroundAttributes = backgroundAttr
	ret.cfg.SelectionConfig.TitleStringConfig.Bg = backgroundAttr.Bg
	ret.cfg.SelectionConfig.OptionStringConfig.BackgroundAttributes = backgroundAttr
	ret.cfg.SelectionConfig.OptionStringConfig.Bg = backgroundAttr.Bg
	ret.cfg.SelectionConfig.CursorStringConfig.BackgroundAttributes = backgroundAttr
	ret.cfg.SelectionConfig.CursorStringConfig.Bg = backgroundAttr.Bg
	ret.cfg.SelectionConfig.HeaderStringConfig.BackgroundAttributes = backgroundAttr
	ret.cfg.SelectionConfig.HeaderStringConfig.Bg = backgroundAttr.Bg
	ret.cfg.SelectionConfig.DescStringConfig.BackgroundAttributes = backgroundAttr
	ret.cfg.SelectionConfig.DescStringConfig.Bg = backgroundAttr.Bg
	ret.cfg.BackgroundColor = backgroundAttr.Bg
	ret.backgroundAttr = backgroundAttr

	sendMsgAttr, err := config.GetAttributes(pconfig, "user_msg_attr")
	if err != nil {
		if err != config.ErrNotFound {
			slog.Warn("get 'user_msg_attr' from extension config", "error", err)
		}
	} else {
		ret.cfg.SendMessageStringConfig.Attributes = sendMsgAttr
	}
	recvMsgAttr, err := config.GetAttributes(pconfig, "assistant_msg_attr")
	if err != nil {
		if err != config.ErrNotFound {
			slog.Warn("get 'assistant_msg_attr' from extension config", "error", err)
		}
	} else {
		ret.cfg.ReceiveMessageStringConfig.Attributes = recvMsgAttr
	}

	inputBoxAttr, err := config.GetAttributes(pconfig, "input_box_attr")
	if err != nil {
		if err != config.ErrNotFound {
			slog.Warn("get 'input_box_attr' from extension config", "error", err)
		}
	} else {
		ret.cfg.InputBox.ContentAttr = inputBoxAttr
	}
	ret.cfg.InputBackgroundColor = resolveInputBackgroundColor(
		ret.cfg.InputBackgroundColor, pconfig)
	inputBoxPlaceholderAttr, err := config.GetAttributes(pconfig, "input_box_placeholder_attr")
	if err != nil {
		if err != config.ErrNotFound {
			slog.Warn("Error getting 'input_box_placeholder_attr'"+
				" from extension config", "error", err)
		}
	} else {
		ret.cfg.InputBox.PlaceholderConfig.Attributes = inputBoxPlaceholderAttr
	}

	inputBoxFrameAttr, err := config.GetAttributes(pconfig, "input_box_frame_attr")
	if err != nil {
		if err != config.ErrNotFound {
			slog.Warn("Error getting 'input_box_frame_attr' from extension config", "error", err)
		}
	} else {
		ret.cfg.InputBox.FrameAttr = inputBoxFrameAttr
	}

	inputBoxFrameCharset, err := tconfig.GetFrameCharset(
		pconfig, "input_box_frame_charset", ret.cfg.InputBox.FrameCharSet)
	if err != nil {
		if err != config.ErrNotFound {
			slog.Warn("get 'input_box_frame_charset' from extension config", "error", err)
		}
	} else {
		ret.cfg.InputBox.FrameCharSet = inputBoxFrameCharset
	}

	inputBgColor, err := pconfig.GetColor("input_background_color")
	if err != nil {
		if err != config.ErrNotFound {
			slog.Warn("get 'input_background_color' from extension config", "error", err)
		}
	} else {
		ret.cfg.InputBackgroundColor = inputBgColor
	}

	ret.contextHintCfg = contextHintConfig{
		labelAttr: term.Attributes{Bg: backgroundAttr.Bg, Attrs: term.AttrDim},
		valueAttr: term.Attributes{Bg: backgroundAttr.Bg, Fg: term.ColorRed},
	}
	contextHintAttr, err := config.GetAttributes(pconfig, "context_hint_attr")
	if err != nil {
		if err != config.ErrNotFound {
			slog.Warn("get 'context_hint_attr' from extension config", "error", err)
		}
	} else {
		ret.contextHintCfg.valueAttr = contextHintAttr
		ret.contextHintCfg.valueAttr.Bg = backgroundAttr.Bg
		ret.contextHintCfg.labelAttr = contextHintAttr
		ret.contextHintCfg.labelAttr.Bg = backgroundAttr.Bg
		ret.contextHintCfg.labelAttr.Attrs |= term.AttrDim
	}

	ret.queryDefaultModel = queryModelAlias

	ret.compactModel = compactModelAlias

	ret.clip = text.NewSystemClipboard()

	// Resolve the configured editor for composing messages. On error the
	// compose editor stays nil and dialoguetui falls back to its inputbox.
	if editor, cerr := extutil.Editor(ret.clip, w.Config(ctx)); cerr != nil {
		slog.Warn("resolve dialogue editor, using inputbox", "error", cerr)
	} else {
		ret.cfg.Editor = editor
		if modal, merr := extutil.EditorModal(w.Config(ctx)); merr != nil {
			slog.Warn("resolve dialogue editor mode", "error", merr)
		} else {
			ret.cfg.EditorModal = modal
		}
		ret.cfg.ModalStartInsert = true
		if startInsert, serr := pconfig.GetBool("editor_modal_start_insert"); serr == nil {
			ret.cfg.ModalStartInsert = startInsert
		} else if !errors.Is(serr, config.ErrNotFound) {
			slog.Warn("get 'editor_modal_start_insert' from config", "error", serr)
		}
	}

	ret.db = db
	if auditEnabled, _ := pconfig.GetBool("audit_enabled"); auditEnabled {
		ret.auditStore = audit.NewStore(ret.db)
	}
	ret.p = w.Interrupter(ctx)
	ret.o = w.ResourceOpener(ctx)
	ret.wm = w.WindowManager(ctx)
	// Wrap Notifications so every Notify call fans out to configured
	// Notification hooks. The wrapper is purely observational; the
	// hook return value is ignored.
	ret.n = newNotificationsWithHooks(
		w.Notifications(ctx), ret.hookRunner, ret.cwd)
	ret.resources = make(map[string]string)

	ret.dialogueStore = dialogueStore

	if compactSvc, _, cerr := ret.modelService(ctx, ret.compactModel); cerr != nil {
		_, _ = noti.Notify(browserapi.LevelWarn,
			"failed to create backend for compact model (%v), compaction will use the chat model",
			cerr)
		ret.compactModel = ""
		ret.compactSvc = nil
	} else {
		ret.compactSvc = compactSvc
	}

	queryService, queryEntry, err := ret.modelService(ctx, ret.queryDefaultModel)
	if err != nil {
		first := firstModel(ret.llmSvc)
		if first == "" || first == ret.queryDefaultModel {
			return nil, fmt.Errorf("new backend for query dialogues: %v", err)
		}
		_, _ = noti.Notify(browserapi.LevelWarn,
			"failed to create backend for model %q (%v), falling back to %q",
			ret.queryDefaultModel, err, first)
		ret.queryDefaultModel = first
		queryService, queryEntry, err = ret.modelService(ctx, first)
		if err != nil {
			return nil, fmt.Errorf("new backend for query dialogues (fallback %q): %v", first, err)
		}
	}
	ret.queryAgent = agent.NewAgent(
		queryService, ret.toolRegistry, ret.skillRegistry,
		ret.dialogueStore, agent.NoMemory(), agent.Config{
			MaxToolOutputBytes:  ret.maxToolOutputBytes,
			AutoCompactRatio:    ret.autoCompactRatio,
			CompactSvc:          ret.compactSvc,
			SystemPrompt:        agent.QuerySystemPrompt(ret.cwd) + agent.ProviderToolAddendum(queryEntry.Provider),
			ProjectInstructions: ret.projectInstructions,
			SessionKey:          "query",
			AgentID:             "query",
			Model:               queryEntry,
			Workspace:           ret.cwd,
			Hooks:               ret.hookRunner,
		},
	)

	// Sub-agent configuration.
	ret.agentsConfig = agent.NewConfig(
		[]agent.Definition{
			{
				ID:           "default",
				Name:         "Default Agent",
				Model:        ret.defaultModel,
				SystemPrompt: ret.systemPrompt,
				AllowAny:     true,
			},
		},
	)

	return ret, nil
}

type aiEditorHandler struct {
	exit                atomic.Uint32
	llmSvc              llmapi.Service
	defaultModel        string
	cfg                 dialoguetui.ComponentConfig
	backgroundAttr      term.Attributes
	contextHintCfg      contextHintConfig
	dialogueStore       dialoguemanager.Store
	queryAgent          *agent.Agent
	queryDefaultModel   string
	compactModel        string // empty means use the chat model
	compactSvc          llmapi.Service
	toolRegistry        *agent.Registry
	systemPrompt        string
	projectInstructions string
	agentsConfig        *agent.Cfg
	baseTools           []agent.Tool
	mcpManager          *runemcp.Manager
	sessionMgr          *agentools.SessionManager

	maxToolOutputBytes int
	autoCompactRatio   float64

	resources      map[string]string
	clip           clipboard.Register
	ed             textapi.Editor
	wm             browserapi.WindowManager
	n              browserapi.Notifications
	o              browserapi.ResourceOpener
	p              term.Interrupter
	db             storageapi.Service
	config         configedit.Config
	skillRegistry  *skills.SkillRegistry
	plansDir       string
	memoryDataPath string
	exec           workspaceapi.Executor
	lsp            semanticapi.LSP
	parser         syntaxapi.Parser
	memoryPath     string
	cwd            workspaceapi.URI
	fs             workspaceapi.FileSystem
	executor       workspaceapi.Executor
	gitID          gitIdentity

	auditStore *audit.Store

	// generateDialogueID overrides the spawner's dialogue ID
	// generator. Testing only.
	generateDialogueID func(ctx context.Context, agentID string) string
	// generatePlanPath overrides the plan path generator. Testing only.
	generatePlanPath func(title string) string
	defaultsMu       sync.Mutex
	defaultEffort    llmapi.ReasoningEffort // global default applied to new chats/queries
	defaultMaxTokens int                    // global default applied to new chats/queries

	openChats      sync.Map
	openChatAgents sync.Map
	// openChatTx maps an open chat's dialogue ID to its dialoguetui event
	// channel, letting workspace command-prompt commands inject in-chat
	// commands into the focused chat. Stored and deleted alongside
	// openChats / openChatAgents.
	openChatTx sync.Map
	ctx        context.Context
	cancelCtx  func()

	// hookRunner dispatches Claude-Code-style hooks. nil when no
	// hooks are configured.
	hookRunner *hooks.Runner
}

// modelService resolves a model entry from the host-provided LLM
// service, optionally wrapping it in an audit decorator. Replaces the
// rune-agent-owned newLLMService/newLlamaCppService dispatch logic;
// provider auth, registries, and local-model lifecycle now live in the
// IDE host behind w.LLM(ctx).
func (h *aiEditorHandler) modelService(ctx context.Context, model string) (
	llmapi.Service, llmapi.ModelEntry, error,
) {
	entry, err := llmarg.Resolve(ctx, h.llmSvc, model)
	if err != nil {
		return nil, llmapi.ModelEntry{}, err
	}
	svc := h.llmSvc
	if h.auditStore != nil {
		svc = audit.NewService(svc, h.auditStore, entry)
	}
	return svc, entry, nil
}

func (h *aiEditorHandler) getDefaultEffort() llmapi.ReasoningEffort {
	h.defaultsMu.Lock()
	defer h.defaultsMu.Unlock()
	return h.defaultEffort
}

func (h *aiEditorHandler) setDefaultEffort(e llmapi.ReasoningEffort) {
	h.defaultsMu.Lock()
	h.defaultEffort = e
	h.defaultsMu.Unlock()
	h.queryAgent.SetEffort(e)
}

func (h *aiEditorHandler) getDefaultMaxTokens() int {
	h.defaultsMu.Lock()
	defer h.defaultsMu.Unlock()
	return h.defaultMaxTokens
}

func (h *aiEditorHandler) setDefaultMaxTokens(n int) {
	h.defaultsMu.Lock()
	h.defaultMaxTokens = n
	h.defaultsMu.Unlock()
	h.queryAgent.SetMaxOutputTokens(n)
	h.openChatAgents.Range(func(_, v any) bool {
		if ag, ok := v.(*agent.Agent); ok {
			ag.SetMaxOutputTokens(n)
		}
		return true
	})
}

func (h *aiEditorHandler) Handle(ctx context.Context, ev textapi.Event) (exit bool) {
	uexit := h.exit.Load()
	exit = uexit != 0
	if exit {
		return
	}
	if ev.URI == (workspaceapi.URI{}) {
		return
	}

	var err error
	switch ev.Type {
	case textapi.EventTypeFlush, textapi.EventTypeOpen:
		content, ok := h.resources[ev.URI.String()]
		if ok {
			err = h.queryAgent.AddContextResource(ctx, ev.URI, content)
		}
		h.resources[ev.URI.String()] = ev.Content
	case textapi.EventTypeFocus:
		content, ok := h.resources[ev.URI.String()]
		if !ok {
			slog.Warn("could not find resource on focus event", "uri", ev.URI)
			return
		}
		err = h.queryAgent.AddContextResource(ctx, ev.URI, content)
	case textapi.EventTypeUnfocus:
		err = h.queryAgent.RemoveContextResource(ctx, ev.URI)
	case textapi.EventTypeClose:
		delete(h.resources, ev.URI.String())
	}

	if err != nil {
		slog.Error("query agent context resource", "uri", ev.URI, "error", err)
	}
	return
}

func (h *aiEditorHandler) HandleCommand(
	ctx context.Context, cmd textapi.Command,
) (err error) {
	switch cmd.Name {
	case commandQuery:
		return h.handleQuery(cmd)
	case commandChat:
		return h.handleChat(cmd)
	case commandModel, commandEffort, commandMaxTokens, commandSkill,
		commandClear, commandCompact, commandFork, commandExport, commandLog:
		return h.routeChatCommand(cmd)
	}

	return nil
}

// dialogueIDFromURI extracts the dialogue ID from a rune-agent chat tab URI
// of the form rune-agent://<model>/<id>. It returns false for any other
// scheme or a URI without a dialogue ID path segment.
func dialogueIDFromURI(uri workspaceapi.URI) (string, bool) {
	if uri.Scheme() != "rune-agent" {
		return "", false
	}
	id := strings.TrimPrefix(uri.Path(), "/")
	if id == "" {
		return "", false
	}
	return id, true
}

// routeChatCommand forwards a workspace command-prompt command (the
// chat-prefixed commands like chatmodel, chateffort, chatclear) to the
// focused rune-agent chat, where it runs exactly as if the user had typed
// the equivalent /command in that chat.
func (h *aiEditorHandler) routeChatCommand(cmd textapi.Command) error {
	id, ok := dialogueIDFromURI(cmd.URI)
	if !ok {
		return fmt.Errorf("%s must be run from an open agent chat tab", cmd.Name)
	}
	v, ok := h.openChatTx.Load(id)
	if !ok {
		return fmt.Errorf("%s must be run from an open agent chat tab", cmd.Name)
	}
	tx := v.(chan<- dialoguetui.MessageEvent)

	name, args := cmd.Name, cmd.Args
	switch cmd.Name {
	case commandModel:
		name = "model"
	case commandEffort:
		name = "effort"
	case commandClear:
		if err := rejectPositionalID("clear", cmd.Args); err != nil {
			return err
		}
		name = "clear"
	case commandCompact:
		if err := rejectPositionalID("compact", cmd.Args); err != nil {
			return err
		}
		name = "compact"
	case commandFork:
		if err := rejectPositionalID("fork", cmd.Args); err != nil {
			return err
		}
		name = "fork"
	case commandExport:
		if err := rejectPositionalID("export", cmd.Args); err != nil {
			return err
		}
		name = "export"
	case commandLog:
		if err := rejectPositionalID("log", cmd.Args); err != nil {
			return err
		}
		name = "log"
	case commandMaxTokens:
		// The prompt command "chatmaxtokens" maps to the in-chat adapter's
		// "max_tokens" command name.
		name = "max_tokens"
	case commandSkill:
		// In-chat skills are invoked as /<skill> [args]; the prompt
		// command "chatskill <name> [args]" shifts the first argument
		// into the command name so the chat's skill resolution path fires.
		if len(cmd.Args) == 0 {
			return fmt.Errorf("%s requires a skill name", cmd.Name)
		}
		name, args = cmd.Args[0], cmd.Args[1:]
	}

	ev := dialoguetui.MessageEvent{
		Type:        dialoguetui.MessageEventCommand,
		CommandName: name,
		CommandArgs: args,
	}
	// Sending on tx must not block the event loop, so dispatch the send
	// to a goroutine bounded by the handler context.
	go debug.CapturePanicReport(func() {
		select {
		case tx <- ev:
		case <-h.ctx.Done():
		}
	})
	return nil
}

func (h *aiEditorHandler) Complete(ctx context.Context, name string, args []string) (
	iterator.Iterator[string], error,
) {
	slog.Debug("complete with", "name", name, "args", args)

	// Strip --all flag from args for length checks, pass it through to the completer.
	showAll := false
	filtered := args
	for i, a := range filtered {
		if a == "--all" {
			showAll = true
			filtered = slices.Delete(filtered, i, i+1)
			break
		}
	}

	switch name {
	case commandChat:
		switch len(filtered) {
		case 0, 1:
			return h.completeWithDialoguesIterator(ctx, showAll)
		case 2:
			return h.completeWithModelsIterator(ctx)
		default:
			return iterator.FromSlice[string](nil), nil
		}
	case commandModel:
		return h.completeWithModelsIterator(ctx)
	case commandEffort:
		levels := make([]string, len(validEffortLevels))
		for i, l := range validEffortLevels {
			levels[i] = string(l)
		}
		return iterator.FromSlice(levels), nil
	case commandSkill:
		skills := h.skillRegistry.List()
		names := make([]string, len(skills))
		for i, s := range skills {
			names[i] = s.Name
		}
		return iterator.FromSlice(names), nil
	default:
		return iterator.FromSlice[string](nil), nil
	}
}

func (h *aiEditorHandler) Close() error {
	closing := h.exit.CompareAndSwap(0, 1)
	if !closing {
		return nil
	}
	if h.sessionMgr != nil {
		_ = h.sessionMgr.Close()
	}
	h.cancelCtx()
	return h.mcpManager.Close()
}

func (h *aiEditorHandler) newDialogueComponent() *dialoguetui.Component {
	return dialoguetui.NewComponent(h.cfg)
}

func (h *aiEditorHandler) newAgentShell() textapi.REPLHandler {
	opts := []agentshell.Option{
		agentshell.WithMCPInfo(h.mcpManager),
		agentshell.WithHistorySystemPrompt(true),
		agentshell.WithEffort(h.getDefaultEffort, h.setDefaultEffort),
		agentshell.WithMaxTokens(h.getDefaultMaxTokens, h.setDefaultMaxTokens),
		agentshell.WithDreamModel(dreamModelAlias),
	}
	if h.auditStore != nil {
		opts = append(opts, agentshell.WithAuditStore(h.auditStore))
	}
	return agentshell.New(
		h.wm,
		h.llmSvc,
		h.defaultModel,
		h.dialogueStore, h.toolRegistry, h.agentsConfig,
		h.config,
		h.skillRegistry, h.cwd, h.fs,
		h.db, h.exec, h.lsp, h.parser, h.n,
		h.memoryDataPath,
		opts...,
	)
}

func (h *aiEditorHandler) handleChat(cmd textapi.Command) error {
	cmd.Args = filterAllFlag(cmd.Args)
	if len(cmd.Args) > 0 {
		if _, err := llmarg.Resolve(h.ctx, h.llmSvc, cmd.Args[0]); err == nil {
			return errors.New("model must be passed as a second argument to a dialogue ID, " +
				"check command manual for more details")
		}
	}
	model := h.defaultModel
	if len(cmd.Args) > 1 {
		model = cmd.Args[1]
		if _, err := llmarg.Resolve(h.ctx, h.llmSvc, model); err != nil {
			return err
		}
	}

	backendService, chatEntry, err := h.modelService(h.ctx, model)
	if err != nil {
		return fmt.Errorf("new backend: %v", err)
	}

	mu := new(sync.Mutex)
	ctx, cancel := context.WithCancel(h.ctx)

	d, err := h.getDialogue(ctx, h.dialogueStore, cmd)
	if err != nil {
		cancel()
		return err
	}
	if _, loaded := h.openChats.LoadOrStore(d.ID, struct{}{}); loaded {
		cancel()
		return fmt.Errorf("agent chat %q is already open", d.ID)
	}
	opened := false
	defer func() {
		if !opened {
			cancel()
			h.openChatAgents.Delete(d.ID)
			h.openChatTx.Delete(d.ID)
			h.openChats.Delete(d.ID)
		}
	}()

	// Create per-session spawner for sub-agent support.
	sessionKey := d.ID
	agentID := "default"
	serviceFactory := func(
		model string,
	) (llmapi.Service, llmapi.ModelEntry, error) {
		return h.modelService(h.ctx, model)
	}
	// Build a separate agentshell for the command adapter. It only needs
	// the base tools for display (e.g. /tools); it carries no mutable state.
	cmdRegistry := agent.NewRegistry(h.baseTools...)
	cmdShellOpts := []agentshell.Option{
		agentshell.WithMCPInfo(h.mcpManager),
	}
	if h.auditStore != nil {
		cmdShellOpts = append(cmdShellOpts, agentshell.WithAuditStore(h.auditStore))
	}
	cmdShell := agentshell.New(
		h.wm, backendService,
		model,
		h.dialogueStore, cmdRegistry, h.agentsConfig,
		h.config,
		h.skillRegistry, h.cwd, h.fs,
		h.db, h.exec, h.lsp, h.parser, h.n,
		h.memoryDataPath,
		cmdShellOpts...,
	)

	// Build command adapter with session-aware overrides.
	var comp *dialoguetui.Component
	hs := &hintSlot{} // shared with syncComponent for hint preservation during compaction
	adapter := newCommandAdapter(commandAdapterDeps{
		handler:       cmdShell,
		dialogueID:    d.ID,
		wm:            h.wm,
		store:         h.dialogueStore,
		llmSvc:        h.llmSvc,
		config:        h.config,
		ctx:           h.ctx,
		storage:       h.db,
		currentModel:  model,
		skillRegistry: h.skillRegistry,
		auditStore:    h.auditStore,
		mu:            mu,
		comp:          &comp,
		hintSlot:      hs,
		interrupter:   h.p,
	})

	comp = dialoguetui.NewComponent(h.cfg)

	// Replay dialogue history.
	pendingTools := make(map[string]llmapi.ToolCall)
	for _, msg := range d.Messages {
		addMessage(comp, msg, pendingTools)
	}

	dhandler, tx, rx := dialoguetui.Handler(ctx, mu, comp, h.p,
		dialoguetui.WithCommands(adapter),
		dialoguetui.WithCloseFunc(cancel),
	)

	// Build the agent registry now that tx is available for ask_user.
	prompter := &tuiPrompter{tx: tx, noti: h.n}
	askUser := agentools.NewAskUser(prompter)
	requestSkill := agentools.NewRequestSkill(prompter)
	exitPlan := agentools.NewExitPlan(h.plansDir, prompter)
	if h.generatePlanPath != nil {
		exitPlan.GeneratePlanPath = h.generatePlanPath
	}
	memRecaller := memory.NewGuardedRecaller(
		memory.NewRecaller(h.fs, h.executor, h.memoryPath), h.config,
	)
	spawner := agent.NewGoroutineSpawner(
		h.dialogueStore, serviceFactory,
		h.agentsConfig,
		h.skillRegistry,
		memRecaller,
		h.projectInstructions,
		sessionKey, agentID, h.cwd,
		prompter,
	)
	spawner.GenerateDialogueID = h.generateDialogueID
	childEvents := make(chan agent.ChildEvent, 64)
	sessionTools := agentools.SessionTools(spawner, spawner.ListAgents(), childEvents, h.skillRegistry)
	skillTool := agentools.NewSkillTool(h.skillRegistry, spawner, childEvents)
	taskStore := taskstore.New()
	progressUpdater := &tuiProgressUpdater{tx: tx}
	taskTools := agentools.NewTaskTools(taskStore, progressUpdater)
	allTools := make([]agent.Tool, 0, len(h.baseTools)+len(sessionTools)+len(taskTools)+4)
	allTools = append(allTools, h.baseTools...)
	allTools = append(allTools, sessionTools...)
	allTools = append(allTools, askUser)
	allTools = append(allTools, requestSkill)
	allTools = append(allTools, exitPlan)
	allTools = append(allTools, skillTool) // overrides nil-spawner skill tool from baseTools
	allTools = append(allTools, taskTools...)
	chatRegistry := agent.NewRegistry(allTools...)
	chatRegistry.CopyOverridesFrom(h.toolRegistry)
	chatRegistry.RegisterOverrides("openai",
		agentools.NewUpdatePlan(progressUpdater),
		agentools.NewRequestUserInput(prompter),
	)
	chatRegistry.RegisterOverrides("codex",
		agentools.NewUpdatePlan(progressUpdater),
		agentools.NewRequestUserInput(prompter),
	)
	chatRegistry.RegisterExclusions("openai",
		"TaskCreate", "TaskUpdate", "TaskGet", "TaskList", "ask_user_question",
	)
	chatRegistry.RegisterExclusions("codex",
		"TaskCreate", "TaskUpdate", "TaskGet", "TaskList", "ask_user_question",
	)
	spawner.SetRegistry(chatRegistry)
	spawner.SetHooks(h.hookRunner)

	syncComp := syncComponent{mu: mu, comp: comp, h: h, hintSlot: hs}
	h.openChats.Store(d.ID, syncComp)

	chatAgent := agent.NewAgent(
		backendService, chatRegistry, h.skillRegistry,
		h.dialogueStore, memRecaller, agent.Config{
			MaxToolOutputBytes:  h.maxToolOutputBytes,
			AutoCompactRatio:    h.autoCompactRatio,
			CompactSvc:          h.compactSvc,
			SystemPrompt:        h.systemPrompt + agent.ProviderToolAddendum(chatEntry.Provider),
			ProjectInstructions: h.projectInstructions,
			SessionKey:          sessionKey,
			AgentID:             agentID,
			Model:               chatEntry,
			Workspace:           h.cwd,
			Hooks:               h.hookRunner,
			Prompter:            prompter,
		},
	)
	if effort := h.getDefaultEffort(); effort != "" {
		chatAgent.SetEffort(effort)
	}
	if maxTokens := h.getDefaultMaxTokens(); maxTokens > 0 {
		chatAgent.SetMaxOutputTokens(maxTokens)
	}
	adapter.agent = chatAgent
	h.openChatAgents.Store(d.ID, chatAgent)
	h.openChatTx.Store(d.ID, tx)

	handler, msgRx := h.wrapDialogueHandler(ctx, syncComp, dhandler, rx)
	go debug.CapturePanicReport(func() {
		createAgentCompletions(ctx, cancel, tx, msgRx, chatAgent, spawner, childEvents, h.skillRegistry, d.ID, syncComp, h.n,
			makeOnCompacted(h.dialogueStore, adapter.compactFn),
			h.dialogueStore)
	})

	bhandler := browserapi.FuncHandler(handler, func() error {
		cancel()
		h.openChatAgents.Delete(d.ID)
		h.openChatTx.Delete(d.ID)
		h.openChats.Delete(d.ID)
		// SessionEnd hook (reason=tab_close): fire-and-forget.
		// Purely observational; no result fields are honored.
		h.hookRunner.Run(h.ctx, hooks.Payload{
			SessionID:     d.ID,
			Cwd:           h.cwd,
			HookEventName: hooks.EventSessionEnd,
			Reason:        "tab_close",
		})
		return nil
	})
	uri, err := getModelUri(d.ID, model)
	if err != nil {
		return err
	}
	tab, err := openChatTab(h.wm, uri, d.ID, bhandler)
	if err != nil {
		return err
	}

	if err := h.wm.SetWindowContent(cmd.Window, tab); err != nil {
		return fmt.Errorf("window set content: %v", err)
	}
	opened = true
	return nil
}

// openChatTab creates the rune-agent chat tab. The visible tab label is the
// dialogue's petname ID (e.g. "rolling-fox"), not the internal
// "rune-agent://<model>/<id>" URI, which would not be useful to users. The
// URI argument remains the tab's unique identity.
func openChatTab(
	wm browserapi.WindowManager, uri workspaceapi.URI, dialogueID string,
	h browserapi.Handler,
) (browserapi.Handler, error) {
	const icon = '󱫆'
	tab, err := wm.Tab(uri, icon, dialogueID, h)
	if err != nil {
		return nil, fmt.Errorf("create tab: %v", err)
	}
	return tab, nil
}

func getModelUri(id, model string) (workspaceapi.URI, error) {
	model = strings.ReplaceAll(model, "/", "_") // i.e. hf.co/org/model
	model = strings.ReplaceAll(model, ":", "_") // i.e. llama4:scout
	model = url.PathEscape(model)
	uriStr := fmt.Sprintf("rune-agent://%s/%s", model, id)
	return workspaceapi.ParseURI(uriStr)
}

func (h *aiEditorHandler) handleQuery(cmd textapi.Command) error {
	mu := new(sync.Mutex)
	comp := h.newDialogueComponent()
	queryID := strconv.Itoa(rand.Int())
	ctx, cancel := context.WithCancel(h.ctx)
	dhandler, tx, rx := dialoguetui.Handler(ctx, mu, comp, h.p)

	query := strings.Join(cmd.Args, " ")
	if query == "" {
		query = " " // avoid library omiting Content field when zero-valued
	}
	msg := llmapi.Message{Content: query, Role: llmapi.RoleUser}
	addMessage(comp, msg, nil)

	syncComp := syncComponent{mu: mu, comp: comp, h: h, hintSlot: &hintSlot{}}

	qrx := make(chan dialoguetui.SubmitMessage)
	handler, msgRx := h.wrapDialogueHandler(ctx, syncComp, dhandler, qrx)

	// wrap rx to enable sending query and so get
	// context cancelation for free
	go debug.CapturePanicReport(
		// do not store queries in store after user is done
		func() {

			defer h.dialogueStore.Delete(ctx, queryID) //nolint:errcheck

			select {
			case qrx <- dialoguetui.SubmitMessage{Text: query}:
			case <-ctx.Done():
				return
			}
			for {
				select {
				case msg := <-rx:
					select {
					case qrx <- msg:
					case <-ctx.Done():
						return
					}
				case <-ctx.Done():
					return
				}
			}

		})

	serviceFactory := func(model string) (llmapi.Service, llmapi.ModelEntry, error) {
		return h.modelService(h.ctx, model)
	}
	prompter := &tuiPrompter{tx: tx, noti: h.n}
	spawner := agent.NewGoroutineSpawner(
		h.dialogueStore, serviceFactory,
		h.agentsConfig,
		h.skillRegistry,
		agent.NoMemory(),
		h.projectInstructions,
		queryID, "query", h.cwd,
		prompter,
	)
	spawner.SetRegistry(agent.NewRegistry(h.baseTools...))
	spawner.GenerateDialogueID = h.generateDialogueID
	childEvents := make(chan agent.ChildEvent, 64)

	go debug.CapturePanicReport(func() {
		createAgentCompletions(ctx, cancel, tx, msgRx,
			h.queryAgent, spawner, childEvents, h.skillRegistry, queryID, syncComp, h.n, nil, h.dialogueStore)
	})

	var err error
	var win browserapi.Window
	bhandler := browserapi.FuncHandler(handler, func() error {
		cancel()
		if win != nil {
			return h.wm.CloseWindow(win)
		}
		return nil
	})
	floating := browserapi.FuncFloating(bhandler, func() (int, int) {
		mu.Lock()
		defer mu.Unlock()
		const width = 100
		return width, comp.Height(width)
	})
	floatingConfig := browserapi.FloatingConfig{
		Alignment: component.AlignmentCentered,
	}
	win, err = h.wm.Floating(floating, floatingConfig)
	if err != nil {
		return fmt.Errorf("floating window: %v", err)
	}
	return nil
}

func (h *aiEditorHandler) getDialogue(
	ctx context.Context, dialogueStore dialoguemanager.Store, cmd textapi.Command,
) (dialoguemanager.Dialogue, error) {
	dialogueID := parseDialogueID(cmd)
	if dialogueID == "" {
		// No user-provided ID — generate a unique one.
		dialogueID = dialoguemanager.GenerateUniqueID(ctx, dialogueStore, "")
	}
	d, err := dialogueStore.Get(ctx, dialogueID)
	if err != nil {
		if !errors.Is(err, storageapi.ErrNotFound) {
			return dialoguemanager.Dialogue{}, fmt.Errorf("get dialogue from store: %w", err)
		}
		d.ID = dialogueID
	}
	return d, nil
}

func (h *aiEditorHandler) completeWithDialoguesIterator(ctx context.Context, showAll bool) (
	iterator.Iterator[string], error,
) {
	listIt, err := h.dialogueStore.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("dialogue store list: %w", err)
	}

	all, err := iterator.ToSlice(ctx, listIt)
	if err != nil {
		return nil, fmt.Errorf("dialogue store list collect: %w", err)
	}

	dialogueTier := func(d dialoguemanager.DialogueHeader) int {
		ws, hasWS := d.Workspace()
		if hasWS && ws.Equal(h.cwd) {
			return 0
		}
		if hasWS && h.gitID.worktrees[ws.String()] != "" {
			return 1
		}
		if !hasWS {
			return 2
		}
		return 3
	}
	isForeign := func(d dialoguemanager.DialogueHeader) bool {
		ws, hasWS := d.Workspace()
		return hasWS && !ws.Equal(h.cwd) && h.gitID.worktrees[ws.String()] == ""
	}

	// Filter: drop empty IDs, sub-agents, and (unless showAll)
	// foreign-workspace dialogues.
	filtered := all[:0]
	for _, d := range all {
		if d.ID == "" || d.SubAgent {
			continue
		}
		if !showAll && isForeign(d) {
			continue
		}
		filtered = append(filtered, d)
	}

	// Current-workspace dialogues stay pinned above newer sibling worktree
	// dialogues so completions prefer the user's active context.
	sort.SliceStable(filtered, func(i, j int) bool {
		ti, tj := dialogueTier(filtered[i]), dialogueTier(filtered[j])
		if ti != tj {
			return ti < tj
		}
		return filtered[i].UpdatedAt.After(filtered[j].UpdatedAt)
	})

	// Map to display strings.
	out := make([]string, len(filtered))
	for i, d := range filtered {
		ws, hasWS := d.Workspace()
		switch {
		case !hasWS || ws.Equal(h.cwd):
			// Legacy dialogues are tagged so the user can tell they
			// have no workspace association.
			if !hasWS {
				out[i] = "<legacy>:" + d.ID
			} else {
				out[i] = d.ID
			}
		case h.gitID.worktrees[ws.String()] != "":
			out[i] = h.gitID.worktrees[ws.String()] + ":" + d.ID
		default:
			out[i] = ws.String() + ":" + d.ID
		}
	}
	return iterator.FromSlice(out), nil
}

func (h *aiEditorHandler) completeWithModelsIterator(ctx context.Context) (
	iterator.Iterator[string], error,
) {
	return iterator.Map(h.llmSvc.Models(), func(e llmapi.ModelEntry) string {
		return e.Provider + "/" + e.Name
	}), nil
}

func (h *aiEditorHandler) wrapDialogueHandler(
	ctx context.Context, comp syncComponent,
	dhandler tui.Handler, rx <-chan dialoguetui.SubmitMessage,
) (tui.Handler, <-chan completionRequest) {
	ret := make(chan completionRequest)

	// wrap dialogue.Handler's rx chan to add adhoc
	// cancelation of completion requests
	var cancel func()
	mu := new(sync.Mutex)

	go debug.CapturePanicReport(func() {

		for {
			select {
			case <-ctx.Done():
				return
			case msg := <-rx:
				reqCtx, cancelFn := context.WithCancel(ctx)
				mu.Lock()
				cancel = cancelFn
				mu.Unlock()
				select {
				case ret <- completionRequest{msg: msg.Text, skillName: msg.SkillName, ctx: reqCtx}:
				case <-ctx.Done():
					return
				}
			}
		}

	})

	// wrap it for ctrl-c cancelation of context
	return handler.Wrap(dhandler, func(ev term.Event) (exit bool, handled bool) {
		if ev.Ch == 'c' && ev.Mod == term.ModCtrl {
			// Let the dialogue handler dismiss any active prompt first.
			// Otherwise the prompt UI would stay visible while only the
			// underlying completion context is cancelled.
			exit, _ = dhandler.Handle(ev)
			mu.Lock()
			cancelFn := cancel
			cancel = nil
			mu.Unlock()
			if cancelFn != nil {
				cancelFn()
				_, _ = comp.h.n.Notify(browserapi.LevelInfo, "canceled completion request")
			}
			handled = true
			return
		}
		return dhandler.Handle(ev)
	}), ret
}

// parseDialogueID extracts a user-provided dialogue ID from the
// command args. Returns empty string when no ID was provided,
// signaling that a new unique ID should be generated.
func parseDialogueID(cmd textapi.Command) string {
	var id string
	for _, arg := range cmd.Args {
		if arg == "--all" || arg == "" {
			continue
		}
		id = arg
		break
	}
	if id == "" {
		return ""
	}
	// Strip workspace prefix (e.g. "worktree-name:myid" → "myid").
	if i := strings.IndexByte(id, ':'); i >= 0 {
		id = id[i+1:]
	}
	return id
}

// filterAllFlag returns args with "--all" removed.
func filterAllFlag(args []string) (out []string) {
	for _, a := range args {
		if a != "--all" {
			out = append(out, a)
		}
	}
	return out
}

// baseDialogueID strips the archive suffix from an archived dialogue ID.
// Handles both "foo-archived" and "foo-archived-2" formats produced by NextArchivedID.
func baseDialogueID(archivedID string) string {
	if i := strings.LastIndex(archivedID, "-archived"); i >= 0 {
		return archivedID[:i]
	}
	return archivedID
}

func replayMessages(d dialoguemanager.Dialogue) []llmapi.Message {
	// New dialogues persist the approved plan as a RoleUser anchor message
	// directly inside d.Messages (see clearContext / CompactDialogue), so we
	// must NOT re-inject when it is already present — that would duplicate
	// the plan in the TUI and in any path that feeds the LLM.
	//
	// Older dialogues (persisted before the fix) only kept the plan as
	// out-of-band metadata. For those we still inject a synthetic user
	// message after the system prompt so the conversation has a valid
	// user/assistant alternation.
	if d.ApprovedPlan == nil {
		return d.Messages
	}
	planMsg := llmapi.Message{
		Role:    llmapi.RoleUser,
		Content: fmt.Sprintf("Plan approved. Saved to %s\n\n%s", d.ApprovedPlan.Path, d.ApprovedPlan.Body),
	}
	if len(d.Messages) >= 2 &&
		d.Messages[1].Role == llmapi.RoleUser &&
		d.Messages[1].Content == planMsg.Content {
		return d.Messages
	}
	msgs := make([]llmapi.Message, 0, len(d.Messages)+1)
	if len(d.Messages) > 0 {
		msgs = append(msgs, d.Messages[0])
	}
	msgs = append(msgs, planMsg)
	if len(d.Messages) > 1 {
		msgs = append(msgs, d.Messages[1:]...)
	}
	return msgs
}

func addMessage(c *dialoguetui.Component, msg llmapi.Message, pendingTools map[string]llmapi.ToolCall) {
	switch msg.Role {
	case llmapi.RoleAssistant:
		if msg.ReasoningContent != "" {
			c.AddReasoningChunk(msg.ReasoningContent)
		}
		if msg.Content != "" {
			c.AddReceiveMessageChunk(msg.Content)
			c.AddReceiveMessageBreak()
		}
		for _, call := range msg.ToolCalls {
			if pendingTools != nil {
				pendingTools[call.ID] = call
			}
		}
	case llmapi.RoleUser:
		if replayed, ok := parseStoredCommandMessage(msg.Content); ok {
			c.AddSendMessage(replayed)
		} else if strings.HasPrefix(msg.Content, agent.CompactSummaryPrefix) ||
			strings.HasPrefix(msg.Content, "Plan approved. Saved to ") {
			c.AddSendMessageMarkdown(msg.Content)
		} else {
			c.AddSendMessage(msg.Content)
		}
	case llmapi.RoleSystem:
	case llmapi.RoleTool:
		id := msg.ToolCallID
		name := "tool"
		args := ""
		if pendingTools != nil {
			if call, ok := pendingTools[id]; ok {
				name = call.Function.Name
				args = call.Function.Arguments
				delete(pendingTools, id)
			}
		}
		if name == "update_plan" {
			var parsed struct {
				Plan []struct {
					Step   string `json:"step"`
					Status string `json:"status"`
				} `json:"plan"`
			}
			if err := json.Unmarshal([]byte(args), &parsed); err == nil {
				for i, step := range parsed.Plan {
					c.UpdateTaskProgress(dialoguetui.ProgressTaskEntry{
						ID:      fmt.Sprintf("plan:%d", i),
						Subject: step.Step,
						Status:  step.Status,
					})
				}
				for i := len(parsed.Plan); i < 32; i++ {
					c.UpdateTaskProgress(dialoguetui.ProgressTaskEntry{
						ID:     fmt.Sprintf("plan:%d", i),
						Status: "deleted",
					})
				}
				return
			}
		}
		c.AddToolCall(id, name, args, "")
		c.CompleteToolCall(id, name, args, "", msg.Content, false)
	}
}

func firstModel(svc llmapi.Service) string {
	it := svc.Models()
	defer func() { _ = it.Close() }()
	e, ok := it.Next(context.Background())
	if !ok {
		return ""
	}
	return e.Name
}

type completionRequest struct {
	msg       string
	skillName string
	ctx       context.Context
}

const truncatedTurnHint = "At high and max/xhigh effort levels, models may think more extensively and can be more likely to exhaust the max_tokens budget. Consider increasing max_tokens to give the model more room (/max_tokens 64000), or lowering the effort level (/effort medium)."

type turnOutcome uint8

const (
	turnOutcomeNone turnOutcome = iota
	turnOutcomeCompleted
	turnOutcomeTruncated
	turnOutcomeCanceled
	turnOutcomeError
)

func notifyTurnOutcome(noti browserapi.Notifications, outcome turnOutcome, reason string) {
	var level browserapi.NotificationLevel
	var msg string
	switch outcome {
	case turnOutcomeCompleted:
		level = browserapi.LevelSuccess
		msg = "Turn completed"
	case turnOutcomeTruncated:
		level = browserapi.LevelWarn
		msg = "Turn stopped after reaching the token limit"
	case turnOutcomeCanceled:
		return
	case turnOutcomeError:
		level = browserapi.LevelError
		if reason == "" {
			msg = "Turn failed"
		} else {
			msg = fmt.Sprintf("Turn failed: %s", reason)
		}
	default:
		return
	}
	if _, err := noti.Notify(level, "%s", msg); err != nil {
		slog.Error("notify", "error", err)
	}
}

func outcomeAndReasonForFinishReason(reason llmapi.FinishReason) (turnOutcome, string) {
	switch reason {
	case llmapi.FinishReasonStop:
		return turnOutcomeCompleted, ""
	case llmapi.FinishReasonLength:
		return turnOutcomeTruncated, ""
	case llmapi.FinishReasonToolCall:
		return turnOutcomeError, "the model stopped while requesting tool calls"
	case llmapi.FinishReasonContentFilter:
		return turnOutcomeError, "the response was blocked by a content filter"
	case llmapi.FinishReasonRefusal:
		return turnOutcomeError, "the model declined to continue"
	case llmapi.FinishReasonNull:
		return turnOutcomeError, "the response ended unexpectedly"
	default:
		return turnOutcomeError, fmt.Sprintf("unexpected finish reason: %s", reason)
	}
}

// makeOnCompacted returns a callback that fetches the compacted dialogue
// from the store and replays it in the TUI via compactFn. It is used by
// createAgentCompletions to handle EventCompacted from both the main
// agent loop and child (sub-agent) events.
func makeOnCompacted(store dialoguemanager.Store, compactFn func([]llmapi.Message)) func(string) {
	return func(dialogueID string) {
		compacted, err := store.Get(context.Background(), dialogueID)
		if err != nil {
			slog.Error("compacted: fetch dialogue", "id", dialogueID, "error", err)
			return
		}
		compactFn(replayMessages(compacted))
	}
}

// parentDialogueContextMessages loads the parent dialogue identified by
// dialogueID and returns the subset of its stored messages suitable for
// seeding a sub-agent whose skill opts into parent-context sharing.
// The returned slice:
//   - excludes messages with role system (the parent system prompt and
//     any transient system injections are not relevant to the sub-agent),
//   - excludes assistant tool-call messages and tool-result messages
//     (the child sub-agent has its own tool set, so forwarding parent
//     tool calls would produce dangling references),
//   - excludes an exact match of currentMessage appearing at the tail
//     (so the slash-command envelope message is not duplicated), and
//   - returns freshly copied llmapi.Message values.
//
// store MUST NOT be nil; passing a nil store is a developer error.
// Returns nil when the dialogue has not been persisted yet or on error.
func parentDialogueContextMessages(
	ctx context.Context, store dialoguemanager.Store,
	dialogueID, currentMessage string,
) []llmapi.Message {
	if store == nil {
		panic("parentDialogueContextMessages: store must not be nil")
	}
	if dialogueID == "" {
		return nil
	}
	d, err := store.Get(ctx, dialogueID)
	if err != nil {
		// Absent parent dialogue (not yet persisted) is expected on the
		// first turn; no context to forward.
		return nil
	}
	msgs := d.Messages
	// Drop a trailing occurrence of the current slash-command envelope
	// message if it has already been appended to the parent dialogue.
	if n := len(msgs); n > 0 {
		tail := msgs[n-1]
		if tail.Role == llmapi.RoleUser && tail.Content == currentMessage {
			msgs = msgs[:n-1]
		}
	}
	out := make([]llmapi.Message, 0, len(msgs))
	for _, m := range msgs {
		if m.Role == llmapi.RoleSystem || m.Role == llmapi.RoleTool {
			continue
		}
		// Drop assistant messages that exist solely to carry tool
		// calls; tool calls from the parent reference tools that may
		// not exist in the child sub-agent.
		if m.Role == llmapi.RoleAssistant && len(m.ToolCalls) > 0 && m.Content == "" && m.ReasoningContent == "" {
			continue
		}
		// For assistant messages that mix text and tool calls, strip
		// the tool calls and keep the text.
		if len(m.ToolCalls) > 0 {
			m.ToolCalls = nil
		}
		out = append(out, m)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func createAgentCompletions(
	ctx context.Context, cancel func(),
	tx chan<- dialoguetui.MessageEvent, rx <-chan completionRequest,
	ag *agent.Agent, spawner agent.Spawner,
	childEvents <-chan agent.ChildEvent,
	skillRegistry *skills.SkillRegistry,
	id string,
	syncComp syncComponent,
	noti browserapi.Notifications,
	onCompacted func(dialogueID string),
	parentStore dialoguemanager.Store,
) {
	if parentStore == nil {
		panic("createAgentCompletions: parentStore must not be nil")
	}
	// Forward child events from sub-agents to the TUI.
	// The goroutine must exit before we close tx to avoid
	// sending on a closed channel.
	var childWg sync.WaitGroup
	childWg.Go(func() {
		debug.CapturePanicReport(func() {
			for {
				select {
				case <-ctx.Done():
					return
				case cev, ok := <-childEvents:
					if !ok {
						return
					}
					var msg dialoguetui.MessageEvent
					switch cev.Event.Type {
					case agent.EventToolCall:
						msg = dialoguetui.MessageEvent{
							Type:             dialoguetui.MessageEventToolCall,
							ToolCallID:       cev.Event.ToolCallID,
							ToolName:         cev.Event.ToolName,
							ToolArgs:         cev.Event.ToolArgs,
							ToolSummary:      cev.Event.ToolSummary,
							ToolStartTime:    cev.Event.ToolStartTime,
							ParentToolCallID: cev.ParentToolCallID,
						}
					case agent.EventToolResult:
						msg = dialoguetui.MessageEvent{
							Type:             dialoguetui.MessageEventToolResult,
							ToolCallID:       cev.Event.ToolCallID,
							ToolName:         cev.Event.ToolName,
							ToolArgs:         cev.Event.ToolArgs,
							ToolSummary:      cev.Event.ToolSummary,
							ToolOutput:       cev.Event.ToolOutput,
							IsError:          cev.Event.IsError,
							ToolDuration:     cev.Event.ToolDuration,
							ParentToolCallID: cev.ParentToolCallID,
						}
					case agent.EventToolsDropped:
						msg = dialoguetui.MessageEvent{
							Type:               dialoguetui.MessageEventToolsDropped,
							DroppedToolCallIDs: cev.Event.DroppedToolCallIDs,
						}
					case agent.EventMemoryRecall:
						msg = agentMemoriesToTUI(cev.Event)
					case agent.EventCompacted:
						if cev.Event.ArchivedDialogueID != "" && onCompacted != nil {
							dialogueID := baseDialogueID(cev.Event.ArchivedDialogueID)
							onCompacted(dialogueID)
						}
						continue
					case agent.EventDone:
						msg = dialoguetui.MessageEvent{
							Type:             dialoguetui.MessageEventChildResult,
							ParentToolCallID: cev.ParentToolCallID,
							ToolOutput:       cev.Event.Text,
							IsError:          cev.Event.IsError,
						}
					default:
						continue
					}
					select {
					case tx <- msg:
					case <-ctx.Done():
						return
					}
				}
			}
		})
	})
	defer func() {
		cancel()
		childWg.Wait()
		close(tx)
	}()

	for {
		var req completionRequest
		select {
		case <-ctx.Done():
			return
		case req = <-rx:
		}
		// Signal the dialogue handler that the agent is busy so
		// follow-up messages are queued instead of sent directly.
		select {
		case tx <- dialoguetui.MessageEvent{Type: dialoguetui.MessageEventBusy, Busy: true}:
		case <-ctx.Done():
			return
		}
		hint := syncComp.addStatusHint()

		// Agent-type skills: spawn a sub-agent whose events are
		// rendered as top-level (tool calls, text, reasoning all
		// visible in the TUI).
		var it iterator.Iterator[agent.Event]
		if req.skillName != "" {
			if skill, ok := skillRegistry.Get(req.skillName); ok && skill.Type == "agent" {
				var allowedTools []string
				if skill.AllowedTools != "" {
					allowedTools = strings.Fields(skill.AllowedTools)
				}
				var initialMessages []llmapi.Message
				if skill.ParentContext {
					initialMessages = parentDialogueContextMessages(
						req.ctx, parentStore, id, req.msg,
					)
				}
				handle, skillErr := spawner.Run(req.ctx, agent.RunRequest{
					Label:           skill.Name,
					Model:           llmarg.Qualify(ag.ModelEntry()),
					Message:         req.msg,
					AllowedTools:    allowedTools,
					SystemPrompt:    skill.Body,
					InitialMessages: initialMessages,
				})
				if skillErr != nil {
					select {
					case tx <- dialoguetui.MessageEvent{Type: dialoguetui.MessageEventError, Text: skillErr.Error()}:
					case <-ctx.Done():
					}
					select {
					case tx <- dialoguetui.MessageEvent{Type: dialoguetui.MessageEventBreak}:
					case <-ctx.Done():
					}
					_, _ = noti.Notify(browserapi.LevelError, "skill %s: %v", skill.Name, skillErr)
					syncComp.removeStatusHint(hint)
					continue
				}
				it = handle.Events
			}
		}
		if it == nil {
			// UserPromptSubmit hook: fires before the agent loop
			// receives the prompt. A blocked decision cancels the
			// turn and surfaces the reason as an error to the TUI;
			// otherwise additionalContext is prepended to the prompt
			// for this turn (transient — never persisted).
			upsRes := ag.Hooks().Run(req.ctx, hooks.Payload{
				SessionID:     id,
				Cwd:           ag.Workspace(),
				HookEventName: hooks.EventUserPromptSubmit,
				Prompt:        req.msg,
			})
			if upsRes.Blocked() {
				reason := upsRes.Reason
				if reason == "" {
					reason = "blocked by UserPromptSubmit hook"
				}
				select {
				case tx <- dialoguetui.MessageEvent{
					Type: dialoguetui.MessageEventError, Text: reason,
				}:
				case <-ctx.Done():
				}
				select {
				case tx <- dialoguetui.MessageEvent{Type: dialoguetui.MessageEventBreak}:
				case <-ctx.Done():
				}
				syncComp.removeStatusHint(hint)
				continue
			}
			if upsRes.AdditionalContext != "" {
				req.msg = upsRes.AdditionalContext + "\n\n" + req.msg
			}
			var runOpts []agent.RunOption
			if req.skillName != "" {
				runOpts = append(runOpts, agent.WithSkillName(req.skillName))
			}
			it = ag.Run(req.ctx, id, req.msg, runOpts...)
		}
		var lastUsage agent.Event // track last usage event for post-turn hint
		func() {
			defer it.Close() //nolint:errcheck
			breakSent := false
			eventErrorSent := false
			// Always signal turn completion to the TUI when we
			// exit, even on context cancellation or abnormal agent
			// exit. AddReceiveMessageBreak is idempotent; a
			// duplicate after EventDone is harmless. This ensures
			// the turn's spinner animation stops and any dropped
			// tool-result events are cleaned up by completeRunning.
			defer func() {
				if breakSent {
					return
				}
				select {
				case tx <- dialoguetui.MessageEvent{Type: dialoguetui.MessageEventBreak}:
				case <-ctx.Done():
				}
			}()
			outcome := turnOutcomeNone
			outcomeReason := ""
			defer func() {
				if outcome == turnOutcomeNone {
					switch {
					case errors.Is(ctx.Err(), context.Canceled), errors.Is(req.ctx.Err(), context.Canceled):
						outcome = turnOutcomeCanceled
						outcomeReason = string(llmapi.FinishReasonNull)
					}
				}
				notifyTurnOutcome(noti, outcome, outcomeReason)
			}()
			for {
				ev, ok := it.Next(req.ctx)
				if !ok {
					break
				}
				switch ev.Type {
				case agent.EventMemoryRecall:
					select {
					case tx <- agentMemoriesToTUI(ev):
					case <-ctx.Done():
						return
					}
				case agent.EventInferenceStart:
					hint.setPhase(phaseSending)
				case agent.EventInferenceReady:
					hint.setPhase(phaseThinking)
				case agent.EventFirstContent:
					hint.setPhase(phaseReceiving)
				case agent.EventReasoning:
					if ev.Reasoning == "" {
						continue
					}
					select {
					case tx <- dialoguetui.MessageEvent{
						Type: dialoguetui.MessageEventReasoning,
						Text: ev.Reasoning,
					}:
					case <-ctx.Done():
						return
					}
				case agent.EventText:
					if ev.Text == "" {
						continue
					}
					select {
					case tx <- dialoguetui.MessageEvent{
						Type: dialoguetui.MessageEventText,
						Text: ev.Text,
					}:
					case <-ctx.Done():
						return
					}
				case agent.EventToolsStart:
					hint.setPhase(phaseToolCalling)
				case agent.EventToolCall:
					select {
					case tx <- dialoguetui.MessageEvent{
						Type:          dialoguetui.MessageEventToolCall,
						ToolCallID:    ev.ToolCallID,
						ToolName:      ev.ToolName,
						ToolArgs:      ev.ToolArgs,
						ToolSummary:   ev.ToolSummary,
						ToolStartTime: ev.ToolStartTime,
					}:
					case <-ctx.Done():
						return
					}
				case agent.EventToolResult:
					select {
					case tx <- dialoguetui.MessageEvent{
						Type:         dialoguetui.MessageEventToolResult,
						ToolCallID:   ev.ToolCallID,
						ToolName:     ev.ToolName,
						ToolArgs:     ev.ToolArgs,
						ToolSummary:  ev.ToolSummary,
						ToolOutput:   ev.ToolOutput,
						IsError:      ev.IsError,
						ToolDuration: ev.ToolDuration,
					}:
					case <-ctx.Done():
						return
					}
				case agent.EventToolsDropped:
					select {
					case tx <- dialoguetui.MessageEvent{
						Type:               dialoguetui.MessageEventToolsDropped,
						DroppedToolCallIDs: ev.DroppedToolCallIDs,
					}:
					case <-ctx.Done():
						return
					}
				case agent.EventRateLimitWarning:
					hint.setPhase(phaseRateLimited)
					if ev.RateLimit != nil {
						select {
						case tx <- dialoguetui.MessageEvent{
							Type: dialoguetui.MessageEventWarning,
							Text: ev.RateLimit.Message,
						}:
						case <-ctx.Done():
							return
						}
					}
				case agent.EventUsageUpdate:
					lastUsage = ev
					u := ev.Usage
					hint.setTokens(u.TokensSent, u.TokensReceived)
				case agent.EventCompacting:
					hint.setPhase(phaseCompacting)
				case agent.EventCompacted:
					hint.setPhase(phaseSending)
					onCompacted(id)
					if ev.ArchivedDialogueID != "" {
						select {
						case tx <- dialoguetui.MessageEvent{
							Type: dialoguetui.MessageEventWarning,
							Text: fmt.Sprintf(
								"Conversation compacted. Old conversation stored as **%s**.",
								ev.ArchivedDialogueID,
							),
						}:
						case <-ctx.Done():
							return
						}
					}
				case agent.EventDone:
					outcome, outcomeReason = outcomeAndReasonForFinishReason(ev.FinishReason)
					if ev.FinishReason == llmapi.FinishReasonLength {
						select {
						case tx <- dialoguetui.MessageEvent{
							Type: dialoguetui.MessageEventWarning,
							Text: truncatedTurnHint,
						}:
						case <-ctx.Done():
							return
						}
					}
					select {
					case tx <- dialoguetui.MessageEvent{
						Type: dialoguetui.MessageEventBreak,
					}:
						breakSent = true
					case <-ctx.Done():
						return
					}
				case agent.EventRefusal:
					outcome, outcomeReason = outcomeAndReasonForFinishReason(ev.FinishReason)
					select {
					case tx <- dialoguetui.MessageEvent{
						Type: dialoguetui.MessageEventWarning,
						Text: "The model declined to continue with this request.",
					}:
					case <-ctx.Done():
						return
					}
					select {
					case tx <- dialoguetui.MessageEvent{
						Type: dialoguetui.MessageEventBreak,
					}:
						breakSent = true
					case <-ctx.Done():
						return
					}
				case agent.EventError:
					if ev.Error != nil && !errors.Is(ev.Error, context.Canceled) {
						eventErrorSent = true
						outcome = turnOutcomeError
						outcomeReason = ev.Error.Error()
						errMsg := fmt.Sprintf("agent: %v", ev.Error)
						select {
						case tx <- dialoguetui.MessageEvent{Type: dialoguetui.MessageEventError, Text: errMsg}:
						case <-ctx.Done():
							return
						}
					}
				}
			}
			if err := it.Err(); err != nil {
				if !errors.Is(err, context.Canceled) &&
					!(eventErrorSent && strings.HasPrefix(err.Error(), "stream:")) {
					if outcome == turnOutcomeNone || outcome == turnOutcomeCompleted {
						outcome = turnOutcomeError
						outcomeReason = err.Error()
					}
					errMsg := fmt.Sprintf("agent stream: %v", err)
					select {
					case tx <- dialoguetui.MessageEvent{Type: dialoguetui.MessageEventError, Text: errMsg}:
					case <-ctx.Done():
						return
					}
				} else if outcome == turnOutcomeNone {
					outcome = turnOutcomeCanceled
					outcomeReason = string(llmapi.FinishReasonNull)
				}
			}
			if outcome == turnOutcomeNone {
				switch {
				case errors.Is(ctx.Err(), context.Canceled), errors.Is(req.ctx.Err(), context.Canceled):
					outcome = turnOutcomeCanceled
					outcomeReason = ""
				default:
					outcome = turnOutcomeError
					outcomeReason = "the response ended unexpectedly"
				}
			}
		}()
		syncComp.removeStatusHint(hint)
		if lastUsage.Context.TokensSent > 0 {
			syncComp.setContextHint(lastUsage)
		}
		// Signal that the agent is idle; the handler will drain
		// any queued follow-up messages at this point.
		select {
		case tx <- dialoguetui.MessageEvent{Type: dialoguetui.MessageEventBusy, Busy: false}:
		case <-ctx.Done():
			return
		}
	}
}

func readWorkspaceFile(wfs workspaceapi.FileSystem, path string) ([]byte, error) {
	f, err := wfs.OpenFile(path, os.O_RDONLY, 0)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	return io.ReadAll(f)
}

// agentMemoriesToTUI converts an agent EventMemoryRecall into a TUI
// MessageEvent, mapping []agent.Memory to []dialoguetui.MemoryRecallEntry.
func agentMemoriesToTUI(ev agent.Event) dialoguetui.MessageEvent {
	entries := make([]dialoguetui.MemoryRecallEntry, len(ev.Memories))
	for i, m := range ev.Memories {
		entries[i] = dialoguetui.MemoryRecallEntry{ID: m.ID, Content: m.Content}
	}
	return dialoguetui.MessageEvent{
		Type:           dialoguetui.MessageEventMemoryRecall,
		Memories:       entries,
		MemoryDuration: ev.MemoryDuration,
	}
}

// resolveInputBackgroundColor returns the compose input background color.
// It keeps def unless input_box_attr.bg is set to an explicit,
// non-default color, letting the shared input_box_attr drive the compose
// background without a dedicated key.
func resolveInputBackgroundColor(def term.Color, pconfig config.Config) term.Color {
	attr, err := config.GetAttributes(pconfig, "input_box_attr")
	if err != nil || attr.Bg == term.ColorDefault {
		return def
	}
	return attr.Bg
}
