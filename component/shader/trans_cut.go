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
	"math"

	"github.com/unstablebuild/rune-go-sdk/term"
)

// TransitionCut jumps from one shader to the other without an interpolation.
func TransitionCut(params TransitionCutParams, shader1, shader2 Shader) Shader {
	if params.ChangeAtPerc < 0.0 || params.ChangeAtPerc > 1.0 {
		panic("ChangeAtPerc must be within the closed interval [0,1]")
	}

	return &transitionCut{
		TransitionCutParams: params,
		shader1:             shader1,
		shader2:             shader2,
	}
}

// DefaultTransitionCutParams return a set of sane TransitionCutParams.
func DefaultTransitionCutParams() TransitionCutParams {
	return TransitionCutParams{
		ChangeAtPerc: 0.5,
	}
}

// TransitionCutParams defines the parameters used by the TransitionCut shader.
type TransitionCutParams struct {
	// Point within closed interval [0.0,1.0] at which the shader change happens.
	ChangeAtPerc float64
}

type transitionCut struct {
	TransitionCutParams
	shader1 Shader
	shader2 Shader
}

func (t *transitionCut) Shade(frame, total int, in [][]term.Cell) {
	shader1Total := int(math.Round(float64(total) * (t.ChangeAtPerc)))
	shader2Total := total - shader1Total
	if frame < shader1Total {
		t.shader1.Shade(frame, shader1Total, in)
	} else {
		t.shader2.Shade(frame-shader1Total, shader2Total, in)
	}
}
