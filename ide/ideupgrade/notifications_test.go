// Unstable Build LLC ("COMPANY") CONFIDENTIAL
//
// Unpublished Copyright (c) 2017-2026 Unstable Build, All Rights Reserved.
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

package ideupgrade

import (
	"strconv"
	"sync"

	"github.com/unstablebuild/rune-go-sdk/api/browserapi"
)

// progressSample records one UpdateNotificationProgress call.
type progressSample struct {
	progress int64
	total    int64
}

// recordingNotifications is a browserapi.Notifications that records the
// number of Notify calls and every UpdateNotificationProgress sample so
// tests can assert the download notification is anchored once and gets
// live progress.
type recordingNotifications struct {
	mu         sync.Mutex
	notifyN    int
	nextID     int
	progresses []progressSample
}

func (n *recordingNotifications) Notify(_ browserapi.NotificationLevel, _ string, _ ...any) (string, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.notifyN++
	n.nextID++
	return "noti-" + strconv.Itoa(n.nextID), nil
}

func (n *recordingNotifications) NotifyOnce(level browserapi.NotificationLevel, msg string, args ...any) (string, error) {
	return n.Notify(level, msg, args...)
}

func (n *recordingNotifications) UpdateNotificationProgress(_, _ string, progress, total int64) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.progresses = append(n.progresses, progressSample{progress: progress, total: total})
	return nil
}

func (n *recordingNotifications) snapshot() (notifyN int, samples []progressSample) {
	n.mu.Lock()
	defer n.mu.Unlock()
	out := make([]progressSample, len(n.progresses))
	copy(out, n.progresses)
	return n.notifyN, out
}
