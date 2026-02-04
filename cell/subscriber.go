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

package cell

import (
	"context"

	"github.com/unstablebuild/rune-go-sdk/term"
)

// Subscriber is the interface that wraps methods to receive to updates to
// an underlying cell.Editor.
//
// Subscribers MUST NOT have mutable access to the underlying cell.Editor
// they're subscribing to, as updates are published synchronously so
// program could enter in an infinite loop.
//
// Subscribers constructors SHOULD subscribe to a Publisher upon initialization.
type Subscriber interface {
	// OnWillEdit start, end and str correspond the input values
	// to an imminent call to Edit.
	OnWillEdit(ctx context.Context, start, end term.Coordinates, str string)
	// OnDidEdit from, to and old correspond to the return values
	// of a call to Edit. See cell.Editor.Edit for more details.
	OnDidEdit(ctx context.Context, from, to term.Coordinates, old string)
}

// Publisher is the interface that wraps the Subscribe method.
//
// Subscribe enables subscription of insert/delete events. See Subscriber.
type Publisher interface {
	Subscribe(Subscriber)
	Unsubscribe(Subscriber)
}

// PublisherView is the interface that groups Publisher and View.
//
// This interface should be used within Subscribers which need to read from
// a View when handling updates. See Subscriber.
type PublisherView interface {
	Publisher
	View
}
