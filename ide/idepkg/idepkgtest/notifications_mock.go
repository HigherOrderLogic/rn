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

package idepkgtest

import (
	"errors"
	"fmt"
	"strconv"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/unstablebuild/rune-go-sdk/api/browserapi"
)

var _ browserapi.Notifications = (*Notifications)(nil)

// Notifications is a version of browserapi.Notifications for testing.
type Notifications struct {
	Wg *sync.WaitGroup

	ExpectErrorNotification bool

	t  *testing.T
	mu sync.Mutex

	i      int
	active map[string]Noti
	err    error
}

// NewNotifications returns an instance of Notifications.
func NewNotifications(t *testing.T) *Notifications {
	return &Notifications{
		t:      t,
		active: make(map[string]Noti),
	}
}

// ExpectReturnErr configures Notifications to return an error
// on the next call to Notify or NotifyOnce.
func (n *Notifications) ExpectReturnErr(err error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.err = err
}

// Notify satisfies browserapi.Notifications.
func (n *Notifications) Notify(
	level browserapi.NotificationLevel, msg string, args ...interface{},
) (string, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if !n.ExpectErrorNotification && level == browserapi.LevelError {
		n.t.Logf("no error notification was expected: %s", fmt.Sprintf(msg, args...))
		if n.Wg != nil {
			n.Wg.Done()
		}
		n.t.FailNow()
	}
	n.i++
	id := strconv.Itoa(n.i)
	n.active[id] = Noti{Level: level, Msg: fmt.Sprintf(msg, args...)}
	switch level {
	case browserapi.LevelError, browserapi.LevelSuccess:
		if n.Wg != nil {
			n.Wg.Done()
		}
	}
	return id, n.err
}

// NotifyOnce satisfies browserapi.Notifications.
func (n *Notifications) NotifyOnce(
	level browserapi.NotificationLevel, msg string, args ...interface{},
) (string, error) {
	panic("unimplemented")
}

// UpdateNotificationProgress satisfies browserapi.Notifications.
func (n *Notifications) UpdateNotificationProgress(
	id, message string, progress, total int64,
) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	_, ok := n.active[id]
	if !ok {
		return errors.New("unknown notification")
	}
	if progress == total {
		delete(n.active, id)
	}
	return n.err
}

// Reset resets the active notifications and error expectations.
func (n *Notifications) Reset() {
	n.mu.Lock()
	defer n.mu.Unlock()

	clear(n.active)
	n.ExpectErrorNotification = false
}

// Active returns the currently active notifications.
func (n *Notifications) Active() (ret map[string]Noti) {
	n.mu.Lock()
	defer n.mu.Unlock()

	ret = make(map[string]Noti)
	for k, v := range n.active {
		ret[k] = v
	}
	return
}

// RequireNoErrorNotification asserts that there are no error notifications active.
func (n *Notifications) RequireNoErrorNotification() {
	n.mu.Lock()
	defer n.mu.Unlock()
	for _, noti := range n.active {
		require.NotEqual(n.t, browserapi.LevelError, noti.Level, noti.Msg)
		require.NotEqual(n.t, browserapi.LevelWarn, noti.Level, noti.Msg)
	}
}

// RequireErrorNotification asserts that there are indeed at least
// one error notifications active.
func (n *Notifications) RequireErrorNotification() {
	n.mu.Lock()
	defer n.mu.Unlock()
	var found bool
	for _, noti := range n.active {
		if noti.Level == browserapi.LevelError {
			found = true
		}
	}
	require.True(n.t, found, n.active)
}

// Noti is an active notification.
type Noti struct {
	Level browserapi.NotificationLevel
	Msg   string
}
