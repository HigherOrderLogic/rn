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

package vi

import (
	"errors"

	"github.com/unstablebuild/rune-go-sdk/api/textapi"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"unstable.build/go-tui/cell"
	"unstable.build/go-tui/ide/vctrl/vctrlcmd"
	"unstable.build/go-tui/text"
)

type viEditor struct {
	text.Publisher
	config   viConfig
	registry text.FileCommandRegistry
	opts     []Option
}

// Editor returns a Vi text.Editor.
func Editor(opts ...Option) text.Editor {
	ret := &viEditor{opts: opts}
	for _, o := range opts {
		o(&ret.config)
	}
	ret.Publisher.Init()
	if ret.config.registry != nil {
		ret.registry = text.NewFileCommandRegistry(ret.config.workspace, ret.config.registry)
	}
	return ret
}

func (e *viEditor) Edit(
	file workspaceapi.URI, buf *cell.Buffer, readOnly, recovered bool,
) (text.Handler, error) {
	root := New(buf, file, e.opts...)
	// publisher does not mutate cursor and it should never do so
	cursor := root.cursor
	scroll := root.less.Scroll()
	var ret text.Handler = root
	if e.registry != nil {
		var err error
		ret, err = text.SubscribeLocationCommands(file, e.registry, ret)
		if err != nil {
			return nil, err
		}
		ret, err = text.SubscribeFoldCommands(file, e.registry, cursor, ret)
		if err != nil {
			return nil, err
		}
		ret, err = vctrlcmd.SubscribeGitCommands(
			file, e.registry, ret, e.config.auxBarConfig.Service,
			e.config.clipboard, e.config.notifications)
		if err != nil {
			return nil, err
		}
	}
	ret = e.Publisher.PublishEdit(file, buf, ret, cursor)
	auxBarConfig := e.config.auxBarConfig
	auxBarConfig.CommandRegistry = e.registry
	gitBarConfig := e.config.gitBarConfig
	gitBarConfig.CommandRegistry = e.registry
	if e.config.enableAuxBar {
		ret = text.WithAuxBar(ret, buf, scroll, auxBarConfig)
		if e.config.enableGitBar {
			ret = text.WithGitBar(e.config.auxBarConfig.Service, ret, buf,
				scroll, gitBarConfig)
		}
	}
	if !e.config.statusBarEnabled {
		return ret, nil
	}
	bar := text.WithStatusBar(ret, buf, scroll,
		readOnly, recovered, e.config.statusBarConfig)
	root.setStatusBar(bar)
	return bar, nil
}

// SubscribeCommand is not supported.
func (e *viEditor) SubscribeCommand(cmd textapi.CommandManual, h text.CommandHandler) error {
	return errors.New("not supported")
}

func (c *viEditor) UnsubscribeCommand(cmd string) error {
	return errors.New("not supported")
}

// Editor is not supported
func (e *viEditor) Editor(file workspaceapi.URI) (text.Handler, error) {
	// NOTE: it would be dead code
	return nil, errors.New("not supported")
}

// SubscribeEvents subsribes sub to ev. Note that this Editor is only capable
// of dispatching EventTypeOpen, EventTypeEdit EventType events.
func (e *viEditor) SubscribeEvents(
	evs []textapi.EventType, sub text.EventHandler,
) error {
	e.Publisher.SubscribeEvents(evs, sub)
	return nil
}

func (e *viEditor) UnsubscribeEvents(sub text.EventHandler) (bool, error) {
	ok := e.Publisher.UnsubscribeEvents(sub)
	return ok, nil
}
