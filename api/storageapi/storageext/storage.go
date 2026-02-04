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

package storageext

import (
	"context"
	"os"

	"github.com/unstablebuild/blue/document"
	"github.com/unstablebuild/rune-go-sdk/api/storageapi"
	"github.com/unstablebuild/rune-go-sdk/api/storageapi/storagerpc"
	"github.com/unstablebuild/rune-go-sdk/api/storageapi/storagerpc/doctoml"
	"unstable.build/go-tui/extension"
	"unstable.build/go-tui/localstorage/bluestore"
	"unstable.build/go-tui/rpc"
)

// NOTE: this is exposing blue/document types which we might not
// want to do directly. If we ever open-source that library
// then remove this comment.
func dialStorage(ctx context.Context, grant extension.Grant, broker rpc.MuxBroker) (
	storageapi.Service, error,
) {
	partition, ok := partitionFromContext(grant.Context)
	if !ok {
		partition = "default"
	}
	conn, err := broker.DialChannel(ctx, grant.Token,
		os.Args[0], "storage", string(grant.Permission))
	if err != nil {
		return nil, err
	}
	c := new(storagerpc.Client)
	c.Init(conn, doctoml.Marshaler())
	return bluestore.AdaptTo(
		document.WithPartition(bluestore.AdaptFrom(c), partition)), nil
}

// Storage acquires a client to persistent storage with
// the given token.
func Storage(ctx context.Context, grant extension.Grant, broker rpc.MuxBroker) (
	storageapi.Service, error,
) {
	return dialStorage(ctx, grant, broker)
}
