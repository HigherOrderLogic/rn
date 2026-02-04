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

package command

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/unstablebuild/blue/document"
	"github.com/unstablebuild/blue/iterator"
	"github.com/unstablebuild/rune-go-sdk/component"
	"github.com/unstablebuild/rune-go-sdk/term"
	"unstable.build/go-tui/handler/handlertest"
	"unstable.build/go-tui/localstorage/bluestore"
)

func TestCommandHandlerManualsDrawTooSmallForManual(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ShowManualAfter = 0
	cfg.HistoryKey = term.KeyComb{Ch: '@'}
	cfg.FrameCharSet = component.FrameCharSetDefault()
	cfg.Sync = true

	tsuite := []struct {
		desc         string
		sequence     string
		commands     []Manual
		expectedDraw string
	}{
		{"initializes no commands empty", "", nil, `
▐                   
                    
                    
                    
                    
                    
                    
                    
                    
                    `},
		{"initializes no commands empty search yields 0", "a", nil, `
a▐                  
                    
                    
                    
                    
                    
                    
                    
                    
                    `},
		{"initializes with some commands", "", goodTestCommands,
			`
▐                   
subaru              
jeep                
────────────────────
USAGE               
subaru outback      
touring xt          
                    
DESCRIPTION         
2021 top of the     `},
		{"initializes with some commands search match", "e", goodTestCommands,
			`
e▐                  
jeep                
mercedes            
────────────────────
USAGE               
jeep gladiator      
sport s             
                    
DESCRIPTION         
2022 bottom of      `},
		{"initializes with lots of commands", "", goodLotsTestCommands,
			`
▐                   
0                   
1                   
────────────────────
USAGE               
0 <nothing>         
                    
DESCRIPTION         
The void.           
                    `},
		{"draw command NOT in list with no args",
			"1", nil, `
1▐                  
                    
                    
                    
                    
                    
                    
                    
                    
                    `},
		{"draw fully typed command with args with auto-complete with expanded last arg and delete in the middle",
			"merce ~^^^^^^^^^erce my", goodTestCommands, `
mercedes my▐        
                    
                    
────────────────────
USAGE               
mercedes GL 450     
                    
DESCRIPTION         
2014 old luxury     
car.                `},
	}

	for _, tcase := range tsuite {
		t.Run(tcase.desc, func(t *testing.T) {
			t.Parallel()
			dispatchFn, cleanup := nopDispatch()
			defer cleanup(t)

			completeFn, cleanupComplete := nopComplete()
			defer cleanupComplete(t)

			storage := bluestore.AdaptTo(document.NewInMemoryService())
			b := NewPrompt(
				storage, FuncCompleter(completeFn), FuncDispatcher(dispatchFn),
				term.NopInterrupter(), tcase.commands, cfg,
			)
			defer b.Close()
			cases := []handlertest.SequenceTestCase{
				{InputSequence: "_" + tcase.sequence + "_", Expected: tcase.expectedDraw[1:]},
			}
			handlertest.TestHandlerSequence(t, testCommandHandler{b}, 20, 10, cases)
		})
	}
}

func TestCommandHandlerPreview(t *testing.T) {
	t.Run("esc at the end", func(t *testing.T) {
		storage := bluestore.AdaptTo(document.NewInMemoryService())
		cfg := DefaultConfig()
		cfg.ShowManualAfter = 1 * time.Hour
		cfg.HistoryKey = term.KeyComb{Ch: '@'}
		cfg.Sync = true

		dispatchFn := func(cmd string, args ...string) bool {
			return true
		}

		var dispatches []string
		var state string
		previewFn := func(cmd string, args ...string) (component.Responsive, func(), bool) {
			prevState := state
			state = args[0]
			dispatches = append(dispatches, strings.Join(append([]string{cmd}, args...), " "))
			return nil, func() {
				state = prevState
			}, true
		}

		completeFn, cleanupComplete := completeWith("arg1", "arg2")()
		defer cleanupComplete(t)

		cmd := Manual{Name: "kotomichi"}
		interrupter := term.NopInterrupter()
		b := NewPrompt(
			storage, FuncCompleter(completeFn), FuncDispatcherWithPreview(dispatchFn, previewFn),
			interrupter, []Manual{cmd}, cfg,
		)
		defer b.Close()

		cases := []handlertest.SequenceTestCase{
			{InputSequence: "kotomichi ⬇⬇", Expected: `kotomichi ▐         
arg1                
arg2                
                    
                    
                    
                    
                    
                    
                    `},
		}
		handlertest.TestHandlerSequence(t, testCommandHandler{b}, 20, 10, cases)
		b.Wait()
		assert.Equal(t, "arg2", state)
		assert.Equal(t, []string{"kotomichi arg1", "kotomichi arg2"}, dispatches)

		cases = []handlertest.SequenceTestCase{
			{InputSequence: "⬆", Expected: `kotomichi ▐         
arg1                
arg2                
                    
                    
                    
                    
                    
                    
                    `},
		}
		handlertest.TestHandlerSequence(t, testCommandHandler{b}, 20, 10, cases)
		b.Wait()
		assert.Equal(t, "arg1", state)
		assert.Equal(t, []string{"kotomichi arg1", "kotomichi arg2", "kotomichi arg1"}, dispatches)

		cases = []handlertest.SequenceTestCase{
			{InputSequence: "⬆", Expected: `kotomichi ▐         
arg1                
arg2                
                    
                    
                    
                    
                    
                    
                    `},
		}
		handlertest.TestHandlerSequence(t, testCommandHandler{b}, 20, 10, cases)
		b.Wait()
		assert.Equal(t, "", state)
		assert.Equal(t, []string{"kotomichi arg1", "kotomichi arg2", "kotomichi arg1"}, dispatches)

		cases = []handlertest.SequenceTestCase{
			{InputSequence: "<", Expected: `kotomichi ▐         
arg1                
arg2                
                    
                    
                    
                    
                    
                    
                    `},
		}
		handlertest.TestHandlerSequence(t, testCommandHandler{b}, 20, 10, cases)
		b.Wait()
		assert.Equal(t, "", state)
		assert.Equal(t, []string{"kotomichi arg1", "kotomichi arg2", "kotomichi arg1"}, dispatches)
	})

	t.Run("dispatch at the end", func(t *testing.T) {
		storage := bluestore.AdaptTo(document.NewInMemoryService())
		cfg := DefaultConfig()
		cfg.ShowManualAfter = 1 * time.Hour
		cfg.HistoryKey = term.KeyComb{Ch: '@'}
		cfg.Sync = true

		var state string
		dispatchFn := func(cmd string, args ...string) bool {
			state = args[0]
			return true
		}

		var dispatches []string
		previewFn := func(cmd string, args ...string) (component.Responsive, func(), bool) {
			prevState := state
			state = args[0]
			dispatches = append(dispatches, strings.Join(append([]string{cmd}, args...), " "))
			return nil, func() {
				state = prevState
			}, true
		}

		completeFn, cleanupComplete := completeWith("arg1", "arg2")()
		defer cleanupComplete(t)

		cmd := Manual{Name: "kotomichi"}
		interrupter := term.NopInterrupter()
		b := NewPrompt(
			storage, FuncCompleter(completeFn), FuncDispatcherWithPreview(dispatchFn, previewFn),
			interrupter, []Manual{cmd}, cfg,
		)
		defer b.Close()

		cases := []handlertest.SequenceTestCase{
			{InputSequence: "kotomichi ⬇⬇", Expected: `kotomichi ▐         
arg1                
arg2                
                    
                    
                    
                    
                    
                    
                    `},
		}
		handlertest.TestHandlerSequence(t, testCommandHandler{b}, 20, 10, cases)
		b.Wait()
		assert.Equal(t, "arg2", state)
		assert.Equal(t, []string{"kotomichi arg1", "kotomichi arg2"}, dispatches)

		cases = []handlertest.SequenceTestCase{
			{InputSequence: "⬆ar✌>", Expected: `▐                   
kotomichi           
                    
                    
                    
                    
                    
                    
                    
                    `}, // unimportant after dispatching
		}
		handlertest.TestHandlerSequence(t, testCommandHandler{b}, 20, 10, cases)
		b.Wait()
		assert.Equal(t, "arg1", state)
		assert.Equal(t, []string{"kotomichi arg1", "kotomichi arg2", "kotomichi arg1"}, dispatches)
	})

	t.Run("preview returns manual", func(t *testing.T) {
		storage := bluestore.AdaptTo(document.NewInMemoryService())
		cfg := DefaultConfig()
		cfg.ShowManualAfter = 0
		cfg.HistoryKey = term.KeyComb{Ch: '@'}
		cfg.Sync = true

		dispatchFn := func(cmd string, args ...string) bool {
			return true
		}

		previewFn := func(cmd string, args ...string) (component.Responsive, func(), bool) {
			str := fmt.Sprintf("YAY %v", args)
			comp := component.NewResponsiveString(str,
				component.StringResponsiveConfig{})
			return comp, nil, true
		}

		completeFn, cleanupComplete := completeWith("arg1", "arg2")()
		defer cleanupComplete(t)

		cmd := Manual{Name: "kotomichi"}
		interrupter := term.NopInterrupter()
		b := NewPrompt(
			storage, FuncCompleter(completeFn), FuncDispatcherWithPreview(dispatchFn, previewFn),
			interrupter, []Manual{cmd}, cfg,
		)
		defer b.Close()

		cases := []handlertest.SequenceTestCase{
			{InputSequence: "<tab><down>", Expected: `kotomichi ▐         
arg1                
arg2                
                    
                    
                    
                    
                    
                    
YAY [arg1]          `},
		}
		handlertest.RunHandlerSequence(t, testCommandHandler{b}, 20, 10, cases)

		cases = []handlertest.SequenceTestCase{
			{InputSequence: "<up>", Expected: `kotomichi ▐         
arg1                
arg2                
                    
USAGE               
kotomichi           
                    
DESCRIPTION         
                    
                    `},
		}
		handlertest.RunHandlerSequence(t, testCommandHandler{b}, 20, 10, cases)

		cases = []handlertest.SequenceTestCase{
			{InputSequence: "<down><down>", Expected: `kotomichi ▐         
arg1                
arg2                
                    
                    
                    
                    
                    
                    
YAY [arg2]          `},
		}
		handlertest.RunHandlerSequence(t, testCommandHandler{b}, 20, 10, cases)

		cases = []handlertest.SequenceTestCase{
			{InputSequence: "<backspace>", Expected: `kotomichi▐          
kotomichi           
                    
                    
USAGE               
kotomichi           
                    
DESCRIPTION         
                    
                    `},
		}
		handlertest.RunHandlerSequence(t, testCommandHandler{b}, 20, 10, cases)

		cases = []handlertest.SequenceTestCase{
			{InputSequence: "<space>a<down>2", Expected: `kotomichi a2▐       
arg2                
                    
                    
USAGE               
kotomichi           
                    
DESCRIPTION         
                    
                    `},
		}
		handlertest.RunHandlerSequence(t, testCommandHandler{b}, 20, 10, cases)

		cases = []handlertest.SequenceTestCase{
			{InputSequence: "<down>", Expected: `kotomichi a2▐       
arg2                
                    
                    
                    
                    
                    
                    
                    
YAY [arg2]          `},
		}
		handlertest.RunHandlerSequence(t, testCommandHandler{b}, 20, 10, cases)
	})
}

func TestCommandHandlerDispatch(t *testing.T) {
	storage := bluestore.AdaptTo(document.NewInMemoryService())
	cfg := DefaultConfig()
	cfg.ShowManualAfter = 1 * time.Hour
	cfg.HistoryKey = term.KeyComb{Ch: '@'}
	cfg.Sync = true

	tsuite := []struct {
		desc        string
		sequence    string
		commands    []string
		completeCmd func() (func(ctx context.Context, args []string) (iterator.Iterator[string], string, error), func(*testing.T))
		dispatchCmd func() (func(command string, args ...string) bool, func(*testing.T))
	}{
		{"dispatches command NOT in list with no args",
			"1>", []string{"lane", "lorelai", "rori"},
			nopComplete, expectDispatch("1")},
		{"dispatches command in list with no args",
			"lo>", []string{"lane", "lorelai", "rori"},
			nopComplete, expectDispatch("lorelai")},
		{"dispatches command NOT in list with args no auto-complete",
			"1 /tmp/a>", []string{"lane", "lorelai", "rori"},
			nopComplete, expectDispatch("1", "/tmp/a")},
		{"dispatches command with args no auto-complete",
			"lo /tmp/a>", []string{"lane", "lorelai", "rori"},
			nopComplete, expectDispatch("lorelai", "/tmp/a")},
		{"dispatches command with args with auto-complete",
			"ro my#>", []string{"lane", "lorelai", "rori"},
			completeWith("myArg"), expectDispatch("rori", "myArg")},
		{"dispatches fully typed command with args with auto-complete",
			"rori my#>", []string{"lane", "lorelai", "rori"},
			completeWith("myArg"), expectDispatch("rori", "myArg")},
		{"dispatches fully typed command with args with auto-complete and delete in the middle",
			"rori ^ my#>", []string{"lane", "lorelai", "rori"},
			completeWith("myArg"), expectDispatch("rori", "myArg")},
		{"dispatches command with args with auto-complete one last space",
			"ro my# >", []string{"lane", "lorelai", "rori"},
			completeRespectively([]string{"myArg", "myArg"}), expectDispatch("rori", "myArg")},
		{"dispatches command with args with auto-complete space that's removed",
			"ro my# ^>", []string{"lane", "lorelai", "rori"},
			completeRespectively([]string{"myArg", "myArg"}), expectDispatch("rori", "myArg")},
		{"dispatches command with args with auto-complete delete and re-typed all",
			"ro my# ^^^^^^^^^^^^ro my# a>", []string{"lane", "lorelai", "rori"},
			completeWith("myArg"), expectDispatch("rori", "myArg", "a")},
		{"dispatches command from history no autocomplete",
			"rori myArg>lorelai myArg>@ oArg>", []string{"lane", "lorelai", "rori"},
			nopComplete, expectDispatch("lorelai", "myArg", "oArg")},
		{"dispatches command with extra spaces in args no auto-complete",
			"lo   /tmp/a>", []string{"lane", "lorelai", "rori"},
			nopComplete, expectDispatch("lorelai", "/tmp/a")},
		{"dispatches command with extra spaces in args that are deleted no auto-complete",
			"lo   ^^/tmp/a>", []string{"lane", "lorelai", "rori"},
			nopComplete, expectDispatch("lorelai", "/tmp/a")},
		{"dispatches command with multiple args and completer gets called for every character",
			"ro my#oro#>", []string{"lane", "lorelai", "rori"},
			expectCompleteWith(
				[][]string{
					{""}, {"m"}, {"myArg", ""}, {"myArg", "o"},
					{"myArg", "oregano", ""},
				},
				[][]string{
					{"myArg"}, {"myArg"}, {"myArg"}, {"oregano", "oregani"}, {"oregano", "oregani"},
					{"oregano", "oregani"}, {"oregano", "oregani"}, {"oregano", "oregani"}, {},
				}),
			expectDispatch("rori", "myArg", "oregano")},
		{"dispatch delete and re-type all with no autocomplete",
			"rori myArg ^^^^^^^^^^^rori myArg a>", []string{"lane", "lorelai", "rori"},
			nopComplete, expectDispatch("rori", "myArg", "a")},
		{"dispatch from history with autocomplete",
			"lo my#>ro my#>@ oArg>", []string{"lane", "lorelai", "rori"},
			completeWith("myArg"), expectDispatch("rori", "myArg", "oArg")},
		{"dispatch delete after load from history with autocomplete",
			"lo my#>ro my#>@^^^^^oArg>", []string{"lane", "lorelai", "rori"},
			completeWith("myArg"), expectDispatch("rori", "oArg")},
		{"dispatch delete after load from history with autocomplete scroll through history",
			"lo my#>ro my#>@@^^^^^oArg>", []string{"lane", "lorelai", "rori"},
			completeWith("myArg"), expectDispatch("lorelai", "oArg")},
		{"dispatch literal arg if history has option but user IS NOT scrolling",
			"lo my#>ro my#>lo m>", []string{"lane", "lorelai", "rori"},
			completeWith("myArg"), expectDispatch("lorelai", "m")},
		{"dispatch complete arg if history has option but user IS scrolling",
			"lo my#>ro my#>lo m*>", []string{"lane", "lorelai", "rori"},
			completeWith("myArg"), expectDispatch("lorelai", "myArg")},
		{"dispatch auto-complete with enter regardless of whether user IS scrolling",
			"lo>", []string{"lane", "lorelai", "rori"},
			completeWith("myArg"), expectDispatch("lorelai")},
		{"dispatch auto-complete with tab and enter regardless of whether user IS scrolling",
			"lo#>", []string{"lane", "lorelai", "rori"},
			completeWith("myArg"), expectDispatch("lorelai")},
	}

	for _, tcase := range tsuite {
		tcase := tcase
		t.Run(tcase.desc, func(t *testing.T) {
			dispatchFn, cleanup := tcase.dispatchCmd()
			defer cleanup(t)

			completeFn, cleanupComplete := tcase.completeCmd()
			defer cleanupComplete(t)

			interrupter := term.NopInterrupter()
			b := NewPrompt(
				storage, FuncCompleter(completeFn), FuncDispatcher(dispatchFn),
				interrupter, testNoManualCommands(tcase.commands), cfg,
			)
			defer b.Close()
			for _, ch := range tcase.sequence {
				b.Wait()
				switch ch {
				case '#':
					b.Handle(term.Event{Type: term.EventKey, Key: term.KeyTab})
				case '*':
					b.Handle(term.Event{Type: term.EventKey, Key: term.KeyArrowDown})
				case '>':
					b.Handle(term.Event{Type: term.EventKey, Key: term.KeyEnter})
				case '^':
					b.Handle(term.Event{Type: term.EventKey, Key: term.KeyBackspace})
				default:
					b.Handle(term.Event{Type: term.EventKey, Ch: ch})
				}
			}
		})
	}
}

func TestCommandHandlerDraw(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ShowManualAfter = 1 * time.Hour
	cfg.HistoryKey = term.KeyComb{Ch: '@'}
	cfg.Sync = true

	tsuite := []struct {
		desc         string
		sequence     string
		commands     []string
		completeCmd  func() (func(ctx context.Context, args []string) (iterator.Iterator[string], string, error), func(*testing.T))
		dispatchCmd  func() (func(command string, args ...string) bool, func(*testing.T))
		expectedDraw string
	}{
		{"initializes no commands empty", "", nil, nopComplete, nopDispatch, `
▐                   
                    
                    
                    
                    
                    
                    
                    
                    
                    `},
		{"initializes no commands empty search yields 0", "a", nil, nopComplete, nopDispatch, `
a▐                  
                    
                    
                    
                    
                    
                    
                    
                    
                    `},
		{"initializes with some commands",
			"", []string{"lane", "lorelai", "rori"},
			nopComplete, nopDispatch, `
▐                   
lane                
lorelai             
rori                
                    
                    
                    
                    
                    
                    `},
		{"initializes with some commands search match",
			"l", []string{"lane", "lorelai", "rori"},
			nopComplete, nopDispatch, `
l▐                  
lane                
lorelai             
                    
                    
                    
                    
                    
                    
                    `},
		{"initializes with lots of commands", "", lotsOfCommands,
			nopComplete, nopDispatch, `
▐                   
0                   
1                   
2                   
3                   
4                   
5                   
6                   
7                   
8                   `},
		{"draw command NOT in list with no args",
			"1", []string{"lane", "lorelai", "rori"},
			nopComplete, nopDispatch, `
1▐                  
                    
                    
                    
                    
                    
                    
                    
                    
                    `},
		{"draw command in list with no args",
			"lo", []string{"lane", "lorelai", "rori"},
			nopComplete, nopDispatch, `
lo▐                 
lorelai             
                    
                    
                    
                    
                    
                    
                    
                    `},
		{"draw command NOT in list with args no auto-complete",
			"1 /tmp/a", []string{"lane", "lorelai", "rori"},
			nopComplete, nopDispatch, `
1 /tmp/a▐           
                    
                    
                    
                    
                    
                    
                    
                    
                    `},
		{"draw command in list with args no auto-complete",
			"lo /tmp/a", []string{"lane", "lorelai", "rori"},
			nopComplete, nopDispatch, `
lorelai /tmp/a▐     
                    
                    
                    
                    
                    
                    
                    
                    
                    `},
		{"draw command with args with auto-complete",
			"rori my", []string{"lane", "lorelai", "rori"},
			completeWith("myArg"), nopDispatch, `
rori my▐            
myArg               
                    
                    
                    
                    
                    
                    
                    
                    `},
		{"draw command with args with auto-complete, expand last arg",
			"rori ~my", []string{"lane", "lorelai", "rori"},
			completeWith("expanded/myArg"), nopDispatch, `
rori expanded/my▐   
expanded/myArg      
                    
                    
                    
                    
                    
                    
                    
                    `},
		{"draw fully typed command with args with auto-complete",
			"rori my", []string{"lane", "lorelai", "rori"},
			completeWith("myArg"), nopDispatch, `
rori my▐            
myArg               
                    
                    
                    
                    
                    
                    
                    
                    `},
		{"draw fully typed command with args with auto-complete with expanded last arg and delete in the middle",
			"rori ~^^^^^^^^^my", []string{"lane", "lorelai", "rori"},
			completeWith("myArg"), nopDispatch, `
rori my▐            
myArg               
                    
                    
                    
                    
                    
                    
                    
                    `},
		{"draw fully typed command with args with auto-complete and delete in the middle",
			"rori ^ my", []string{"lane", "lorelai", "rori"},
			completeWith("myArg"), nopDispatch, `
rori my▐            
myArg               
                    
                    
                    
                    
                    
                    
                    
                    `},
		{"draw command with args with auto-complete one last space",
			"ro my ", []string{"lane", "lorelai", "rori"},
			completeRespectively([]string{"myArg"}), nopDispatch, `
rori my ▐           
                    
                    
                    
                    
                    
                    
                    
                    
                    `},
		{"draw command with args with auto-complete one last space that's removed",
			"ro my ^", []string{"lane", "lorelai", "rori"},
			completeRespectively([]string{"myArg"}), nopDispatch, `
rori my▐            
myArg               
                    
                    
                    
                    
                    
                    
                    
                    `},
		{"draw command with args with auto-complete delete and re-typed all",
			"ro my ^^^^^^^^^^^ro my a", []string{"lane", "lorelai", "rori"},
			completeRespectively([]string{"myArg"}), nopDispatch, `
rori my a▐          
                    
                    
                    
                    
                    
                    
                    
                    
                    `},
		{"draw command with args with auto-complete delete and re-typed all with expanded last arg",
			"ro ~my ^^^^^^^^^^^^^^^^^^^^ro my", []string{"lane", "lorelai", "rori"},
			completeWith("myArg"), nopDispatch, `
rori my▐            
myArg               
                    
                    
                    
                    
                    
                    
                    
                    `},
		{"draw delete and re-type all with no autocomplete",
			"rori myArg ^^^^^^^^^^^rori myArg a", []string{"lane", "lorelai", "rori"},
			nopComplete, nopDispatch, `
rori myArg a▐       
                    
                    
                    
                    
                    
                    
                    
                    
                    `},
		{"draw command from history no autocomplete",
			"rori myArg>lorelai myArg>@ oArg", []string{"lane", "lorelai", "rori"},
			nopComplete, expectDispatch("lorelai", "myArg"), `
lorelai myArg oAr   
g▐                  
                    
                    
                    
                    
                    
                    
                    
                    `},
		{"draw from history with autocomplete",
			"lo my✌>ro my✌>@", []string{"lane", "lorelai", "rori"},
			completeWith("myArg"), expectDispatch("rori", "myArg"), `
rori myArg▐         
myArg               
                    
                    
                    
                    
                    
                    
                    
                    `},
		{"draw from history with autocomplete with expanded last arg",
			"lo my✌>ro ~my✌>@", []string{"lane", "lorelai", "rori"},
			completeWith("expanded/myArg"), expectDispatch("rori", "expanded/myArg"), `
rori expanded/myA   
rg▐                 
expanded/myArg      
                    
                    
                    
                    
                    
                    
                    `},
		{"draw delete after load from history with autocomplete",
			"lo my✌>ro my✌>@^^^^^", []string{"lane", "lorelai", "rori"},
			completeWith("myArg"), expectDispatch("rori", "myArg"), `
rori ▐              
myArg               
                    
                    
                    
                    
                    
                    
                    
                    `},
		{"draw delete after load from history with autocomplete scroll through history",
			"lo my✌>ro my✌>@@^^^^^oArg", []string{"lane", "lorelai", "rori"},
			completeWith("myArg"), expectDispatch("rori", "myArg"), `
lorelai oArg▐       
                    
                    
                    
                    
                    
                    
                    
                    
                    `},
		{"draw command in list with extra spaces in args no auto-complete",
			"lo   /tmp/a", []string{"lane", "lorelai", "rori"},
			nopComplete, nopDispatch, `
lorelai   /tmp/a▐   
                    
                    
                    
                    
                    
                    
                    
                    
                    `},
		{"draw command in list with extra spaces in args that are deleted no auto-complete",
			"lo   ^^^ /tmp/a", []string{"lane", "lorelai", "rori"},
			nopComplete, nopDispatch, `
lorelai /tmp/a▐     
                    
                    
                    
                    
                    
                    
                    
                    
                    `},
		{"draw command with multiple args with auto-complete",
			"ro my✌ oro", []string{"lane", "lorelai", "rori"},
			expectCompleteWith(
				[][]string{
					{""}, {"m"}, {"myArg", ""}, {"myArg", ""},
					{"myArg", "o"},
				},
				[][]string{
					{"myArg"}, {"myArg"}, {"myArg"}, {"oregano", "oregani"}, {"oregano", "oregani"},
					{"oregano", "oregani"}, {"oregano", "oregani"}, {"oregano", "oregani"},
				}),
			nopDispatch, `
rori myArg  oro▐    
oregano             
                    
                    
                    
                    
                    
                    
                    
                    `},
		{"no completion uses historical positional args as completion list items",
			"ro myArg>ro my✌", []string{"lane", "lorelai", "rori"},
			expectCompleteWith(
				[][]string{
					{""}, {"m"}, {""}, {"m"}, {"myArg", ""},
				},
				[][]string{}),
			expectDispatch("rori", "myArg"), `
rori myArg ▐        
                    
                    
                    
                    
                    
                    
                    
                    
                    `},
		{"no completion uses historical positional args as completion list items (3rd argument)",
			"ro myArg oro>ro myArg or", []string{"lane", "lorelai", "rori"},
			expectCompleteWith(
				[][]string{
					{""}, {"m"}, {"myArg", ""}, {"myArg", "o"},
					{""}, {"m"}, {"myArg", ""}, {"myArg", "o"},
				},
				[][]string{}),
			expectDispatch("rori", "myArg", "oro"), `
rori myArg or▐      
oro                 
                    
                    
                    
                    
                    
                    
                    
                    `},
		{"no completion uses historical positional args as completion list items (3rd argument, tab)",
			"ro myArg oro>ro myArg o✌", []string{"lane", "lorelai", "rori"},
			expectCompleteWith(
				[][]string{
					{""}, {"m"}, {"myArg", ""}, {"myArg", "o"},
					{""}, {"m"}, {"myArg", ""}, {"myArg", "o"}, {"myArg", "oro", ""},
				},
				[][]string{}),
			expectDispatch("rori", "myArg", "oro"), `
rori myArg oro ▐    
                    
                    
                    
                    
                    
                    
                    
                    
                    `},
		{"fuzzy complete tab expands from historical args",
			"ro myArg oro>ro mo✌", []string{"lane", "lorelai", "rori"},
			expectCompleteWith(
				[][]string{
					{""}, {"m"}, {"myArg", ""}, {"myArg", "o"},
					{""}, {"m"}, {"myArg", "oro", ""},
				},
				[][]string{}),
			expectDispatch("rori", "myArg", "oro"), `
rori myArg oro ▐    
                    
                    
                    
                    
                    
                    
                    
                    
                    `},
		{"history items can be deleted on backspace keypress when scrolling",
			"ro myArg 1>ro myArg 2>ro myArg 3>ro myArg 4>ro myArg 5>ro my⬇⬇^",
			[]string{"lane", "lorelai", "rori"},
			nopComplete,
			expectDispatch("rori", "myArg", "5"), `
rori my▐            
myArg 1             
myArg 3             
myArg 4             
myArg 5             
                    
                    
                    
                    
                    `},

		{"when you deleted all list elements and keep pressing backspace you delete prompt chars",
			"ro myArg 1>ro myArg 2>ro myArg 3>ro my⬇^^^^",
			[]string{"lane", "lorelai", "rori"},
			nopComplete,
			expectDispatch("rori", "myArg", "3"), `
rori m▐             
                    
                    
                    
                    
                    
                    
                    
                    
                    `},
		{"non historical items cannot be deleted, instead command prompt takes the backspace as a char remove",
			"ro my✌ ore⬇^", []string{"lane", "lorelai", "rori"},
			expectCompleteWith(
				[][]string{
					{""}, {"m"}, {"myArg", ""}, {"myArg", ""}, {"myArg", "o"},
				},
				[][]string{
					{"myArg"}, {"myArg"}, {"myArg"}, {"oregano", "oregani"}, {"oregano", "oregani"},
					{"oregano", "oregani"}, {"oregano", "oregani"}, {"oregano", "oregani"}, {"oregano", "oregani"},
				}),
			nopDispatch, `
rori myArg  or▐     
oregani             
oregano             
                    
                    
                    
                    
                    
                    
                    `},

		{"backspace deletes characters instead of history elements when not scrolling",
			"ro myArg 1>ro myArg 2>ro myArg 3>ro myArg 4>ro myArg 5>ro myAr^^^",
			[]string{"lane", "lorelai", "rori"},
			nopComplete,
			expectDispatch("rori", "myArg", "5"), `
rori m▐             
myArg 1             
myArg 2             
myArg 3             
myArg 4             
myArg 5             
                    
                    
                    
                    `},
	}

	for _, tcase := range tsuite {
		tcase := tcase
		t.Run(tcase.desc, func(t *testing.T) {
			dispatchFn, cleanup := tcase.dispatchCmd()
			defer cleanup(t)

			completeFn, cleanupComplete := tcase.completeCmd()
			defer cleanupComplete(t)

			storage := bluestore.AdaptTo(document.NewInMemoryService())
			b := NewPrompt(
				storage, FuncCompleter(completeFn), FuncDispatcher(dispatchFn),
				term.NopInterrupter(), testNoManualCommands(tcase.commands), cfg,
			)
			defer b.Close()
			cases := []handlertest.SequenceTestCase{
				{InputSequence: tcase.sequence, Expected: tcase.expectedDraw[1:]},
			}
			handlertest.TestHandlerSequence(t, testCommandHandler{b}, 20, 10, cases)
		})
	}
}

type testNeverEndingIterator struct {
}

func (c testNeverEndingIterator) Next(context.Context) (string, bool) {
	time.Sleep(10 * time.Millisecond)
	return "hola 123", true
}

func (c testNeverEndingIterator) Err() error {
	return nil
}

func (c testNeverEndingIterator) Close() error {
	return nil
}

func neverEndingComplete() (func(context.Context, []string) (iterator.Iterator[string], string, error), func(*testing.T)) {
	return func(ctx context.Context, args []string) (iterator.Iterator[string], string, error) {
		it := testNeverEndingIterator{}
		return it, "123", nil
	}, func(*testing.T) {}
}

func TestCommandHandlerCancel(t *testing.T) {
	storage := bluestore.AdaptTo(document.NewInMemoryService())
	t.Run("ctrl-c once cancels search; twice closes window", func(t *testing.T) {
		dispatchFn, cleanup := nopDispatch()
		defer cleanup(t)

		completeFn, cleanupComplete := neverEndingComplete()
		defer cleanupComplete(t)

		b := NewPrompt(
			storage, FuncCompleter(completeFn), FuncDispatcher(dispatchFn),
			term.NopInterrupter(), nil, DefaultConfig(),
		)

		// Type in the command to stimulate the `neverEndingComplete` completion iterator
		for _, runeValue := range "hello " {
			quit, handled := b.handle(term.Event{Type: term.EventKey, Ch: runeValue}, false)
			require.False(t, quit)
			require.True(t, handled)
		}

		// must not quit window, instead it must cancel the never ending completion we set up
		quit, handled := b.handle(term.Event{Type: term.EventKey, Ch: 'c', Mod: term.ModCtrl}, true)
		assert.False(t, quit)
		assert.True(t, handled)

		// check the completion is canceled
		var completionCanceled bool
		select {
		case <-b.completionCtx.Done():
			completionCanceled = true
		default:
			completionCanceled = false
		}
		require.True(t, completionCanceled)

		// must quit window, since the completion is already canceled by the previous Ctrl-C
		quit, handled = b.handle(term.Event{Type: term.EventKey, Mod: term.ModCtrl, Ch: 'c'}, false)
		assert.True(t, quit)
		assert.True(t, handled)
	})
}

type testFeederIterator struct {
	feeder chan string
}

func (c testFeederIterator) Next(context.Context) (string, bool) {
	s, ok := <-c.feeder
	return s, ok
}

func (c testFeederIterator) Err() error {
	return errors.New("bang")
}

func (c testFeederIterator) Close() error {
	return nil
}

func TestCommandHandlerHistory(t *testing.T) {
	t.Run("remove historical item (start, middle and end of list)", func(t *testing.T) {
		dispatchFn, cleanup := nopDispatch()
		defer cleanup(t)

		completeFn, cleanupComplete := nopComplete()
		defer cleanupComplete(t)

		b := NewPrompt(
			bluestore.AdaptTo(document.NewInMemoryService()),
			FuncCompleter(completeFn),
			FuncDispatcher(dispatchFn),
			term.NopInterrupter(),
			nil,
			DefaultConfig(),
		)
		defer b.Close()

		// only historical items can be removed
		b.completingWithHistory = true

		// mock an async iterator we can feed elements to using a channel
		var slice []string
		ctx, cancel := context.WithCancel(b.ctx)
		for i := 0; i < 10; i++ {
			slice = append(slice, fmt.Sprintf("! echo xyz_%d", i))
		}
		it := iterator.FromSlice(slice)
		b.pushCompletionListSync(ctx, cancel, []string{"! echo"}, it)

		require.Equal(t, 10, b.list.TotalCount())

		// focus end and check it's xyz_9
		ok := b.list.FocusEnd()
		require.True(t, ok)
		match, ok := b.list.Focus()
		require.True(t, ok)
		require.Equal(t, "! echo xyz_9", string(match.Data()))

		// remove it!
		ok = b.list.RemoveFocus() // remove "! echo xyz_9"
		assert.True(t, ok)

		// focus should go up to xyz_8 because there was no more nodes after de
		// removed one
		match, ok = b.list.Focus()
		require.True(t, ok)
		assert.Equal(t, "! echo xyz_8", string(match.Data()))

		// focus end and check it's xyz_0
		ok = b.list.FocusStart()
		require.True(t, ok)
		match, ok = b.list.Focus()
		require.True(t, ok)
		require.Equal(t, "! echo xyz_0", string(match.Data()))

		// remove it!
		ok = b.list.RemoveFocus() // remove "! echo xyz_0"
		assert.True(t, ok)

		// focus should go up to xyz_1 because it's what comes next
		match, ok = b.list.Focus()
		require.True(t, ok)
		assert.Equal(t, "! echo xyz_1", string(match.Data()))

		// focus middle of list
		ok = b.list.FocusDown() // ! echo xyz_2
		require.True(t, ok)
		ok = b.list.FocusDown() // ! echo xyz_3
		require.True(t, ok)
		match, ok = b.list.Focus()
		require.True(t, ok)
		require.Equal(t, "! echo xyz_3", string(match.Data()))

		// remove it!
		ok = b.list.RemoveFocus() // remove "! echo xyz_0"
		assert.True(t, ok)

		// focus should go up to xyz_4 because it's what comes next
		match, ok = b.list.Focus()
		require.True(t, ok)
		assert.Equal(t, "! echo xyz_4", string(match.Data()))
	})

	t.Run("history concurrently pushing and removing doesn't "+
		"panic nor cause data races", func(t *testing.T) {
		dispatchFn, cleanup := nopDispatch()
		defer cleanup(t)

		completeFn, cleanupComplete := nopComplete()
		defer cleanupComplete(t)

		b := NewPrompt(
			bluestore.AdaptTo(document.NewInMemoryService()),
			FuncCompleter(completeFn),
			FuncDispatcher(dispatchFn),
			term.NopInterrupter(),
			nil,
			DefaultConfig(),
		)
		defer b.Close()

		// only historical items can be removed
		b.completingWithHistory = true

		// mock an async iterator we can feed elements to using a channel
		it := testFeederIterator{feeder: make(chan string)}

		// connect the consumption of the iterator to the population of the search list
		ctx, cancel := context.WithCancel(b.ctx)
		ch := b.list.Push(ctx)
		go b.pushCompletionList(ctx, ch, cancel, []string{"echo"}, it)

		startingPistol := make(chan struct{})

		var wg sync.WaitGroup
		wg.Add(2)

		numAdditions := 500
		numRemovals := 300

		go func() {
			defer wg.Done()
			<-startingPistol

			defer close(it.feeder)
			for i := 0; i < numAdditions; i++ {
				it.feeder <- fmt.Sprintf("abc_%d", i)
			}
		}()

		go func() {
			defer wg.Done()
			<-startingPistol

			time.Sleep(5 * time.Millisecond)
			b.list.FocusStart()

			// copy var so we don't introduce side effects when changing a variable
			// that's used in the for loop iteration scope
			nr := numRemovals

			for i := 0; i < numRemovals; i++ {
				b.list.FocusDown()
				if ok := b.list.RemoveFocus(); !ok {
					nr--
				}
			}
		}()

		close(startingPistol)
		wg.Wait()
	})
}

type testCommandHandler struct {
	*Prompt
}

func (t testCommandHandler) Handle(ev term.Event) (bool, bool) {
	t.Wait()
	quit, handled := t.Prompt.Handle(ev)
	t.Wait()
	return quit, handled
}

var (
	lotsOfCommands []string
)

func init() {
	for i := 0; i < 100; i++ {
		lotsOfCommands = append(lotsOfCommands, strconv.Itoa(i))
	}
}

func nopComplete() (
	func(context.Context, []string) (iterator.Iterator[string], string, error), func(*testing.T),
) {
	return func(ctx context.Context, args []string) (iterator.Iterator[string], string, error) {
		return iterator.FromSlice[string](nil), "", nil
	}, func(*testing.T) {}
}

func completeWith(data ...string) func() (
	func(context.Context, []string) (iterator.Iterator[string], string, error), func(*testing.T),
) {
	return func() (
		func(ctx context.Context, args []string) (iterator.Iterator[string], string, error), func(*testing.T),
	) {
		return func(ctx context.Context, args []string) (
				iterator.Iterator[string], string, error,
			) {
				args = args[1:]
				if len(args) != 0 && args[len(args)-1] == "~" {
					return iterator.FromSlice(data), "expanded/", nil
				}
				return iterator.FromSlice(data), "", nil
			},
			func(*testing.T) {}
	}
}

func completeRespectively(data []string) func() (
	func(context.Context, []string) (iterator.Iterator[string], string, error), func(*testing.T),
) {
	return func() (func(context.Context, []string) (iterator.Iterator[string], string, error), func(*testing.T)) {
		return func(ctx context.Context, args []string) (iterator.Iterator[string], string, error) {
			args = args[1:]
			if len(args) > len(data) {
				return iterator.FromSlice[string](nil), "", nil
			}
			completing := []string{data[len(args)-1]}
			return iterator.FromSlice(completing), "", nil
		}, func(*testing.T) {}
	}
}

func expectCompleteWith(expectedArgs [][]string, data [][]string) func() (
	func(ctx context.Context, args []string) (iterator.Iterator[string], string, error), func(*testing.T),
) {
	var actualArgsSlice [][]string
	var called int
	return func() (func(context.Context, []string) (iterator.Iterator[string], string, error), func(*testing.T)) {
		return func(ctx context.Context, args []string) (iterator.Iterator[string], string, error) {
				args = args[1:]
				if called >= len(data) {
					called++ // cleanup will catch it
					actualArgsSlice = append(actualArgsSlice, args)
					return iterator.FromSlice[string](nil), "", nil
				}
				actualArgsSlice = append(actualArgsSlice, args)
				ret := iterator.FromSlice(data[called])
				called++
				return ret, "", nil
			}, func(t *testing.T) {
				require.Equal(t, len(expectedArgs), called,
					"actual => %v", actualArgsSlice)
				for i, actualArgs := range actualArgsSlice {
					assert.Equal(t, expectedArgs[i], actualArgs, i)
				}
			}
	}
}

func nopDispatch() (func(string, ...string) bool, func(*testing.T)) {
	var called bool
	ret := func(command string, args ...string) bool {
		called = true
		return true
	}
	return ret, func(t *testing.T) {
		assert.False(t, called)
	}
}

func expectDispatch(expectedCmd string, expectedArgs ...string) func() (func(string, ...string) bool, func(*testing.T)) {
	return func() (func(string, ...string) bool, func(*testing.T)) {
		var called bool
		var actualCmd string
		var actualArgs []string
		ret := func(command string, args ...string) bool {
			called = true
			actualCmd = command
			actualArgs = args
			return true
		}
		return ret, func(t *testing.T) {
			assert.True(t, called)
			assert.Equal(t, expectedCmd, actualCmd)
			assert.Equal(t, append([]string{}, expectedArgs...), append([]string{}, actualArgs...))
		}
	}
}

func testNoManualCommands(cmds []string) (ret []Manual) {
	for _, cmd := range cmds {
		ret = append(ret, Manual{Name: cmd})
	}
	return
}
