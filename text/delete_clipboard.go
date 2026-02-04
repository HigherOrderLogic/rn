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

package text

import (
	"context"

	"unstable.build/go-tui/cell"
	"unstable.build/go-tui/clipboard"
	"github.com/unstablebuild/rune-go-sdk/term"
)

type delClip struct {
	clipboard  clipboard.Register
	registerID string
	pub        cell.Publisher
	cur        *Cursor
	mode       SelectMode
}

// WithCopyDelete installs a cell.Subscriber to a cell.Buffer
// which persists all the deleted content to a Clipboard.
func WithCopyDelete(
	registerID string, clipboard clipboard.Register,
	cur *Cursor, buf *cell.Buffer,
) {
	c := new(delClip)
	c.clipboard = clipboard
	c.pub = buf
	c.cur = cur
	c.registerID = registerID
	buf.SubscribeUsage(c)
}

func (c *delClip) OnWillEdit(
	ctx context.Context, start, end term.Coordinates, str string,
) {
	if start == end {
		return
	}

	var ok bool
	c.mode, ok = c.cur.SelectionMode()
	if !ok {
		c.mode = StandardSelection
	}
}

func (c *delClip) OnDidEdit(
	ctx context.Context, from, to term.Coordinates, old string,
) {
	if old != "" {
		_ = c.clipboard.Copy(c.registerID, clipboard.Data{Text: old, Metadata: c.mode})
	}
}
