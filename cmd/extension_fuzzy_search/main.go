// Unstable Build LLC ("COMPANY") CONFIDENTIAL
//
// Unpublished Copyright (c) 2017-2026 Unstable Build, All Rights Reserved.

package main

import (
	"fmt"
	"net/http"
	_ "net/http/pprof"

	log "github.com/sirupsen/logrus"
	"github.com/unstablebuild/rune-go-sdk/api/extensionapi"
	"unstable.build/go-tui/cmd/extension_fuzzy_search/extension"
)

var (
	// Tag is a compile-time variable
	Tag = "development"
	// Commit is a compile-time variable
	Commit = "HEAD"
	// Version is injected at compile time.
	Version string
)

func init() {
	Version = fmt.Sprintf("%s (HEAD is %s)", Tag, Commit)
}

func main() {
	go func() {
		log.Println(http.ListenAndServe("localhost:6061", nil))
	}()

	ext, meta := extension.NewExtension()
	meta.ExtensionVersion = Version
	if err := extensionapi.ServeWorkspaceExtension(ext, meta); err != nil {
		log.Fatal(err)
	}
}
