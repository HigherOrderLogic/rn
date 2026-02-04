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

	"github.com/unstablebuild/rune-go-sdk/api/config"
	"github.com/unstablebuild/rune-go-sdk/api/config/configrpc"
	"github.com/unstablebuild/rune-go-sdk/api/extensionapi"
	"unstable.build/go-tui/rpc"
)

type configResourceServer struct {
	cfg config.Config
}

func newConfigResourceServer(cfg config.Config) *configResourceServer {
	ret := new(configResourceServer)
	ret.cfg = cfg
	return ret
}

func (s *configResourceServer) Register(
	registrar rpc.ServiceRegistrar, lock sync.Locker,
) (io.Closer, error) {
	server := configrpc.NewServer(s.cfg, lock)
	configrpc.RegisterConfigServer(registrar, server)
	return nopCloser{}, nil
}

// ConfigResources returns a map of Permission to a ResourceServer
// capable of serving each of the b Config's resources.
func ConfigResources(b config.Config) map[extensionapi.Permission]ResourceRegistrar {
	s := newConfigResourceServer(b)
	return map[extensionapi.Permission]ResourceRegistrar{
		extensionapi.PermissionConfig: s,
	}
}

type nopCloser struct {
}

func (c nopCloser) Close() error {
	return nil
}
