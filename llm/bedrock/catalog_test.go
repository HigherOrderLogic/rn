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
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/bedrock"
	bedrocktypes "github.com/aws/aws-sdk-go-v2/service/bedrock/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/unstablebuild/rune-go-sdk/api/llmapi"
)

type fakeControlPlane struct {
	profiles    []bedrocktypes.InferenceProfileSummary
	profilesErr error
	models      []bedrocktypes.FoundationModelSummary
	modelsErr   error
}

func (f *fakeControlPlane) ListInferenceProfiles(
	context.Context, *bedrock.ListInferenceProfilesInput, ...func(*bedrock.Options),
) (*bedrock.ListInferenceProfilesOutput, error) {
	if f.profilesErr != nil {
		return nil, f.profilesErr
	}
	return &bedrock.ListInferenceProfilesOutput{InferenceProfileSummaries: f.profiles}, nil
}

func (f *fakeControlPlane) ListFoundationModels(
	context.Context, *bedrock.ListFoundationModelsInput, ...func(*bedrock.Options),
) (*bedrock.ListFoundationModelsOutput, error) {
	if f.modelsErr != nil {
		return nil, f.modelsErr
	}
	return &bedrock.ListFoundationModelsOutput{ModelSummaries: f.models}, nil
}

func profile(id, status string, modelARNs ...string) bedrocktypes.InferenceProfileSummary {
	models := make([]bedrocktypes.InferenceProfileModel, 0, len(modelARNs))
	for _, arn := range modelARNs {
		models = append(models, bedrocktypes.InferenceProfileModel{ModelArn: aws.String(arn)})
	}
	return bedrocktypes.InferenceProfileSummary{
		InferenceProfileId: aws.String(id),
		Status:             bedrocktypes.InferenceProfileStatus(status),
		Models:             models,
	}
}

func foundationModel(id string, streaming bool) bedrocktypes.FoundationModelSummary {
	return bedrocktypes.FoundationModelSummary{
		ModelId:                    aws.String(id),
		ResponseStreamingSupported: aws.Bool(streaming),
	}
}

func drainCatalog(t *testing.T, c *catalogIterator) []llmapi.ModelEntry {
	t.Helper()
	var out []llmapi.ModelEntry
	for {
		entry, ok := c.Next(t.Context())
		if !ok {
			return out
		}
		out = append(out, entry)
	}
}

func newCatalog(api controlPlaneAPI) *catalogIterator {
	return &catalogIterator{
		list: func(ctx context.Context) ([]llmapi.ModelEntry, error) {
			return listCatalog(ctx, api)
		},
		fallback: ModelEntries(),
	}
}

func TestListCatalog_PrefersInferenceProfiles(t *testing.T) {
	api := &fakeControlPlane{
		profiles: []bedrocktypes.InferenceProfileSummary{
			profile(ClaudeSonnet45, string(bedrocktypes.InferenceProfileStatusActive),
				"arn:aws:bedrock:us-east-1::foundation-model/anthropic.claude-sonnet-4-5-20250929-v1:0"),
			profile("us.legacy-v1:0", "INACTIVE"),
		},
		models: []bedrocktypes.FoundationModelSummary{
			// Already routed by the profile above, so it must not appear twice.
			foundationModel("anthropic.claude-sonnet-4-5-20250929-v1:0", true),
			foundationModel("mistral.mistral-large-2407-v1:0", true),
			foundationModel("amazon.titan-embed-text-v2:0", false),
		},
	}

	entries := drainCatalog(t, newCatalog(api))
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name)
		assert.Equal(t, LLMProvider, e.Provider)
	}
	assert.Equal(t, []string{MistralLarge, ClaudeSonnet45}, names)
}

func TestListCatalog_ResolvesContextWindowFromStaticCatalog(t *testing.T) {
	api := &fakeControlPlane{
		models: []bedrocktypes.FoundationModelSummary{
			// Bare foundation-model ID of a catalog inference profile.
			foundationModel("amazon.nova-pro-v1:0", true),
			foundationModel("some.unknown-model-v1:0", true),
		},
	}

	entries := drainCatalog(t, newCatalog(api))
	require.Len(t, entries, 2)
	byName := map[string]int{}
	for _, e := range entries {
		byName[e.Name] = e.ContextWindow
	}
	assert.Equal(t, AvailableModels()[NovaPro], byName["amazon.nova-pro-v1:0"])
	assert.Zero(t, byName["some.unknown-model-v1:0"])
}

func TestCatalog_FallsBackWhenControlPlaneDenied(t *testing.T) {
	denied := errors.New("AccessDeniedException")
	c := newCatalog(&fakeControlPlane{profilesErr: denied})

	entries := drainCatalog(t, c)
	assert.Len(t, entries, len(AvailableModels()))
	// The static catalog is a complete answer, so the failure must not
	// propagate as an iterator error and truncate an aggregate.
	assert.NoError(t, c.Err())
}

func TestCatalog_KeepsProfilesWhenFoundationModelsDenied(t *testing.T) {
	c := newCatalog(&fakeControlPlane{
		profiles: []bedrocktypes.InferenceProfileSummary{
			profile(ClaudeOpus45, string(bedrocktypes.InferenceProfileStatusActive)),
		},
		modelsErr: errors.New("AccessDeniedException"),
	})

	entries := drainCatalog(t, c)
	require.Len(t, entries, 1)
	assert.Equal(t, ClaudeOpus45, entries[0].Name)
	assert.NoError(t, c.Err())
}

func TestCatalog_FallsBackOnEmptyLiveCatalog(t *testing.T) {
	c := newCatalog(&fakeControlPlane{})

	entries := drainCatalog(t, c)
	assert.Len(t, entries, len(AvailableModels()))
	assert.NoError(t, c.Err())
}
