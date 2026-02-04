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

	"github.com/unstablebuild/rune-go-sdk/api/textapi/textrpc"
)

// this structure wraps a text.Editor to
// provide interrupt on write requests coming from the wire
type interruptEditor struct {
	textrpc.EditorServer
	interruptDraw func()
}

func interruptEditorServer(srv textrpc.EditorServer, interruptDraw func()) textrpc.EditorServer {
	return &interruptEditor{EditorServer: srv, interruptDraw: interruptDraw}
}

func (e *interruptEditor) Edit(ctx context.Context, req *textrpc.EditRequest) (
	*textrpc.EditResponse, error,
) {
	res, err := e.EditorServer.Edit(ctx, req)
	e.interruptDraw()
	return res, err
}

func (e *interruptEditor) SubscribeEvent(stream textrpc.Editor_SubscribeEventServer) error {
	return e.EditorServer.SubscribeEvent(stream)
}

func (e *interruptEditor) SubscribeCommand(stream textrpc.Editor_SubscribeCommandServer) error {
	return e.EditorServer.SubscribeCommand(stream)

}

func (e *interruptEditor) SetLocationList(ctx context.Context, req *textrpc.SetLocationListRequest) (
	*textrpc.SetLocationListResponse, error,
) {
	res, err := e.EditorServer.SetLocationList(ctx, req)
	e.interruptDraw()
	return res, err

}
func (e *interruptEditor) MoveToNextLocation(ctx context.Context, req *textrpc.MoveToLocationRequest) (
	*textrpc.MoveToLocationResponse, error,
) {
	res, err := e.EditorServer.MoveToNextLocation(ctx, req)
	e.interruptDraw()
	return res, err

}
func (e *interruptEditor) MoveToPrevLocation(ctx context.Context, req *textrpc.MoveToLocationRequest) (
	*textrpc.MoveToLocationResponse, error,
) {
	res, err := e.EditorServer.MoveToPrevLocation(ctx, req)
	e.interruptDraw()
	return res, err

}
func (e *interruptEditor) EditCell(ctx context.Context, req *textrpc.EditCellRequest) (
	*textrpc.EditCellResponse, error,
) {
	res, err := e.EditorServer.EditCell(ctx, req)
	e.interruptDraw()
	return res, err

}
