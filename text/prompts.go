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

package text

import (
	"fmt"

	log "github.com/sirupsen/logrus"
	"github.com/unstablebuild/rune-go-sdk/api/browserapi"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"github.com/unstablebuild/rune-go-sdk/term"
	"unstable.build/go-tui/handler"
	"unstable.build/go-tui/workspace"
)

func (c *Component) openAreYouSurePrompt(file workspaceapi.URI) {
	const (
		yesOpt = "Yes"
		noOpt  = "No"
	)

	msg := fmt.Sprintf(`File %s
has been updated since the last back up was created.
Are you sure you want to recover it
and lose all the new updates?`, file)

	c.comp.Prompt(msg, []string{yesOpt, noOpt},
		[]term.KeyComb{{Ch: 'Y'}, {Ch: 'N'}},
		handler.FuncPromptHandler(
			func(i int, opt string) {

				var h browserapi.Handler
				var err error

				switch opt {
				case yesOpt:
					var swapDir, swapFile workspaceapi.URI
					swapDir, err = c.getSwapDir(file)
					if err == nil {
						swapFile, err = workspace.DefaultSwapFile(swapDir, file)
						if err == nil {
							h, err = c.openFileTab(file, swapFile, false, true)
						}
					}
				case noOpt:
				}
				if h != nil {
					err = c.comp.Focus().SetContent(h)
				}
				if err != nil {
					c.log(log.ErrorLevel, "recovery prompt: %v", err)
					_, _ = c.Notify(browserapi.LevelError, "%v", err)
					return
				}
			},
			func() error { return nil }))
}

func (c *Component) openRecoveryPrompt(file workspaceapi.URI) {
	const (
		recoverOpt  = "Recover"
		readOnlyOpt = "Open rdonly"
		editOpt     = "force Edit"
		skipOpt     = "Skip"
	)

	msg := fmt.Sprintf("File %s is already open by another process "+
		"or an edit session for this file crashed.", file)

	invokeWindow := c.comp.Focus()
	if invokeWindow.IsFloating() {
		// NOTE: opened recovery prompt from another prompt or floating window.
		// User probably wants to set the content of a tiled window,
		// not a floating window
		nextWindow, ok := c.comp.Shiftable()
		// this should always be true, since the last tile can never be closed
		// and Shiftable/ShiftFocus only return tiled windows.
		if ok {
			invokeWindow = nextWindow
		}
	}

	c.comp.Prompt(msg, []string{recoverOpt, readOnlyOpt, editOpt, skipOpt},
		[]term.KeyComb{{Ch: 'R'}, {Ch: 'O'}, {Ch: 'E'}, {Ch: 'S'}},
		handler.FuncPromptHandler(
			func(i int, opt string) {
				var h browserapi.Handler
				var err error

				switch opt {
				case recoverOpt:
					var swapDir, swapFile workspaceapi.URI
					swapDir, err = c.getSwapDir(file)
					if err == nil {
						swapFile, err = workspace.DefaultSwapFile(swapDir, file)
						if err == nil {
							h, err = c.recoverOpenFileTab(file, swapFile, false)
							if err == workspaceapi.ErrStaleData {
								c.openAreYouSurePrompt(file)
								return
							}
						}
					}
				case readOnlyOpt:
					h, err = c.OpenFileTab(file, true)
				case editOpt:
					var swapDir, swapFile workspaceapi.URI
					swapDir, err = c.getSwapDir(file)
					if err == nil {
						swapFile, err = workspace.DefaultSwapFile(swapDir, file)
						if err == nil {
							err = c.workspace.Remove(swapFile.Path())
							if err == nil {
								h, err = c.OpenFileTab(file, false)
							}
						}
					}
				case skipOpt:
				}
				if h != nil {
					if invokeWindow.Closed() {
						invokeWindow = c.comp.Focus()
					}
					err = invokeWindow.SetContent(h)
				}
				if err != nil {
					c.log(log.ErrorLevel, "recovery prompt: %v", err)
					_, _ = c.Notify(browserapi.LevelError, "%v", err)
				}
			},
			func() error { return nil }))
}
