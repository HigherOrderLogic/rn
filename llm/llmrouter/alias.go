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

package llmrouter

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/unstablebuild/rune-go-sdk/api/llmapi"
	"unstable.build/go-tui/llm/anthropic"
	"unstable.build/go-tui/llm/bedrock"
	"unstable.build/go-tui/llm/claude"
	"unstable.build/go-tui/llm/codex"
	"unstable.build/go-tui/llm/gemini"
	"unstable.build/go-tui/llm/openai"
)

// reservedAliasNames are the alias names the router recognizes even when
// unset. An unset reserved alias other than `default` resolves to
// whatever `default` resolves to; `default` itself falls back to the
// latest authenticated provider's flagship model.
var reservedAliasNames = []string{
	llmapi.DefaultModel,
	"query",
	"compact",
	"dream",
}

var reservedAliases = func() map[string]bool {
	m := make(map[string]bool, len(reservedAliasNames))
	for _, name := range reservedAliasNames {
		m[name] = true
	}
	return m
}()

// ReservedAliasNames returns the alias names the router resolves even
// when unset. The first element is always llmapi.DefaultModel.
func ReservedAliasNames() []string {
	return slices.Clone(reservedAliasNames)
}

func (r *Router) resolveAlias(
	ctx context.Context, model llmapi.ModelEntry,
) (llmapi.ModelEntry, bool, error) {
	if model.Provider != "" || model.Name == "" {
		return llmapi.ModelEntry{}, false, nil
	}
	entry, ok, err := r.aliasStore.get(ctx, model.Name)
	if err != nil {
		return llmapi.ModelEntry{}, false, err
	}
	if ok {
		return entry, true, nil
	}
	if !reservedAliases[model.Name] {
		return llmapi.ModelEntry{}, false, nil
	}
	// An unset reserved alias other than `default` defers to `default`,
	// which is the single knob users set; only `default` itself falls
	// back to the latest authenticated provider's flagship.
	if model.Name != llmapi.DefaultModel {
		return r.resolveAlias(ctx, llmapi.ModelEntry{Name: llmapi.DefaultModel})
	}
	if entry, found := r.latestAuthenticatedFlagship(ctx); found {
		return entry, true, nil
	}
	return llmapi.ModelEntry{}, false, nil
}

func flagshipFor(provider string) (string, bool) {
	switch provider {
	case ProviderOpenAI:
		return openai.FlagshipModel(), true
	case ProviderAnthropic:
		return anthropic.FlagshipModel(), true
	case ProviderGemini:
		return gemini.FlagshipModel(), true
	case ProviderBedrock:
		return bedrock.FlagshipModel(), true
	case ProviderCodex:
		return codex.FlagshipModel(), true
	case ProviderClaude:
		return claude.FlagshipModel(), true
	default:
		return "", false
	}
}

func (r *Router) latestAuthenticatedFlagship(ctx context.Context) (llmapi.ModelEntry, bool) {
	type candidate struct {
		provider string
		authAt   time.Time
	}
	order := []string{
		ProviderOpenAI,
		ProviderAnthropic,
		ProviderGemini,
		ProviderBedrock,
		ProviderCodex,
		ProviderClaude,
	}
	var best *candidate
	consider := func(provider string, at time.Time, ok bool) {
		if !ok {
			return
		}
		if best == nil || at.After(best.authAt) {
			best = &candidate{provider: provider, authAt: at}
		}
	}
	for _, provider := range order {
		switch provider {
		case ProviderOpenAI, ProviderAnthropic, ProviderGemini, ProviderBedrock:
			at, ok, err := r.store.activeUpdatedAt(ctx, provider)
			if err != nil {
				continue
			}
			consider(provider, at, ok)
		case ProviderCodex:
			if st, err := codex.Status(ctx, r.storage); err == nil && st.Authenticated {
				consider(provider, st.LastRefresh, true)
			}
		case ProviderClaude:
			if st, err := claude.Status(ctx, r.storage); err == nil && st.Authenticated {
				consider(provider, st.LastRefresh, true)
			}
		}
	}
	if best != nil {
		if name, ok := flagshipFor(best.provider); ok {
			if entry, err := r.GetModel(ctx, llmapi.ModelEntry{Provider: best.provider, Name: name}); err == nil {
				return entry, true
			}
		}
	}
	return r.firstModel(ctx)
}

func (r *Router) firstModel(ctx context.Context) (llmapi.ModelEntry, bool) {
	it := r.Models()
	defer func() { _ = it.Close() }()
	entry, ok := it.Next(ctx)
	return entry, ok
}

// SetAlias resolves target and stores the canonical entry as name's
// resolution. target must be a `provider/name` string.
func (r *Router) SetAlias(ctx context.Context, name, target string) error {
	provider, modelName, ok := strings.Cut(target, "/")
	if !ok || provider == "" || modelName == "" {
		return fmt.Errorf("llmrouter: alias target %q must be in provider/name form", target)
	}
	entry, err := r.GetModel(ctx, llmapi.ModelEntry{Provider: provider, Name: modelName})
	if err != nil {
		if errors.Is(err, llmapi.ErrModelNotFound) {
			return fmt.Errorf("llmrouter: alias target %q is not an available model", target)
		}
		return err
	}
	return r.aliasStore.set(ctx, name, entry)
}

// RemoveAlias deletes a named alias. It returns ErrAliasNotFound when
// the alias is not set.
func (r *Router) RemoveAlias(ctx context.Context, name string) error {
	return r.aliasStore.remove(ctx, name)
}

// Aliases returns a copy of every stored alias mapping to its resolved
// model entry.
func (r *Router) Aliases(ctx context.Context) (map[string]llmapi.ModelEntry, error) {
	return r.aliasStore.all(ctx)
}
