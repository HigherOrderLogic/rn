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

package idedebug

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/google/go-dap"
	"github.com/unstablebuild/rune-go-sdk/api/debugapi"
)

// substituteArgs expands a static argument template into a DAP
// argument map. Each template value has its {placeholder} tokens
// replaced using the provided substitutions. A dotted key such as
// "connect.host" nests the value under intermediate maps
// ({"connect":{"host":...}}) so adapters like debugpy that require
// structured attach arguments can be configured from a flat template.
// It returns nil when the template is empty so callers can fall back
// to built-in defaults.
func substituteArgs(
	template map[string]string, subs map[string]string,
) map[string]any {
	if len(template) == 0 {
		return nil
	}
	out := make(map[string]any, len(template))
	for key, val := range template {
		for name, replacement := range subs {
			val = strings.ReplaceAll(val, "{"+name+"}", replacement)
		}
		setNestedArg(out, strings.Split(key, "."), val)
	}
	return out
}

// setNestedArg assigns val at the path described by keys, creating
// intermediate map[string]any nodes as needed. A non-map value found
// along the path is overwritten with a fresh map so the deeper key
// can be set.
func setNestedArg(m map[string]any, keys []string, val string) {
	for i := 0; i < len(keys)-1; i++ {
		next, ok := m[keys[i]].(map[string]any)
		if !ok {
			next = map[string]any{}
			m[keys[i]] = next
		}
		m = next
	}
	m[keys[len(keys)-1]] = val
}

// CreateSession starts a new debug session for the given
// language. It looks up the adapter config under
// Manager.cfg.Adapters[langID], spawns a fresh *debugServer,
// performs the DAP Initialize handshake, and stores the server
// under a minted sessionID. DAP events are delivered to
// subscriber for the lifetime of the session.
func (m *Manager) CreateSession(
	ctx context.Context, langID string,
	client debugapi.ClientCapabilities, subscriber debugapi.EventSubscriber,
) (string, *dap.Capabilities, error) {
	adapter, ok := m.cfg.Adapters[langID]
	if !ok {
		return "", nil, fmt.Errorf("%w: %s",
			debugapi.ErrNoAdapterConfigured, langID)
	}
	if len(adapter.Command) == 0 {
		return "", nil, fmt.Errorf(
			"debug adapter for %s has empty command", langID)
	}
	cfg := debugConfig{
		langID:     langID,
		adapterID:  adapter.AdapterID,
		command:    adapter.Command[0],
		args:       append([]string(nil), adapter.Command[1:]...),
		launchArgs: adapter.LaunchArgs,
		attachArgs: adapter.AttachArgs,
	}
	if cfg.adapterID == "" {
		cfg.adapterID = langID
	}
	sessionID, err := newSessionID()
	if err != nil {
		return "", nil, err
	}
	srv, err := m.startSession(ctx, sessionID, cfg, client, subscriber)
	if err != nil {
		return "", nil, fmt.Errorf("start session %s: %w", langID, err)
	}
	return sessionID, srv.caps, nil
}

// Launch starts the debuggee. The request is sent without
// waiting for a response because in DAP the LaunchResponse
// only arrives after ConfigurationDone.
func (m *Manager) Launch(
	_ context.Context, sessionID string, args debugapi.LaunchRequestArguments,
) error {
	srv, err := m.sessionFor(sessionID)
	if err != nil {
		return err
	}
	launchArgs := substituteArgs(srv.cfg.launchArgs, map[string]string{
		"program":     args.Program,
		"cwd":         args.Cwd,
		"stopOnEntry": strconv.FormatBool(args.StopOnEntry),
		"noDebug":     strconv.FormatBool(args.NoDebug),
	})
	if launchArgs == nil {
		launchArgs = map[string]any{}
	}
	launchArgs["program"] = args.Program
	launchArgs["stopOnEntry"] = args.StopOnEntry
	launchArgs["noDebug"] = args.NoDebug
	if len(args.Args) > 0 {
		launchArgs["args"] = args.Args
	}
	if args.Cwd != "" {
		launchArgs["cwd"] = args.Cwd
	}
	if len(args.Env) > 0 {
		launchArgs["env"] = args.Env
	}
	argsJSON, err := json.Marshal(launchArgs)
	if err != nil {
		return fmt.Errorf("marshal launch args: %w", err)
	}
	req := &dap.LaunchRequest{
		Request:   srv.newRequest("launch"),
		Arguments: argsJSON,
	}
	return srv.writeRequest(req)
}

// Attach connects to an already running debuggee.
// Like Launch, the response only arrives after
// ConfigurationDone.
func (m *Manager) Attach(
	_ context.Context, sessionID string, args debugapi.AttachRequestArguments,
) error {
	srv, err := m.sessionFor(sessionID)
	if err != nil {
		return err
	}
	attachArgs := substituteArgs(srv.cfg.attachArgs, map[string]string{
		"program": args.Program,
		"pid":     strconv.Itoa(args.PID),
	})
	if attachArgs == nil {
		attachArgs = map[string]any{}
	}
	if args.PID != 0 {
		attachArgs["processId"] = args.PID
	}
	if args.Program != "" {
		attachArgs["program"] = args.Program
	}
	argsJSON, err := json.Marshal(attachArgs)
	if err != nil {
		return fmt.Errorf("marshal attach args: %w", err)
	}
	req := &dap.AttachRequest{
		Request:   srv.newRequest("attach"),
		Arguments: argsJSON,
	}
	return srv.writeRequest(req)
}

// ConfigurationDone signals that configuration is done.
func (m *Manager) ConfigurationDone(ctx context.Context, sessionID string) error {
	srv, err := m.sessionFor(sessionID)
	if err != nil {
		return err
	}
	req := &dap.ConfigurationDoneRequest{
		Request: srv.newRequest("configurationDone"),
	}
	_, err = srv.sendRequest(ctx, req)
	if err != nil {
		// If ConfigurationDone failed, check whether the root
		// cause is a failed Launch/Attach. Its error message
		// is more informative than the generic "No debug
		// session started" that the adapter returns.
		srv.mu.Lock()
		launchErr := srv.launchErr
		srv.launchErr = nil
		srv.mu.Unlock()
		if launchErr != nil {
			return launchErr
		}
	}
	return err
}

// Disconnect ends the debug session.
func (m *Manager) Disconnect(
	ctx context.Context, sessionID string, args *dap.DisconnectArguments,
) error {
	srv, err := m.sessionFor(sessionID)
	if err != nil {
		return err
	}
	req := &dap.DisconnectRequest{
		Request:   srv.newRequest("disconnect"),
		Arguments: args,
	}
	_, err = srv.sendRequest(ctx, req)
	return err
}

// Terminate requests graceful termination.
func (m *Manager) Terminate(
	ctx context.Context, sessionID string, args *dap.TerminateArguments,
) error {
	srv, err := m.sessionFor(sessionID)
	if err != nil {
		return err
	}
	req := &dap.TerminateRequest{
		Request:   srv.newRequest("terminate"),
		Arguments: args,
	}
	_, err = srv.sendRequest(ctx, req)
	return err
}

// Restart restarts the debug session.
func (m *Manager) Restart(ctx context.Context, sessionID string) error {
	srv, err := m.sessionFor(sessionID)
	if err != nil {
		return err
	}
	req := &dap.RestartRequest{
		Request: srv.newRequest("restart"),
	}
	_, err = srv.sendRequest(ctx, req)
	return err
}

// SetBreakpoints sets breakpoints for a source file.
func (m *Manager) SetBreakpoints(
	ctx context.Context, sessionID string, args *dap.SetBreakpointsArguments,
) ([]dap.Breakpoint, error) {
	srv, err := m.sessionFor(sessionID)
	if err != nil {
		return nil, err
	}
	req := &dap.SetBreakpointsRequest{
		Request:   srv.newRequest("setBreakpoints"),
		Arguments: *args,
	}
	resp, err := srv.sendRequest(ctx, req)
	if err != nil {
		return nil, err
	}
	bpResp, ok := resp.(*dap.SetBreakpointsResponse)
	if !ok {
		return nil, fmt.Errorf("unexpected response type: %T", resp)
	}
	return bpResp.Body.Breakpoints, nil
}

// SetFunctionBreakpoints sets breakpoints on functions.
func (m *Manager) SetFunctionBreakpoints(
	ctx context.Context, sessionID string,
	args *dap.SetFunctionBreakpointsArguments,
) ([]dap.Breakpoint, error) {
	srv, err := m.sessionFor(sessionID)
	if err != nil {
		return nil, err
	}
	req := &dap.SetFunctionBreakpointsRequest{
		Request:   srv.newRequest("setFunctionBreakpoints"),
		Arguments: *args,
	}
	resp, err := srv.sendRequest(ctx, req)
	if err != nil {
		return nil, err
	}
	r, ok := resp.(*dap.SetFunctionBreakpointsResponse)
	if !ok {
		return nil, fmt.Errorf("unexpected response type: %T", resp)
	}
	return r.Body.Breakpoints, nil
}

// SetExceptionBreakpoints configures exception bps.
func (m *Manager) SetExceptionBreakpoints(
	ctx context.Context, sessionID string,
	args *dap.SetExceptionBreakpointsArguments,
) ([]dap.Breakpoint, error) {
	srv, err := m.sessionFor(sessionID)
	if err != nil {
		return nil, err
	}
	req := &dap.SetExceptionBreakpointsRequest{
		Request:   srv.newRequest("setExceptionBreakpoints"),
		Arguments: *args,
	}
	resp, err := srv.sendRequest(ctx, req)
	if err != nil {
		return nil, err
	}
	r, ok := resp.(*dap.SetExceptionBreakpointsResponse)
	if !ok {
		return nil, fmt.Errorf("unexpected response type: %T", resp)
	}
	return r.Body.Breakpoints, nil
}

// Continue resumes execution of all threads.
func (m *Manager) Continue(
	ctx context.Context, sessionID string, args *dap.ContinueArguments,
) (*dap.ContinueResponseBody, error) {
	srv, err := m.sessionFor(sessionID)
	if err != nil {
		return nil, err
	}
	req := &dap.ContinueRequest{
		Request:   srv.newRequest("continue"),
		Arguments: *args,
	}
	resp, err := srv.sendRequest(ctx, req)
	if err != nil {
		return nil, err
	}
	r, ok := resp.(*dap.ContinueResponse)
	if !ok {
		return nil, fmt.Errorf("unexpected response type: %T", resp)
	}
	return &r.Body, nil
}

// Next executes one step over.
func (m *Manager) Next(
	ctx context.Context, sessionID string, args *dap.NextArguments,
) error {
	srv, err := m.sessionFor(sessionID)
	if err != nil {
		return err
	}
	req := &dap.NextRequest{
		Request:   srv.newRequest("next"),
		Arguments: *args,
	}
	_, err = srv.sendRequest(ctx, req)
	return err
}

// StepIn steps into a function call.
func (m *Manager) StepIn(
	ctx context.Context, sessionID string, args *dap.StepInArguments,
) error {
	srv, err := m.sessionFor(sessionID)
	if err != nil {
		return err
	}
	req := &dap.StepInRequest{
		Request:   srv.newRequest("stepIn"),
		Arguments: *args,
	}
	_, err = srv.sendRequest(ctx, req)
	return err
}

// StepOut steps out of the current function.
func (m *Manager) StepOut(
	ctx context.Context, sessionID string, args *dap.StepOutArguments,
) error {
	srv, err := m.sessionFor(sessionID)
	if err != nil {
		return err
	}
	req := &dap.StepOutRequest{
		Request:   srv.newRequest("stepOut"),
		Arguments: *args,
	}
	_, err = srv.sendRequest(ctx, req)
	return err
}

// StepBack executes one backward step.
func (m *Manager) StepBack(
	ctx context.Context, sessionID string, args *dap.StepBackArguments,
) error {
	srv, err := m.sessionFor(sessionID)
	if err != nil {
		return err
	}
	req := &dap.StepBackRequest{
		Request:   srv.newRequest("stepBack"),
		Arguments: *args,
	}
	_, err = srv.sendRequest(ctx, req)
	return err
}

// ReverseContinue resumes backward execution.
func (m *Manager) ReverseContinue(
	ctx context.Context, sessionID string, args *dap.ReverseContinueArguments,
) error {
	srv, err := m.sessionFor(sessionID)
	if err != nil {
		return err
	}
	req := &dap.ReverseContinueRequest{
		Request:   srv.newRequest("reverseContinue"),
		Arguments: *args,
	}
	_, err = srv.sendRequest(ctx, req)
	return err
}

// Pause suspends execution.
func (m *Manager) Pause(
	ctx context.Context, sessionID string, args *dap.PauseArguments,
) error {
	srv, err := m.sessionFor(sessionID)
	if err != nil {
		return err
	}
	req := &dap.PauseRequest{
		Request:   srv.newRequest("pause"),
		Arguments: *args,
	}
	_, err = srv.sendRequest(ctx, req)
	return err
}

// Threads retrieves all threads.
func (m *Manager) Threads(ctx context.Context, sessionID string) ([]dap.Thread, error) {
	srv, err := m.sessionFor(sessionID)
	if err != nil {
		return nil, err
	}
	req := &dap.ThreadsRequest{
		Request: srv.newRequest("threads"),
	}
	resp, err := srv.sendRequest(ctx, req)
	if err != nil {
		return nil, err
	}
	r, ok := resp.(*dap.ThreadsResponse)
	if !ok {
		return nil, fmt.Errorf("unexpected response type: %T", resp)
	}
	return r.Body.Threads, nil
}

// StackTrace returns the call stack for a thread.
func (m *Manager) StackTrace(
	ctx context.Context, sessionID string, args *dap.StackTraceArguments,
) (*dap.StackTraceResponseBody, error) {
	srv, err := m.sessionFor(sessionID)
	if err != nil {
		return nil, err
	}
	req := &dap.StackTraceRequest{
		Request:   srv.newRequest("stackTrace"),
		Arguments: *args,
	}
	resp, err := srv.sendRequest(ctx, req)
	if err != nil {
		return nil, err
	}
	r, ok := resp.(*dap.StackTraceResponse)
	if !ok {
		return nil, fmt.Errorf("unexpected response type: %T", resp)
	}
	return &r.Body, nil
}

// Scopes returns variable scopes for a stack frame.
func (m *Manager) Scopes(
	ctx context.Context, sessionID string, args *dap.ScopesArguments,
) ([]dap.Scope, error) {
	srv, err := m.sessionFor(sessionID)
	if err != nil {
		return nil, err
	}
	req := &dap.ScopesRequest{
		Request:   srv.newRequest("scopes"),
		Arguments: *args,
	}
	resp, err := srv.sendRequest(ctx, req)
	if err != nil {
		return nil, err
	}
	r, ok := resp.(*dap.ScopesResponse)
	if !ok {
		return nil, fmt.Errorf("unexpected response type: %T", resp)
	}
	return r.Body.Scopes, nil
}

// Variables retrieves child variables.
func (m *Manager) Variables(
	ctx context.Context, sessionID string, args *dap.VariablesArguments,
) ([]dap.Variable, error) {
	srv, err := m.sessionFor(sessionID)
	if err != nil {
		return nil, err
	}
	req := &dap.VariablesRequest{
		Request:   srv.newRequest("variables"),
		Arguments: *args,
	}
	resp, err := srv.sendRequest(ctx, req)
	if err != nil {
		return nil, err
	}
	r, ok := resp.(*dap.VariablesResponse)
	if !ok {
		return nil, fmt.Errorf("unexpected response type: %T", resp)
	}
	return r.Body.Variables, nil
}

// SetVariable modifies a variable's value.
func (m *Manager) SetVariable(
	ctx context.Context, sessionID string, args *dap.SetVariableArguments,
) (*dap.SetVariableResponseBody, error) {
	srv, err := m.sessionFor(sessionID)
	if err != nil {
		return nil, err
	}
	req := &dap.SetVariableRequest{
		Request:   srv.newRequest("setVariable"),
		Arguments: *args,
	}
	resp, err := srv.sendRequest(ctx, req)
	if err != nil {
		return nil, err
	}
	r, ok := resp.(*dap.SetVariableResponse)
	if !ok {
		return nil, fmt.Errorf("unexpected response type: %T", resp)
	}
	return &r.Body, nil
}

// Source retrieves source code.
func (m *Manager) Source(
	ctx context.Context, sessionID string, args *dap.SourceArguments,
) (*dap.SourceResponseBody, error) {
	srv, err := m.sessionFor(sessionID)
	if err != nil {
		return nil, err
	}
	req := &dap.SourceRequest{
		Request:   srv.newRequest("source"),
		Arguments: *args,
	}
	resp, err := srv.sendRequest(ctx, req)
	if err != nil {
		return nil, err
	}
	r, ok := resp.(*dap.SourceResponse)
	if !ok {
		return nil, fmt.Errorf("unexpected response type: %T", resp)
	}
	return &r.Body, nil
}

// Evaluate evaluates an expression.
func (m *Manager) Evaluate(
	ctx context.Context, sessionID string, args *dap.EvaluateArguments,
) (*dap.EvaluateResponseBody, error) {
	srv, err := m.sessionFor(sessionID)
	if err != nil {
		return nil, err
	}
	req := &dap.EvaluateRequest{
		Request:   srv.newRequest("evaluate"),
		Arguments: *args,
	}
	resp, err := srv.sendRequest(ctx, req)
	if err != nil {
		return nil, err
	}
	r, ok := resp.(*dap.EvaluateResponse)
	if !ok {
		return nil, fmt.Errorf("unexpected response type: %T", resp)
	}
	return &r.Body, nil
}

// SetExpression assigns a value to an expression.
func (m *Manager) SetExpression(
	ctx context.Context, sessionID string, args *dap.SetExpressionArguments,
) (*dap.SetExpressionResponseBody, error) {
	srv, err := m.sessionFor(sessionID)
	if err != nil {
		return nil, err
	}
	req := &dap.SetExpressionRequest{
		Request:   srv.newRequest("setExpression"),
		Arguments: *args,
	}
	resp, err := srv.sendRequest(ctx, req)
	if err != nil {
		return nil, err
	}
	r, ok := resp.(*dap.SetExpressionResponse)
	if !ok {
		return nil, fmt.Errorf("unexpected response type: %T", resp)
	}
	return &r.Body, nil
}

// Completions provides completion suggestions.
func (m *Manager) Completions(
	ctx context.Context, sessionID string, args *dap.CompletionsArguments,
) ([]dap.CompletionItem, error) {
	srv, err := m.sessionFor(sessionID)
	if err != nil {
		return nil, err
	}
	req := &dap.CompletionsRequest{
		Request:   srv.newRequest("completions"),
		Arguments: *args,
	}
	resp, err := srv.sendRequest(ctx, req)
	if err != nil {
		return nil, err
	}
	r, ok := resp.(*dap.CompletionsResponse)
	if !ok {
		return nil, fmt.Errorf("unexpected response type: %T", resp)
	}
	return r.Body.Targets, nil
}

// ExceptionInfo retrieves exception details.
func (m *Manager) ExceptionInfo(
	ctx context.Context, sessionID string, args *dap.ExceptionInfoArguments,
) (*dap.ExceptionInfoResponseBody, error) {
	srv, err := m.sessionFor(sessionID)
	if err != nil {
		return nil, err
	}
	req := &dap.ExceptionInfoRequest{
		Request:   srv.newRequest("exceptionInfo"),
		Arguments: *args,
	}
	resp, err := srv.sendRequest(ctx, req)
	if err != nil {
		return nil, err
	}
	r, ok := resp.(*dap.ExceptionInfoResponse)
	if !ok {
		return nil, fmt.Errorf("unexpected response type: %T", resp)
	}
	return &r.Body, nil
}

// Modules retrieves loaded modules.
func (m *Manager) Modules(
	ctx context.Context, sessionID string, args *dap.ModulesArguments,
) (*dap.ModulesResponseBody, error) {
	srv, err := m.sessionFor(sessionID)
	if err != nil {
		return nil, err
	}
	req := &dap.ModulesRequest{
		Request:   srv.newRequest("modules"),
		Arguments: *args,
	}
	resp, err := srv.sendRequest(ctx, req)
	if err != nil {
		return nil, err
	}
	r, ok := resp.(*dap.ModulesResponse)
	if !ok {
		return nil, fmt.Errorf("unexpected response type: %T", resp)
	}
	return &r.Body, nil
}

// LoadedSources retrieves all loaded sources.
func (m *Manager) LoadedSources(
	ctx context.Context, sessionID string,
) ([]dap.Source, error) {
	srv, err := m.sessionFor(sessionID)
	if err != nil {
		return nil, err
	}
	req := &dap.LoadedSourcesRequest{
		Request: srv.newRequest("loadedSources"),
	}
	resp, err := srv.sendRequest(ctx, req)
	if err != nil {
		return nil, err
	}
	r, ok := resp.(*dap.LoadedSourcesResponse)
	if !ok {
		return nil, fmt.Errorf("unexpected response type: %T", resp)
	}
	return r.Body.Sources, nil
}

// ReadMemory reads bytes from memory.
func (m *Manager) ReadMemory(
	ctx context.Context, sessionID string, args *dap.ReadMemoryArguments,
) (*dap.ReadMemoryResponseBody, error) {
	srv, err := m.sessionFor(sessionID)
	if err != nil {
		return nil, err
	}
	req := &dap.ReadMemoryRequest{
		Request:   srv.newRequest("readMemory"),
		Arguments: *args,
	}
	resp, err := srv.sendRequest(ctx, req)
	if err != nil {
		return nil, err
	}
	r, ok := resp.(*dap.ReadMemoryResponse)
	if !ok {
		return nil, fmt.Errorf("unexpected response type: %T", resp)
	}
	return &r.Body, nil
}

// WriteMemory writes bytes to memory.
func (m *Manager) WriteMemory(
	ctx context.Context, sessionID string, args *dap.WriteMemoryArguments,
) (*dap.WriteMemoryResponseBody, error) {
	srv, err := m.sessionFor(sessionID)
	if err != nil {
		return nil, err
	}
	req := &dap.WriteMemoryRequest{
		Request:   srv.newRequest("writeMemory"),
		Arguments: *args,
	}
	resp, err := srv.sendRequest(ctx, req)
	if err != nil {
		return nil, err
	}
	r, ok := resp.(*dap.WriteMemoryResponse)
	if !ok {
		return nil, fmt.Errorf("unexpected response type: %T", resp)
	}
	return &r.Body, nil
}

// Disassemble returns disassembled instructions.
func (m *Manager) Disassemble(
	ctx context.Context, sessionID string, args *dap.DisassembleArguments,
) ([]dap.DisassembledInstruction, error) {
	srv, err := m.sessionFor(sessionID)
	if err != nil {
		return nil, err
	}
	req := &dap.DisassembleRequest{
		Request:   srv.newRequest("disassemble"),
		Arguments: *args,
	}
	resp, err := srv.sendRequest(ctx, req)
	if err != nil {
		return nil, err
	}
	r, ok := resp.(*dap.DisassembleResponse)
	if !ok {
		return nil, fmt.Errorf("unexpected response type: %T", resp)
	}
	return r.Body.Instructions, nil
}

// GotoTargets returns possible goto targets.
func (m *Manager) GotoTargets(
	ctx context.Context, sessionID string, args *dap.GotoTargetsArguments,
) ([]dap.GotoTarget, error) {
	srv, err := m.sessionFor(sessionID)
	if err != nil {
		return nil, err
	}
	req := &dap.GotoTargetsRequest{
		Request:   srv.newRequest("gotoTargets"),
		Arguments: *args,
	}
	resp, err := srv.sendRequest(ctx, req)
	if err != nil {
		return nil, err
	}
	r, ok := resp.(*dap.GotoTargetsResponse)
	if !ok {
		return nil, fmt.Errorf("unexpected response type: %T", resp)
	}
	return r.Body.Targets, nil
}

// Goto sets execution to continue from a target.
func (m *Manager) Goto(
	ctx context.Context, sessionID string, args *dap.GotoArguments,
) error {
	srv, err := m.sessionFor(sessionID)
	if err != nil {
		return err
	}
	req := &dap.GotoRequest{
		Request:   srv.newRequest("goto"),
		Arguments: *args,
	}
	_, err = srv.sendRequest(ctx, req)
	return err
}
