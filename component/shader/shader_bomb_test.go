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

package shader

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/unstablebuild/rune-go-sdk/term"
)

func TestBomb(t *testing.T) {
	t.Run("the smaller the ring start the greater the ring scale is", func(t *testing.T) {
		params := DefaultBombParams()
		sh := Bomb(params, term.Attributes{}).(*bomb)
		ringScale1 := sh.ringScale
		params.RingStart -= 0.1
		sh = Bomb(params, term.Attributes{}).(*bomb)
		ringScale2 := sh.ringScale
		assert.Less(t, ringScale2, ringScale1)
	})

	t.Run("the greater the ring start the smaller the ring scale is", func(t *testing.T) {
		params := DefaultBombParams()
		sh := Bomb(params, term.Attributes{}).(*bomb)
		ringScale1 := sh.ringScale
		params.RingStart += 0.1
		sh = Bomb(params, term.Attributes{}).(*bomb)
		ringScale2 := sh.ringScale
		assert.Greater(t, ringScale2, ringScale1)
	})

	t.Run("the smaller the ring end the smaller the ring scale is", func(t *testing.T) {
		params := DefaultBombParams()
		sh := Bomb(params, term.Attributes{}).(*bomb)
		ringScale1 := sh.ringScale
		params.RingEnd -= 0.1
		sh = Bomb(params, term.Attributes{}).(*bomb)
		ringScale2 := sh.ringScale
		assert.Greater(t, ringScale2, ringScale1)
	})

	t.Run("the greater the ring end the greater the ring scale is", func(t *testing.T) {
		params := DefaultBombParams()
		sh := Bomb(params, term.Attributes{}).(*bomb)
		ringScale1 := sh.ringScale
		params.RingEnd += 0.1
		sh = Bomb(params, term.Attributes{}).(*bomb)
		ringScale2 := sh.ringScale
		assert.Less(t, ringScale2, ringScale1)
	})

}
