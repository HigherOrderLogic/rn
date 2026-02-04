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

package texttest

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/unstablebuild/rune-go-sdk/api/textapi"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"unstable.build/go-tui/cell"
	"unstable.build/go-tui/component"
	"github.com/unstablebuild/rune-go-sdk/component/comptest"
	"github.com/unstablebuild/rune-go-sdk/term"
	"unstable.build/go-tui/text"
)

func TestGitBarDraw(t *testing.T) {
	buf := cell.NewBuffer()
	buf.WriteString(copy)
	fs := &testFoldsService{}
	fs.view = buf.WithView(fs)
	scroll := component.NewScroll(buf)
	h := newTestHandler(scroll)
	var wg sync.WaitGroup
	var mu sync.Mutex
	cb := func(fn func()) bool {
		mu.Lock()
		defer mu.Unlock()
		fn()
		wg.Done()
		return true
	}
	uri, err := workspaceapi.ParseURI("memory:///")
	require.NoError(t, err)

	registry := text.NewFileCommandRegistry(uri, newWorkspaceRegistry())
	ed := &TestEditor{}
	mockSvc := &differ{}
	wg.Add(1)
	mu.Lock()
	cfg := text.GitBarConfig{CommandRegistry: registry, ScheduleNextTick: cb, Publisher: ed}
	bar := text.WithGitBar(mockSvc, h, buf, scroll, cfg)
	bar.Resize(15, 10)
	mu.Unlock()
	w := term.NewStringWriter(15, 10)

	tests := []comptest.TestCase{
		{Expected: `
  package main 
               
  import (     
      "fmt"    
+              
+     "github.c
  )            
               
  func main() {
-     fmt.Print`,
		},
	}
	wg.Wait()
	mu.Lock()
	comptest.TestComponent(t, bar, w, tests)
	require.True(t, scroll.SeekDown())
	mu.Unlock()

	tests = []comptest.TestCase{
		{Expected: `
               
  import (     
      "fmt"    
+              
+     "github.c
  )            
               
  func main() {
-     fmt.Print
      for i := `,
		},
	}
	mu.Lock()
	comptest.TestComponent(t, bar, w, tests)
	mu.Unlock()

	require.NoError(t, bar.Close())

	// unsubscribes
	require.Equal(t, 2, len(ed.subs))
	assert.Equal(t, 0, len(ed.subs[textapi.EventTypeFlush]))
	assert.Equal(t, 0, len(ed.subs[textapi.EventTypeFocus]))
}
