// Unstable Build LLC ("COMPANY") CONFIDENTIAL
//
// Unpublished Copyright (c) 2017-2026 Unstable Build, All Rights Reserved.
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

package llmshell

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/unstablebuild/rune-go-sdk/api/browserapi"
	"github.com/unstablebuild/rune-go-sdk/api/storageapi/storagestub"
	"github.com/unstablebuild/rune-go-sdk/component"
	"github.com/unstablebuild/rune-go-sdk/handler/repl"
	"github.com/unstablebuild/rune-go-sdk/iterator"
	"github.com/unstablebuild/rune-go-sdk/term"
	"unstable.build/go-tui/llm/claude"
	"unstable.build/go-tui/llm/llmrouter"
)

// newProvidersHandlerForTest builds a providersHandler wired to a real
// router (for the key store) plus UI stubs.
func newProvidersHandlerForTest(t *testing.T) (
	*providersHandler, *stubWindowManager, *stubNotifications, *stubPromptOpener,
) {
	t.Helper()
	wm := &stubWindowManager{}
	notifs := &stubNotifications{}
	prompt := &stubPromptOpener{selectIdx: -1}
	h := newProvidersHandler(providersConfig{
		storage:  storagestub.NewInMemoryService(),
		router:   newRouterForTest(t),
		wm:       wm,
		notifs:   notifs,
		schedule: syncSchedule,
		prompt:   prompt,
	})
	h.spawn = func(fn func()) { fn() }
	return h, wm, notifs, prompt
}

// TestProvidersNilDependencyPanics verifies New refuses to build a
// handler against a nil dependency.
func TestProvidersNilDependencyPanics(t *testing.T) {
	defer func() {
		assert.NotNil(t, recover(), "expected panic for nil storage")
	}()
	router := newRouterForTest(t)
	_ = New(Config{
		Service:          router,
		Router:           router,
		LocalRegistry:    newRegistryForTest(t),
		Storage:          nil,
		WindowManager:    &stubWindowManager{},
		Notifications:    &stubNotifications{},
		ScheduleNextTick: syncSchedule,
	})
}

// renderedOnce asserts that the command produced exactly one rendered
// (markdown) responsive and no error, returning that single component.
func renderedOnce(
	t *testing.T, it iterator.Iterator[component.Responsive], err error,
) component.Responsive {
	t.Helper()
	require.NoError(t, err)
	got, err := iterator.ToSlice(context.Background(), it)
	require.NoError(t, err)
	require.Len(t, got, 1)
	return got[0]
}

// TestProvidersNoArgsShowsHelp renders the providers help page instead of
// erroring when invoked without a provider.
func TestProvidersNoArgsShowsHelp(t *testing.T) {
	h, _, _, _ := newProvidersHandlerForTest(t)
	it, err := h.HandleCommand(context.Background(),
		repl.Command{Name: "models providers", Args: nil}, nil)
	renderedOnce(t, it, err)
}

// TestProvidersUnknownProviderShowsHelp renders guidance for an unknown
// provider rather than returning a bare error.
func TestProvidersUnknownProviderShowsHelp(t *testing.T) {
	h, _, _, _ := newProvidersHandlerForTest(t)
	it, err := h.HandleCommand(context.Background(),
		repl.Command{Name: "models providers", Args: []string{"bogus", "status"}}, nil)
	renderedOnce(t, it, err)
}

// TestProvidersCodexNoArgsShowsHelp renders codex help when no codex
// subcommand is given.
func TestProvidersCodexNoArgsShowsHelp(t *testing.T) {
	h, _, _, _ := newProvidersHandlerForTest(t)
	it, err := h.HandleCommand(context.Background(),
		repl.Command{Name: "models providers", Args: []string{"codex"}}, nil)
	renderedOnce(t, it, err)
}

// TestProvidersClaudeNoArgsShowsHelp renders claude help when no claude
// subcommand is given.
func TestProvidersClaudeNoArgsShowsHelp(t *testing.T) {
	h, _, _, _ := newProvidersHandlerForTest(t)
	it, err := h.HandleCommand(context.Background(),
		repl.Command{Name: "models providers", Args: []string{"claude"}}, nil)
	renderedOnce(t, it, err)
}

// TestProvidersClaudeStatusDispatches renders claude status (not
// authenticated) without error.
func TestProvidersClaudeStatusDispatches(t *testing.T) {
	h, _, _, _ := newProvidersHandlerForTest(t)
	it, err := h.HandleCommand(context.Background(),
		repl.Command{Name: "models providers", Args: []string{"claude", "status"}}, nil)
	renderedOnce(t, it, err)
}

// TestFormatClaudeStatusUsageCreditNote verifies the authenticated status
// surfaces the Agent-SDK usage-credit guidance.
func TestFormatClaudeStatusUsageCreditNote(t *testing.T) {
	out := formatClaudeStatus(claude.AuthStatus{
		Authenticated: true,
		Email:         "dev@example.com",
		PlanType:      "max_20x",
	})
	assert.Contains(t, out, "dev@example.com")
	assert.Contains(t, out, "Agent-SDK credit")
	assert.Contains(t, out, "Settings > Usage")
}

// TestProvidersHostedNoArgsShowsHelp renders provider help when no hosted
// subcommand is given.
func TestProvidersHostedNoArgsShowsHelp(t *testing.T) {
	h, _, _, _ := newProvidersHandlerForTest(t)
	it, err := h.HandleCommand(context.Background(),
		repl.Command{Name: "models providers", Args: []string{"openai"}}, nil)
	renderedOnce(t, it, err)
}

// TestProvidersHostedUnknownSubcommandShowsHelp renders guidance for an
// unknown hosted subcommand instead of returning a bare error.
func TestProvidersHostedUnknownSubcommandShowsHelp(t *testing.T) {
	h, _, _, _ := newProvidersHandlerForTest(t)
	it, err := h.HandleCommand(context.Background(),
		repl.Command{Name: "models providers", Args: []string{"openai", "frobnicate"}}, nil)
	renderedOnce(t, it, err)
}

// TestProvidersHostedStatusEmpty reports no stored keys.
func TestProvidersHostedStatusEmpty(t *testing.T) {
	h, _, _, _ := newProvidersHandlerForTest(t)
	it, err := h.HandleCommand(context.Background(),
		repl.Command{Name: "providers", Args: []string{"anthropic", "status"}}, nil)
	require.NoError(t, err)
	got, err := iterator.ToSlice(context.Background(), it)
	require.NoError(t, err)
	require.Len(t, got, 1)
}

// TestProvidersHostedUseAndRemove drives use/remove against stored keys.
func TestProvidersHostedUseAndRemove(t *testing.T) {
	ctx := context.Background()
	h, _, _, _ := newProvidersHandlerForTest(t)
	require.NoError(t, h.router.AddProviderKey(ctx, llmrouter.ProviderOpenAI, "work", "k1", ""))
	require.NoError(t, h.router.AddProviderKey(ctx, llmrouter.ProviderOpenAI, "home", "k2", ""))

	_, err := h.HandleCommand(ctx,
		repl.Command{Name: "providers", Args: []string{"openai", "use", "home"}}, nil)
	require.NoError(t, err)
	active, err := h.router.ProviderKeyActiveName(ctx, llmrouter.ProviderOpenAI)
	require.NoError(t, err)
	assert.Equal(t, "home", active)

	_, err = h.HandleCommand(ctx,
		repl.Command{Name: "providers", Args: []string{"openai", "remove", "home"}}, nil)
	require.NoError(t, err)
	names, err := h.router.ProviderKeyNames(ctx, llmrouter.ProviderOpenAI)
	require.NoError(t, err)
	assert.Equal(t, []string{"work"}, names)

	_, err = h.HandleCommand(ctx,
		repl.Command{Name: "providers", Args: []string{"openai", "use"}}, nil)
	require.NoError(t, err, "use without a name renders help, not an error")
}

// TestProvidersComplete verifies the three completion levels.
func TestProvidersComplete(t *testing.T) {
	ctx := context.Background()
	h, _, _, _ := newProvidersHandlerForTest(t)
	require.NoError(t, h.router.AddProviderKey(ctx, llmrouter.ProviderOpenAI, "work", "k1", ""))
	require.NoError(t, h.router.AddProviderKey(ctx, llmrouter.ProviderOpenAI, "home", "k2", ""))

	level1, err := h.Complete(ctx, "providers", nil)
	require.NoError(t, err)
	names, err := iterator.ToSlice(ctx, level1)
	require.NoError(t, err)
	assert.ElementsMatch(t,
		[]string{"codex", "claude", "openai", "anthropic", "gemini", "bedrock"}, names)

	codexSubs, err := h.Complete(ctx, "providers", []string{"codex", ""})
	require.NoError(t, err)
	names, err = iterator.ToSlice(ctx, codexSubs)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"login", "status"}, names)

	claudeSubs, err := h.Complete(ctx, "providers", []string{"claude", ""})
	require.NoError(t, err)
	names, err = iterator.ToSlice(ctx, claudeSubs)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"login", "status"}, names)

	hostedSubs, err := h.Complete(ctx, "providers", []string{"openai", ""})
	require.NoError(t, err)
	names, err = iterator.ToSlice(ctx, hostedSubs)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"add", "remove", "use", "status"}, names)

	keyNames, err := h.Complete(ctx, "providers", []string{"openai", "use", ""})
	require.NoError(t, err)
	names, err = iterator.ToSlice(ctx, keyNames)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"home", "work"}, names)

	addNoNames, err := h.Complete(ctx, "providers", []string{"openai", "add", ""})
	require.NoError(t, err)
	names, err = iterator.ToSlice(ctx, addNoNames)
	require.NoError(t, err)
	assert.Empty(t, names, "add does not complete from stored names")
}

// TestProviderAddNoNameShowsHelp renders the add help when no key name is
// supplied, instead of returning a bare error.
func TestProviderAddNoNameShowsHelp(t *testing.T) {
	h, wm, _, _ := newProvidersHandlerForTest(t)
	it, err := h.HandleCommand(context.Background(),
		repl.Command{Name: "models providers", Args: []string{"openai", "add"}}, nil)
	renderedOnce(t, it, err)
	assert.Nil(t, wm.lastFloating, "no prompt should open without a key name")
}

// TestHostedProviderHelpIsRich asserts the openai help page explains the
// command, gives a usage line, documents each subcommand with its own
// synopsis, and includes a worked example block.
func TestHostedProviderHelpIsRich(t *testing.T) {
	man, ok := providerManual(llmrouter.ProviderOpenAI)
	require.True(t, ok)
	help := usageMarkdown(man)

	assert.Contains(t, help, "## `models providers openai`")
	assert.Contains(t, help, "**Usage:** `models providers openai")
	assert.Contains(t, help, "### Subcommands")
	assert.Contains(t, help, "`add <name>`")
	assert.Contains(t, help, "`remove <name>`")
	assert.Contains(t, help, "`use <name>`")
	assert.Contains(t, help, "tests it against the provider")
	assert.Contains(t, help, "### Examples")
	assert.Contains(t, help, "models providers openai add work")
}

// submitKey drives the floating inputbox to type key and press Enter.
func submitKey(t *testing.T, fl browserapi.Floating, key string) {
	t.Helper()
	for _, r := range key {
		fl.Handle(term.Event{Type: term.EventKey, Ch: r})
	}
	fl.Handle(term.Event{Type: term.EventKey, Key: term.KeyEnter})
}

// TestProviderAddVerifySuccessStores exercises the redacted prompt happy
// path: a key that verifies is stored and a success notification fires.
func TestProviderAddVerifySuccessStores(t *testing.T) {
	ctx := context.Background()
	h, wm, notifs, _ := newProvidersHandlerForTest(t)
	h.verify = func(context.Context, string, string, string) error { return nil }

	_, err := h.HandleCommand(ctx,
		repl.Command{Name: "providers", Args: []string{"openai", "add", "work"}}, nil)
	require.NoError(t, err)
	require.NotNil(t, wm.lastFloating, "expected a floating prompt to open")

	submitKey(t, wm.lastFloating, "sk-secret")

	active, err := h.router.ProviderKeyActive(ctx, llmrouter.ProviderOpenAI)
	require.NoError(t, err)
	assert.Equal(t, "sk-secret", active)
	require.NotEmpty(t, notifs.notes)
	assert.Equal(t, browserapi.LevelSuccess, notifs.notes[len(notifs.notes)-1].level)
}

// TestProviderAddBedrockRequiresRegion pins that the bedrock add flow
// refuses to open the key prompt until a region is supplied: keys are
// region-scoped, and storing one without its region reproduces the
// wrong-region 403 the region argument exists to prevent.
func TestProviderAddBedrockRequiresRegion(t *testing.T) {
	ctx := context.Background()
	h, wm, _, _ := newProvidersHandlerForTest(t)
	verifyCalled := false
	h.verify = func(context.Context, string, string, string) error {
		verifyCalled = true
		return nil
	}

	_, err := h.HandleCommand(ctx,
		repl.Command{Name: "providers", Args: []string{"bedrock", "add", "work"}}, nil)
	require.NoError(t, err)
	assert.Nil(t, wm.lastFloating, "no key prompt without a region")
	assert.False(t, verifyCalled)
}

// TestProviderAddBedrockStoresRegionWithKey exercises the happy path:
// the region rides through verification and lands in the keystore.
func TestProviderAddBedrockStoresRegionWithKey(t *testing.T) {
	ctx := context.Background()
	h, wm, _, _ := newProvidersHandlerForTest(t)
	var verifiedRegion string
	h.verify = func(_ context.Context, _, _, region string) error {
		verifiedRegion = region
		return nil
	}

	_, err := h.HandleCommand(ctx,
		repl.Command{Name: "providers", Args: []string{"bedrock", "add", "work", "eu-west-1"}}, nil)
	require.NoError(t, err)
	require.NotNil(t, wm.lastFloating)

	submitKey(t, wm.lastFloating, "bedrock-api-key-abc")

	assert.Equal(t, "eu-west-1", verifiedRegion, "verification must probe the key's region")
	regions, err := h.router.ProviderKeyRegions(ctx, llmrouter.ProviderBedrock)
	require.NoError(t, err)
	assert.Equal(t, "eu-west-1", regions["work"])
}

// TestProviderAddTrimsWhitespace asserts a key pasted with surrounding
// whitespace is trimmed before it is verified and stored, so the
// Authorization header never carries stray spaces or newlines.
func TestProviderAddTrimsWhitespace(t *testing.T) {
	ctx := context.Background()
	h, wm, _, _ := newProvidersHandlerForTest(t)
	var verified string
	h.verify = func(_ context.Context, _, key, _ string) error {
		verified = key
		return nil
	}

	_, err := h.HandleCommand(ctx,
		repl.Command{Name: "providers", Args: []string{"openai", "add", "work"}}, nil)
	require.NoError(t, err)
	require.NotNil(t, wm.lastFloating)

	submitKey(t, wm.lastFloating, "  sk-secret  ")

	assert.Equal(t, "sk-secret", verified, "verification must use the trimmed key")
	active, err := h.router.ProviderKeyActive(ctx, llmrouter.ProviderOpenAI)
	require.NoError(t, err)
	assert.Equal(t, "sk-secret", active, "stored key must be trimmed")
}

// TestProviderAddWhitespaceOnlyKeyAborts asserts a whitespace-only paste
// is treated as empty: nothing is verified or stored.
func TestProviderAddWhitespaceOnlyKeyAborts(t *testing.T) {
	ctx := context.Background()
	h, wm, _, _ := newProvidersHandlerForTest(t)
	verifyCalled := false
	h.verify = func(context.Context, string, string, string) error {
		verifyCalled = true
		return nil
	}

	_, err := h.HandleCommand(ctx,
		repl.Command{Name: "providers", Args: []string{"openai", "add", "work"}}, nil)
	require.NoError(t, err)
	submitKey(t, wm.lastFloating, "   ")

	assert.False(t, verifyCalled, "whitespace-only key must not be verified")
	_, err = h.router.ProviderKeyActive(ctx, llmrouter.ProviderOpenAI)
	require.ErrorIs(t, err, llmrouter.ErrAPIKeyNotSet)
}

// drainDone reports a channel that closes once it has been fully drained
// (Next returned false). Used to observe when providerAdd's completion
// iterator resolves.
func drainDone(
	t *testing.T, it iterator.Iterator[component.Responsive],
) <-chan struct{} {
	t.Helper()
	done := make(chan struct{})
	go func() {
		defer close(done)
		ctx := context.Background()
		for {
			if _, ok := it.Next(ctx); !ok {
				return
			}
		}
	}()
	return done
}

// TestProviderAddIteratorBlocksUntilStore proves the command's output
// iterator stays open until the asynchronous key prompt resolves: it does
// not complete at dispatch, only after the key is submitted and stored.
func TestProviderAddIteratorBlocksUntilStore(t *testing.T) {
	ctx := context.Background()
	h, wm, _, _ := newProvidersHandlerForTest(t)
	h.verify = func(context.Context, string, string, string) error { return nil }

	it, err := h.HandleCommand(ctx,
		repl.Command{Name: "providers", Args: []string{"openai", "add", "work"}}, nil)
	require.NoError(t, err)
	require.NotNil(t, wm.lastFloating, "expected a floating prompt to open")

	done := drainDone(t, it)
	select {
	case <-done:
		t.Fatal("iterator completed before the key was submitted")
	case <-time.After(50 * time.Millisecond):
	}

	submitKey(t, wm.lastFloating, "sk-secret")

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("iterator did not complete after the key was stored")
	}

	active, err := h.router.ProviderKeyActive(ctx, llmrouter.ProviderOpenAI)
	require.NoError(t, err)
	assert.Equal(t, "sk-secret", active)
}

// TestProviderAddIteratorUnblocksOnContextCancel proves a closed shell tab
// (cancelled context) releases the still-open completion iterator instead
// of leaking the draining goroutine.
func TestProviderAddIteratorUnblocksOnContextCancel(t *testing.T) {
	h, wm, _, _ := newProvidersHandlerForTest(t)
	h.verify = func(context.Context, string, string, string) error { return nil }

	it, err := h.HandleCommand(context.Background(),
		repl.Command{Name: "providers", Args: []string{"openai", "add", "work"}}, nil)
	require.NoError(t, err)
	require.NotNil(t, wm.lastFloating)

	cancelCtx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			if _, ok := it.Next(cancelCtx); !ok {
				return
			}
		}
	}()

	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("iterator did not unblock on context cancellation")
	}
}

// TestProviderAddIteratorCloseIsIdempotent proves Close releases a
// still-pending iterator and is safe to call more than once.
func TestProviderAddIteratorCloseIsIdempotent(t *testing.T) {
	h, wm, _, _ := newProvidersHandlerForTest(t)
	h.verify = func(context.Context, string, string, string) error { return nil }

	it, err := h.HandleCommand(context.Background(),
		repl.Command{Name: "providers", Args: []string{"openai", "add", "work"}}, nil)
	require.NoError(t, err)
	require.NotNil(t, wm.lastFloating)

	require.NoError(t, it.Close())
	require.NoError(t, it.Close())

	_, ok := it.Next(context.Background())
	assert.False(t, ok, "Next must report completion after Close")
}

// TestProviderAddVerifyFailOpensConfirm exercises the failure path: a key
// that fails verification opens a yes/no prompt through the shared
// PromptOpener; selecting "yes" stores it.
func TestProviderAddVerifyFailOpensConfirm(t *testing.T) {
	ctx := context.Background()
	h, wm, _, prompt := newProvidersHandlerForTest(t)
	prompt.selectIdx = -1 // capture the prompt without auto-selecting
	h.verify = func(context.Context, string, string, string) error {
		return errors.New("401 invalid api key")
	}

	_, err := h.HandleCommand(ctx,
		repl.Command{Name: "providers", Args: []string{"openai", "add", "work"}}, nil)
	require.NoError(t, err)
	require.NotNil(t, wm.lastFloating, "redacted key prompt should open")

	submitKey(t, wm.lastFloating, "sk-bad")

	// The confirmation prompt is opened through the shared PromptOpener,
	// not as a hand-rolled floating window.
	require.True(t, prompt.called, "a confirmation prompt should open")
	assert.Equal(t, []string{" yes ", " no "}, prompt.options)
	assert.Contains(t, prompt.message, "401 invalid api key")
	assert.Contains(t, prompt.message, "Do you still want to add it?")

	// Not stored until the user confirms.
	_, err = h.router.ProviderKeyActive(ctx, llmrouter.ProviderOpenAI)
	require.ErrorIs(t, err, llmrouter.ErrAPIKeyNotSet)

	// Select "yes" (option index 0).
	prompt.handler.OnSelect(0, prompt.options[0])

	active, err := h.router.ProviderKeyActive(ctx, llmrouter.ProviderOpenAI)
	require.NoError(t, err)
	assert.Equal(t, "sk-bad", active)
}

// TestProviderAddVerifyFailConfirmNo declines the confirmation and asserts
// the key is not stored.
func TestProviderAddVerifyFailConfirmNo(t *testing.T) {
	ctx := context.Background()
	h, wm, _, prompt := newProvidersHandlerForTest(t)
	prompt.selectIdx = -1
	h.verify = func(context.Context, string, string, string) error {
		return errors.New("401 invalid api key")
	}

	_, err := h.HandleCommand(ctx,
		repl.Command{Name: "providers", Args: []string{"openai", "add", "work"}}, nil)
	require.NoError(t, err)
	submitKey(t, wm.lastFloating, "sk-bad")

	require.True(t, prompt.called)
	prompt.handler.OnSelect(1, prompt.options[1]) // "no"

	_, err = h.router.ProviderKeyActive(ctx, llmrouter.ProviderOpenAI)
	require.ErrorIs(t, err, llmrouter.ErrAPIKeyNotSet)
}
