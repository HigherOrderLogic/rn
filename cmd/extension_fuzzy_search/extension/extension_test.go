// Unstable Build LLC ("COMPANY") CONFIDENTIAL
//
// Unpublished Copyright (c) 2017-2026 Unstable Build, All Rights Reserved.

package extension

import (
	"testing"

	"github.com/unstablebuild/rune-go-sdk/api/extensionapi"
	"unstable.build/go-tui/cmd/extension_fuzzy_search/finder"
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
	if meta.ExtensionID != "fuzzy_search" {
		t.Fatalf("extension id = %q, want fuzzy_search", meta.ExtensionID)
	}
	if meta.ExtensionName != "Fuzzy Search" {
		t.Fatalf("extension name = %q, want Fuzzy Search", meta.ExtensionName)
	}
	if meta.ExtensionVersion == "" {
		t.Fatal("extension version is empty")
	}
	want := append([]extensionapi.Permission{
		extensionapi.PermissionCommands,
		extensionapi.PermissionConfig,
	}, finder.Permissions()...)
	for _, perm := range want {
		if _, ok := meta.Permissions[perm]; !ok {
			t.Fatalf("permission %q missing", perm)
		}
	}
}
