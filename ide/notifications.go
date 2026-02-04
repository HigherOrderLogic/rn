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
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/unstablebuild/blue/document"
	"github.com/unstablebuild/rune-go-sdk/api/browserapi"
	"unstable.build/go-tui/component/notifications"
)

type notifier interface {
	Notify(level notifications.Level, msg string) string
	ID(level notifications.Level, msg string) string
	UpdateProgress(id, message string, progress, total int64) bool
	PauseAll()
	ResumeAll()
	CloseAll()
}

type workspaceNotifications struct {
	notifier notifier
	storage  document.Service
}

func newWorkspaceNotifications(
	storage document.Service, notifier notifier,
) *workspaceNotifications {
	return &workspaceNotifications{
		storage:  storage,
		notifier: notifier,
	}
}

// stand-in type for NotifyOnce
type storedNotification struct {
	ID string
}

// Notify formats the given msg and args and displays it on next Draw.
func (c *workspaceNotifications) Notify(
	level browserapi.NotificationLevel, msg string, args ...any,
) (string, error) {
	return c.notifier.Notify(notifications.Level(level), fmt.Sprintf(msg, args...)), nil
}

// NotifyOnce behaves like Notify, but only sends this notification once.
func (c *workspaceNotifications) NotifyOnce(
	level browserapi.NotificationLevel, msg string, args ...any,
) (string, error) {
	ctx := context.Background()
	id := c.notifier.ID(notifications.Level(level), fmt.Sprintf(msg, args...))
	var value storedNotification
	value.ID = id
	err := c.storage.Create(ctx, id, value)
	if err == document.ErrAlreadyExists {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("storage create: %v", err)
	}
	return c.notifier.Notify(notifications.Level(level), fmt.Sprintf(msg, args...)), nil
}

// UpdateNotificationProgress satisfies browser.Notifications.
func (c *workspaceNotifications) UpdateNotificationProgress(
	id, message string, progress, total int64,
) error {
	ok := c.notifier.UpdateProgress(id, message, progress, total)
	if !ok {
		return errors.New("could not find notification with the " +
			"given id, or it already expired")
	}
	return nil
}

func (c *workspaceNotifications) Close() error {
	if closer, ok := c.notifier.(io.Closer); ok {
		return closer.Close()
	}
	return nil
}
