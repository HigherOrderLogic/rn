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

package extension

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/unstablebuild/blue/logging"
	"github.com/unstablebuild/rune-go-sdk/api/browserapi"
	"github.com/unstablebuild/rune-go-sdk/api/config"
	"github.com/unstablebuild/rune-go-sdk/api/extensionapi"
	"github.com/unstablebuild/rune-go-sdk/api/textapi"
	"github.com/unstablebuild/rune-go-sdk/component"
	"github.com/unstablebuild/rune-go-sdk/iterator"
	"github.com/unstablebuild/rune-go-sdk/term"
	"github.com/unstablebuild/rune-go-sdk/tui"
	"unstable.build/go-tui/api/browserapi/browserext"
	"unstable.build/go-tui/extension"
	"unstable.build/go-tui/extension/extutil"
	"unstable.build/go-tui/rpc"
)

const (
	commandChaosHandler              = "chaoshandler"
	commandChaosUpdateEventLatency   = "chaoseventlatency"
	commandChaosUpdateCommandLatency = "chaoscommandlatency"
	copy                             = `
              . . .                         
              \|/                          
            '--+--'                        
              /|\                          
             ' | '                         
               |                           
               |                           
           ,--'#'--.                       
           |#######|                       
        _.-'#######'-._                    
     ,-'###############'-.                 
   ,'#####################',               
  /#########################\              
 |###########################|             
|#############################|            
|#############################|            
|#############################|            
|#############################|            
 |###########################|             
  \#########################/              
   '.#####################,'               
     '._###############_,'                 
        '--..#####..--'
`
)

type chaosCommandHandler struct {
	ctx            context.Context
	cancelCtx      func()
	ed             textapi.Editor
	wm             browserapi.WindowManager
	eventLatency   atomic.Value
	commandLatency atomic.Value
}

func newChaosCommandHandler(
	ctx context.Context, ed textapi.Editor, grants []extension.Grant,
	broker rpc.MuxBroker, pconfig config.Config,
) (extutil.CommandEventHandler, error) {
	ret := new(chaosCommandHandler)
	ret.ed = ed
	ret.ctx, ret.cancelCtx = context.WithCancel(context.Background())
	ret.eventLatency.Store(time.Duration(0))
	ret.commandLatency.Store(time.Duration(0))

	var err error
	for _, grant := range grants {
		switch grant.Permission {
		case extensionapi.PermissionBrowserWindowManager:
			ret.wm, err = browserext.WindowManager(ctx, grant, broker)
			if err != nil {
				return nil, err
			}
		}
	}

	return ret, nil
}

func (h *chaosCommandHandler) Handle(ctx context.Context, ev textapi.Event) bool {
	h.log(log.TraceLevel, "handle event start")
	timer := time.NewTimer(h.eventLatency.Load().(time.Duration))
	select {
	case <-h.ctx.Done():
	case <-timer.C:
	}
	h.log(log.TraceLevel, "handle event end")
	return false
}

func (s *chaosCommandHandler) log(level log.Level, msg string, args ...interface{}) {
	log.WithField(logging.KeyClass, "extchaos.chaosCommandHandler").
		Logf(level, msg, args...)
}

func (t *chaosCommandHandler) Complete(
	ctx context.Context, name string, args []string,
) (iterator.Iterator[string], error) {
	if name != commandChaosHandler || len(args) > 1 {
		return iterator.FromSlice[string](nil), nil
	}

	return iterator.FromSlice[string]([]string{"panic", "slow"}), nil
}

func (h *chaosCommandHandler) HandleCommand(ctx context.Context, cmd textapi.Command) (
	err error,
) {
	h.log(log.TraceLevel, "handle command start")
	defer h.log(log.TraceLevel, "handle command end")

	timer := time.NewTimer(h.commandLatency.Load().(time.Duration))
	select {
	case <-h.ctx.Done():
		err = h.ctx.Err()
		return
	case <-timer.C:
	}
	switch cmd.Name {
	case commandChaosHandler:
		return h.handleNewChaosHandler(ctx, cmd)
	case commandChaosUpdateEventLatency:
		return h.handleUpdateEventLatency(ctx, cmd)
	case commandChaosUpdateCommandLatency:
		return h.handleUpdateCommandLatency(ctx, cmd)
	}

	return
}

func (h *chaosCommandHandler) Close() error {
	h.cancelCtx()
	return nil
}

func (h *chaosCommandHandler) handleUpdateEventLatency(
	ctx context.Context, cmd textapi.Command,
) error {
	return h.handleUpdateDuration(ctx, cmd, &h.eventLatency)
}

func (h *chaosCommandHandler) handleUpdateCommandLatency(
	ctx context.Context, cmd textapi.Command,
) error {
	return h.handleUpdateDuration(ctx, cmd, &h.commandLatency)
}

func (h *chaosCommandHandler) handleUpdateDuration(
	ctx context.Context, cmd textapi.Command,
	duration *atomic.Value,
) error {
	var nextDuration time.Duration
	if len(cmd.Args) > 0 {
		dur, err := time.ParseDuration(cmd.Args[0])
		if err != nil {
			return fmt.Errorf("parse duration: %w", err)
		}
		nextDuration = dur
	}

	duration.Store(nextDuration)
	return nil
}

func (h *chaosCommandHandler) handleNewChaosHandler(
	ctx context.Context, cmd textapi.Command,
) error {
	comp := component.NewStringWithConfig(copy, component.StringConfig{
		Alignment: component.AlignmentCentered,
	})

	panic := len(cmd.Args) == 0 || cmd.Args[0] == "panic"

	var sleepDuration time.Duration
	if len(cmd.Args) > 1 && cmd.Args[0] == "slow" {
		dur, err := time.ParseDuration(cmd.Args[1])
		if err != nil {
			return fmt.Errorf("parse duration: %w", err)
		}
		sleepDuration = dur
	}

	tuiHandler := &chaosHandler{sleepTime: sleepDuration, panic: panic, comp: comp}
	syncHandler := browserapi.SyncHandler(new(sync.Mutex), tuiHandler)

	_, err := h.wm.Split(browserapi.OrientationDefault, cmd.Window, syncHandler)
	if err != nil {
		return fmt.Errorf("split: %w", err)
	}
	return nil
}

type chaosHandler struct {
	comp      tui.Component
	panic     bool
	sleepTime time.Duration
}

func (h *chaosHandler) Resize(width, height int) {
	if !h.panic {
		time.Sleep(h.sleepTime)
	}
	h.comp.Resize(width, height)
}

func (h *chaosHandler) Draw(w term.Writer) {
	if !h.panic {
		time.Sleep(h.sleepTime)
	}
	h.comp.Draw(w)
}

func (h *chaosHandler) Handle(ev term.Event) (exit, handled bool) {
	if h.panic {
		panic("kaboom")
	}
	time.Sleep(h.sleepTime)
	handled = true
	return
}

func (h *chaosHandler) Selection() (string, bool) {
	if !h.panic {
		time.Sleep(h.sleepTime)
	}
	return "", false
}

func (h *chaosHandler) Cursor() (
	pos term.Coordinates, style term.CursorStyle, show bool,
) {
	if !h.panic {
		time.Sleep(h.sleepTime)
	}
	return
}

func (h *chaosHandler) Close() error {
	if !h.panic {
		time.Sleep(h.sleepTime)
	}
	return nil
}
