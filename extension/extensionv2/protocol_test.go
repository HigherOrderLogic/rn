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

package extensionv2

import (
	"context"
	"encoding/json"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/unstablebuild/blue/auth"
	"github.com/unstablebuild/rune-go-sdk/api/config"
	"github.com/unstablebuild/rune-go-sdk/api/extensionapi"
	"unstable.build/go-tui/extension"
	"unstable.build/go-tui/ide/ideauthorizer"
)

func TestProtocolGrantorGatesRunButKeepsRequestedPermissions(t *testing.T) {
	t.Parallel()

	keys, err := auth.GenerateKeys()
	require.NoError(t, err)
	verifyKeys, err := keys.Verify(context.Background())
	require.NoError(t, err)
	require.NotEmpty(t, verifyKeys)
	meta := extensionapi.Metadata{
		DeveloperID:    "dev-id",
		DeveloperEmail: "dev@example.com",
		DeveloperKey:   "dev-key",
		ExtensionID:    "ext-id",
		ExtensionName:  "Test Extension",
		Permissions: extensionapi.NewPermissions(
			extensionapi.PermissionLSP,
			extensionapi.PermissionStorage,
		),
	}
	p := newProtocol(context.Background(), extension.GrantAll(),
		"ext-id", "/tmp/rune.sock", "/tmp/rune-data", "/tmp/rune-install",
		[]byte("cert"),
		false, config.MapConfig(map[string]any{}), keys, nil, "")
	encoded, err := json.Marshal(meta)
	require.NoError(t, err)

	_, err = p.Write(encoded)
	require.NoError(t, err)
	var cfg extensionapi.Config
	data, err := io.ReadAll(p)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(data, &cfg))
	require.NotNil(t, cfg.Token)
	assert.Equal(t, "/tmp/rune-install", cfg.InstallDir,
		"handshake carries the install dir distinct from the data dir")
	assert.Equal(t, "/tmp/rune-data", cfg.DataDir)
	claims, err := auth.VerifyToken[ideauthorizer.Extension](verifyKeys[0], cfg.Token.AccessToken)
	require.NoError(t, err)

	assert.False(t, claims.Extra.Plugin)
	assert.Equal(t, meta.Permissions, claims.Extra.Permissions)
}

func TestProtocolCarriesVerifiedPublisherOnlyWhenDeveloperKeyMatches(t *testing.T) {
	t.Parallel()

	keys, err := auth.GenerateKeys()
	require.NoError(t, err)
	verifyKeys, err := keys.Verify(context.Background())
	require.NoError(t, err)
	meta := extensionapi.Metadata{
		DeveloperID: "dev-id", DeveloperEmail: "dev@example.com",
		DeveloperKey: "064D4ABCFA6D9338", ExtensionID: "ext-id",
		ExtensionName: "Test Extension", Permissions: extensionapi.NewPermissions(extensionapi.PermissionStorage),
	}
	trustedFingerprint := "D3F9E65DE72888CC03D45CF5064D4ABCFA6D9338"
	p := newProtocol(context.Background(), extension.GrantAll(),
		"ext-id", "/tmp/rune.sock", "/tmp/rune-data", "/tmp/rune-install",
		[]byte("cert"), false, config.MapConfig(map[string]any{}), keys, nil, trustedFingerprint)
	encoded, err := json.Marshal(meta)
	require.NoError(t, err)
	_, err = p.Write(encoded)
	require.NoError(t, err)
	data, err := io.ReadAll(p)
	require.NoError(t, err)
	var cfg extensionapi.Config
	require.NoError(t, json.Unmarshal(data, &cfg))
	claims, err := auth.VerifyToken[ideauthorizer.Extension](verifyKeys[0], cfg.Token.AccessToken)
	require.NoError(t, err)
	assert.Equal(t, trustedFingerprint, claims.Extra.VerifiedPublisher)

	meta.DeveloperKey = "different"
	p = newProtocol(context.Background(), extension.GrantAll(),
		"ext-id", "/tmp/rune.sock", "/tmp/rune-data", "/tmp/rune-install",
		[]byte("cert"), false, config.MapConfig(map[string]any{}), keys, nil, trustedFingerprint)
	encoded, err = json.Marshal(meta)
	require.NoError(t, err)
	_, err = p.Write(encoded)
	require.NoError(t, err)
	data, err = io.ReadAll(p)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(data, &cfg))
	claims, err = auth.VerifyToken[ideauthorizer.Extension](verifyKeys[0], cfg.Token.AccessToken)
	require.NoError(t, err)
	assert.Empty(t, claims.Extra.VerifiedPublisher)
}
