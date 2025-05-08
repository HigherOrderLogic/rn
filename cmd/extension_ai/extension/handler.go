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
	"errors"
	"fmt"
	"math/rand"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/unstablebuild/blue/ai/llm"
	"github.com/unstablebuild/blue/document"
	"github.com/unstablebuild/blue/iterator"
	"github.com/unstablebuild/blue/logging"
	"unstable.build/go-tui"
	browserapi "unstable.build/go-tui/api/browser"
	browserextension "unstable.build/go-tui/api/browser/extension"
	configapi "unstable.build/go-tui/api/config"
	configextension "unstable.build/go-tui/api/config/extension"
	storageextension "unstable.build/go-tui/api/storage/extension"
	textapi "unstable.build/go-tui/api/text"
	workspaceapi "unstable.build/go-tui/api/workspace"
	"unstable.build/go-tui/clipboard"
	"unstable.build/go-tui/clipboard/sysclip"
	aiDialogue "unstable.build/go-tui/cmd/extension_ai/dialogue"
	"unstable.build/go-tui/component"
	"unstable.build/go-tui/component/notifications"
	"unstable.build/go-tui/extension"
	"unstable.build/go-tui/extension/extutil"
	"unstable.build/go-tui/handler"
	"unstable.build/go-tui/handler/dialogue"
	"unstable.build/go-tui/handler/input"
	"unstable.build/go-tui/rpc"
	"unstable.build/go-tui/term"
	"unstable.build/go-tui/text"
)

const (
	commandQuery      = "?"
	commandChat       = "assistantChat"
	commandResetChat  = "assistantResetChat"
	defaultRPCTimeout = 20 * time.Second
	defaultChatName   = "default"
)

var (
	aIHandlerCommands = []textapi.CommandManual{
		{
			Name: commandQuery,
			Summary: "Send a coding question to your AI assistant. " +
				"The current active file is loaded and available in the model's context. " +
				"The default coding model used is configured via extension configuration. ",
			Synopsis: "[message]",
		},
		{
			Name: commandChat,
			Summary: "Open a new conversation tab with your AI assistant. " +
				"If no dialogue ID is provided, a new conversation is started. " +
				"If not passed, the default model used is configured via extension configuration. ",
			Synopsis: "[dialogue_id [model]]",
		},
		{
			Name: commandResetChat, Summary: "Clear all current chat's history.",
			Synopsis: "[dialogue_id]",
		},
	}
	aIHandlerEvents      = append(extutil.ResourceTrackerEventsComplete(), textapi.EventTypeUnfocus)
	aIHandlerPermissions = []extension.Permission{
		extension.PermissionBrowserWindowManager,
		extension.PermissionBrowserResourceOpener,
		extension.PermissionBrowserNotifications,
		extension.PermissionBrowserEventPublisher,
		extension.PermissionStorage,
		extension.PermissionEditor,
		extension.PermissionConfig,
	}
	defaultComponentCfg = dialogue.ComponentConfig{
		MessagesRowConfig: component.SpanConfig{
			PadHorizontal:    -80,
			PadVertical:      2,
			ContentAlignment: component.SpanAlignmentCentered,
		},
		InputRowColumns: 10,
		InputConfig: input.BoxConfig{
			Placeholder: "Message your assistant...",
			PlaceholderConfig: component.StringConfig{
				Alignment:            component.SpanAlignmentLeft,
				Attributes:           term.Attributes{Fg: 239},
				BackgroundAttributes: term.Attributes{},
			},
			ContentConfig:    term.Attributes{},
			DefaultFrameAttr: term.Attributes{},
		},
		ReceiveMessageStringConfig: component.StringConfig{
			Alignment:            component.SpanAlignmentLeft,
			Attributes:           term.Attributes{Fg: 250},
			BackgroundAttributes: term.Attributes{},
		},
		ReceiveMessageSpanConfig: component.SpanConfig{
			PadVertical:      1,
			ContentAlignment: component.SpanAlignmentLeft,
		},
		SendMessageStringConfig: component.StringConfig{
			Alignment:            component.SpanAlignmentLeft,
			Attributes:           term.Attributes{},
			BackgroundAttributes: term.Attributes{},
		},
		SendMessageSpanConfig: component.SpanConfig{
			PadVertical:      1,
			ContentAlignment: component.SpanAlignmentLeft,
		},
	}
	defaultOpts = []aiDialogue.Option{}
)

// CommandEventHandler returns a plugutil.CommandEventHandler that manages
// this extension's logic.
func CommandEventHandler(
	ctx context.Context, ed textapi.Editor, grants []extension.Grant,
	broker rpc.MuxBroker, pconfig configapi.Config,
	svcFn func(configapi.Config, map[string]int, string) (llm.Service, error),
	defaultAvailableModels map[string]int,
	defaultModel string,
	queryOptions ...aiDialogue.Option,
) (hret extutil.CommandEventHandler, err error) {
	ret := new(aiEditorHandler)
	ret.ctx, ret.cancelCtx = context.WithCancel(context.Background())
	ret.ed = ed
	ret.svcFn = svcFn
	ret.config = pconfig
	ret.defaultModel, err = pconfig.GetString("default_model")
	if err != nil {
		if err != configapi.ErrNotFound {
			ret.log(log.WarnLevel, "get 'model' from config: %v", err)
		}
		ret.defaultModel = defaultModel
	}
	ret.availableModels, err = configapi.GetMapInt(pconfig, "available_models")
	if err != nil {
		if err != configapi.ErrNotFound {
			err = fmt.Errorf("get 'available_models' from config: %v. ", err)
			return nil, err
		}
		ret.availableModels = defaultAvailableModels
	}

	if err := isAvailableModel(ret.availableModels, ret.defaultModel); err != nil {
		return nil, err
	}

	ret.cfg = defaultComponentCfg
	backgroundAttr, err := configapi.GetAttributes(pconfig, "background_attr")
	if err != nil {
		if err != configapi.ErrNotFound {
			ret.log(log.WarnLevel, "get 'background_attr' from extension config: %v", err)
		}
	}
	ret.cfg.InputConfig.PlaceholderConfig.BackgroundAttributes = backgroundAttr
	ret.cfg.ReceiveMessageStringConfig.BackgroundAttributes = backgroundAttr
	ret.cfg.SendMessageStringConfig.BackgroundAttributes = backgroundAttr
	ret.cfg.InputConfig.DefaultFrameAttr.Bg = backgroundAttr.Bg
	ret.cfg.InputConfig.PlaceholderConfig.Attributes.Bg = backgroundAttr.Bg
	ret.cfg.InputConfig.ContentConfig.Bg = backgroundAttr.Bg
	ret.cfg.ReceiveMessageStringConfig.Attributes.Bg = backgroundAttr.Bg
	ret.cfg.SendMessageStringConfig.Attributes.Bg = backgroundAttr.Bg
	ret.backgroundAttr = backgroundAttr

	opts := defaultOpts

	sendMsgAttr, err := configapi.GetAttributes(pconfig, "user_msg_attr")
	if err != nil {
		if err != configapi.ErrNotFound {
			ret.log(log.WarnLevel, "get 'user_msg_attr' from extension config: %v", err)
		}
	} else {
		ret.cfg.SendMessageStringConfig.Attributes = sendMsgAttr
	}
	recvMsgAttr, err := configapi.GetAttributes(pconfig, "assistant_msg_attr")
	if err != nil {
		if err != configapi.ErrNotFound {
			ret.log(log.WarnLevel, "get 'assistant_msg_attr' from extension config: %v", err)
		}
	} else {
		ret.cfg.ReceiveMessageStringConfig.Attributes = recvMsgAttr
	}

	inputBoxAttr, err := configapi.GetAttributes(pconfig, "input_box_attr")
	if err != nil {
		if err != configapi.ErrNotFound {
			ret.log(log.WarnLevel, "get 'input_box_attr' from extension config: %v", err)
		}
	} else {
		ret.cfg.InputConfig.ContentConfig = inputBoxAttr
	}
	inputBoxPlaceholderAttr, err := configapi.GetAttributes(pconfig, "input_box_placeholder_attr")
	if err != nil {
		if err != configapi.ErrNotFound {
			ret.log(log.WarnLevel, "Error getting 'input_box_placeholder_attr'"+
				" from extension config: %v", err)
		}
	} else {
		ret.cfg.InputConfig.PlaceholderConfig.Attributes = inputBoxPlaceholderAttr
	}

	inputBoxFrameAttr, err := configapi.GetAttributes(pconfig, "input_box_frame_attr")
	if err != nil {
		if err != configapi.ErrNotFound {
			ret.log(log.WarnLevel, "Error getting 'input_box_frame_attr' from extension config: %v", err)
		}
	} else {
		ret.cfg.InputConfig.DefaultFrameAttr = inputBoxFrameAttr
	}

	initialContext, err := pconfig.GetString("initial_context")
	if err != nil {
		if err != configapi.ErrNotFound {
			ret.log(log.WarnLevel, "Error getting 'initial_context' from extension config: %v", err)
		}
	} else {
		opts = append(opts, aiDialogue.WithInitialContext([]llm.ChatCompletionMessage{
			{
				Role:    llm.RoleSystem,
				Content: initialContext,
			},
		}))
	}

	ret.clip, err = sysclip.NewRegister()
	if err != nil {
		ret.log(log.WarnLevel, "system clipboard unsupported: %v", err)
		ret.clip = clipboard.NewInMemory()
	}

	for _, g := range grants {
		switch g.Permission {
		case extension.PermissionStorage:
			ret.db, err = storageextension.Storage(ctx, g, broker)
		case extension.PermissionBrowserEventPublisher:
			ret.p, err = browserextension.EventPublisher(ctx, g, broker)
		case extension.PermissionBrowserResourceOpener:
			ret.o, err = browserextension.ResourceOpener(ctx, g, broker)
		case extension.PermissionBrowserWindowManager:
			ret.wm, err = browserextension.WindowManager(ctx, g, broker)
		case extension.PermissionBrowserNotifications:
			ret.n, err = browserextension.Notifications(ctx, g, broker)
		case extension.PermissionConfig:
			config, err := configextension.FetchConfig(ctx, g, broker)
			if err != nil {
				return nil, err
			}
			ret.editor, err = extutil.Editor(ret.clip, config)
			if err != nil {
				ret.editor = text.DefaultSimpleEditor(ret.clip)
				ret.log(log.WarnLevel, "Could not get editor.mode from config: "+
					"%s.. Using 'modeless' editor.", err)
			}
			ret.cfg.InputEditor = ret.editor
			tabspaces, err := extutil.Tabspaces(config)
			if err != nil {
				return nil, fmt.Errorf("get configured tabspaces: %v", err)
			}
			wrap, err := extutil.Wrap(config)
			if err != nil {
				return nil, fmt.Errorf("get configured wrap mode: %v", err)
			}
			ret.tracker.Init(tabspaces, wrap)
			ret.log(log.DebugLevel, "initialized content tracker with tabspaces: %d and wrap mode: %v",
				tabspaces, wrap)
		}
		if err != nil {
			return nil, err
		}
	}
	ret.dialogueStore = aiDialogue.NewStore(ret.db)
	queryService, err := ret.svcFn(ret.config, ret.availableModels, ret.defaultModel)
	if err != nil {
		return nil, fmt.Errorf("new backend for query dialogues: %v", err)
	}
	opts = append(opts, queryOptions...)
	opts = append(opts, aiDialogue.WithCompleter((*aiEditorHandlerCompleter)(ret)))
	ret.queryDialogueManager = aiDialogue.NewManager(queryService, ret.dialogueStore, opts...)

	return ret, nil
}

type aiEditorHandler struct {
	exit                 atomic.Uint32
	availableModels      map[string]int
	defaultModel         string
	editor               text.Editor
	cfg                  dialogue.ComponentConfig
	backgroundAttr       term.Attributes
	dialogueStore        aiDialogue.Store
	tracker              extutil.ResourceTracker
	queryDialogueManager *aiDialogue.Manager

	clip   clipboard.Register
	svcFn  func(configapi.Config, map[string]int, string) (llm.Service, error)
	ed     textapi.Editor
	wm     browserapi.WindowManager
	n      browserapi.Notifications
	o      browserapi.ResourceOpener
	p      browserapi.EventPublisher
	db     document.Service
	config configapi.Config

	openChats sync.Map
	ctx       context.Context
	cancelCtx func()
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

	h.tracker.Handle(ctx, ev)

	var err error
	switch ev.Type {
	case textapi.EventTypeEdit, textapi.EventTypeFlush:
		focus, ok := h.tracker.Focus()
		if ok && ev.URI == focus.URI() {
			err = h.queryDialogueManager.AddContextResource(ctx, ev.URI, focus.Buffer().String())
		}
	case textapi.EventTypeFocus:
		res, ok := h.tracker.Resource(ev.URI)
		if !ok {
			h.log(log.WarnLevel, "could not find resource on focus event: %s", ev.URI)
			return
		}
		err = h.queryDialogueManager.AddContextResource(ctx, ev.URI, res.Buffer().String())
	case textapi.EventTypeUnfocus:
		err = h.queryDialogueManager.RemoveContextResource(ctx, ev.URI)
	}

	if err != nil {
		h.log(log.ErrorLevel, "dialogue manager: %s", ev.URI)
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
	case commandResetChat:
		return h.handleResetChat(cmd)
	}

	return nil
}

func (h *aiEditorHandler) Complete(ctx context.Context, name string, args []string) (
	iterator.Iterator[string], error,
) {
	h.log(log.TraceLevel, "complete %q with %#v", name, args)

	switch name {
	case commandChat:
		switch len(args) {
		case 0, 1:
			return h.completeWithDialoguesIterator(ctx)
		case 2:
			return h.completeWithModelsIterator(ctx)
		default:
			return iterator.FromSlice[string](nil), nil
		}
	case commandResetChat:
		if len(args) <= 1 {
			return h.completeWithDialoguesIterator(ctx)
		}
		return iterator.FromSlice[string](nil), nil
	default:
		return iterator.FromSlice[string](nil), nil
	}
}

func (h *aiEditorHandler) Close() error {
	closing := h.exit.CompareAndSwap(0, 1)
	if !closing {
		return nil
	}
	h.cancelCtx()
	return nil
}

func (h *aiEditorHandler) newDialogueComponent() *dialogue.Component {
	return dialogue.NewComponent(h.cfg)
}

func (h *aiEditorHandler) handleChat(cmd textapi.Command) error {
	if len(cmd.Args) > 0 {
		if err := isAvailableModel(h.availableModels, cmd.Args[0]); err == nil {
			return errors.New("Model must be passed as a second argument to a dialogue ID. " +
				"Check command manual for more details.")
		}
	}
	model := h.defaultModel
	if len(cmd.Args) > 1 {
		model = cmd.Args[1]
		if err := isAvailableModel(h.availableModels, model); err != nil {
			return err
		}
	}

	comp := h.newDialogueComponent()
	backendService, err := h.svcFn(h.config, h.availableModels, model)
	if err != nil {
		return fmt.Errorf("new backend: %v", err)
	}

	mu := new(sync.Mutex)

	// do not append queryOptions, the default ones suffice
	opts := defaultOpts
	dialogueManager := aiDialogue.NewManager(backendService, h.dialogueStore, opts...)
	dhandler, tx, rx := dialogue.Handler(mu, comp, h.p)

	ctx, cancel := context.WithCancel(h.ctx)
	d, err := h.getDialogue(ctx, h.dialogueStore, cmd)
	if err != nil {
		cancel()
		return err
	}

	syncComp := syncComponent{mu: mu, comp: comp, h: h}
	h.openChats.Store(d.ID, syncComp)

	// dialogue history
	for _, msg := range d.Messages {
		addMessage(comp, msg)
	}

	handler, msgRx := h.wrapDialogueHandler(ctx, syncComp, dhandler, rx)
	go createCompletions(ctx, cancel, tx, msgRx, dialogueManager, d.ID, syncComp, h.n)

	bhandler := browserapi.FuncHandler(handler, func() error {
		cancel()
		h.openChats.Delete(d.ID)
		return nil
	})
	uriStr := fmt.Sprintf("assistant://%s/%s", url.PathEscape(model), d.ID)
	uri, err := workspaceapi.ParseURI(uriStr)
	if err != nil {
		return err
	}
	const icon = '󱫆'
	tab, err := h.wm.Tab(uri, icon, uri.String(), bhandler)
	if err != nil {
		return fmt.Errorf("create tab: %v", err)
	}

	if err := cmd.Window.SetContent(tab); err != nil {
		return fmt.Errorf("window set content: %v", err)
	}
	return nil
}

func (h *aiEditorHandler) handleQuery(cmd textapi.Command) error {
	mu := new(sync.Mutex)
	comp := h.newDialogueComponent()
	dhandler, tx, rx := dialogue.Handler(mu, comp, h.p)

	queryID := strconv.Itoa(rand.Int())
	ctx, cancel := context.WithCancel(h.ctx)

	query := strings.Join(cmd.Args, " ")
	msg := llm.ChatCompletionMessage{Content: query, Role: llm.RoleUser}
	addMessage(comp, msg)

	syncComp := syncComponent{mu: mu, comp: comp, h: h}

	qrx := make(chan string)
	handler, msgRx := h.wrapDialogueHandler(ctx, syncComp, dhandler, qrx)

	// wrap rx to enable sending query and so get
	// context cancelation for free
	go func() {
		// do not store queries in store after user is done
		defer h.dialogueStore.Delete(ctx, queryID) //nolint:errcheck

		select {
		case qrx <- query:
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
	}()

	go createCompletions(ctx, cancel, tx, msgRx,
		h.queryDialogueManager, queryID, syncComp, h.n)

	var win browserapi.Window
	bhandler := browserapi.FuncHandler(handler, func() error {
		cancel()
		if win != nil {
			return win.Close()
		}
		return nil
	})
	floating := browserapi.FuncFloating(bhandler, func() (int, int) {
		const width = 100
		return width, comp.Height(width)
	})
	floatingConfig := component.FloatingConfig{
		Alignment: component.SpanAlignmentCentered,
	}
	win, err := h.wm.Floating(floating, floatingConfig)
	if err != nil {
		return fmt.Errorf("floating window: %v", err)
	}
	return nil
}

func (h *aiEditorHandler) getDialogue(
	ctx context.Context, dialogueStore aiDialogue.Store, cmd textapi.Command,
) (aiDialogue.Dialogue, error) {
	dialogueID := getDialogueID(cmd)
	d, err := dialogueStore.Get(ctx, dialogueID)
	if err != nil {
		if !errors.Is(err, document.ErrNotFound) {
			return aiDialogue.Dialogue{}, fmt.Errorf("get dialogue from store: %w", err)
		}
		d.ID = dialogueID
	}
	return d, nil
}

func (h *aiEditorHandler) handleResetChat(cmd textapi.Command) error {
	dialogueID := getDialogueID(cmd)
	err := h.dialogueStore.Delete(h.ctx, dialogueID)
	if err != nil {
		return fmt.Errorf("remove dialogue store: %w", err)
	}
	comp, ok := h.openChats.Load(dialogueID)
	if !ok {
		return fmt.Errorf("dialogue %q does not exist", dialogueID)
	}
	syncComp := comp.(syncComponent)
	syncComp.mu.Lock()
	syncComp.comp.Reset()
	syncComp.mu.Unlock()
	return nil
}

func (h *aiEditorHandler) log(level log.Level, msg string, args ...any) {
	log.WithFields(log.Fields{
		logging.KeyClass: "extension.aiEditorHandler",
	}).Logf(level, msg, args...)
}

func (h *aiEditorHandler) completeWithDialoguesIterator(ctx context.Context) (
	iterator.Iterator[string], error,
) {
	it, err := h.dialogueStore.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("dialoge store list: %w", err)
	}
	return iterator.Map(it, func(v aiDialogue.Dialogue) string {
		return v.ID
	}), nil
}

func (h *aiEditorHandler) completeWithModelsIterator(ctx context.Context) (
	iterator.Iterator[string], error,
) {
	models := make([]string, 0, len(h.availableModels))
	for name := range h.availableModels {
		models = append(models, name)
	}
	return iterator.FromSlice(models), nil
}

func (h *aiEditorHandler) wrapDialogueHandler(
	ctx context.Context, comp syncComponent,
	dhandler tui.Handler, rx <-chan string,
) (tui.Handler, <-chan completionRequest) {
	// use background as component of the final browserapi.Handler
	// ensuring its access is synchronized via component.Sync
	background := component.WithBackground(comp.comp, term.Cell{
		Attributes: h.backgroundAttr,
	})
	synced := component.Sync(comp.mu, background)
	withComp := handler.WithComponent(dhandler, synced)

	ret := make(chan completionRequest)

	// wrap dialogue.Handler's rx chan to add adhoc
	// cancelation of completion requests
	var cancel func()
	mu := new(sync.Mutex)

	go func() {
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
				case ret <- completionRequest{msg, reqCtx}:
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	// wrap it for ctrl-c cancelation of context
	return handler.Wrap(withComp, func(ev term.Event) (exit bool, handled bool) {
		if ev.Ch == 'c' && ev.Mod == term.ModCtrl {
			mu.Lock()
			cancelFn := cancel
			cancel = nil
			mu.Unlock()
			if cancelFn != nil {
				cancelFn()
				_ = comp.h.n.Notify(notifications.LevelInfo, "canceled completion request")
			}
			handled = true
			return
		}
		return withComp.Handle(ev)
	}), ret
}

// type alias avoids colliding Complete
type aiEditorHandlerCompleter aiEditorHandler

func (h *aiEditorHandlerCompleter) Complete(
	ctx context.Context, dialogueID, completionID string,
	reason llm.FinishReason, msg llm.ChatCompletionMessage,
) {
}

func drawMessage(
	ctx context.Context,
	it iterator.Iterator[string], tx chan<- string,
	noti browserapi.Notifications,
) {
	defer it.Close()
	for {
		response, ok := it.Next(ctx)
		if !ok {
			break
		}
		if response == "" {
			continue
		}
		select {
		case tx <- response:
		case <-ctx.Done():
			return
		}
	}
	if it.Err() != nil {
		if !errors.Is(it.Err(), context.Canceled) {
			err := noti.Notify(notifications.LevelError, "stream completion: %v", it.Err())
			if err != nil {
				log.Errorf("notify: %v", err)
			}
		}
	}

	// signal end of message
	select {
	case tx <- dialogue.EOM:
	case <-ctx.Done():
		return
	}
}

func getDialogueID(cmd textapi.Command) string {
	id := defaultChatName
	if len(cmd.Args) > 0 {
		id = cmd.Args[0]
	}
	return id
}

func addMessage(c *dialogue.Component, msg llm.ChatCompletionMessage) {
	switch msg.Role {
	case llm.RoleAssistant:
		c.AddReceiveMessageChunk(msg.Content)
		c.AddReceiveMessageBreak()
	case llm.RoleUser:
		c.AddSendMessage(msg.Content)
	// case llm.RoleSystem, llm.RoleTool:
	default:
		/* do not render */
	}
}

func availableModelsString(availableModels map[string]int) string {
	var availableStr strings.Builder
	var i int
	for k := range availableModels {
		if i != 0 {
			availableStr.WriteString(", ")
		}
		availableStr.WriteString(k)
		i++
	}
	return availableStr.String()
}

func isAvailableModel(available map[string]int, model string) error {
	if _, ok := available[model]; !ok {
		availableStr := availableModelsString(available)
		return fmt.Errorf("Model '%s' is not supported. Available models: %s",
			model, availableStr)
	}
	return nil
}

type completionRequest struct {
	msg string
	ctx context.Context
}

func createCompletions(
	ctx context.Context, cancel func(),
	tx chan<- string, rx <-chan completionRequest,
	dialogueManager *aiDialogue.Manager,
	id string, syncComp syncComponent,
	noti browserapi.Notifications,
) {
	defer close(tx)
	defer cancel()
	for {
		var req completionRequest
		select {
		case <-ctx.Done():
			return
		case req = <-rx:
		}
		cancelAnimation := syncComp.addWaitingAnimation()

		it, err := dialogueManager.CreateCompletion(req.ctx, id, []string{req.msg})
		if err != nil {
			cancelAnimation()
			if errors.Is(err, context.Canceled) {
				// if global ctx has been cancel, rather than req.ctx
				// then the next iteration will handle it
				continue
			}
			err := noti.Notify(notifications.LevelError,
				"dialogue manager create completion: %v", err)
			if err != nil {
				log.Errorf("notify: %v", err)
			}
			continue
		}
		drawMessage(ctx, it, tx, noti)
		cancelAnimation()
	}
}

type syncComponent struct {
	mu   *sync.Mutex
	comp *dialogue.Component
	h    *aiEditorHandler
}

func (s syncComponent) addWaitingAnimation() func() {
	frames := []string{".  ", ".. ", "...", " ..", "  .", "   "}
	seq := make([]int, len(frames))
	for i := range seq {
		seq[i] = i
	}
	animation := component.NewAnimation(s.h.p, frames, seq, 8)
	comp := component.WithBackground(animation, term.Cell{
		Attributes: s.h.backgroundAttr,
		Ch:         ' ', // override animation attributes
		Width:      1,
	})
	s.mu.Lock()
	defer s.mu.Unlock()

	s.comp.AddReceiveMessageHint(comp, component.SpanConfig{
		PadHorizontal:    -3,
		ContentAlignment: component.SpanAlignmentLeft,
	})

	return func() {
		animation.Close()

		s.mu.Lock()
		defer s.mu.Unlock()

		s.comp.RemoveReceiveMessageHint()
	}
}
