// Unstable Build LLC ("COMPANY") CONFIDENTIAL
//
// Unpublished Copyright (c) 2023-2024 Unstable Build, All Rights Reserved.
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

package font

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestDeviceScaleNoMonitor reproduces the startup crash where
// ebiten.Monitor() returns nil before the window is associated with a
// monitor. Previously this panicked with a nil pointer dereference.
func TestDeviceScaleNoMonitor(t *testing.T) {
	orig := monitorScaleFactor
	t.Cleanup(func() { monitorScaleFactor = orig })

	monitorScaleFactor = func() (float64, bool) { return 0, false }

	assert.NotPanics(t, func() {
		assert.Equal(t, 1.0, deviceScale())
	})
}

func TestDeviceScaleWithMonitor(t *testing.T) {
	orig := monitorScaleFactor
	t.Cleanup(func() { monitorScaleFactor = orig })

	monitorScaleFactor = func() (float64, bool) { return 2.0, true }

	assert.Equal(t, 2.0, deviceScale())
}