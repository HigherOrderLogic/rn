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

package config

import (
	"github.com/unstablebuild/rune-go-sdk/api/config"
	"github.com/unstablebuild/rune-go-sdk/component"
	"github.com/unstablebuild/rune-go-sdk/term"
	tterm "unstable.build/go-tui/term"
)

// GetFrameCharset is a helper which extracts and parses a component.FrameCharSet.
func GetFrameCharset(c config.Config, key string, def component.FrameCharSet) (
	component.FrameCharSet, error,
) {
	cfg, err := c.GetConfig(key)
	if err != nil {
		return def, err
	}

	cs := def
	r, err := cfg.GetRune("topleft")
	if err == nil {
		cs.TopLeft = r
	}
	r, err = cfg.GetRune("topright")
	if err == nil {
		cs.TopRight = r
	}
	r, err = cfg.GetRune("bottomleft")
	if err == nil {
		cs.BottomLeft = r
	}
	r, err = cfg.GetRune("bottomright")
	if err == nil {
		cs.BottomRight = r
	}
	r, err = cfg.GetRune("horizontaltop")
	if err == nil {
		cs.HorizontalTop = r
	}
	r, err = cfg.GetRune("horizontalbottom")
	if err == nil {
		cs.HorizontalBottom = r
	}
	r, err = cfg.GetRune("verticalleft")
	if err == nil {
		cs.VerticalLeft = r
	}
	r, err = cfg.GetRune("verticalright")
	if err == nil {
		cs.VerticalRight = r
	}
	return cs, nil
}

// GetKey is a helper which extracts and parses a term.KeyComb as a string
// from a Config.
func GetKey(c config.Config, key string) (term.KeyComb, error) {
	s, err := c.GetString(key)
	if err != nil {
		return term.KeyComb{}, err
	}
	return tterm.ParseKey(s)
}
