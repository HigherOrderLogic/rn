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

package extensionapi

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	log "github.com/sirupsen/logrus"
	"golang.org/x/oauth2"
	"unstable.build/go-tui/api/config"
	"unstable.build/go-tui/debug"
)

// Metadata represents the developer and extension metadata sent by the extension
// over stdout to the extension host.
type Metadata struct {
	DeveloperID      string      `json:"developer_id"`
	DeveloperEmail   string      `json:"developer_email"`
	DeveloperKey     string      `json:"developer_key"`
	ExtensionID      string      `json:"id"`
	ExtensionName    string      `json:"name"`
	ExtensionVersion string      `json:"version"`
	Permissions      Permissions `json:"permissions"`
}

// WorkspaceExtension abstracts ExtendWorkspace, which needs to
// be satisfied by extensions passed to ServeWorkspaceExtension.
type WorkspaceExtension interface {
	ExtendWorkspace(context.Context, *Workspace, config.Config) error
}

// ServeWorkspaceExtension serves the given workspace extension with the given
// developer and extension metadata. This function blocks until a signal is received
// for the extension to shutdown, in which case the returned error is nil,
// or another error occurs.
func ServeWorkspaceExtension(extension WorkspaceExtension, meta Metadata) error {
	return serveWorkspaceExtension(extension, meta, os.Stdin, os.Stdout)
}

// Config is sent by the host to the extension over stdin.
// It contains the connection configuration needed to establish
// gRPC connections and access workspace resources.
type Config struct {
	// Socket is the unix socket used to establish
	// a secure communication channel with host.
	Socket string `json:"socket"`
	// Token is the oauth2 token used to authenticate
	// and authorize requests against workspace resources.
	Token *oauth2.Token `json:"token"`
	// Certificate is the certificate used to secure the connections.
	Certificate []byte `json:"certificate"`
	// Config is the user's configuration for the running extension.
	Config  map[string]any `json:"config"`
	DataDir string         `json:"datadir"`
}

// FuncWorkspaceExtension returns a WorkspaceExtension that calls fn
// on calls to ExtendWorkspace.
func FuncWorkspaceExtension(
	fn func(context.Context, *Workspace, config.Config) error,
) WorkspaceExtension {
	return fnWorkspaceExtension{fn: fn}
}

func serveWorkspaceExtension(
	extension WorkspaceExtension, meta Metadata,
	in io.ReadCloser, out io.Writer,
) error {
	sigchan := make(chan os.Signal, 1)
	signal.Notify(sigchan, syscall.SIGINT, syscall.SIGTERM)
	signal.Ignore(syscall.SIGPIPE)
	defer signal.Stop(sigchan)

	level := getLogLevelEnv()
	setupExtensionLogging(level)

	data, err := json.Marshal(meta)
	if err != nil {
		return fmt.Errorf("marshal protocol response: %w", err)
	}

	if _, err := out.Write(data); err != nil {
		return fmt.Errorf("write protocol response: %w", err)
	}
	os.Stdout.Sync()

	scanner := bufio.NewScanner(in)
	ok := scanner.Scan()
	// ensure that nothing can read secrets from stdin beyond this point
	cleanStdin(in)
	if !ok {
		return fmt.Errorf("scan protocol exchange: %w", scanner.Err())
	}

	var req Config
	if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
		return fmt.Errorf("unmarshal protocol request: %w", err)
	}

	if req.Socket == "" {
		return errors.New("protocol exchange error: socket is missing" +
			" in request")
	}

	errchan := make(chan error, 1)
	cfg := config.MapConfig(req.Config)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go debug.CapturePanicReport(func() {
		workspace, err := NewWorkspace(ctx, req)
		if err != nil {
			errchan <- err
			return
		}
		ok, reportname, captureErr := debug.CapturePanicReportWith(req.DataDir,
			meta.ExtensionID, meta.ExtensionVersion, func() {
				errchan <- extension.ExtendWorkspace(ctx, workspace, cfg)
			})
		if ok {
			return
		}
		if captureErr != nil {
			panic(fmt.Sprintf("capture panic report: error capturing: %v", captureErr))
		}
		panicErr := fmt.Errorf("extension panic: report saved %s", reportname)
		log.Error(panicErr.Error())
		// ensure log is delivered
		_ = os.Stderr.Sync()
		errchan <- panicErr
	})

	for {
		select {
		case err := <-errchan:
			if err != nil {
				log.Errorf("extension is exiting: %v", err)
				return err
			}
		case <-sigchan:
			return nil
		}
	}
}

func cleanStdin(in io.ReadCloser) {
	_ = in.Close()
	devNull, _ := os.Open("/dev/null")
	_ = syscall.Dup2(int(devNull.Fd()), 0)
}

type fnWorkspaceExtension struct {
	fn func(context.Context, *Workspace, config.Config) error
}

func (fn fnWorkspaceExtension) ExtendWorkspace(
	ctx context.Context, w *Workspace, cfg config.Config,
) error {
	return fn.fn(ctx, w, cfg)
}
