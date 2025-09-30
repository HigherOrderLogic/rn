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

package ide

import (
	"os"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/unstablebuild/blue/release"
	"unstable.build/go-tui/api/workspaceapi"
	"unstable.build/go-tui/component"
	"unstable.build/go-tui/component/notifications"
	"unstable.build/go-tui/handler"
	"unstable.build/go-tui/handler/handlertest"
	"unstable.build/go-tui/ide/idepkg/idepkgtest"
	"unstable.build/go-tui/term"
	"unstable.build/go-tui/workspace"
)

func TestPackageManagerIntegration(t *testing.T) {
	pkgs := idepkgtest.MakePackages(
		release.Package{Name: "go"},
		release.Package{Name: "six", Latest: "2"},
	)
	bundles := idepkgtest.MakeBundles(
		[]release.Bundle{
			{Package: "go", Version: "1"},
			{Package: "go", Version: "2"},
			{Package: "go", Version: "3"},
		},
		[]release.Bundle{
			{Package: "six", Version: "1"},
			{Package: "six", Version: "2"},
		},
	)
	rm := idepkgtest.NewReleaseManager(pkgs, bundles)
	rm.SetMissProgressComplete(true)
	m := newTestWorkspaceManagerHandlerForPkgManager(t, rm)

	cases := []handlertest.SequenceTestCase{
		{":pkginstall ",
			`┌──────────────────────────────────────┐
│                                      │
├──────────────────────────────────────┤
│                                      │
│                                      │
│                                      │
┌──────────────────────────────────────┐
│pkginstall ▐                          │
│go                                    │
│six                                   │
└──────────────────────────────────────┘
│                                      │
├──────────────────────────────────────┤
│1                                     │
└──────────────────────────────────────┘`},
		{"six ",
			`┌──────────────────────────────────────┐
│                                      │
├──────────────────────────────────────┤
│                                      │
│                                      │
│                                      │
┌──────────────────────────────────────┐
│pkginstall six ▐                      │
│1                                     │
│2                                     │
└──────────────────────────────────────┘
│                                      │
├──────────────────────────────────────┤
│1                                     │
└──────────────────────────────────────┘`},
		{"1>",
			`┌────────────────────────┌─────────────┐
│                        │ downloading │
├────────────────────────│  version 1  │
│                        │ of package  │
│                        │ six         │
│                        └─────────────┘
│                                      │
│          workspaceWallpaper          │
│                                      │
│                                      │
│                                      │
│                                      │
├──────────────────────────────────────┤
│1                                     │
└──────────────────────────────────────┘`},
		{":noticlose>________________:pkgcurrent ",
`┌────────────────────────┌─────────────┐
│                        │ downdloaded │
├────────────────────────│  version 1  │
│                        │ of package  │
│                        │ six         │
│                        └─────────────┘
┌──────────────────────────────────────┐
│pkgcurrent ▐                          │
│six                                   │
│                                      │
└──────────────────────────────────────┘
│                                      │
├──────────────────────────────────────┤
│1                                     │
└──────────────────────────────────────┘`},
		{"six>",
			`┌────────────────────────┌─────────────┐
│                        │ version 1   │
├────────────────────────│ of package  │
│                        │ six is in   │
│                        │ use         │
│                        └─────────────┘
│                        ┌─────────────┐
│          workspaceWallp│ downdloaded │
│                        │  version 1  │
│                        │ of package  │
│                        │ six         │
│                        └─────────────┘
├──────────────────────────────────────┤
│1                                     │
└──────────────────────────────────────┘`},
		{":noticlose>:pkginstall six 1>",
			`┌────────────────────────┌─────────────┐
│                        │ version 1   │
├────────────────────────│ of package  │
│                        │ six has     │
│                        │ already     │
│                        │ been        │
│                        │ installed   │
│          workspaceWallp└─────────────┘
│                                      │
│                                      │
│                                      │
│                                      │
├──────────────────────────────────────┤
│1                                     │
└──────────────────────────────────────┘`},
		{":noticlose>:pkginstall go 1>___________",
			`┌────────────────────────┌─────────────┐
│                        │ downdloaded │
├────────────────────────│  version 1  │
│                        │ of package  │
│                        │ go          │
│                        └─────────────┘
│                                      │
│          workspaceWallpaper          │
│                                      │
│                                      │
│                                      │
│                                      │
├──────────────────────────────────────┤
│1                                     │
└──────────────────────────────────────┘`},
		{":noticlose>:pkgupgradeall>_____________",
			`┌────────────────────────┌─────────────┐
│                        │ downdloaded │
├────────────────────────│  version 2  │
│                        │ of package  │
│                        │ six         │
│                        └─────────────┘
│                        ┌─────────────┐
│          workspaceWallp│ 1 error     │
│                        │ occurred:   │
│                        │ cannot      │
│                        │ upgrade     │
│                        │ package     │
├────────────────────────│ go: latest  │
│1                       │ version is  │
└────────────────────────│ not known   │`},
		{":noticlose>:pkguse six 1>",
			`┌────────────────────────┌─────────────┐
│                        │ version 1   │
├────────────────────────│ of package  │
│                        │ six is now  │
│                        │ in use      │
│                        └─────────────┘
│                                      │
│          workspaceWallpaper          │
│                                      │
│                                      │
│                                      │
│                                      │
├──────────────────────────────────────┤
│1                                     │
└──────────────────────────────────────┘`},
		{":noticlose>:pkginstall six>",
			`┌────────────────────────┌─────────────┐
│                        │ version 2   │
├────────────────────────│ of package  │
│                        │ six has     │
│                        │ already     │
│                        │ been        │
│                        │ installed   │
│          workspaceWallp└─────────────┘
│                                      │
│                                      │
│                                      │
│                                      │
├──────────────────────────────────────┤
│1                                     │
└──────────────────────────────────────┘`},
		{":noticlose>:pkginstall go>",
			`┌────────────────────────┌─────────────┐
│                        │ install     │
├────────────────────────│ package:    │
│                        │ latest      │
│                        │ version is  │
│                        │ not known   │
│                        └─────────────┘
│          workspaceWallpaper          │
│                                      │
│                                      │
│                                      │
│                                      │
├──────────────────────────────────────┤
│1                                     │
└──────────────────────────────────────┘`},
		{":noticlose>:pkgremove six 1>",
			`┌────────────────────────┌─────────────┐
│                        │ version 1   │
├────────────────────────│ of package  │
│                        │ six is      │
│                        │ currently   │
│                        │ in use,     │
│                        │ run         │
│          workspaceWallp│ 'pkguse'    │
│                        │ with some   │
│                        │ other       │
│                        │ version     │
│                        │ first       │
├────────────────────────│ before      │
│1                       │ removing,   │
└────────────────────────│ or pass no  │`},
		{":noticlose>:pkgremove six 2>",
			`┌────────────────────────┌─────────────┐
│                        │ version 2   │
├────────────────────────│ of package  │
│                        │ six has     │
│                        │ been        │
│                        │ removed     │
│                        └─────────────┘
│          workspaceWallpaper          │
│                                      │
│                                      │
│                                      │
│                                      │
├──────────────────────────────────────┤
│1                                     │
└──────────────────────────────────────┘`},
		{":noticlose>:pkgremove go>",
			`┌────────────────────────┌─────────────┐
│                        │ package go  │
├────────────────────────│ has been    │
│                        │ removed     │
│                        └─────────────┘
│                                      │
│                                      │
│          workspaceWallpaper          │
│                                      │
│                                      │
│                                      │
│                                      │
├──────────────────────────────────────┤
│1                                     │
└──────────────────────────────────────┘`},
		{":noticlose>:pkginstall ox>",
			`┌────────────────────────┌─────────────┐
│                        │ install     │
├────────────────────────│ package:    │
│                        │ not found   │
│                        └─────────────┘
│                                      │
│                                      │
│          workspaceWallpaper          │
│                                      │
│                                      │
│                                      │
│                                      │
├──────────────────────────────────────┤
│1                                     │
└──────────────────────────────────────┘`},
		{":noticlose>:pkgremove ox>",
			`┌────────────────────────┌─────────────┐
│                        │ package ox  │
├────────────────────────│ is not      │
│                        │ installed   │
│                        └─────────────┘
│                                      │
│                                      │
│          workspaceWallpaper          │
│                                      │
│                                      │
│                                      │
│                                      │
├──────────────────────────────────────┤
│1                                     │
└──────────────────────────────────────┘`},
		{":noticlose>:pkguse six 2>",
			`┌────────────────────────┌─────────────┐
│                        │ package     │
├────────────────────────│ version is  │
│                        │ not         │
│                        │ installed   │
│                        └─────────────┘
│                                      │
│          workspaceWallpaper          │
│                                      │
│                                      │
│                                      │
│                                      │
├──────────────────────────────────────┤
│1                                     │
└──────────────────────────────────────┘`},
		{":noticlose>:pkgremove six>",
			`┌────────────────────────┌─────────────┐
│                        │ package     │
├────────────────────────│ six has     │
│                        │ been        │
│                        │ removed     │
│                        └─────────────┘
│                                      │
│          workspaceWallpaper          │
│                                      │
│                                      │
│                                      │
│                                      │
├──────────────────────────────────────┤
│1                                     │
└──────────────────────────────────────┘`},
		{":noticlose>:pkguse six 1>",
			`┌────────────────────────┌─────────────┐
│                        │ package     │
├────────────────────────│ version is  │
│                        │ not         │
│                        │ installed   │
│                        └─────────────┘
│                                      │
│          workspaceWallpaper          │
│                                      │
│                                      │
│                                      │
│                                      │
├──────────────────────────────────────┤
│1                                     │
└──────────────────────────────────────┘`},
	}
	h := newSafeHandler(m)

	handlertest.TestHandlerSequence(t, h, 40, 15, cases)

	require.NoError(t, m.Close())
}

func newTestWorkspaceManagerHandlerForPkgManager(
	t *testing.T, releaseManager release.Manager,
) *testWorkspaceManagerHandler {
	dir, err := os.MkdirTemp("", "")
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = os.RemoveAll(dir)
	})
	cfg := defaultCfg()
	manager := workspace.NewManager(cfg.workspace())
	manager.RegisterScheme(workspace.MemoryScheme, workspace.NewMemoryScheme)
	ret := newTestWorkspaceManagerHandlerWithReleaseManager(t, manager,
		cfg, FuncExtensionsRunner(testRunnerFn), nil, nil, dir, nil,
		nopShutdownShaderConfig(), releaseManager)
	// only home workspace has a sync command prompt
	ret.empty.syncCommandPrompt = true
	return ret
}

func newTestWorkspaceManagerHandlerWithReleaseManager(
	t *testing.T, manager *workspace.Manager,
	cfg ideConfig, runner ExtensionsRunner,
	extensions map[string]Extension, files []string, dir string,
	onTabsClick func(int) bool,
	shutdownShaderCfg shutdownShaderConfig,
	releaseManager release.Manager,
) *testWorkspaceManagerHandler {
	homeURI, err := workspaceapi.ParseURI("memory:///home")
	require.NoError(t, err)

	m := new(testWorkspaceManagerHandler)
	n := notifications.New(m, notificationsConfig())
	m.workspaceManagerHandler = new(workspaceManagerHandler)
	// ensure that command manual is never shown
	cfg.cfg["command"] = defaultCfg().cfg["command"]

	shRunner := new(shaderRunner)
	shRunner.init(handler.Nop(component.Nop()), term.NopInterrupter(), term.Attributes{},
		shutdownShaderCfg)

	err = m.workspaceManagerHandler.init(nil, homeURI, manager, n, cfg, "", files,
		dir, func(term.Event) bool {
			return true
		}, runner, new(sync.Mutex), extensions,
		func() (ideConfig, error) { return cfg, nil },
		".sixrc", 0, 0, '1', 0, 0, true, onTabsClick, releaseManager, shRunner)

	require.NoError(t, err)
	return m
}
