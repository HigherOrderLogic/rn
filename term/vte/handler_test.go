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
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/unstablebuild/rune-go-sdk/api/browserapi"
	"github.com/unstablebuild/rune-go-sdk/api/config"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"github.com/unstablebuild/rune-go-sdk/term"
	"unstable.build/go-tui/term/vte/vtetest"
	"unstable.build/go-tui/workspace"
)

// this is the timeout to wait for the shell to stop updating the
// internal state of the vte, to call a test case "complete", so
// assertions can run. The slower the host of the tests, the longer
// this timeout should be.
var defaultWaitForIdleVte = 100 * time.Millisecond

func init() {
	if os.Getenv("CI") == "true" {
		defaultWaitForIdleVte = 150 * time.Millisecond
	}
}

func TestHandlerIntegration(t *testing.T) {
	t.Parallel()
	cases := []vtetest.Case{
		{"",
			`$ ▐                 
                    
                    
                    
                    
                    
                    
                    
                    
                    `},
		{"ls",
			`$ ls▐               
                    
                    
                    
                    
                    
                    
                    
                    
                    `},
		{"^^echo bla>",
			`$ echo bla          
bla                 
$ ▐                 
                    
                    
                    
                    
                    
                    
                    `},
		{"vi>ihello",
			`hello▐              
~                   
~                   
~                   
~                   
~                   
~                   
~                   
~                   
-- INSERT --        `},
		{"<:quit!>",
			`$ echo bla          
bla                 
$ vi                
$ ▐                 
                    
                    
                    
                    
                    
                    `},
	}

	cfg := DefaultConfig()

	// vi needs quite a bit of tiem to exit
	waitForIdleVte := defaultWaitForIdleVte * 4

	testSequence(t, cfg, waitForIdleVte, cases)
}

func TestHandlerCloseExit(t *testing.T) {
	t.Parallel()
	cases := []vtetest.Case{
		{"exit>",
			`$ exit              
exit                
                    
                    
                    
                    
                    
                    
                    
                    `},
	}

	cfg := DefaultConfig()

	handler, _ := testSequence(t, cfg, defaultWaitForIdleVte, cases)
	exit, handled := handler.Handle(term.Event{})
	require.True(t, exit)
	assert.False(t, handled)

	_, _, show := handler.Cursor()
	assert.False(t, show)
}

func TestResetPrimaryBuffer(t *testing.T) {
	t.Parallel()
	t.Run("non modal", func(t *testing.T) {
		t.Parallel()
		cases := []vtetest.Case{
			{"echo bla>echo bla>",
				`$ echo bla          
bla                 
$ echo bla          
bla                 
$ ▐                 
                    
                    
                    
                    
                    `},
		}
		cfg := DefaultConfig()
		handler, ch := testSequence(t, cfg, defaultWaitForIdleVte, cases)

		handler.ClearPrimaryBuffer()
		time.Sleep(defaultWaitForIdleVte)

		cases = []vtetest.Case{
			{"echo XXX>",
				`                    
$ echo XXX          
XXX                 
$ ▐                 
                    
                    
                    
                    
                    
                    `},
		}

		vtetest.TestSequence(t, handler, 20, 10, defaultWaitForIdleVte, ch, cases)
	})

	t.Run("on modal mode", func(t *testing.T) {
		t.Parallel()
		cases := []vtetest.Case{
			{"echo bla>echo bla><",
				`$ echo bla          
bla                 
$ echo bla          
bla                 
$ ▐                 
                    
                    
                    
                    
                    `},
		}
		cfg := DefaultConfig()
		cfg.Modal = true
		handler, ch := testSequence(t, cfg, defaultWaitForIdleVte, cases)

		handler.ClearPrimaryBuffer()
		time.Sleep(defaultWaitForIdleVte)

		cases = []vtetest.Case{
			{"echo XXX><kv0yjP",
				`                    
$ echo XXX          
XXX                 
$ XX▐               
                    
                    
                    
                    
                    
                    `},
		}

		vtetest.TestSequence(t, handler, 20, 10, defaultWaitForIdleVte, ch, cases)
	})
}

func TestHandlerResizeViIntegration(t *testing.T) {
	t.Parallel()
	cases := []vtetest.Case{
		{"echo 'a\nb\nc\nd\ne\nf\ng\nh\ni\nj\nk'",
			`> b                 
> c                 
> d                 
> e                 
> f                 
> g                 
> h                 
> i                 
> j                 
> k'▐               `},
		{">",
			`c                   
d                   
e                   
f                   
g                   
h                   
i                   
j                   
k                   
$ ▐                 `},
	}

	cfg := DefaultConfig()
	cfg.Modal = true
	handler, ch := testSequence(t, cfg, defaultWaitForIdleVte, cases)

	// test same width/height resize, which simulates window manager
	// calling Resize on every children after a window re-configuration.
	handler.Resize(20, 10)

	cases = []vtetest.Case{
		{"",
			`c                   
d                   
e                   
f                   
g                   
h                   
i                   
j                   
k                   
$ ▐                 `},
	}

	vtetest.TestCases(t, handler, 20, 10, defaultWaitForIdleVte, ch, cases)
}

func testSequence(t *testing.T, cfg Config, timeout time.Duration, cases []vtetest.Case) (
	*Handler, chan struct{},
) {
	shell := "sh" // all systems were this runs should have sh
	return testSequenceShell(t, cfg, timeout, shell, cases)
}

func testSequenceShell(t *testing.T, cfg Config, timeout time.Duration, shell string, cases []vtetest.Case) (
	*Handler, chan struct{},
) {
	ctx := context.Background()
	ctx, cancel := context.WithCancel(context.Background())
	temp := os.TempDir()

	uri, err := workspaceapi.CurrentUserHostURI(temp)
	require.NoError(t, err)

	scheme, err := workspace.NewFileScheme(ctx, config.NopConfig(), uri)
	require.NoError(t, err)

	ps1 := os.Getenv("PS1")
	os.Setenv("PS1", "$ ")

	ch := make(chan struct{}, 50 /* big enough for the max length sequence of events */)
	cfg.WidthHint = 20
	cfg.HeightHint = 10
	cfg.CommandAndArgs = []string{shell}
	handler, err := NewHandler(chanEventPublisher{ch}, nopNotifications{},
		scheme, scheme, nopTabManager{}, cfg, "")
	require.NoError(t, err)

	if ci := os.Getenv("CI"); ci == "true" {
		// the version of sh running on the CI docker containers
		// doesn't support bell (neither ctrl+g or ctrl+a + <-)
		t.SkipNow()
	}

	t.Cleanup(func() {
		handler.Close()
		scheme.Close()
		cancel()
		os.Setenv("PS1", ps1)
	})

	vtetest.TestSequence(t, handler, cfg.WidthHint, cfg.HeightHint,
		timeout, ch, cases)

	return handler, ch
}

type chanEventPublisher struct {
	ch chan struct{}
}

func (p chanEventPublisher) PublishEvent(term.Event) error {
	p.ch <- struct{}{}
	return nil
}

type nopNotifications struct {
}

func (nopNotifications) Notify(
	level browserapi.NotificationLevel, msg string, args ...any,
) (string, error) {
	return "", nil
}

func (nopNotifications) NotifyOnce(
	level browserapi.NotificationLevel, msg string, args ...any,
) (string, error) {
	return "", nil
}

func (n nopNotifications) UpdateNotificationProgress(
	id, message string, progress, total int64,
) error {
	return nil
}
