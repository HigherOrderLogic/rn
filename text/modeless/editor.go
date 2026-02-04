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

package modeless

import (
	"errors"

	"github.com/unstablebuild/rune-go-sdk/api/textapi"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"unstable.build/go-tui/cell"
	"unstable.build/go-tui/ide/vctrl/vctrlcmd"
	"unstable.build/go-tui/text"
)

// Editor allocates storage for a new Editor and initializes it.
func Editor(opts ...Option) text.Editor {
	ret := new(editor)
	ret.modelessConfig = defaultConfig()
	for _, o := range opts {
		o(&ret.modelessConfig)
	}
	ret.pub.Init()
	if ret.modelessConfig.registry != nil {
		ret.fileRegistry = text.NewFileCommandRegistry(
			ret.modelessConfig.workspace, ret.modelessConfig.registry)
	}
	ret.opts = opts
	return ret
}

type editor struct {
	modelessConfig
	fileRegistry text.FileCommandRegistry
	pub          text.Publisher
	opts         []Option
}

func (e *editor) Edit(
	file workspaceapi.URI, buf *cell.Buffer, readOnly, recovered bool,
) (ret text.Handler, err error) {
	handler := NewHandler(buf, file, e.opts...)
	ret = handler
	cursor := &handler.(*editorHandler).cursor
	if e.fileRegistry != nil {
		var err error
		ret, err = text.SubscribeLocationCommands(file, e.fileRegistry, ret)
		if err != nil {
			return nil, err
		}
		ret, err = text.SubscribeFoldCommands(file, e.fileRegistry, cursor, ret)
		if err != nil {
			return nil, err
		}
		ret, err = vctrlcmd.SubscribeGitCommands(file, e.fileRegistry,
			ret, e.auxBarConfig.Service, e.clipboard, e.notifications)
		if err != nil {
			return nil, err
		}
	}
	scroll := handler.(*editorHandler).less.Scroll()
	ret = e.pub.PublishEdit(file, buf, ret, cursor)
	auxBarConfig := e.auxBarConfig
	auxBarConfig.CommandRegistry = e.fileRegistry
	gitBarConfig := e.gitBarConfig
	gitBarConfig.CommandRegistry = e.fileRegistry
	if e.enableAuxBar {
		ret = text.WithAuxBar(ret, buf, scroll, auxBarConfig)
		if e.enableGitBar {
			ret = text.WithGitBar(e.auxBarConfig.Service, ret, buf,
				scroll, gitBarConfig)
		}
	}
	if !e.statusBarEnabled {
		return ret, nil
	}
	bar := text.WithStatusBar(ret, buf, scroll, readOnly, recovered, e.statusBarConfig)
	handler.(*editorHandler).setStatusBar(bar)
	if !e.commandBar {
		bar.ShowCommandBar(false)
	}
	return bar, nil
}

func (e *editor) SubscribeCommand(cmd textapi.CommandManual, h text.CommandHandler) error {
	return errors.New("not supported")
}

func (c *editor) UnsubscribeCommand(cmd string) error {
	return errors.New("not supported")
}

func (e *editor) Editor(file workspaceapi.URI) (text.Handler, error) {
	return nil, errors.New("not supported")
}

func (e *editor) SubscribeEvents(evs []textapi.EventType, sub text.EventHandler) error {
	e.pub.SubscribeEvents(evs, sub)
	return nil
}

func (e *editor) UnsubscribeEvents(sub text.EventHandler) (bool, error) {
	ok := e.pub.UnsubscribeEvents(sub)
	return ok, nil
}
