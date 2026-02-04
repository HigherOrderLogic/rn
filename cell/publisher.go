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

// A cell.Editor that synchronously
// publishes updated to a list of Subscribers
type syncPublisher struct {
	w           Editor
	subscribers []Subscriber
}

func newPublisher(w Editor) *syncPublisher {
	p := new(syncPublisher)
	p.w = w
	return p
}

func (p *syncPublisher) Edit(ctx context.Context, start, end term.Coordinates, str string) (
	from, to term.Coordinates, old string,
) {
	for _, sub := range p.subscribers {
		sub.OnWillEdit(ctx, start, end, str)
	}
	from, to, old = p.w.Edit(ctx, start, end, str)
	for _, sub := range p.subscribers {
		sub.OnDidEdit(ctx, from, to, old)
	}
	return
}

func (p *syncPublisher) Subscribe(s Subscriber) {
	p.subscribers = append(p.subscribers, s)
}

func (p *syncPublisher) Unsubscribe(s Subscriber) {
	unsubs := -1
	for i, sub := range p.subscribers {
		if sub == s {
			unsubs = i
			break
		}
	}
	if unsubs < 0 {
		panic("Subscriber is not subscribed")
	}
	p.subscribers = append(p.subscribers[:unsubs], p.subscribers[unsubs+1:]...)
}
