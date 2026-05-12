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

package vte

import (
	"context"
	"os"
	"path"
	"sync"
	"testing"

	"github.com/ernestrc/sensible/find"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/unstablebuild/rune-go-sdk/api/browserapi"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"github.com/unstablebuild/rune-go-sdk/clipboard"
	"github.com/unstablebuild/rune-go-sdk/term"
	"unstable.build/go-tui/cell"
	"unstable.build/go-tui/component"
	"unstable.build/go-tui/term/vte/vtescreen"
	"unstable.build/go-tui/term/vte/vtetest"
	"unstable.build/go-tui/text"
)

func TestHandlerViIntegration(t *testing.T) {
	t.Parallel()
	t.Run("search", func(t *testing.T) {
		t.Parallel()
		cases := []vtetest.Case{
			{"echo bla></bla",
				`$ echo bla          
bla                 
$                   
                    
                    
                    
                    
                    
                    
/bla▐               `},
			{">",
				`$ echo ▐la          
bla                 
$                   
                    
                    
                    
                    
                    
                    
     searching 'bla'`},
			{"n",
				`$ echo bla          
▐la                 
$                   
                    
                    
                    
                    
                    
                    
     searching 'bla'`},
			{"/.bla>",
				`$ echo bla          
▐la                 
$                   
                    
                    
                    
                    
                    
                    
    searching '.bla'`},
			{"/ibla>",
				`$ echo bla          
▐la                 
$                   
                    
                    
                    
                    
                    
                    
    searching 'ibla'`},
		}
		cfg := DefaultConfig()
		cfg.Modal = true
		testSequence(t, cfg, defaultWaitForIdleVte, cases)
	})

	t.Run("edit/movement", func(t *testing.T) {
		t.Parallel()
		cases := []vtetest.Case{
			{"echo bla>",
				`$ echo bla          
bla                 
$ ▐                 
                    
                    
                    
                    
                    
                    
                    `},
			{"<", // cursor should stay the same entering vi mode
				`$ echo bla          
bla                 
$ ▐                 
                    
                    
                    
                    
                    
                    
                    `},
			{"k0veyj\\$iecho <p", // copy and paste
				`$ echo bla          
bla                 
$ echo bl▐          
                    
                    
                    
                    
                    
                    
                    `},
			{"bved",
				`$ echo bla          
bla                 
$ echo▐             
                    
                    
                    
                    
                    
                    
                    `},
			{"0Cecho bla12345678901234567890", // delete line and insert wrap around
				`$ echo bla          
bla                 
$ echo bla1234567890
1234567890▐         
                    
                    
                    
                    
                    
                    `},
			{"<hhhrolll", // replace
				`$ echo bla          
bla                 
$ echo bla1234567890
123456o89▐          
                    
                    
                    
                    
                    
                    `},
			{"u", // undo doesn' panic
				`$ echo bla          
bla                 
$ echo bla1234567890
123456o89▐          
                    
                    
                    
                    
                    
                    `},
			{"aaaaaaaaaaaaaaaaaaaaaaaaa",
				`$ echo bla          
bla                 
$ echo bla1234567890
123456o890aaaaaaaaaa
aaaaaaaaaaaaaa▐     
                    
                    
                    
                    
                    `},
			{"<rXa",
				`$ echo bla          
bla                 
$ echo bla1234567890
123456o890aaaaaaaaaa
aaaaaaaaaaaaaX▐     
                    
                    
                    
                    
                    `},
			{"<>>>", // ensure that attr bar doesn't occlude last line in shell mode
				`bla                 
$ echo bla1234567890
123456o890aaaaaaaaaa
aaaaaaaaaaaaaX      
bla1234567890123456o
890aaaaaaaaaaaaaaaaa
aaaaaaX             
$                   
$                   
$ ▐                 `},
			{"<", // ensure that attr bar doesn't occlude last line in vi mode
				`bla                 
$ echo bla1234567890
123456o890aaaaaaaaaa
aaaaaaaaaaaaaX      
bla1234567890123456o
890aaaaaaaaaaaaaaaaa
aaaaaaX             
$                   
$                   
$ ▐                 `},
			{"iclear>",
				`$ ▐                 
                    
                    
                    
                    
                    
                    
                    
                    
                    `},
			{"echo bla", // clear is respected on shell insert
				`$ echo bla▐         
                    
                    
                    
                    
                    
                    
                    
                    
                    `},
			{"<hi", // clear is respected when switching in/out of vi
				`$ echo b▐a          
                    
                    
                    
                    
                    
                    
                    
                    
                    `},
			{">", // enter on vi mode, bypasses vi
				`$ echo bla          
bla                 
$ ▐                 
                    
                    
                    
                    
                    
                    
                    `},
			{"echo \"<k0llvk0yG0lllllpjla\"", // multiline paste
				`$ echo bla          
bla                 
$ echo "$ echo blabl
a"▐                 
                    
                    
                    
                    
                    
                    `},
			{">", // execute paste
				`$ echo bla          
bla                 
$ echo "$ echo blabl
a"                  
$ echo blabla       
$ ▐                 
                    
                    
                    
                    `},
			{"1Z<0\\$aX<0", // a after $
				`$ echo bla          
bla                 
$ echo "$ echo blabl
a"                  
$ echo blabla       
$ ▐ZX               
                    
                    
                    
                    `},
			{"⬆⬆", // position after scroll up through history
				`$ echo bla          
bla                 
$ echo "$ echo blabl
a"                  
$ echo blabla       
$ echo bl▐          
                    
                    
                    
                    `},
			{"\\$", // $ after scroll through history
				`$ echo bla          
bla                 
$ echo "$ echo blabl
a"                  
$ echo blabla       
$ echo bl▐          
                    
                    
                    
                    `},
			{"kkkk#a", // ctrl-c exits vi mode, no matter where cursor is
				`$ echo bla          
bla                 
$ echo "$ echo blabl
a"                  
$ echo blabla       
$ echo blaa▐        
                    
                    
                    
                    `},
			{"<0D\\$", // $ end of line if only prompt stays at prompt
				`$ echo bla          
bla                 
$ echo "$ echo blabl
a"                  
$ echo blabla       
$ ▐                 
                    
                    
                    
                    `},
			{"iecho '.i\\$'>", // is able to use special characters in shell mode
				`$ echo bla          
bla                 
$ echo "$ echo blabl
a"                  
$ echo blabla       
$ echo '.i$'        
.i$                 
$ ▐                 
                    
                    `},
			{"<⬇⬇⬇⬇", // position after scroll down through history to the start
				`$ echo bla          
bla                 
$ echo "$ echo blabl
a"                  
$ echo blabla       
$ echo '.i$'        
.i$                 
$ ▐                 
                    
                    `},
		}

		cfg := DefaultConfig()
		cfg.Modal = true
		testSequence(t, cfg, defaultWaitForIdleVte, cases)
	})

	t.Run("dollar key with multiline prompt line", func(t *testing.T) {
		t.Parallel()
		cases := []vtetest.Case{
			{"echo blaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				`$ echo blaaaaaaaaaaa
aaaaaaaaaaaaaaaaaaaa
aaaaaaaaaaaaaaaa▐   
                    
                    
                    
                    
                    
                    
                    `},
			{"<0",
				`$ echo blaaaaaaaaaaa
aaaaaaaaaaaaaaaaaaaa
▐aaaaaaaaaaaaaaa    
                    
                    
                    
                    
                    
                    
                    `},
			{"\\$\\$",
				`$ echo blaaaaaaaaaaa
aaaaaaaaaaaaaaaaaaaa
aaaaaaaaaaaaaaa▐    
                    
                    
                    
                    
                    
                    
                    `},
		}
		cfg := DefaultConfig()
		cfg.Modal = true
		testSequence(t, cfg, defaultWaitForIdleVte, cases)
	})

	t.Run("multiline go up before prompt start", func(t *testing.T) {
		t.Parallel()
		cases := []vtetest.Case{
			{"echo blaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				`$ echo blaaaaaaaaaaa
aaaaaaaaaaaaaaaaaaaa
aaaaaaaaaaaaaaaa▐   
                    
                    
                    
                    
                    
                    
                    `},
			{"<0kk",
				`$ ▐cho blaaaaaaaaaaa
aaaaaaaaaaaaaaaaaaaa
aaaaaaaaaaaaaaaa    
                    
                    
                    
                    
                    
                    
                    `},
			{"\\$\\$0",
				`$ ▐cho blaaaaaaaaaaa
aaaaaaaaaaaaaaaaaaaa
aaaaaaaaaaaaaaaa    
                    
                    
                    
                    
                    
                    
                    `},
		}
		cfg := DefaultConfig()
		cfg.Modal = true
		testSequence(t, cfg, defaultWaitForIdleVte, cases)
	})

	t.Run("multiline paste", func(t *testing.T) {
		t.Parallel()
		cases := []vtetest.Case{
			{"echo blaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa>",
				`$ echo blaaaaaaaaaaa
aaaaaaaaaaaaaaaaaaaa
aaaaaaaaaaaaaaaa    
blaaaaaaaaaaaaaaaaaa
aaaaaaaaaaaaaaaaaaaa
aaaaaaaaa           
$ ▐                 
                    
                    
                    `},
		}
		cfg := DefaultConfig()
		cfg.Modal = true
		testSequence(t, cfg, defaultWaitForIdleVte, cases)
	})

	t.Run("last line wrap around", func(t *testing.T) {
		t.Parallel()
		cases := []vtetest.Case{
			{">>>>>>>>>>",
				`$                   
$                   
$                   
$                   
$                   
$                   
$                   
$                   
$                   
$ ▐                 `},
			{"echo<0Cecho aaaaaaaaaaaaabcde",
				`$                   
$                   
$                   
$                   
$                   
$                   
$                   
$                   
$ echo aaaaaaaaaaaaa
bcde▐               `},
		}
		cfg := DefaultConfig()
		cfg.Modal = true
		testSequence(t, cfg, defaultWaitForIdleVte, cases)
	})

	t.Run("tab", func(t *testing.T) {
		t.Parallel()
		cases := []vtetest.Case{
			{"echo<0Cecho \ta",
				`$ echo  ▐           
                    
                    
                    
                    
                    
                    
                    
                    
                    `},
		}
		cfg := DefaultConfig()
		cfg.Modal = true
		testSequence(t, cfg, defaultWaitForIdleVte, cases)
	})

	t.Run("copy/paste", func(t *testing.T) {
		t.Parallel()
		cases := []vtetest.Case{
			{"printf 'abc\\\\x00\\\\n'><k0velllyjj0iecho '<p",
				`$ printf 'abc\x00\n'
abc                 
$ echo 'ab▐         
                    
                    
                    
                    
                    
                    
                    `},
		}
		cfg := DefaultConfig()
		clip := clipboard.NewInMemory()
		cfg.Clipboard = clip
		cfg.Modal = true
		testSequence(t, cfg, defaultWaitForIdleVte, cases)

		// assert data that leaks outside of handler via clipboard
		data, err := clip.Paste(clipboard.DefaultRegisterID)
		require.NoError(t, err)
		assert.Equal(t, "abc", data.Text)
	})

	t.Run("go to start of buffer, go to end of buffer", func(t *testing.T) {
		t.Parallel()
		cases := []vtetest.Case{
			{"echo a>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>echo<",
				`$                   
$                   
$                   
$                   
$                   
$                   
$                   
$                   
$                   
$ ech▐              `},
			{"gg",
				`$ ech▐ a            
a                   
$                   
$                   
$                   
$                   
$                   
$                   
$                   
$                   `},
			{"G",
				`$                   
$                   
$                   
$                   
$                   
$                   
$                   
$                   
$                   
$ ech▐              `},
		}
		cfg := DefaultConfig()
		cfg.Modal = true
		testSequence(t, cfg, defaultWaitForIdleVte, cases)
	})
}

func TestZshEdgeCases(t *testing.T) {
	t.Parallel()
	// only run this if zsh is present in system running test harness
	zshPath, err := find.Executable("zsh")
	if err != nil {
		t.SkipNow()
	}

	tempDir, err := os.MkdirTemp("", "")
	require.NoError(t, err)

	f, err := os.Create(path.Join(tempDir, ".zshrc"))
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = f.Close()
		_ = os.Remove(f.Name())
	})

	_, err = f.Write([]byte(`
bindkey '^a' beginning-of-line
bindkey '^g' beep
setopt COMBINING_CHARS
PS1='$ '
`))
	require.NoError(t, err)

	os.Setenv("ZDOTDIR", tempDir)

	t.Run("insert mode edit wrap-around", func(t *testing.T) {
		cases := []vtetest.Case{
			{"echo blaaaaaa<0Cecho blaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				`$ echo blaaaaaaaaaaa
aaaaaaaaaaaaaaaaaaaa
aaaaaaaaaaaaaaaa▐   
                    
                    
                    
                    
                    
                    
                    `},
		}
		cfg := DefaultConfig()
		cfg.Modal = true
		testSequenceShell(t, cfg, defaultWaitForIdleVte, zshPath, cases)
	})
}

func TestViEditUnit(t *testing.T) {
	t.Parallel()
	t.Run("screen context bypasses Edit", func(t *testing.T) {
		t.Parallel()
		comp := newTestParentComponent("a\nb", term.Coordinates{})
		var vi viHandler
		vi.doInit(comp, DefaultConfig())

		ctx := vtescreen.NewContext(context.Background())
		vi.Edit(ctx, term.Coordinates{}, term.Coordinates{Y: 1, X: 1}, "b\nb")
		assert.Equal(t, "b\nb", comp.scroll.Buffer().String())
	})

	t.Run("restore primary scroll refreshes editable buffers", func(t *testing.T) {
		t.Parallel()
		comp := newTestParentComponent("$ stale ", term.Coordinates{X: 2})
		var vi viHandler
		cfg := DefaultConfig()
		var actualBellsRung int
		cfg.RingBell = func() {
			actualBellsRung++
		}
		vi.doInit(comp, cfg)
		vi.remote = newTestRemote(comp.scroll, comp.cursor)
		vi.Resize(18, 18)

		restored := newTestParentComponent("$ restored ", term.Coordinates{X: 11})
		comp.scroll = restored.scroll
		comp.cursor = restored.cursor
		vi.restorePrimaryScroll()
		vi.remote = newTestRemote(comp.scroll, comp.cursor)

		from, to, old := vi.Edit(context.Background(), comp.cursor, comp.cursor, "X")

		assert.Equal(t, "$ restored X", comp.scroll.Buffer().String())
		assert.Equal(t, term.Coordinates{X: 11}, from)
		assert.Equal(t, term.Coordinates{X: 12}, to)
		assert.Equal(t, "", old)
		assert.Equal(t, 0, actualBellsRung)
	})

	// Pins the contract that constructing a fresh viSyncState does not
	// mutate the underlying cell.Buffer's editor. Doing so off-lock
	// previously created a race where restorePrimaryScroll's
	// newSyncState swapped b.Cells.editor → viHandler, but the
	// follow-up applySyncState (which captures the prior editor as
	// v.sync.editor) was still blocked on the component lock. A
	// concurrent vte-parser Input then routed its mutation through the
	// dangling v.sync.editor (an editor over the previous Cells), the
	// new Cells was never extended, and AltBuffer.WriteAt looped
	// forever calling InsertAt → WriteAt → InsertAt at full CPU.
	//
	// The fix moves the WithEditor swap into applySyncState (under the
	// caller's lock). This test guards against regressions by
	// asserting newSyncState leaves the buffer's editor untouched.
	t.Run("newSyncState does not swap editor off-lock", func(t *testing.T) {
		t.Parallel()
		comp := newTestParentComponent("hi", term.Coordinates{})
		var vi viHandler
		vi.doInit(comp, DefaultConfig())

		// doInit's applySyncState installed viHandler as the editor on
		// comp.scroll.Buffer(). Restore it to a known sentinel so we
		// can detect whether the next newSyncState alters it.
		buf := comp.scroll.Buffer()
		var sentinel sentinelEditor
		prev := buf.WithEditor(&sentinel)
		require.Equal(t, cell.Editor(&vi), prev,
			"doInit must register viHandler as the buffer's editor; "+
				"got %T", prev)

		_ = vi.newSyncState(comp, vi.viOptions())

		// If newSyncState calls WithEditor (the regressed behavior),
		// it would have replaced our sentinel with viHandler. Detect
		// that by triggering a write through the buffer's editor: a
		// hit on the sentinel proves newSyncState left it alone.
		_, _, _ = buf.Editor().Edit(
			context.Background(),
			term.Coordinates{}, term.Coordinates{}, "x")
		assert.True(t, sentinel.called,
			"newSyncState must NOT mutate the buffer's editor "+
				"off-lock; doing so opens a race with concurrent "+
				"vte-parser writes that recurses AltBuffer.WriteAt "+
				"↔ InsertAt forever. Editor swap belongs in "+
				"applySyncState, under the caller's lock.")
	})

	suite := []struct {
		description    string
		initialContent string
		// it is assumed that when Edit is invoked the component cursor
		// is at the start of the prompt.
		promptStart term.Coordinates
		start       term.Coordinates
		end         term.Coordinates
		str         string

		expectedRingBell bool
		expectedContent  string
		expectedFrom     term.Coordinates
		expectedTo       term.Coordinates
		expectedOld      string
	}{
		{
			description:      "(invalid) zero Edit",
			initialContent:   "",
			promptStart:      term.Coordinates{},
			start:            term.Coordinates{},
			end:              term.Coordinates{},
			str:              "",
			expectedRingBell: true,
			expectedContent:  "",
		},
		{
			description: "insert within last prompt line, exactly after prompt",
			initialContent: `
~/src/blue master
$ 
`,
			start:            term.Coordinates{Y: 2, X: 2},
			end:              term.Coordinates{Y: 2, X: 2},
			promptStart:      term.Coordinates{Y: 2, X: 2},
			str:              "echo",
			expectedRingBell: false,
			expectedFrom:     term.Coordinates{Y: 2, X: 2},
			expectedTo:       term.Coordinates{Y: 2, X: 6},
			expectedOld:      "",
			expectedContent: `
~/src/blue master
$ echo
`,
		},
		{
			description: "insert within last prompt line, before prompt is shifted",
			initialContent: `
~/src/blue master
$ 
`,
			start:            term.Coordinates{Y: 2},
			end:              term.Coordinates{Y: 2},
			promptStart:      term.Coordinates{Y: 2, X: 2},
			str:              "$ echo",
			expectedRingBell: false,
			expectedFrom:     term.Coordinates{Y: 2, X: 2},
			expectedTo:       term.Coordinates{Y: 2, X: 6},
			expectedOld:      "",
			expectedContent: `
~/src/blue master
$ echo
`,
		},
		{
			description: "delete until end of prompt line starting at prompt",
			initialContent: `
~/src/blue master
$ echo
`,
			start:            term.Coordinates{Y: 2, X: 2},
			end:              term.Coordinates{Y: 2, X: 7},
			promptStart:      term.Coordinates{Y: 2, X: 2},
			str:              "",
			expectedRingBell: false,
			expectedFrom:     term.Coordinates{Y: 2, X: 2},
			expectedTo:       term.Coordinates{Y: 2, X: 6},
			expectedOld:      "echo",
			expectedContent: `
~/src/blue master
$ 
`,
		},
		{
			description: "delete until end of prompt line starting before prompt is trimmed to prompt start",
			initialContent: `
~/src/blue master
$ echo
`,
			start:            term.Coordinates{Y: 2},
			end:              term.Coordinates{Y: 2, X: 7},
			promptStart:      term.Coordinates{Y: 2, X: 2},
			str:              "",
			expectedRingBell: false,
			expectedFrom:     term.Coordinates{Y: 2, X: 2},
			expectedTo:       term.Coordinates{Y: 2, X: 6},
			expectedOld:      "echo",
			expectedContent: `
~/src/blue master
$ 
`,
		},
		{
			description: "delete prompt line until next line, (vi's dd), last line",
			initialContent: `
~/src/blue master
$ echo`,
			start:            term.Coordinates{Y: 2},
			end:              term.Coordinates{Y: 3},
			promptStart:      term.Coordinates{Y: 2, X: 2},
			str:              "",
			expectedRingBell: false,
			expectedFrom:     term.Coordinates{Y: 2, X: 2},
			expectedTo:       term.Coordinates{Y: 2, X: 6},
			expectedOld:      "echo",
			expectedContent: `
~/src/blue master
$ `,
		},
		{
			description: "delete prompt line until next line, (vi's dd), not last line",
			initialContent: `
~/src/blue master
$ echo
`,
			start:            term.Coordinates{Y: 2},
			end:              term.Coordinates{Y: 3},
			promptStart:      term.Coordinates{Y: 2, X: 2},
			str:              "",
			expectedRingBell: false,
			expectedFrom:     term.Coordinates{Y: 2, X: 2},
			expectedTo:       term.Coordinates{Y: 2, X: 6},
			expectedOld:      "echo",
			expectedContent: `
~/src/blue master
$ 
`,
		},
		{
			description: "delete prompt multiline, no last line",
			initialContent: `
~/src/blue master
$ echo blaaaaaaaaa
aaaaaaaaa`,
			start:            term.Coordinates{Y: 2},
			end:              term.Coordinates{Y: 3, X: 16},
			promptStart:      term.Coordinates{Y: 2, X: 2},
			str:              "",
			expectedRingBell: false,
			expectedFrom:     term.Coordinates{Y: 2, X: 2},
			expectedTo:       term.Coordinates{Y: 3, X: 9},
			// must guarantee reversibility: since shell wraps lines
			// automatically, we must remove newlines.
			// expectedOld:      "echo blaaaaaaaaa\naaaaaaaaa",
			expectedOld: "echo blaaaaaaaaaaaaaaaaaa",
			expectedContent: `
~/src/blue master
$ `,
		},
		{
			description: "delete prompt multiline, with last line",
			initialContent: `
~/src/blue master
$ echo blaaaaaaaaa
aaaaaaaaa
`,
			start:            term.Coordinates{Y: 2},
			end:              term.Coordinates{Y: 3, X: 16},
			promptStart:      term.Coordinates{Y: 2, X: 2},
			str:              "",
			expectedRingBell: false,
			expectedFrom:     term.Coordinates{Y: 2, X: 2},
			expectedTo:       term.Coordinates{Y: 3, X: 9},
			expectedOld:      "echo blaaaaaaaaaaaaaaaaaa",
			expectedContent: `
~/src/blue master
$ 
`,
		},
		{
			description: "insert with new line, with last line",
			initialContent: `
~/src/blue master
$ echo 
`,
			start:            term.Coordinates{Y: 2, X: 7},
			end:              term.Coordinates{Y: 2, X: 7},
			promptStart:      term.Coordinates{Y: 2, X: 2},
			str:              "aaaaaaaaaaaaaaaaaaaa",
			expectedRingBell: false,
			expectedFrom:     term.Coordinates{Y: 2, X: 7},
			// must guarantee reversibility so must be multiline
			expectedTo:  term.Coordinates{Y: 3, X: 9},
			expectedOld: "",
			expectedContent: `
~/src/blue master
$ echo aaaaaaaaaaa
aaaaaaaaa
`,
		},
		{
			description: "insert with new line, no last line",
			initialContent: `
~/src/blue master
$ echo `,
			start:            term.Coordinates{Y: 2, X: 7},
			end:              term.Coordinates{Y: 2, X: 7},
			promptStart:      term.Coordinates{Y: 2, X: 2},
			str:              "aaaaaaaaaaaaaaaaaaaa",
			expectedRingBell: false,
			expectedFrom:     term.Coordinates{Y: 2, X: 7},
			expectedTo:       term.Coordinates{Y: 3, X: 9},
			expectedOld:      "",
			expectedContent: `
~/src/blue master
$ echo aaaaaaaaaaa
aaaaaaaaa`,
		},
		{
			description: "insert above last prompt rings bell",
			initialContent: `
~/src/blue master
$ 
~/src/blue master
$ echo `,
			start:            term.Coordinates{Y: 2, X: 2},
			end:              term.Coordinates{Y: 2, X: 2},
			promptStart:      term.Coordinates{Y: 4, X: 2},
			str:              "a",
			expectedRingBell: true,
			expectedFrom:     term.Coordinates{Y: 2, X: 2},
			expectedTo:       term.Coordinates{Y: 2, X: 2},
			expectedContent: `
~/src/blue master
$ 
~/src/blue master
$ echo `,
		},
		{
			description: "delete above last prompt rings bell",
			initialContent: `
~/src/blue master
$ 
~/src/blue master
$ echo `,
			start:            term.Coordinates{Y: 2, X: 2},
			end:              term.Coordinates{Y: 2, X: 3},
			promptStart:      term.Coordinates{Y: 4, X: 2},
			str:              "",
			expectedRingBell: true,
			expectedFrom:     term.Coordinates{Y: 2, X: 2},
			expectedTo:       term.Coordinates{Y: 2, X: 2},
			expectedContent: `
~/src/blue master
$ 
~/src/blue master
$ echo `,
		},
		{
			description: "delete up to prompt start rings a bell",
			initialContent: `
~/src/blue master
$ 
~/src/blue master
$ echo `,
			start:            term.Coordinates{Y: 2, X: 2},
			end:              term.Coordinates{Y: 4, X: 2},
			promptStart:      term.Coordinates{Y: 4, X: 2},
			str:              "",
			expectedRingBell: true,
			expectedFrom:     term.Coordinates{Y: 4, X: 2},
			expectedTo:       term.Coordinates{Y: 4, X: 2},
			expectedContent: `
~/src/blue master
$ 
~/src/blue master
$ echo `,
		},
		{
			description: "insert at shell-wrapped line",
			initialContent: `
~/src/blue master
$ 
~/src/blue master
$ echo aaaaaaaaaaa
a`,
			start:            term.Coordinates{Y: 5, X: 1},
			end:              term.Coordinates{Y: 5, X: 1},
			promptStart:      term.Coordinates{Y: 4, X: 2},
			str:              "xyz",
			expectedRingBell: false,
			expectedFrom:     term.Coordinates{Y: 5, X: 1},
			expectedTo:       term.Coordinates{Y: 5, X: 4},
			expectedOld:      "",
			expectedContent: `
~/src/blue master
$ 
~/src/blue master
$ echo aaaaaaaaaaa
axyz`,
		},
		{
			description: "insert at shell-wrapped line, with more than line wrapped line",
			initialContent: `
~/src/blue master
$ 
~/src/blue master
$ echo aaaaaaaaaaa
aaaaaaaaaaaaaaaaaa
a`,
			start:            term.Coordinates{Y: 6, X: 1},
			end:              term.Coordinates{Y: 6, X: 1},
			promptStart:      term.Coordinates{Y: 4, X: 2},
			str:              "xyz",
			expectedRingBell: false,
			expectedFrom:     term.Coordinates{Y: 6, X: 1},
			expectedTo:       term.Coordinates{Y: 6, X: 4},
			expectedOld:      "",
			expectedContent: `
~/src/blue master
$ 
~/src/blue master
$ echo aaaaaaaaaaa
aaaaaaaaaaaaaaaaaa
axyz`,
		},
		{
			description: "delete entire content, deletes only prompt lines, last line + 1",
			initialContent: `
~/src/blue master
$ 
~/src/blue master
$ echo aaaaaaaaaaa
aaaaaaaaaaaaaaaaaa
a`,
			start:            term.Coordinates{},
			end:              term.Coordinates{Y: 7},
			promptStart:      term.Coordinates{Y: 4, X: 2},
			str:              "",
			expectedRingBell: false,
			expectedFrom:     term.Coordinates{Y: 4, X: 2},
			expectedTo:       term.Coordinates{Y: 6, X: 1},
			expectedOld:      "echo aaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			expectedContent: `
~/src/blue master
$ 
~/src/blue master
$ `,
		},
		{
			description: "delete entire content, deletes only prompt lines, last column + 1",
			initialContent: `
~/src/blue master
$ 
~/src/blue master
$ echo aaaaaaaaaaa
aaaaaaaaaaaaaaaaaa
a`,
			start:            term.Coordinates{},
			end:              term.Coordinates{Y: 6, X: 1},
			promptStart:      term.Coordinates{Y: 4, X: 2},
			str:              "",
			expectedRingBell: false,
			expectedFrom:     term.Coordinates{Y: 4, X: 2},
			expectedTo:       term.Coordinates{Y: 6, X: 1},
			expectedOld:      "echo aaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			expectedContent: `
~/src/blue master
$ 
~/src/blue master
$ `,
		},
		{
			description: "entire content replace, replaces only prompt lines, exact length",
			initialContent: `
~/src/blue master
$ 
~/src/blue master
$ echo aaaaaaaaaaa
aaaaaaaaaaaaaaaaaa
a`,
			start:       term.Coordinates{},
			end:         term.Coordinates{Y: 6, X: 1},
			promptStart: term.Coordinates{Y: 4, X: 2},
			str: `
~/SRC/BLUE MASTER
% ECHO XXXXXXXXXX
~/src/blue master
$ ECHO AAAAAAAAAAAAAAAAAAAAAAAAAAAAAA`,
			expectedRingBell: false,
			expectedFrom:     term.Coordinates{Y: 4, X: 2},
			expectedTo:       term.Coordinates{Y: 6, X: 1},
			expectedOld:      "echo aaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			expectedContent: `
~/src/blue master
$ 
~/src/blue master
$ ECHO AAAAAAAAAAA
AAAAAAAAAAAAAAAAAA
A`,
		},
		{
			description: "entire content replace, replaces prompt lines + newlines until height",
			initialContent: `
~/src/blue master
$ 
~/src/blue master
$ echo aaaaaaaaaaa
aaaaaaaaaaaaaaaaaa
a`,
			start:       term.Coordinates{},
			end:         term.Coordinates{Y: 6, X: 1},
			promptStart: term.Coordinates{Y: 4, X: 2},
			str: `
~/SRC/BLUE MASTER
% ECHO XXXXXXXXXX
~/src/blue master
$ ECHO AAAAAAAAAAAAAAAAAAAAAAAAAAAAAA












`,
			expectedRingBell: false,
			expectedFrom:     term.Coordinates{Y: 4, X: 2},
			expectedTo:       term.Coordinates{Y: 17, X: 0},
			expectedOld:      "echo aaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			expectedContent: `
~/src/blue master
$ 
~/src/blue master
$ ECHO AAAAAAAAAAA
AAAAAAAAAAAAAAAAAA
A












`,
		},
		{
			description: "paste at prompt, not last line",
			initialContent: `
~/src/blue master
$ 
`,
			start:            term.Coordinates{Y: 2, X: 2},
			end:              term.Coordinates{Y: 2, X: 2},
			promptStart:      term.Coordinates{Y: 2, X: 2},
			str:              "echo bla",
			expectedRingBell: false,
			expectedFrom:     term.Coordinates{Y: 2, X: 2},
			expectedTo:       term.Coordinates{Y: 2, X: 10},
			expectedOld:      "",
			expectedContent: `
~/src/blue master
$ echo bla
`,
		},
		{
			description: "paste at prompt, last line",
			initialContent: `
~/src/blue master
$ `,
			start:            term.Coordinates{Y: 2, X: 2},
			end:              term.Coordinates{Y: 2, X: 2},
			promptStart:      term.Coordinates{Y: 2, X: 2},
			str:              "echo bla",
			expectedRingBell: false,
			expectedFrom:     term.Coordinates{Y: 2, X: 2},
			expectedTo:       term.Coordinates{Y: 2, X: 10},
			expectedOld:      "",
			expectedContent: `
~/src/blue master
$ echo bla`,
		},
		{
			description: "undo on wrapped line",
			initialContent: `
~/src/blue master
$ 
~/src/blue master
$ echo aaaaaaaaaaa
aaaaaaaXXaaaa





`,
			start:       term.Coordinates{},
			end:         term.Coordinates{Y: 6},
			promptStart: term.Coordinates{Y: 4, X: 2},
			str: `
~/SRC/BLUE MASTER
% ECHO XXXXXXXXXX
~/src/blue master
$ echo aaaaaaaaaaaaaaaaaaooaaaa`,
			expectedRingBell: false,
			expectedFrom:     term.Coordinates{Y: 4, X: 2},
			expectedTo:       term.Coordinates{Y: 5, X: 13},
			expectedOld:      "echo aaaaaaaaaaaaaaaaaaXXaaaa",
			expectedContent: `
~/src/blue master
$ 
~/src/blue master
$ echo aaaaaaaaaaa
aaaaaaaooaaaa





`,
		},
	}

	for _, test := range suite {
		t.Run(test.description, func(t *testing.T) {
			t.Parallel()
			comp := newTestParentComponent(test.initialContent, test.promptStart)
			var vi viHandler
			cfg := DefaultConfig()
			var actualBellsRung int
			cfg.RingBell = func() {
				actualBellsRung++
			}
			vi.doInit(comp, cfg)
			vi.remote = newTestRemote(comp.scroll, test.promptStart)
			vi.Resize(18, 18)

			ctx := context.Background()
			actualFrom, actualTo, actualOld := vi.Edit(ctx, test.start, test.end, test.str)

			assert.Equal(t, test.expectedContent, comp.scroll.Buffer().String())
			assert.Equal(t, test.expectedFrom, actualFrom, "from")
			assert.Equal(t, test.expectedTo, actualTo, "to")
			assert.Equal(t, test.expectedOld, actualOld)

			var expectedBellsRung int
			if test.expectedRingBell {
				expectedBellsRung = 1
			}
			assert.Equal(t, expectedBellsRung, actualBellsRung, "bells rung")
		})
	}
}

type testParentComponent struct {
	scroll *component.Scroll
	uri    workspaceapi.URI
	cursor term.Coordinates
}

func newTestParentComponent(content string, cursorAtScroll term.Coordinates) *testParentComponent {
	buf := new(cell.Buffer)
	buf.InitPerformance(1, 1, vtescreen.DefaultChar)

	scroll := new(component.Scroll)
	scroll.InitPerformance(buf)
	// do not use ReadFrom or InsertString as null characters
	// will be elided.
	var next term.Coordinates
	for _, ch := range content {
		next = buf.Insert(next, ch)
	}
	testURI, _ := workspaceapi.ParseURI("memory:///")
	return &testParentComponent{
		scroll: scroll,
		uri:    testURI,
		cursor: cursorAtScroll,
	}
}

func (c *testParentComponent) PrimaryScroll() *component.Scroll {
	return c.scroll
}

func (c *testParentComponent) URI() workspaceapi.URI {
	return c.uri
}

func (c *testParentComponent) Locker() sync.Locker {
	return nopLocker{}
}

func (c *testParentComponent) cursorAtScroll() term.Coordinates {
	return c.cursor
}

func (c *testParentComponent) pendingCallbacks() int {
	return 0
}

func (c *testParentComponent) scheduleBellCallback(callback func()) bool {
	callback()
	return true
}

type nopLocker struct {
}

func (nopLocker) Lock() {
}

func (nopLocker) Unlock() {
}

// sentinelEditor is a no-op cell.Editor used in tests to detect whether
// a buffer's editor has been silently swapped behind the test's back.
// Edit returns the start coordinates unchanged so the caller cannot
// distinguish it from a successful no-op edit, and sets called=true so
// the test can assert it was reached.
type sentinelEditor struct {
	called bool
}

func (s *sentinelEditor) Edit(
	_ context.Context, start, _ term.Coordinates, _ string,
) (from, to term.Coordinates, old string) {
	s.called = true
	return start, start, ""
}

type testRemote struct {
	cursor             *text.Cursor
	keyArrowUpCalled   int
	keyArrowDownCalled int
	formFeedCalled     int
	lineFeedCalled     int
	ops                []func()
	ctx                context.Context
}

func newTestRemote(scroll *component.Scroll, cursorPosition term.Coordinates) *testRemote {
	ret := new(testRemote)
	ret.cursor = new(text.Cursor)
	ret.cursor.InitPerformance(scroll)
	// needed to ensure that remote edits bypass Edit checks
	ret.ctx = vtescreen.NewContext(context.Background())
	ret.cursor.MoveToScroll(cursorPosition)
	return ret
}

func (r *testRemote) moveStartOfLine() {
	r.cursor.MoveStartLine()
}

func (r *testRemote) keyArrowUp() {
	r.keyArrowUpCalled++
}

func (r *testRemote) keyArrowDown() {
	r.keyArrowDownCalled++
}

func (r *testRemote) deleteChar() {
	r.ops = append(r.ops, func() {
		r.cursor.DeleteContext(r.ctx)
	})
}

func (r *testRemote) insertChar(ch rune) {
	r.ops = append(r.ops, func() {
		r.cursor.InsertContext(r.ctx, ch, text.IndentRuneTab, 0)
	})
}

func (r *testRemote) linefeed() {
	r.lineFeedCalled++
}

func (r *testRemote) formFeed() {
	r.formFeedCalled++
}

func (r *testRemote) moveLeft() {
	r.ops = append(r.ops, func() {
		r.cursor.MoveLeft()
	})
}

func (r *testRemote) moveRight() {
	r.ops = append(r.ops, func() {
		r.cursor.MoveRight()
	})
}

func (r *testRemote) conflate() {
	r.ops = append(r.ops, func() {
		r.cursor.ConflateContext(r.ctx)
	})
}

func (r *testRemote) wrapLine() {
	r.ops = append(r.ops, func() {
		r.cursor.InsertContext(r.ctx, '\n', text.IndentRuneTab, 0)
	})
}

func (r *testRemote) cursorCRLF() {
	r.ops = append(r.ops, func() {
		r.cursor.MoveDown()
		r.cursor.MoveStartLine()
	})
}

func (r *testRemote) flush() error {
	for _, op := range r.ops {
		op()
	}
	r.ops = r.ops[:0]
	return nil
}

func (r *testRemote) triggerBell() error {
	return nil
}

type nopTabManager struct {
}

func (nopTabManager) Tab(
	uri workspaceapi.URI, icon rune, name string, h browserapi.Handler,
) (browserapi.Handler, error) {
	panic("not implemented")
}

func (nopTabManager) SetTabName(workspaceapi.URI, string, term.Attributes) error {
	return nil
}
