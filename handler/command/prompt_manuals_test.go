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
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/unstablebuild/blue/document"
	"github.com/unstablebuild/rune-go-sdk/component"
	"github.com/unstablebuild/rune-go-sdk/term"
	"unstable.build/go-tui/handler/handlertest"
	"unstable.build/go-tui/localstorage/bluestore"
)

var goodTestCommands = []Manual{
	{Name: "subaru", Summary: "2021 top of the line, high tech.", Synopsis: "outback touring xt"},
	{Name: "jeep", Summary: "2022 bottom of the line, great offroading.", Synopsis: "gladiator sport s"},
	{Name: "mercedes", Summary: "2014 old luxury car.", Synopsis: "GL 450",
		Commands: []Manual{
			{Name: "GL", Summary: "GLs are 7 seater.", Synopsis: "[450]",
				Commands: []Manual{
					{Name: "350", Summary: "i don't know", Synopsis: ""},
					{Name: "450", Summary: "450 is middle tier", Synopsis: ""},
				},
			}},
	},
	{Name: "gladiator", AliasOf: []string{"jeep"}},
	{Name: "current", AliasOf: []string{"subaru", "jeep"}},
}

var goodLotsTestCommands = []Manual{
	{Name: "0", Summary: "The void.", Synopsis: "<nothing>"},
	{Name: "1", Summary: "Top of the line.", Synopsis: "<nothing>"},
	{Name: "2", Summary: "Next in kin"},
	{Name: "3", Summary: "Podium."},
	{Name: "4", Summary: "Who knows."},
	{Name: "5", Summary: "Who cares."},
	{Name: "6", Summary: "Say what?"},
	{Name: "7", Summary: "Cool."},
	{Name: "8", Summary: "Numbers."},
	{Name: "9", Summary: "Bottom of the line."},
}

func TestCommandHandlerManualsDraw(t *testing.T) {
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
mercedes                                
gladiator                               
current                                 
                                        
                                        
                                        
                                        
                                        
                                        
────────────────────────────────────────
                                        
USAGE                                   
subaru outback touring xt               
                                        
DESCRIPTION                             
2021 top of the line, high tech.        
                                        `},
		{"initializes with some commands search match", "e", goodTestCommands,
			`
e▐                                      
jeep                                    
mercedes                                
current                                 
                                        
                                        
                                        
                                        
                                        
                                        
                                        
────────────────────────────────────────
                                        
USAGE                                   
jeep gladiator sport s                  
                                        
DESCRIPTION                             
2022 bottom of the line, great          
offroading.                             
                                        `},
		{"initializes with lots of commands", "", goodLotsTestCommands,
			`
▐                                       
0                                       
1                                       
2                                       
3                                       
4                                       
5                                       
6                                       
7                                       
8                                       
9                                       
                                        
────────────────────────────────────────
                                        
USAGE                                   
0 <nothing>                             
                                        
DESCRIPTION                             
The void.                               
                                        `},

		{"command NOT in list with no args",
			"1", nil, `
1▐                                      
                                        
                                        
                                        
                                        
                                        
                                        
                                        
                                        
                                        
                                        
                                        
                                        
                                        
                                        
                                        
                                        
                                        
                                        
                                        `},
		{"fully typed command with args no subcommand delete in the middle",
			"merce ~^^^^^^^^^erce my", goodTestCommands, `
mercedes my▐                            
                                        
                                        
                                        
                                        
                                        
                                        
                                        
────────────────────────────────────────
                                        
USAGE                                   
mercedes GL 450                         
                                        
DESCRIPTION                             
2014 old luxury car.                    
                                        
SUB-COMMANDS                            
    - GL                                
                                        
                                        `},
		{"partially typed command with args no subcommand delete in the middle",
			"merce ~^^^^^^^^^erce my", goodTestCommands, `
mercedes my▐                            
                                        
                                        
                                        
                                        
                                        
                                        
                                        
────────────────────────────────────────
                                        
USAGE                                   
mercedes GL 450                         
                                        
DESCRIPTION                             
2014 old luxury car.                    
                                        
SUB-COMMANDS                            
    - GL                                
                                        
                                        `},
		{"fully typed command with space, completed via manual",
			"mercedes ", goodTestCommands, `
mercedes ▐                              
GL                                      
                                        
                                        
                                        
                                        
                                        
                                        
────────────────────────────────────────
                                        
USAGE                                   
mercedes GL 450                         
                                        
DESCRIPTION                             
2014 old luxury car.                    
                                        
SUB-COMMANDS                            
    - GL                                
                                        
                                        `},
		{"fully typed command with partially typed subcommand, completed via manual",
			"mercedes G", goodTestCommands, `
mercedes G▐                             
GL                                      
                                        
                                        
                                        
                                        
                                        
                                        
────────────────────────────────────────
                                        
USAGE                                   
mercedes GL 450                         
                                        
DESCRIPTION                             
2014 old luxury car.                    
                                        
SUB-COMMANDS                            
    - GL                                
                                        
                                        `},
		{"fully typed command with fully typed 1st subcommand, completed via manual",
			"mercedes GL", goodTestCommands, `
mercedes GL▐                            
GL                                      
                                        
                                        
                                        
                                        
                                        
                                        
────────────────────────────────────────
                                        
USAGE                                   
mercedes GL 450                         
                                        
DESCRIPTION                             
2014 old luxury car.                    
                                        
SUB-COMMANDS                            
    - GL                                
                                        
                                        `},
		{"fully typed command with fully typed 1st subcommand, with space, completed via manual",
			"mercedes GL ", goodTestCommands, `
mercedes GL ▐                           
350                                     
450                                     
                                        
                                        
                                        
                                        
────────────────────────────────────────
                                        
USAGE                                   
GL [450]                                
                                        
DESCRIPTION                             
GLs are 7 seater.                       
                                        
SUB-COMMANDS                            
    - 350                               
    - 450                               
                                        
                                        `},
		{"fully typed command with partially typed 2nd subcommand, completed via manual",
			"mercedes GL 4", goodTestCommands, `
mercedes GL 4▐                          
450                                     
                                        
                                        
                                        
                                        
                                        
────────────────────────────────────────
                                        
USAGE                                   
GL [450]                                
                                        
DESCRIPTION                             
GLs are 7 seater.                       
                                        
SUB-COMMANDS                            
    - 350                               
    - 450                               
                                        
                                        `},
		{"fully typed command with fully typed 2nd subcommand, completed via manual",
			"mercedes GL 450", goodTestCommands, `
mercedes GL 450▐                        
450                                     
                                        
                                        
                                        
                                        
                                        
────────────────────────────────────────
                                        
USAGE                                   
GL [450]                                
                                        
DESCRIPTION                             
GLs are 7 seater.                       
                                        
SUB-COMMANDS                            
    - 350                               
    - 450                               
                                        
                                        `},
		{"fully typed command with fully typed 2nd subcommand, with space, completed via manual",
			"mercedes GL 450 ", goodTestCommands, `
mercedes GL 450 ▐                       
                                        
                                        
                                        
                                        
                                        
                                        
                                        
                                        
                                        
                                        
                                        
────────────────────────────────────────
                                        
USAGE                                   
450                                     
                                        
DESCRIPTION                             
450 is middle tier                      
                                        `},
		{"fully typed command with fully typed 2nd subcommand, deleted last space, completed via manual",
			"mercedes GL 450 ^", goodTestCommands, `
mercedes GL 450▐                        
450                                     
                                        
                                        
                                        
                                        
                                        
────────────────────────────────────────
                                        
USAGE                                   
GL [450]                                
                                        
DESCRIPTION                             
GLs are 7 seater.                       
                                        
SUB-COMMANDS                            
    - 350                               
    - 450                               
                                        
                                        `},
		{"fully typed command with fully typed 2nd subcommand, deleted space and , half arg, completed via manual",
			"mercedes GL 450 ^^^", goodTestCommands, `
mercedes GL 4▐                          
450                                     
                                        
                                        
                                        
                                        
                                        
────────────────────────────────────────
                                        
USAGE                                   
GL [450]                                
                                        
DESCRIPTION                             
GLs are 7 seater.                       
                                        
SUB-COMMANDS                            
    - 350                               
    - 450                               
                                        
                                        `},
		{"fully typed command with fully typed 2nd subcommand, deleted last arg, completed via manual",
			"mercedes GL 450 ^^^^", goodTestCommands, `
mercedes GL ▐                           
350                                     
450                                     
                                        
                                        
                                        
                                        
────────────────────────────────────────
                                        
USAGE                                   
GL [450]                                
                                        
DESCRIPTION                             
GLs are 7 seater.                       
                                        
SUB-COMMANDS                            
    - 350                               
    - 450                               
                                        
                                        `},
		{"fully typed command with fully typed 2nd subcommand, deleted last arg, completed via manual",
			"mercedes GL 450 ^^^^^", goodTestCommands, `
mercedes GL▐                            
GL                                      
                                        
                                        
                                        
                                        
                                        
                                        
────────────────────────────────────────
                                        
USAGE                                   
mercedes GL 450                         
                                        
DESCRIPTION                             
2014 old luxury car.                    
                                        
SUB-COMMANDS                            
    - GL                                
                                        
                                        `},
		{"alias of multiple commands",
			"current", goodTestCommands, `
current▐                                
current                                 
                                        
                                        
                                        
                                        
────────────────────────────────────────
                                        
USAGE                                   
current                                 
                                        
DESCRIPTION                             
Alias of the following sequence of      
commands:                               
                                        
- subaru                                
- jeep                                  
                                        
                                        
                                        `},
		{"alias one command",
			"gladiator", goodTestCommands, `
gladiator▐                              
gladiator                               
                                        
                                        
                                        
                                        
                                        
                                        
                                        
                                        
                                        
────────────────────────────────────────
                                        
USAGE                                   
gladiator                               
                                        
DESCRIPTION                             
Alias of jeep                           
                                        
                                        `},
	}

	log.SetLevel(log.InfoLevel)
	for _, tcase := range tsuite {
		tcase := tcase
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
				{InputSequence: tcase.sequence, Expected: tcase.expectedDraw[1:]},
			}
			handlertest.TestHandlerSequence(t, testCommandHandler{b}, 40, 20, cases)
		})
	}
}
