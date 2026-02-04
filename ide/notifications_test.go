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

package ide

import (
	"hash/fnv"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/unstablebuild/blue/document"
	"github.com/unstablebuild/rune-go-sdk/api/browserapi"
	"unstable.build/go-tui/component/notifications"
)

func newTestNotifications(t *testing.T) (*testNotifier, *workspaceNotifications) {
	mock := newTestNotify()
	b := &workspaceNotifications{
		notifier: mock,
		storage:  document.NewInMemoryService(),
	}
	return mock, b
}

func TestNotifyOnce(t *testing.T) {
	t.Run("delivers notifications only the first time it's invoked", func(t *testing.T) {
		svc := document.NewInMemoryService()
		mock, b := newTestNotifications(t)

		_, err := b.NotifyOnce(browserapi.LevelError, "a")
		require.NoError(t, err)
		assert.Equal(t, 1, mock.messages["a"])

		_, err = b.NotifyOnce(browserapi.LevelError, "%v", svc)
		require.NoError(t, err)
		assert.Equal(t, 1, mock.messages["a"])
	})

	t.Run("delivers notifications with different args multiple times, if args are different", func(t *testing.T) {
		mock, b := newTestNotifications(t)

		_, err := b.NotifyOnce(browserapi.LevelError, "a %d", 0)
		require.NoError(t, err)
		assert.Equal(t, 1, mock.messages["a 0"])

		_, err = b.NotifyOnce(browserapi.LevelError, "a %d", 1)
		require.NoError(t, err)
		assert.Equal(t, 1, mock.messages["a 1"])
		assert.Equal(t, 1, mock.messages["a 0"])
	})

	t.Run("delivers notifications with different args only once if args are the same", func(t *testing.T) {
		mock, b := newTestNotifications(t)

		_, err := b.NotifyOnce(browserapi.LevelError, "a %d", 0)
		require.NoError(t, err)
		assert.Equal(t, 1, mock.messages["a 0"])

		_, err = b.NotifyOnce(browserapi.LevelError, "a %d", 0)
		require.NoError(t, err)
		assert.Equal(t, 1, mock.messages["a 0"])
	})

	t.Run("delivers notifications multiple times if subsequent uses Notify rather than NotifyOnce", func(t *testing.T) {
		mock, b := newTestNotifications(t)

		_, err := b.NotifyOnce(browserapi.LevelError, "a")
		require.NoError(t, err)
		assert.Equal(t, 1, mock.messages["a"])

		b.Notify(browserapi.LevelError, "a")
		assert.Equal(t, 2, mock.messages["a"])

		_, err = b.NotifyOnce(browserapi.LevelError, "a")
		require.NoError(t, err)
		assert.Equal(t, 2, mock.messages["a"])
	})
}

type testNotifier struct {
	messages map[string]int
}

func newTestNotify() *testNotifier {
	ret := new(testNotifier)
	ret.messages = make(map[string]int)
	return ret
}

func (t *testNotifier) Notify(
	level notifications.Level, msg string,
) string {
	t.messages[msg] = t.messages[msg] + 1
	return t.ID(level, msg)
}

func (t *testNotifier) ID(
	level notifications.Level, msg string,
) string {
	h := fnv.New64a()
	h.Write([]byte(msg))
	return strconv.FormatUint(h.Sum64(), 10)
}

func (t *testNotifier) UpdateProgress(
	id, message string, progress, total int64,
) bool {
	return false
}

func (t testNotifier) CloseAll() {
}
func (t testNotifier) ResumeAll() {
}
func (t testNotifier) PauseAll() {
}
