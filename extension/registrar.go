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
	"io"
	"sync"

	"github.com/unstablebuild/rune-go-sdk/api/extensionapi"
	"unstable.build/go-tui/rpc"
)

// ResourceRegistrar wraps the basic Serve method, to serve resources over a mux broker.
type ResourceRegistrar interface {
	Register(registar rpc.ServiceRegistrar, locker sync.Locker) (
		io.Closer, error,
	)
}

// MergeResourceMap merges m1 with mn.
// If permissions are overlapping, the last of passed prevails.
func MergeResourceMap(
	m1 map[extensionapi.Permission]ResourceRegistrar,
	mn ...map[extensionapi.Permission]ResourceRegistrar,
) map[extensionapi.Permission]ResourceRegistrar {
	ret := make(map[extensionapi.Permission]ResourceRegistrar)
	for k, v := range m1 {
		ret[k] = v
	}
	for _, m := range mn {
		for k, v := range m {
			ret[k] = v
		}
	}
	return ret
}
