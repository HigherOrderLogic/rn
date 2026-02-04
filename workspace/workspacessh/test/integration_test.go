// Unstable Build LLC ("COMPANY") CONFIDENTIAL
//
// Unpublished Copyright (c) 2017-2024 Unstable Build, All Rights Reserved.
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

package workspacetest

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/unstablebuild/rune-go-sdk/api/config"
	"github.com/unstablebuild/rune-go-sdk/api/schemeapi"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"unstable.build/go-tui/workspace/walkdir"
	"unstable.build/go-tui/workspace/workspacessh"
	"unstable.build/go-tui/workspace/workspacetest"
)

// NOTE if this is failing or you are iterating on functionality
// used by SSH, remember to call make run build_docker.sh before running these tests again.
func TestIntegrationScheme(t *testing.T) {
	if os.Getenv("CI") == "true" {
		t.SkipNow()
	}
	hostname, teardown := runDockerOrSkip(t)
	t.Cleanup(func() { teardown() })

	cfgs := map[string]config.Config{
		"openssh_proc_remote": config.MapConfig(map[string]interface{}{
			"command": "ssh -o StrictHostKeyChecking=no -i ./id_ed25519 %h -p %p",
			"timeout": "20s",
		}),
		"go_stdlib_remote": config.MapConfig(map[string]interface{}{
			"private_keys": []interface{}{"./id_ed25519"},
			"timeout":      "20s",
			"insecure":     true,
		}),
	}
	for desc, cfg := range cfgs {
		cfg := cfg
		t.Run(desc, func(t *testing.T) {
			workspacetest.TestWorkspaceSchemeFiles(t, func(t *testing.T) schemeapi.Scheme {
				return newSchemeIntegration(t, hostname, cfg)
			})

			workspacetest.TestWorkspaceSchemeExecutor(t, func(t *testing.T) schemeapi.Scheme {
				return newSchemeIntegration(t, hostname, cfg)
			})
		})
	}
}

func teardownFn(container string) func() error {
	return func() error {
		cmd := exec.Cmd{
			Path: "stop_docker.sh",
			Args: []string{"stop_docker.sh", container},
		}
		err := cmd.Run()
		if err != nil {
			return fmt.Errorf("stop_docker.sh %q", container)
		}
		return nil
	}
}

func runContainer() (string, func() error, error) {
	cmd := exec.Cmd{
		Path: "run_docker.sh",
		Dir:  ".",
	}
	pipe, err := cmd.StdoutPipe()
	if err != nil {
		return "", nil, err
	}
	errPipe, err := cmd.StderrPipe()
	if err != nil {
		return "", nil, err
	}

	err = cmd.Start()
	if err != nil {
		return "", nil, err
	}

	data, err := io.ReadAll(pipe)
	errdata, _ := io.ReadAll(errPipe)
	if err != nil {
		_ = cmd.Wait()
		err = fmt.Errorf("%v: %s", err, string(errdata))
		return "", nil, err
	}

	err = cmd.Wait()
	if err != nil {
		err = fmt.Errorf("%v: %s", err, string(errdata))
		return "", nil, err
	}

	// $CONTAINER:$IP:$PORT
	chunks := strings.Split(string(data), ":")
	if len(chunks) != 3 {
		panic("could not parse script stdout")
	}

	container := strings.Trim(chunks[0], "\n ")
	ip := strings.Trim(chunks[1], "\n ")
	port := strings.Trim(chunks[2], "\n ")

	if ip == "" || port == "" || container == "" {
		panic(fmt.Sprintf("missing one of ip, port or container: %q %q %q", ip, port, container))
	}

	return fmt.Sprintf("%s:%s", ip, port), teardownFn(container), nil
}

func runDockerOrSkip(t *testing.T) (string, func()) {
	hostname, teardown, err := runContainer()
	if err != nil {
		t.Logf("problem starting container, maybe you want to build_docker.sh first? skipping test: %s\n", err)
		t.SkipNow()
		return "", func() {}
	}
	return hostname, func() {
		err := teardown()
		if err != nil {
			t.Logf("error stopping container for %q: %s\n", hostname, err)
		}
	}
}

func newSchemeIntegration(
	t *testing.T, hostname string, cfg config.Config,
) schemeapi.Scheme {
	workspaceURI, err := workspaceapi.ParseURI("ssh://test@" + hostname + "/tmp")
	require.NoError(t, err)

	var mu sync.Mutex
	ctx := context.Background()
	mu.Lock()

	s, err := workspacessh.New(ctx, cfg, workspaceURI)
	require.NoError(t, err)

	t.Cleanup(func() {
		s, err := workspacessh.New(ctx, cfg, workspaceURI)
		require.NoError(t, err)
		it, err := walkdir.ListFiles(context.Background(), s, "/tmp")
		require.NoError(t, err)
		for {
			f, ok := it.Next(context.Background())
			if !ok {
				break
			}
			require.NoError(t, s.Remove(f))
		}
		require.NoError(t, it.Err())
		s.Close()
	})
	return s
}
