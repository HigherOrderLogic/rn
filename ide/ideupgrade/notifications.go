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
	"errors"

	"github.com/google/uuid"
	"github.com/unstablebuild/rune-go-sdk/api/browserapi"
)

type scheduledNotifications struct {
	notifications browserapi.Notifications
	ids           map[string]string
	schedule      func(func()) bool
}

func newScheduledNotifications(
	notifications browserapi.Notifications,
	schedule func(func()) bool,
) *scheduledNotifications {
	return &scheduledNotifications{
		notifications: notifications,
		schedule:      schedule,
		ids:           make(map[string]string),
	}
}

func (s scheduledNotifications) Notify(
	level browserapi.NotificationLevel, msg string, args ...any,
) (string, error) {
	if s.notifications == nil {
		return "", nil
	}
	id := uuid.New().String()
	ok := s.schedule(func() {
		actualID, _ := s.notifications.Notify(level, msg, args...)
		s.ids[id] = actualID
	})
	if !ok {
		return "", errors.New("could not schedule notification")
	}
	return id, nil
}

func (s scheduledNotifications) NotifyOnce(
	level browserapi.NotificationLevel, msg string, args ...any,
) (string, error) {
	if s.notifications == nil {
		return "", nil
	}
	id := uuid.New().String()
	ok := s.schedule(func() {
		actualID, _ := s.notifications.NotifyOnce(level, msg, args...)
		s.ids[id] = actualID
	})
	if !ok {
		return "", errors.New("could not schedule notification")
	}
	return id, nil
}

func (s scheduledNotifications) UpdateNotificationProgress(
	id, message string, progress, total int64,
) error {
	if s.notifications == nil {
		return nil
	}
	actualID := s.ids[id]
	if actualID == "" {
		return errors.New("notification not found")
	}
	ok := s.schedule(func() {
		_ = s.notifications.UpdateNotificationProgress(actualID, message, progress, total)
		s.ids[id] = actualID
	})
	if !ok {
		return errors.New("could not schedule notification update")
	}
	return nil
}
