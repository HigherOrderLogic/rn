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
	"fmt"

	"github.com/unstablebuild/rune-go-sdk/term"
	"unstable.build/go-tui/text"
)

func validateConfig(cfg map[string]any) (err error) {
	c := ideConfig{cfg: cfg, errors: make(map[string]error)}

	if err = validateAliases(&c, cfg); err != nil {
		return
	}

	if err = validateCommandPrompt(&c, cfg); err != nil {
		return
	}
	return
}

func validateAliases(c *ideConfig, cfg map[string]any) (err error) {
	err = text.ValidateCommandAliases(c.commandAliases())
	if err != nil {
		// void aliases but keep the rest of config intact.
		// this ensures that text.NewComponent doesn't hard error,
		// preventing user from editing using this same IDE.
		_, ok := c.cfg["command"]
		if !ok {
			panic("empty command config but detected invalid aliases")
		}
		cfg["command"].(map[string]any)[keyCommandAliases] = map[string]any{}
		err = fmt.Errorf("'command.%s' is invalid: %w", keyCommandAliases, err)
	}
	return
}

func validateCommandPrompt(c *ideConfig, cfg map[string]any) (err error) {
	commandKey := c.commandKey()
	editorMode := c.editorMode()

	switch editorMode {
	case editorModeModal:
		/* no validation needed */
	case editorModeModeless:
		// unfortunately <c-space> is mapped and dispatched with Ch == ' '.
		// Handle edge case to avoid false positive.
		isCtrlSpace := commandKey.Ch == ' ' &&
			commandKey.Key == term.KeySpace && commandKey.Mod == term.ModCtrl
		isIncompatible := !isCtrlSpace && commandKey.Ch != 0 && commandKey.Mod == 0

		if isIncompatible {
			cfg["command"].(map[string]any)[keyCommandKey] = "<c-space>"
			return fmt.Errorf("command key must use ctrl or alt modifiers in " +
				"modeless editor mode otherwise you wouldn't be able to activate it," +
				" falling back to <c-space>")
		}
	}
	return
}
