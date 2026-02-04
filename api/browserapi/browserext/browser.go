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

package browserext

import (
	"context"
	"os"

	"github.com/unstablebuild/rune-go-sdk/api/browserapi"
	"github.com/unstablebuild/rune-go-sdk/api/browserapi/browserrpc"
	"unstable.build/go-tui/extension"
	"unstable.build/go-tui/rpc"
)

func dialBrowser(ctx context.Context, grant extension.Grant, broker rpc.MuxBroker) (
	browserapi.Browser, error,
) {
	conn, err := broker.DialChannel(ctx, grant.Token,
		os.Args[0], "browser", string(grant.Permission))
	if err != nil {
		return nil, err
	}
	c := browserrpc.NewClient(grant.Context, conn)
	return c, nil
}

// WindowManager acquires the browser's WindowManager
// resource with the given token.
func WindowManager(ctx context.Context, grant extension.Grant, broker rpc.MuxBroker) (
	browserapi.WindowManager, error,
) {
	return dialBrowser(ctx, grant, broker)
}

// ResourceOpener acquires the browser's ResourceOpener
// resource with the given token.
func ResourceOpener(ctx context.Context, grant extension.Grant, broker rpc.MuxBroker) (
	browserapi.ResourceOpener, error,
) {
	return dialBrowser(ctx, grant, broker)
}

// Notifications acquires the browser's Notifications
// resource with the given token.
func Notifications(ctx context.Context, grant extension.Grant, broker rpc.MuxBroker) (
	browserapi.Notifications, error,
) {
	return dialBrowser(ctx, grant, broker)
}

// EventPublisher acquires the browser's EventPublisher
// resource with the given token.
func EventPublisher(ctx context.Context, grant extension.Grant, broker rpc.MuxBroker) (
	browserapi.EventPublisher, error,
) {
	return dialBrowser(ctx, grant, broker)
}
