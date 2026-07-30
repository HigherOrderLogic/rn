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

package ide

import (
	"context"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/ProtonMail/go-crypto/openpgp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/unstablebuild/rune-go-sdk/api/extensionapi"
	"github.com/unstablebuild/rune-go-sdk/api/storageapi"
	"github.com/unstablebuild/rune-go-sdk/api/storageapi/storagestub"
	"github.com/unstablebuild/rune-go-sdk/handler"
	"github.com/unstablebuild/rune-go-sdk/term"
	"unstable.build/go-tui/browser"
	"unstable.build/go-tui/browser/browsertest"
	"unstable.build/go-tui/ide/ideauthorizer"
	"unstable.build/go-tui/ide/pkgtrust"
)

type fakePromptOpener struct {
	option  string
	close   bool
	calls   int
	message string
	options []string
}

func (f *fakePromptOpener) Prompt(
	message string, options []string,
	bindings []term.KeyComb,
	promptHandler handler.PromptHandler,
) browser.Window {
	f.calls++
	f.message = message
	f.options = append([]string(nil), options...)
	if f.close {
		if err := promptHandler.OnClose(); err != nil {
			panic(err)
		}
		return browsertest.NopWindow()
	}
	for i, opt := range options {
		if opt == f.option {
			promptHandler.OnSelect(i, opt)
			return browsertest.NopWindow()
		}
	}
	panic("test prompt option was not provided")
}

func TestPermissionGrantor(t *testing.T) {
	t.Parallel()

	entity, err := openpgp.NewEntity("Trusted Publisher", "", "trusted@example.com", nil)
	require.NoError(t, err)
	trustedFingerprint := strings.ToUpper(hex.EncodeToString(entity.PrimaryKey.Fingerprint))
	trust := pkgtrust.NewStoreWithKeyring(openpgp.EntityList{entity})

	cases := []struct {
		name              string
		promptOption      string
		promptClose       bool
		storedDecision    string
		verifiedPublisher string
		wantGrant         bool
		wantPrompt        bool
		wantStored        string
	}{
		{
			name:         "allow once grants without persisting",
			promptOption: permissionPromptAllowOnce,
			wantGrant:    true,
			wantPrompt:   true,
		},
		{
			name:         "allow always grants and persists",
			promptOption: permissionPromptAllowAlways,
			wantGrant:    true,
			wantPrompt:   true,
			wantStored:   permissionDecisionAllow,
		},
		{
			name:         "deny once denies without persisting",
			promptOption: permissionPromptDenyOnce,
			wantPrompt:   true,
		},
		{
			name:           "stored allow bypasses prompt",
			promptOption:   permissionPromptDenyOnce,
			storedDecision: permissionDecisionAllow,
			wantGrant:      true,
		},
		{
			name:         "prompt close denies once",
			promptOption: permissionPromptAllowOnce,
			promptClose:  true,
			wantPrompt:   true,
		},
		{
			name:              "trusted publisher bypasses prompt",
			promptOption:      permissionPromptDenyOnce,
			verifiedPublisher: trustedFingerprint,
			wantGrant:         true,
		},
		{
			name:              "untrusted fingerprint prompts",
			promptOption:      permissionPromptAllowOnce,
			verifiedPublisher: strings.Repeat("AB", 20),
			wantGrant:         true,
			wantPrompt:        true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			grantor, prompt, storage, meta := newTestPermissionGrantor(tc.promptOption)
			grantor.trust = trust
			prompt.close = tc.promptClose

			if tc.storedDecision != "" {
				key := permissionStorageKey(meta)
				require.NoError(t, storage.Set(context.Background(), key,
					permissionDecision{Decision: tc.storedDecision}))
			}

			ok, err := grantor.Grant(meta, tc.verifiedPublisher)
			require.NoError(t, err)

			assert.Equal(t, tc.wantGrant, ok)
			if tc.wantPrompt {
				assert.Equal(t, 1, prompt.calls)
				assert.NotContains(t, prompt.options, "   Never   ")
				assert.Contains(t, prompt.message,
					"Allow extension **Test Extension** (v1.2.3) by **dev-id** to run?")
				assert.Contains(t, prompt.message, "It requests permission to:")
				assert.Contains(t, prompt.message, "- access editor buffers and file events")
				assert.Contains(t, prompt.message, "- use persistent storage")
			} else {
				assert.Zero(t, prompt.calls)
			}
			if tc.wantStored != "" {
				assertStoredPermissionDecision(t, storage, meta, tc.wantStored)
			} else if tc.storedDecision == "" {
				assertNoStoredPermissionDecision(t, storage, meta)
			}
		})
	}
}

func newTestPermissionGrantor(
	option string,
) (*permissionGrantor, *fakePromptOpener, storageapi.Service, extensionapi.Metadata) {
	prompt := &fakePromptOpener{option: option}
	storage := storagestub.NewInMemoryService()
	grantor := &permissionGrantor{
		promptOpener: prompt,
		storage:      storage,
		scheduleNextTick: func(fn func()) bool {
			fn()
			return true
		},
	}
	return grantor, prompt, storage, testPermissionMetadata()
}

func testPermissionMetadata() extensionapi.Metadata {
	return extensionapi.Metadata{
		DeveloperID:      "dev-id",
		DeveloperEmail:   "dev@example.com",
		DeveloperKey:     "dev-key",
		ExtensionID:      "test-extension",
		ExtensionName:    "Test Extension",
		ExtensionVersion: "v1.2.3",
		Permissions: extensionapi.NewPermissions(
			extensionapi.PermissionEditor,
			extensionapi.PermissionStorage,
		),
	}
}

func assertStoredPermissionDecision(
	t *testing.T, storage storageapi.Service, meta extensionapi.Metadata, expected string,
) {
	t.Helper()

	key := permissionStorageKey(meta)
	var stored permissionDecision
	require.NoError(t, storage.Get(context.Background(), key, &stored))
	assert.Equal(t, expected, stored.Decision)
}

func assertNoStoredPermissionDecision(
	t *testing.T, storage storageapi.Service, meta extensionapi.Metadata,
) {
	t.Helper()

	key := permissionStorageKey(meta)
	var stored permissionDecision
	err := storage.Get(context.Background(), key, &stored)
	assert.ErrorIs(t, err, storageapi.ErrNotFound)
}

var _ ideauthorizer.PromptOpener = (*fakePromptOpener)(nil)
