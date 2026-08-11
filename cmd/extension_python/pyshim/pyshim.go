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

// Package pyshim owns the venv-aware python/python3 shims the Python
// extension installs into <dataDir>/python/bin. The shims are consumed
// by every Rune terminal (the dir is first on PATH) and by the debugger
// launch template (debugger.python.launch.python), so the writer lives
// in its own package where both the extension and integration tests can
// use it.
package pyshim

import (
	"errors"
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
)

// names are the interpreter entrypoints shadowed by Rune-owned shims
// in Dir(dataDir), which is first on the Rune PATH. uv's own
// interpreter links and tool bins live in <dataDir>/python/uvbin (later
// on PATH), so the shim can fall back to the managed interpreter without
// the two fighting over one directory.
var names = []string{"python", "python3"}

// Dir returns the directory the shims are written to.
func Dir(dataDir string) string {
	return path.Join(dataDir, "python", "bin")
}

// FallbackPath returns the uv-managed interpreter the shims exec when
// no venv applies: the `python3` link uv installs into
// UV_PYTHON_BIN_DIR (<dataDir>/python/uvbin per the package config).
func FallbackPath(dataDir string) string {
	return path.Join(dataDir, "python", "uvbin", "python3")
}

// Write installs the venv-aware python/python3 shims under
// Dir(dataDir). It is idempotent and self-healing: every bootstrap
// rewrites the shims, which also migrates existing installs where uv's
// interpreter symlinks used to occupy python/bin.
func Write(fs workspaceapi.FileSystem, dataDir string) error {
	binDir := Dir(dataDir)
	if err := fs.MkdirAll(binDir, 0o755); err != nil {
		return fmt.Errorf("create shim dir: %w", err)
	}
	body := []byte(script(dataDir))
	for _, name := range names {
		p := path.Join(binDir, name)
		// Remove before create: on upgraded installs the path is a uv
		// symlink into the managed interpreter, and opening through it
		// would truncate the interpreter itself. A failed removal of an
		// existing entry aborts for the same reason.
		if err := fs.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove old %s: %w", p, err)
		}
		f, err := fs.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o755)
		if err != nil {
			return fmt.Errorf("create shim %s: %w", p, err)
		}
		_, werr := f.Write(body)
		cerr := f.Close()
		if werr != nil {
			return fmt.Errorf("write shim %s: %w", p, werr)
		}
		if cerr != nil {
			return fmt.Errorf("close shim %s: %w", p, cerr)
		}
	}
	return nil
}

// script renders the POSIX shim body. Resolution order: an activated
// venv, the nearest enclosing project .venv walking up from $PWD (which
// makes launches monorepo-correct per invocation), then the uv-managed
// interpreter embedded at write time.
func script(dataDir string) string {
	fallback := shQuote(FallbackPath(dataDir))
	return `#!/bin/sh
# Rune-managed Python shim: prefer the activated venv, then the nearest
# enclosing project venv, then the uv-managed interpreter.
if [ -n "$VIRTUAL_ENV" ] && [ -x "$VIRTUAL_ENV/bin/python" ]; then
	exec "$VIRTUAL_ENV/bin/python" "$@"
fi
d=$PWD
while :; do
	if [ -x "$d/.venv/bin/python" ]; then
		exec "$d/.venv/bin/python" "$@"
	fi
	n=$(dirname "$d")
	[ "$n" = "$d" ] && break
	d=$n
done
fallback=` + fallback + `
if [ -x "$fallback" ]; then
	exec "$fallback" "$@"
fi
echo 'rune: no Python found; open a .py file in Rune to bootstrap one' >&2
exit 127
`
}

// shQuote single-quotes s for safe embedding in a POSIX shell script.
func shQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
