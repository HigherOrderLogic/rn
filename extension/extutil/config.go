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

package extutil

import (
	"errors"
	"fmt"

	"github.com/unstablebuild/rune-go-sdk/api/config"
	"unstable.build/go-tui/browser"
	"unstable.build/go-tui/clipboard"
	"unstable.build/go-tui/clipboard/sysclip"
	"unstable.build/go-tui/component"
	"unstable.build/go-tui/text"
	"unstable.build/go-tui/text/modeless"
	"unstable.build/go-tui/text/vi"
)

// Tabspaces extract editor.tabspaces from the given cfg.
func Tabspaces(cfg config.Config) (int, error) {
	editorConfig, err := cfg.GetConfig("editor")
	if err != nil {
		if err != config.ErrNotFound {
			err = fmt.Errorf("failed to get 'editor' from config: %v", err)
			return 0, err
		}
		return component.DefaultTabspaces, nil
	}

	ret, err := editorConfig.GetInt("tabspaces")
	if err != nil {
		if err != config.ErrNotFound {
			err = fmt.Errorf("failed to get 'tabspaces' from config: %v", err)
			return 0, err
		}
		ret = component.DefaultTabspaces
	}
	return ret, nil
}

// WindowManagerFrame extract browser.window_manager.frame from the given cfg.
func WindowManagerFrame(cfg config.Config) (bool, error) {
	browserConfig, err := cfg.GetConfig("browser")
	if err != nil {
		if err != config.ErrNotFound {
			err = fmt.Errorf("failed to get 'browser' from config: %v", err)
			return false, err
		}
		return browser.DefaultConfig().Frame, nil
	}

	wmConfig, err := browserConfig.GetConfig("window_manager")
	if err != nil {
		if err != config.ErrNotFound {
			err = fmt.Errorf("failed to get 'window_manager' from config: %v", err)
			return false, err
		}
		return browser.DefaultConfig().Frame, nil
	}

	ret, err := wmConfig.GetBool("frame")
	if err != nil {
		if err != config.ErrNotFound {
			err = fmt.Errorf("failed to get 'frame' from config: %v", err)
			return false, err
		}
		ret = browser.DefaultConfig().Frame
	}
	return ret, nil
}

// Clipboard returns the configured clipboard.
func Clipboard(cfg config.Config) (clipboard.Register, error) {
	sys, err := cfg.GetString("clipboard")
	if err != nil && err != config.ErrNotFound {
		err = fmt.Errorf("failed to get 'clipboard' from config: %v", err)
		return nil, err
	}

	if err == config.ErrNotFound {
		sys = "memory"
	}

	switch sys {
	case "memory":
		return clipboard.NewInMemory(), nil
	case "system":
		clip, err := sysclip.NewRegister()
		if err != nil {
			return clipboard.NewInMemory(), nil
		}
		return clip, nil
	default:
		return nil, errors.New("unknown clipboard")
	}
}

// Editor returns the editor implementation as configured.
func Editor(clipboard clipboard.Register, cfg config.Config) (text.Editor, error) {
	mode, err := editorMode(cfg)
	if err != nil {
		return nil, err
	}
	switch mode {
	case "modal":
		return vi.Editor(vi.WithClipboard(clipboard)), nil
	default:
		return modeless.Editor(modeless.WithClipboard(clipboard)), nil
	}
}

// Wrap returns the current editor implementation is configured with wrap mode.
func Wrap(cfg config.Config) (bool, error) {
	var def bool
	edConfig, err := cfg.GetConfig("editor")
	if err != nil {
		if err != config.ErrNotFound {
			err = fmt.Errorf("failed to get 'editor' from config: %v", err)
			return false, err
		}
		return false, nil
	}

	mode, err := editorMode(cfg)
	if err != nil {
		return false, err
	}
	modeConfig, err := edConfig.GetConfig(mode)
	if err != nil {
		if err != config.ErrNotFound {
			err = fmt.Errorf("failed to get '%s' from editor config: %v", mode, err)
			return false, err
		}
		return def, nil
	}

	wrap, err := modeConfig.GetBool("wrap")
	if err != nil {
		if err != config.ErrNotFound {
			err = fmt.Errorf("failed to get 'wrap' from editor config: %v", err)
			return false, err
		}
	}
	return wrap, nil
}

func editorMode(cfg config.Config) (string, error) {
	def := "modeless"
	edConfig, err := cfg.GetConfig("editor")
	if err != nil {
		if err != config.ErrNotFound {
			err = fmt.Errorf("failed to get 'editor' from config: %v", err)
			return "", err
		}
		return def, nil
	}

	mode, err := edConfig.GetString("mode")
	if err != nil {
		if err != config.ErrNotFound {
			err = fmt.Errorf("failed to get 'mode' from editor config: %v", err)
			return "", err
		}
		mode = def
	}
	return mode, nil
}
