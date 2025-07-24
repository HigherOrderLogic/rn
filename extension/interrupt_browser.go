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

package extension

import (
	"context"

	"unstable.build/go-tui/browser/browserrpc"
)

type browserServer interface {
	browserrpc.WindowManagerServer
	browserrpc.EventPublisherServer
	browserrpc.NotificationsServer
	browserrpc.ResourceOpenerServer
}

// this structure wraps a browser.Browser to
// provide interrupt on write requests coming from the wire
type interruptBrowser struct {
	browserrpc.UnimplementedEventPublisherServer
	browserrpc.UnimplementedNotificationsServer
	browserrpc.UnimplementedResourceOpenerServer
	browserrpc.UnimplementedWindowManagerServer
	browserServer browserServer
	interruptDraw func()
}

func interruptBrowserServer(srv browserServer, interruptDraw func()) browserServer {
	return &interruptBrowser{browserServer: srv, interruptDraw: interruptDraw}
}

// Focus satisfies browserrpc.BrowserServer
func (s *interruptBrowser) Publish(
	ctx context.Context, req *browserrpc.PublishRequest,
) (*browserrpc.PublishResponse, error) {
	res, err := s.browserServer.Publish(ctx, req)
	return res, err
}

// Focus satisfies browserrpc.BrowserServer
func (s *interruptBrowser) Focus(
	ctx context.Context, req *browserrpc.FocusRequest,
) (*browserrpc.FocusResponse, error) {
	res, err := s.browserServer.Focus(ctx, req)
	return res, err
}

// Floating satisfies browserrpc.BrowserServer
func (s *interruptBrowser) Floating(srv browserrpc.WindowManager_FloatingServer) error {
	err := s.browserServer.Floating(srv)
	s.interruptDraw()
	return err
}

// Tab satisfies browserrpc.BrowserServer
func (s *interruptBrowser) Tab(srv browserrpc.WindowManager_TabServer) error {
	err := s.browserServer.Tab(srv)
	s.interruptDraw()
	return err
}

// Split satisfies browserrpc.BrowserServer
func (s *interruptBrowser) Split(srv browserrpc.WindowManager_SplitServer) error {
	err := s.browserServer.Split(srv)
	s.interruptDraw()
	return err
}

// Bar satisfies browserrpc.BrowserServer
func (s *interruptBrowser) Bar(srv browserrpc.WindowManager_BarServer) error {
	err := s.browserServer.Bar(srv)
	s.interruptDraw()
	return err
}

// Notify satisfies browserrpc.BrowserServer
func (s *interruptBrowser) Notify(
	ctx context.Context, req *browserrpc.NotifyRequest,
) (*browserrpc.NotifyResponse, error) {
	res, err := s.browserServer.Notify(ctx, req)
	s.interruptDraw()
	return res, err
}

// Open satisfies browserrpc.BrowserServer
func (s *interruptBrowser) Open(
	ctx context.Context, req *browserrpc.OpenResourceRequest,
) (*browserrpc.OpenResourceResponse, error) {
	res, err := s.browserServer.Open(ctx, req)
	s.interruptDraw()
	return res, err
}

// SetContent satisfies browserrpc.BrowserServer
func (s *interruptBrowser) SetContent(srv browserrpc.WindowManager_SetContentServer) error {
	err := s.browserServer.SetContent(srv)
	s.interruptDraw()
	return err
}

// CloseWindow satisfies browserrpc.BrowserServer
func (s *interruptBrowser) CloseWindow(
	ctx context.Context, req *browserrpc.WindowCloseRequest,
) (*browserrpc.WindowCloseResponse, error) {
	res, err := s.browserServer.CloseWindow(ctx, req)
	s.interruptDraw()
	return res, err
}
