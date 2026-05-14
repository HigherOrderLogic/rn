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
	"context"
	"os"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/unstablebuild/rune-go-sdk/api/browserapi"
	"github.com/unstablebuild/rune-go-sdk/api/schemeapi"
	"github.com/unstablebuild/rune-go-sdk/api/textapi"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"unstable.build/go-tui/ide/vctrl"
)

func dispatchFilesystemEvents(
	ctx context.Context, ex *ex, mu sync.Locker,
	ch chan schemeapi.EventInfo, ignores vctrl.Matcher,
) {
	for {
		select {
		case fsev, ok := <-ch:
			if !ok {
				return
			}
			dispatchFilesystemEvent(ex, mu, ignores, fsev)
		case <-ctx.Done():
			return
		}
	}
}

func dispatchFilesystemEvent(
	ex *ex, mu sync.Locker, ignores vctrl.Matcher, fsev schemeapi.EventInfo,
) {
	uri := fsev.URI()
	flag := fsev.Event()

	ex.log(log.TraceLevel, "received filesystem event %s for %s", flag, uri)

	isDir, _ := fsev.IsDir()
	if ignores.Match(uri, isDir) {
		return
	}

	ex.log(log.DebugLevel, "dispatching filesystem event %s for %s", flag, uri)

	ev := textapi.Event{
		URI: uri,
	}
	switch flag {
	case schemeapi.Create:
		ev.Type = textapi.EventTypeCreate
	case schemeapi.Write:
		ev.Type = textapi.EventTypeChange
	case schemeapi.Rename:
		ev.Type = textapi.EventTypeRename
	case schemeapi.Remove:
		ev.Type = textapi.EventTypeRemove
	default:
		ex.log(log.WarnLevel, "extraneous filesystem event %s for %s", flag, uri)
		return
	}

	mu.Lock()
	handleFSChange(ex, flag, uri)
	ex.comp.DispatchEvent(ev)
	mu.Unlock()
}

func handleFSChange(ex *ex, flag schemeapi.Event, uri workspaceapi.URI) {
	t, open := ex.comp.Resource(uri)
	dirty, _ := ex.comp.IsDirty(uri)
	ex.log(log.DebugLevel, "handling event %d for file %s, open=%t dirty=%t",
		flag, uri.Name(), open, dirty)
	if !open {
		return
	}

	var modTime time.Time
	lastFlush, _ := ex.comp.LastFlush(t)
	info, err := ex.workspace.Stat(uri.Path())
	if err != nil && !os.IsNotExist(err) {
		if os.IsPermission(err) {
			return
		}
		ex.log(log.WarnLevel, "could not stat file to "+
			"dispatch fs change prompt: %v", err)
		return
	} else if err == nil {
		modTime = info.ModTime()
	}
	if lastFlush.Equal(modTime) && !modTime.IsZero() { // both zero might be a removed file
		ex.log(log.TraceLevel, "ignoring fs %d event: user flushed file", flag)
		return
	}

	ex.log(log.TraceLevel, "continuing processing with fs %d event: "+
		"last flush %s is before mod time %s",
		flag, lastFlush, modTime)

	if dirty {
		switch flag {
		case schemeapi.Create:
			ex.openFileChangedPrompt(uri, t, "created on", true)
		case schemeapi.Write:
			ex.openFileChangedPrompt(uri, t, "changed on", true)
		case schemeapi.Rename:
			_, err := ex.workspace.Stat(uri.Path())
			if err == nil {
				ex.openFileChangedPrompt(uri, t, "renamed into", true)
			} else if os.IsNotExist(err) {
				ex.openFileChangedPrompt(uri, t, "renamed on", false)
			} else {
				_, _ = ex.comp.Notify(browserapi.LevelError,
					"Failed to reload renamed file %s: stat: %v", uri.Path(), err)
			}
		case schemeapi.Remove:
			ex.openFileChangedPrompt(uri, t, "removed from", false)
		}
		return
	}

	switch flag {
	case schemeapi.Create, schemeapi.Write:
		// Filesystem-watcher-triggered reloads run synchronously
		// inside the host's IDE lock to preserve the pre-async
		// invariant that buffer/tab-attr mutations are serialized
		// with UI reads. The async path exists for user-initiated
		// :write/:reloadfile where the UI must stay responsive on
		// a stuck remote scheme.
		ch, err := ex.comp.ReloadTab(context.Background(), t)
		if err == nil {
			err = <-ch
		}
		if err != nil {
			_, _ = ex.comp.Notify(browserapi.LevelError,
				"Failed to reload file %s: %v", uri.Name(), err)
			break
		}
		_, _ = ex.comp.Notify(browserapi.LevelInfo,
			"File '%s' changed on disk and does not have unflushed "+
				"changes so it was reloaded", uri.Name())

	case schemeapi.Rename:
		_, err := ex.workspace.Stat(uri.Path())
		if err == nil {
			ch, rerr := ex.comp.ReloadTab(context.Background(), t)
			if rerr == nil {
				rerr = <-ch
			}
			if rerr != nil {
				_, _ = ex.comp.Notify(browserapi.LevelError,
					"Failed to reload renamed file %s: %v", uri.Name(), rerr)
				return
			}
			_, _ = ex.comp.Notify(browserapi.LevelInfo,
				"File '%s' was renamed on disk and does not have "+
					"unflushed changes so it was reloaded",
				uri.Name())
			return
		}
		if os.IsNotExist(err) {
			if err := ex.comp.RemoveTab(t); err == nil {
				_, _ = ex.comp.Notify(browserapi.LevelInfo,
					"File '%s' was renamed on disk and does not have unflushed changes "+
						"so it was closed", uri.Name())
			}
			return
		}
		_, _ = ex.comp.Notify(browserapi.LevelError,
			"Failed to reload renamed file %s: stat: %v", uri.Name(), err)

		// don't manage schemeapi.Remove: it's sometimes dispatched
		// in conjunction with other events so it's not useful.
	}
}
