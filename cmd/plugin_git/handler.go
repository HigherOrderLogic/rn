package main

import (
	"bufio"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"
	"sync/atomic"
	"sync"

	"github.com/ernestrc/go-tui/browser"
	"github.com/ernestrc/go-tui/component"
	"github.com/ernestrc/go-tui/editor"
	"github.com/ernestrc/go-tui/cell"
	"github.com/ernestrc/go-tui/handler"
	"github.com/ernestrc/go-tui/plugin"
	plugutil "github.com/ernestrc/go-tui/plugin/util"
	"github.com/ernestrc/go-tui/proto"
	"github.com/ernestrc/go-tui/term"
	log "github.com/sirupsen/logrus"
	"github.com/sourcegraph/go-diff/diff"
)


const (
	defaultGitDiffListID = "git_diff"
	commandNextChange    = "gitNextChange"
	commandPrevChange    = "gitPrevChange"
)

var (
	gitHandlerCommands = []string{commandNextChange, commandPrevChange}
	gitHandlerEvents   = []editor.EventType{
		editor.EventTypeOpen,
		editor.EventTypeFlush,
		editor.EventTypeScroll,
		editor.EventTypeFocus,
	}
	gitHandlerPermissions = []plugin.Permission{
		plugin.PermissionBrowserWindowManager,
		plugin.PermissionBrowserEventPublisher,
	}
)

type gitEditorHandler struct {
	ed            editor.Editor
	wm            browser.WindowManager
	p             browser.EventPublisher
	exit          uint32
	ch            chan editor.Event
	scroll        struct {
		sync.Mutex
		scroll component.Scroll
	}
	gitDiffListID string
	rows          map[string]int
	offsets        map[string]term.Coordinates
}

func logStderr(langID string, stderr io.ReadCloser) {
	reader := bufio.NewReader(stderr)
	for {
		line, err := reader.ReadString('\n')
		log.Debugf("%s: %s", langID, line)
		if err != nil {
			if err != io.EOF {
				log.Errorf("failed to read from '%v' server stderr: %v", langID, err)
			}
			return
		}
	}
}

func newGitHandler(
	ed editor.Editor, grants []plugin.Grant,
	broker proto.MuxBroker, pconfig plugin.Config,

) (plugutil.CommandEventHandler, error) {
	ret := new(gitEditorHandler)
	ret.ed = ed
	ret.rows = make(map[string]int)
	ret.offsets =  make(map[string]term.Coordinates)
	ret.ch = make(chan editor.Event)
	ret.scroll.scroll.Init()
	ret.scroll.scroll.Attributes = term.Attributes{Bg: 235}

	var err error
	for _, grant := range grants {
		switch grant.Permission {
		case plugin.PermissionBrowserEventPublisher:
			ret.p, err = plugin.EventPublisher(grant.Token, broker)
			if err != nil {
				return nil, err
			}
		case plugin.PermissionBrowserWindowManager:
			ret.wm, err = plugin.WindowManager(grant.Token, broker)
			if err != nil {
				return nil, err
			}
			comp := component.Sync(&ret.scroll, &ret.scroll.scroll)
			err = ret.wm.Bar(browser.OrientationLeft, handler.Nop(comp))
			if err != nil {
				return nil, err
			}
		}
	}

	ret.gitDiffListID, err = pconfig.GetString("git_diff_list_id")
	if err != nil {
		if err != plugin.ErrNotFound {
			err = fmt.Errorf("failed to get 'git_diff_list_id' from config: %v", err)
			return nil, err
		}
		ret.gitDiffListID = defaultGitDiffListID
	}

	go ret.handleEvents()

	return ret, nil
}

func (h *gitEditorHandler) HandleCommand(cmd editor.Command) (exit bool) {
	switch cmd.Name {
	case commandNextChange:
		err := h.ed.MoveToNextLocation(cmd.Resource, h.gitDiffListID)
		if err != nil {
			log.Errorf("lspEditorHandler.MoveToNextLocation(%s): %v", cmd.Name, err)
		}
	case commandPrevChange:
		err := h.ed.MoveToPrevLocation(cmd.Resource, h.gitDiffListID)
		if err != nil {
			log.Errorf("lspEditorHandler.MoveToNextLocation(%s): %v", cmd.Name, err)
		}
	}

	return false
}

func (h *gitEditorHandler) parseDiff(diff *diff.FileDiff) []editor.Location {
	var (
		deleteAttr = term.Attributes{Bg: term.ColorRed}
		addAttr    = term.Attributes{Bg: term.ColorGreen}
	)

	h.scroll.Lock()
	defer h.scroll.Unlock()

	var locs []editor.Location
	for _, hunk := range diff.Hunks {
		log.Tracef("Read file diff hunk: %#v", hunk)
		if hunk.NewLines == 0 {
			at := term.Coordinates{Y: int(hunk.OrigStartLine - 1)}
			locs = append(locs, editor.Location{
				From: at,
				To:   term.Coordinates{Y: at.Y, X: 1},
				// Attr: deleteAttr,
			})
			h.scroll.scroll.Buffer().DeleteCell(at)
			h.scroll.scroll.Buffer().InsertStringWithAttr(at, "-", deleteAttr)
			continue
		}

		from := term.Coordinates{Y: int(hunk.NewStartLine - 1)}
		to := term.Coordinates{Y: int(hunk.NewStartLine - 1 + hunk.NewLines)}
		locs = append(locs, editor.Location{From: from, To: to})

		for y := from.Y; y < to.Y; y++ {
			at := term.Coordinates{Y: y}
			h.scroll.scroll.Buffer().DeleteCell(at)
			h.scroll.scroll.Buffer().InsertStringWithAttr(at, "+", addAttr)
		}
	}

	return locs
}

func (h *gitEditorHandler) pushDiffLocations(
	filename string, resource editor.Handler,
) error {
	c := exec.Command("git", "diff", "-U0", filename)

	stdout, err := c.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdout pipe: %v", err)
	}

	err = c.Start()
	if err != nil {
		return fmt.Errorf("failed to start executable: %v", err)
	}

	h.initScroll(filename)

	r := diff.NewFileDiffReader(stdout)
	diff, err := r.Read()
	if err != nil {
		if strings.Contains(err.Error(), io.EOF.Error()) {
			// make sure that an interrupt is called
			// so the new scroll bar is updated, when
			// focus switched to a file with no changes.
			return h.p.PublishInterrupt()
		}
		return fmt.Errorf("failed to read from stdout: %v", err)
	}

	locs := h.parseDiff(diff)

	return h.ed.SetLocationList(resource, h.gitDiffListID, editor.LocationSlice(locs))
}

func (h *gitEditorHandler) initScroll(name string) {
	h.scroll.Lock()
	defer h.scroll.Unlock()

	h.scroll.scroll.Init()
	rows, ok := h.rows[name]
	if !ok {
		log.Warnf("could not find max rows for file %s", name)
		return
	}

	// bar could have different height (+-3) depending on frames
	for y := 0; y < rows+3; y++ {
		h.scroll.scroll.Buffer().Insert(term.Coordinates{Y: y}, ' ')
	}

	offset, ok := h.offsets[name]
	if !ok {
		log.Warnf("could not find last scroll offset for file %s", name)
		return
	}

	h.scroll.scroll.SetOffset(offset)

	log.Debugf("gitEditorHandler.initScroll(%s): lenScroll=%d; rows=%d; offset=%#v",
		name, h.scroll.scroll.Buffer().Rows(), rows, offset)
}

// allow scroll to seek to same positions as editor buffer
func (h *gitEditorHandler) setScrollMaxContent(ev editor.Event) {
	rows := len(cell.StringToCells(ev.Content))
	prev, ok := h.rows[ev.ResourceName]
	if ok && prev > rows {
		// if content is removed then scroll could
		// remain seek out of bounds.
		rows = prev
	}
	h.rows[ev.ResourceName] = rows
	log.Debugf("gitEditorHandler.setScrollMaxContent(%s): %d", ev.ResourceName, rows)
}

func (h *gitEditorHandler) setScrollOffset(filename string, pos term.Coordinates) {
	h.scroll.Lock()
	defer h.scroll.Unlock()

	ok := h.scroll.scroll.SetOffset(pos)
	h.offsets[filename] = pos

	if !ok {
		log.Warnf("gitEditorHandler.setScrollOffset(%s): %#v: could not set offset", filename, pos)
		return
	}
	log.Tracef("gitEditorHandler.setScrollOffset(%s): %#v OK", filename, pos)
}

func (h *gitEditorHandler) handleEvents() {
	for ev := range h.ch {
		var start time.Time
		if log.IsLevelEnabled(log.TraceLevel) {
			start = time.Now()
			log.Tracef("gitEditorHandler.Handle(%#v)", ev.Type)
		}
	
		var err error
		switch ev.Type {
		case editor.EventTypeOpen:
			h.setScrollOffset(ev.ResourceName, term.Coordinates{})
			fallthrough
		case editor.EventTypeFlush:
			h.setScrollMaxContent(ev)
			fallthrough
		case editor.EventTypeFocus:
			err = h.pushDiffLocations(ev.ResourceName, ev.Resource)
		case editor.EventTypeScroll:
			h.setScrollOffset(ev.ResourceName, ev.Start)
			err = h.p.PublishInterrupt()
		}
		if err != nil {
			log.Errorf("Handle(%#v): %v", ev.Type, err)
		}
	
		if log.IsLevelEnabled(log.TraceLevel) {
			log.Tracef("gitEditorHandler.Handle(%#v) in %s", ev.Type, time.Since(start))
		}
	}
}

func (h *gitEditorHandler) Handle(ev editor.Event) (exit bool) {
	uexit  := atomic.LoadUint32(&h.exit)
	exit = uexit != 0
	if exit {
		return
	}

	switch ev.Type {
	case editor.EventTypeFlush:
		select {
			case h.ch<-ev:
			default:
		}
	case editor.EventTypeFocus, editor.EventTypeOpen, editor.EventTypeScroll:
		h.ch <- ev
	}
	return
}

func (h *gitEditorHandler) Close() error {
	atomic.StoreUint32(&h.exit, 1)
	close(h.ch)
	return nil
}
