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
	"github.com/unstablebuild/rune-go-sdk/api/textapi/textrpc"
	"github.com/unstablebuild/rune-go-sdk/term"
	"unstable.build/go-tui/browser"
	"unstable.build/go-tui/rpc"
	"unstable.build/go-tui/text"
	ttextrpc "unstable.build/go-tui/text/textrpc"
)

// EditorResources returns a map of Permission to a ResourceServer
// capable of serving each of the b Editor's resources.
func EditorResources(
	b browser.Notifications, ed text.Editor,
	publishEvent func(term.Event) bool,
) map[extensionapi.Permission]ResourceRegistrar {
	s := newEditorResourceServer(b, ed, publishEvent)
	return map[extensionapi.Permission]ResourceRegistrar{
		extensionapi.PermissionEditor: s,
	}
}

type editorResourceServer struct {
	b            browser.Notifications
	ed           text.Editor
	publishEvent func(term.Event) bool
}

func newEditorResourceServer(
	b browser.Notifications, ed text.Editor,
	publishEvent func(term.Event) bool,
) *editorResourceServer {
	ret := new(editorResourceServer)
	ret.ed = ed
	ret.b = b
	ret.publishEvent = publishEvent
	return ret
}

func (s *editorResourceServer) Register(
	registrar rpc.ServiceRegistrar, lock sync.Locker,
) (io.Closer, error) {
	server := ttextrpc.NewServer(s.b, s.ed, lock)
	textrpc.RegisterEditorServer(registrar,
		interruptEditorServer(server, func() {
			s.publishEvent(term.Event{Type: term.EventInterrupt})
		}))
	return server, nil
}
