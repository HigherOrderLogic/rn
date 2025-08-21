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
	"sort"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/unstablebuild/blue/document"
	"github.com/unstablebuild/blue/logging"
	"unstable.build/go-tui/api/textapi"
	"unstable.build/go-tui/api/workspaceapi"
	"unstable.build/go-tui/term"
	"unstable.build/go-tui/text"
)

var (
	historyEventInterests = []textapi.EventType{
		textapi.EventTypeOpen,
		textapi.EventTypeClose,
		textapi.EventTypeFlush,
		textapi.EventTypeEdit,
		textapi.EventTypeCursor,
	}
)

type file struct {
	Dirty     bool
	OpenAt    time.Time
	Cursor    term.Coordinates
	URIString string
}

type cache struct {
	Files map[string]file
}

type history struct {
	svc   document.Service
	cache map[string]cache
}

func newHistory(storage document.Service) *history {
	ret := &history{
		svc:   storage,
		cache: make(map[string]cache),
	}
	return ret
}

func (h *history) log(level log.Level, msg string, args ...interface{}) {
	if !log.IsLevelEnabled(level) {
		return
	}
	log.WithFields(log.Fields{logging.KeyClass: "ide.history"}).
		Logf(level, msg, args...)
}

func (h *history) loadWorkspaceData(uri workspaceapi.URI, restore bool) {
	const cacheSetTimeout = 1 * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), cacheSetTimeout)
	defer cancel()

	uriStr := uri.String()

	var workspace cache
	err := h.svc.Get(ctx, uriStr, &workspace)
	if err != nil && err != document.ErrNotFound {
		log.WithFields(log.Fields{logging.KeyClass: "ide.history"}).
			Warnf("could not persist updated cache to durable storage: %v", err)
	}

	if !restore || err != nil {
		h.log(log.TraceLevel, "reseting cache for workspace: %s", uriStr)
		h.resetWorkspaceCache(uri)
	} else {
		// do not trust what's coming from storage
		if workspace.Files == nil {
			workspace.Files = make(map[string]file)
		}
		h.cache[uriStr] = workspace
	}
	h.log(log.TraceLevel, "loaded workspace %s cache from storage: %v", uriStr, h.cache)
}

func (h *history) recordAddWorkspace(
	uri workspaceapi.URI, ed text.Editor, restore bool,
) (ret []file) {
	h.loadWorkspaceData(uri, restore)

	uriStr := uri.String()
	err := ed.SubscribeEvents(historyEventInterests,
		&workspaceHistory{svc: h.svc, uri: uriStr, cache: h.cache})
	if err != nil {
		h.log(log.ErrorLevel, "could not subscribe to file events: %v", err)
	}
	mapFiles := h.cache[uriStr].Files
	ret = make([]file, 0, len(h.cache))
	for _, f := range mapFiles {
		ret = append(ret, f)
	}
	h.log(log.TraceLevel, "record add workspace: %v", ret)
	sort.Slice(ret, func(i, j int) bool {
		return ret[i].OpenAt.Before(ret[j].OpenAt)
	})
	return ret
}

func (h *history) recordCloseWorkspace(uri workspaceapi.URI) {
	for uri, cache := range h.cache {
		persistUpdateCache(h.svc, uri, cache)
	}
	// no need to unsubscibe as everything will be garbage collected
	// and workspaceHistory has protection against receiving events
	// once already deleted.
	delete(h.cache, uri.String())
}

func (h *history) dirtyFilesOpen() (ret bool) {
	for _, w := range h.cache {
		for _, f := range w.Files {
			if f.Dirty {
				return true
			}
		}
	}
	return false
}

func (h *history) resetWorkspaceCache(uri workspaceapi.URI) {
	const cacheSetTimeout = 1 * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), cacheSetTimeout)
	defer cancel()

	fresh := cache{Files: make(map[string]file)}
	h.cache[uri.String()] = fresh

	err := h.svc.Set(ctx, uri.String(), fresh)
	if err != nil {
		h.log(log.WarnLevel, "could not persist new cache to durable storage: %v", err)
		return
	}
	h.log(log.TraceLevel, "persisted new cache for workspace %s", uri.String())
}

type workspaceHistory struct {
	uri   string
	cache map[string]cache
	svc   document.Service
}

func (h *workspaceHistory) Handle(ctx context.Context, ev textapi.Event) bool {
	workspaceCache, ok := h.cache[h.uri]
	if !ok {
		return true // we're done if workspace was deleted
	}

	// do not assume dispatch of events is correct
	if ev.URI == (workspaceapi.URI{}) {
		return false
	}

	evUriStr := ev.URI.String()
	prev, ok := workspaceCache.Files[evUriStr]

	switch ev.Type {
	case textapi.EventTypeOpen:
		workspaceCache.Files[evUriStr] = makeFile(
			evUriStr, prev.Cursor, false, time.Now())
		persistUpdateCache(h.svc, h.uri, workspaceCache)
	case textapi.EventTypeClose:
		delete(workspaceCache.Files, evUriStr)
		persistUpdateCache(h.svc, h.uri, workspaceCache)
	case textapi.EventTypeFlush:
		// EventTypeFlush might or might not be from an open file.
		// When change is out-of-band, do not persist
		// otherwise next session files might include files
		// that were never open.
		if ok {
			workspaceCache.Files[evUriStr] = makeFile(
				evUriStr, prev.Cursor, false, prev.OpenAt)
			persistUpdateCache(h.svc, h.uri, workspaceCache)
		}
	case textapi.EventTypeCursor:
		workspaceCache.Files[evUriStr] = makeFile(
			evUriStr, ev.From, prev.Dirty, prev.OpenAt)
		// do not store on cursor, as it could significantly impact performance
	case textapi.EventTypeEdit:
		workspaceCache.Files[evUriStr] = makeFile(
			evUriStr, prev.Cursor, true, prev.OpenAt)
		// do not store on edit, as it could significantly impact performance
	}

	return false
}

func makeFile(uri string, cursor term.Coordinates, dirty bool, updated time.Time) file {
	return file{
		Dirty:     dirty,
		URIString: uri,
		Cursor:    cursor,
		OpenAt:    updated,
	}
}

func persistUpdateCache(svc document.Service, uri string, cache cache) {
	const cacheSetTimeout = 1 * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), cacheSetTimeout)
	defer cancel()

	err := svc.Set(ctx, uri, cache)
	if err != nil {
		log.WithFields(log.Fields{logging.KeyClass: "ide.history"}).
			Warnf("could not persist updated cache to durable storage: %v", err)
		return
	}
	log.WithFields(log.Fields{logging.KeyClass: "ide.history"}).
		Tracef("persisted updated cache for workspace %s", uri)
}
