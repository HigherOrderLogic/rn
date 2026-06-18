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

package idelsp

import (
	"context"
	"errors"
	"sync"

	"github.com/unstablebuild/rune-go-sdk/api/semanticapi"
)

// multiLangServer multiplexes one language id across several
// langServers. children[0] is the default backend that handles any
// method without an explicit route and whose initialize result is
// reported for the language. routes maps a request method to the
// index of the child that owns it; requests have a single response so
// they go to exactly one child. Notifications are fire-and-forget and
// are always fanned out to every child so each backend keeps a
// consistent view of documents and workspace state.
type multiLangServer struct {
	cfg      langConfig
	children []server
	routes   map[string]int
	mu       sync.Mutex
	init     semanticapi.InitializeResult
}

var _ server = (*multiLangServer)(nil)

func (m *multiLangServer) childFor(method string) server {
	m.mu.Lock()
	defer m.mu.Unlock()
	if idx, ok := m.routes[method]; ok && idx >= 0 && idx < len(m.children) {
		return m.children[idx]
	}
	return m.children[0]
}

func (m *multiLangServer) allChildren() []server {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]server, len(m.children))
	copy(out, m.children)
	return out
}

// replaceChild swaps old for replacement by pointer identity. It is
// used by the manager's restart supervisor when a single child
// crashes and is re-spawned, leaving the other children untouched.
func (m *multiLangServer) replaceChild(old, replacement server) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, c := range m.children {
		if c == old {
			m.children[i] = replacement
			return
		}
	}
}

func (m *multiLangServer) call(
	ctx context.Context, method string, params, result any,
) error {
	return m.childFor(method).call(ctx, method, params, result)
}

func (m *multiLangServer) notify(
	ctx context.Context, method string, params any,
) error {
	var errs []error
	for _, c := range m.allChildren() {
		if err := c.notify(ctx, method, params); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (m *multiLangServer) initialize(ctx context.Context) (
	semanticapi.InitializeResult, error,
) {
	var errs []error
	children := m.allChildren()
	for _, c := range children {
		if _, err := c.initialize(ctx); err != nil {
			errs = append(errs, err)
		}
	}
	if err := errors.Join(errs...); err != nil {
		return semanticapi.InitializeResult{}, err
	}
	res := children[0].initResult()
	m.mu.Lock()
	m.init = res
	m.mu.Unlock()
	return res, nil
}

func (m *multiLangServer) start(ctx context.Context) error {
	var errs []error
	children := m.allChildren()
	for _, c := range children {
		if err := c.start(ctx); err != nil {
			errs = append(errs, err)
		}
	}
	if err := errors.Join(errs...); err != nil {
		return err
	}
	m.mu.Lock()
	m.init = children[0].initResult()
	m.mu.Unlock()
	return nil
}

func (m *multiLangServer) stop(ctx context.Context) error {
	var errs []error
	for _, c := range m.allChildren() {
		if err := c.stop(ctx); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (m *multiLangServer) Close() error {
	var errs []error
	for _, c := range m.allChildren() {
		if err := c.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (m *multiLangServer) config() langConfig {
	return m.cfg
}

func (m *multiLangServer) initResult() semanticapi.InitializeResult {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.init
}

func (m *multiLangServer) isAlive() bool {
	for _, c := range m.allChildren() {
		if c.isAlive() {
			return true
		}
	}
	return false
}
