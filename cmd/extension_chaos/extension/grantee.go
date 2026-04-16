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
	"github.com/unstablebuild/rune-go-sdk/api/extensionapi"
	"github.com/unstablebuild/rune-go-sdk/api/textapi"
)

var (
	// ChaosHandlerCommands returns the commands that this extension is
	// interested in registering.
	ChaosHandlerCommands = []textapi.CommandManual{
		{
			Name: commandChaosHandler,
			Summary: "Creates a new split window with a broken TUI handler." +
				"Two modes can be specified (panic or slow)." +
				"If no argument is passed then 'panic' is assumed.",
			Synopsis: "[(panic|slow [duration])]",
		},
		{
			Name: commandChaosUpdateEventLatency,
			Summary: "Updates the (added) latency of the event subscriber " +
				"installed by the chaos extension. If no duration is passed, then " +
				"this acts as a reset to zero, which is the default.",
			Synopsis: "[duration]",
		},
		{
			Name: commandChaosUpdateCommandLatency,
			Summary: "Updates the (added) latency of the chaos extension's command handler. " +
				"If no duration is passed, then this acts as a reset to zero, " +
				"which is the default. Note that this hinders the ability",
			Synopsis: "[duration]",
		},
	}

	// ChaosHandlerEvents returns the events that this extension is
	// interested in subscribing to.
	ChaosHandlerEvents = []textapi.EventType{
		textapi.EventTypeOpen,
		textapi.EventTypeClose,
		textapi.EventTypeEdit,
		textapi.EventTypeFlush,
		textapi.EventTypeScroll,
		textapi.EventTypeHidden,
		textapi.EventTypeVisible,
		textapi.EventTypeFocus,
		textapi.EventTypeUnfocus,
		textapi.EventTypeCursor,
	}
	// ChaosHandlerPermissions are the required permissions for this extension to run.
	// Deprecated: use NewExtension metadata instead.
	ChaosHandlerPermissions = []extensionapi.Permission{
		extensionapi.PermissionBrowserWindowManager,
		extensionapi.PermissionEditor,
		extensionapi.PermissionCommands,
	}
)
