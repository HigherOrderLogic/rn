// Unstable Build LLC ("COMPANY") CONFIDENTIAL
//
// Unpublished Copyright (c) 2017-2026 Unstable Build, All Rights Reserved.

package extension

import (
	"testing"

	"github.com/unstablebuild/rune-go-sdk/api/extensionapi"
)

func TestNewExtensionMetadata(t *testing.T) {
	_, meta := NewExtension()

	if meta.DeveloperID == "" {
		t.Fatal("developer id is empty")
	}
	if meta.DeveloperEmail == "" {
		t.Fatal("developer email is empty")
	}
	if meta.DeveloperKey == "" {
		t.Fatal("developer key is empty")
	}
	if meta.ExtensionID != "chaos" {
		t.Fatalf("extension id = %q, want chaos", meta.ExtensionID)
	}
	if meta.ExtensionName == "" {
		t.Fatal("extension name is empty")
	}
	if meta.ExtensionVersion == "" {
		t.Fatal("extension version is empty")
	}
	for _, perm := range []extensionapi.Permission{
		extensionapi.PermissionBrowserWindowManager,
		extensionapi.PermissionEditor,
		extensionapi.PermissionCommands,
	} {
		if _, ok := meta.Permissions[perm]; !ok {
			t.Fatalf("permission %q missing", perm)
		}
	}
}
