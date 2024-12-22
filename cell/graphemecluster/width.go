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

package graphemecluster

import "github.com/rivo/uniseg"

// StepString returns the first grapheme cluster (user-perceived character) found in
// the given string. It also returns the monospace width of the cluster.
//
// See uniseg.StepString for more details.
func StepString(str string, state int) (
	cluster, rest string, width, newState int,
) {
	var boundaries int
	cluster, rest, boundaries, newState = uniseg.StepString(str, state)
	width = graphemeClusterWidth(cluster, boundaries)
	return
}

// StringWidth returns the monospace width for the given string, that is, the
// number of same-size cells to be occupied by the string.
func StringWidth(s string) (width int) {
	state := -1
	var w int
	for len(s) > 0 {
		_, s, w, state = StepString(s, state)
		width += w
	}
	return
}

func graphemeClusterWidth(cluster string, boundaries int) int {
	unisegWidth := boundaries >> uniseg.ShiftWidth
	if unisegWidth == 0 || unisegWidth > 1 || len(cluster) == 0 /* don't trust uniseg */ {
		return unisegWidth
	}

	switch cluster {
	// NOTE: this is just the icons that we're interested in
	// but we should add the full list of nerd font icons
	// width width > 1.
	case "", "", "", "", "", "", "", "", "", "", "", "",
		"", "", "", "", "", "", "", "", "","", " ",
		"󱫆", "", "", "":
		return 2
	default:
		return unisegWidth
	}
}
