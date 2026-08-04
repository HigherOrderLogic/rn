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

package bedrock

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/bedrock"
	bedrocktypes "github.com/aws/aws-sdk-go-v2/service/bedrock/types"
	"github.com/unstablebuild/rune-go-sdk/api/llmapi"
)

// catalogTimeout bounds the control-plane round-trip behind a live catalog
// query.
const catalogTimeout = 5 * time.Second

// controlPlaneAPI is the subset of the Bedrock control plane the live
// catalog uses. Tests substitute a stub.
type controlPlaneAPI interface {
	ListInferenceProfiles(
		ctx context.Context, in *bedrock.ListInferenceProfilesInput, optFns ...func(*bedrock.Options),
	) (*bedrock.ListInferenceProfilesOutput, error)
	ListFoundationModels(
		ctx context.Context, in *bedrock.ListFoundationModelsInput, optFns ...func(*bedrock.Options),
	) (*bedrock.ListFoundationModelsOutput, error)
}

// catalogIterator serves the live catalog, falling back to the static one
// when the control plane is unreachable or the credentials cannot read it.
// The long-term API key policy does not always grant control-plane access,
// and bootstrap lists models before any key is verified, so an empty list
// would otherwise be the common case. A failed live query is logged rather
// than reported through Err: the static catalog is a complete answer, and
// callers aggregate this iterator with other providers' catalogs, which an
// error would truncate.
type catalogIterator struct {
	list     func(context.Context) ([]llmapi.ModelEntry, error)
	fallback []llmapi.ModelEntry

	entries []llmapi.ModelEntry
	index   int
	loaded  bool
}

func (c *catalogIterator) Next(ctx context.Context) (llmapi.ModelEntry, bool) {
	if !c.loaded {
		c.loaded = true
		entries, err := c.list(ctx)
		if err != nil {
			slog.Warn("bedrock: live model catalog unavailable, using static catalog",
				"error", err)
		}
		if err != nil || len(entries) == 0 {
			c.entries = c.fallback
		} else {
			c.entries = entries
		}
	}
	if c.index >= len(c.entries) {
		return llmapi.ModelEntry{}, false
	}
	entry := c.entries[c.index]
	c.index++
	return entry, true
}

func (c *catalogIterator) Err() error { return nil }

func (c *catalogIterator) Close() error { return nil }

// listCatalog queries the control plane for the models this account can
// invoke, preferring cross-region inference profiles over the raw
// foundation-model IDs they route to.
func listCatalog(ctx context.Context, api controlPlaneAPI) ([]llmapi.ModelEntry, error) {
	profiles, err := api.ListInferenceProfiles(ctx, &bedrock.ListInferenceProfilesInput{
		TypeEquals: bedrocktypes.InferenceProfileTypeSystemDefined,
	})
	if err != nil {
		return nil, fmt.Errorf("bedrock: list inference profiles: %w", err)
	}

	var entries []llmapi.ModelEntry
	routed := make(map[string]bool)
	for _, profile := range profiles.InferenceProfileSummaries {
		if profile.Status != bedrocktypes.InferenceProfileStatusActive {
			continue
		}
		id := derefString(profile.InferenceProfileId)
		if id == "" {
			continue
		}
		for _, m := range profile.Models {
			routed[modelIDFromARN(derefString(m.ModelArn))] = true
		}
		entries = append(entries, catalogEntry(id))
	}

	models, err := api.ListFoundationModels(ctx, &bedrock.ListFoundationModelsInput{
		ByOutputModality: bedrocktypes.ModelModalityText,
		ByInferenceType:  bedrocktypes.InferenceTypeOnDemand,
	})
	if err != nil {
		// Inference profiles alone are a usable catalog; a denied
		// ListFoundationModels should not discard them.
		return entries, nil
	}
	for _, m := range models.ModelSummaries {
		id := derefString(m.ModelId)
		if id == "" || routed[id] {
			continue
		}
		if m.ResponseStreamingSupported == nil || !*m.ResponseStreamingSupported {
			continue
		}
		if m.ModelLifecycle != nil &&
			m.ModelLifecycle.Status == bedrocktypes.FoundationModelLifecycleStatusLegacy {
			continue
		}
		entries = append(entries, catalogEntry(id))
	}

	slices.SortFunc(entries, func(a, b llmapi.ModelEntry) int {
		return strings.Compare(a.Name, b.Name)
	})
	return entries, nil
}

func catalogEntry(id string) llmapi.ModelEntry {
	return llmapi.ModelEntry{
		Name:          id,
		Provider:      LLMProvider,
		ContextWindow: contextWindowFor(id),
	}
}

// modelIDFromARN extracts the model identifier from a foundation-model ARN,
// whose last path segment is the model ID.
func modelIDFromARN(arn string) string {
	if i := strings.LastIndex(arn, "/"); i >= 0 {
		return arn[i+1:]
	}
	return arn
}

// contextWindowFor reports the nominal context window for a live catalog
// entry. The control plane does not report context windows, so the static
// catalog is consulted first by exact ID and then by region-stripped ID; an
// unknown model reports 0, which disables the client-side capacity check.
func contextWindowFor(id string) int {
	avail := AvailableModels()
	if n, ok := avail[id]; ok {
		return n
	}
	bare := stripRegionPrefix(id)
	for known, n := range avail {
		if stripRegionPrefix(known) == bare {
			return n
		}
	}
	return 0
}

// stripRegionPrefix removes the cross-region routing prefix ("us.", "eu.",
// "apac.") that distinguishes an inference profile from the foundation model
// it routes to.
func stripRegionPrefix(id string) string {
	for _, prefix := range []string{"us.", "eu.", "apac.", "us-gov."} {
		if rest, ok := strings.CutPrefix(id, prefix); ok {
			return rest
		}
	}
	return id
}
