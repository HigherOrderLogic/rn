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

package vtereservoir

import (
	"sync"

	"github.com/ernestrc/go-multierror"
	"github.com/ernestrc/logd-go/logging"
	log "github.com/sirupsen/logrus"
	"unstable.build/go-tui/api/browserapi"
	"unstable.build/go-tui/api/schemeapi"
	"unstable.build/go-tui/api/workspaceapi"
	"unstable.build/go-tui/browser"
	"unstable.build/go-tui/component"
	"unstable.build/go-tui/debug"
	"unstable.build/go-tui/term"
	"unstable.build/go-tui/term/vte"
)

// VTE abstracts a vte.Handler.
type VTE interface {
	browserapi.Handler
	component.Scrollable
	OnFocusChange(bool)
	SetDefaultAttributes(attr term.Attributes)
	IsComplete() bool
	URI() workspaceapi.URI
	Title() string
	UsedAlternateBuffer() bool
	ClearPrimaryBuffer() bool
}

// Facility is a pool of vte instances.It only caches vte instances
// that start with no initial commands,
type Facility struct {
	mu   sync.Mutex
	pool []VTE
	new  func(bool) (VTE, error)
}

// New allocates storage for a new Facility and initializes it.
func New(
	publisher browser.EventPublisher, n browser.Notifications,
	terminal schemeapi.Terminal, executor schemeapi.Executor,
	tm browser.TabManager, config vte.Config, initialCapacity int,
) *Facility {
	if config.WidthHint == 0 {
		config.WidthHint = 100
	}
	if config.HeightHint == 0 {
		config.HeightHint = config.WidthHint / 2
	}
	ret := new(Facility)
	ret.new = func(initialAlloc bool) (VTE, error) {
		ret.log(log.TraceLevel, "called pool.New, width hint: %d, height hint: %d",
			config.WidthHint, config.HeightHint)
		i, err := vte.NewHandler(publisher, n, terminal, executor, tm, config, "")
		if err != nil {
			return nil, err
		}
		if initialAlloc {
			i.Resize(config.WidthHint, config.HeightHint)
		}
		return &vteAdapter{Handler: i, f: ret}, nil
	}

	go debug.CapturePanicReport(func() {
		ret.initCap(initialCapacity)
	})
	return ret
}

// Get selects an arbitrary vte from the [Facility], removes it from the
// Facility, and returns it to the caller.
func (f *Facility) Get() (VTE, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if len(f.pool) != 0 {
		head := f.pool[0]
		f.pool = f.pool[1:]
		// ensure there's always at least one available
		if len(f.pool) == 0 {
			newvte, err := f.new(true)
			if err != nil {
				return nil, err
			}
			f.pool = append(f.pool, newvte)
		}
		return head, nil
	}

	return f.new(false)
}

// Resize can be used to update the width/height of all the vtes in the reservoir.
func (f *Facility) Resize(width, height int) {
	f.mu.Lock()
	defer f.mu.Unlock()

	for _, vte := range f.pool {
		vte.Resize(width, height)
	}
}

// Close closes all vtes associated with this Facility.
func (f *Facility) Close() (ret error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	pool := f.pool
	f.pool = nil
	for _, vte := range pool {
		if err := vte.Close(); err != nil {
			ret = multierror.Append(ret, err)
		}
	}
	return
}

const maxPoolSize = 10

func (f *Facility) initCap(initialCapacity int) {
	pool := make([]VTE, initialCapacity)

	var wg sync.WaitGroup
	wg.Add(initialCapacity)
	for i := range initialCapacity {
		go debug.CapturePanicReport(func() {
			defer wg.Done()
			vte, err := f.new(true)
			if err == nil {
				pool[i] = vte
			} else {
				f.log(log.WarnLevel, "new vte: %v", err)
			}
		})
	}
	wg.Wait()

	f.mu.Lock()
	defer f.mu.Unlock()

	for _, vte := range pool {
		if vte != nil {
			f.pool = append(f.pool, vte)
		}
	}
}

func (f *Facility) put(v VTE) bool {
	f.mu.Lock()
	defer f.mu.Unlock()

	if len(f.pool) == maxPoolSize {
		return false
	}

	v.ClearPrimaryBuffer()
	f.pool = append(f.pool, v)
	return true
}

func (e *Facility) log(level log.Level, msg string, args ...any) {
	if !log.IsLevelEnabled(level) {
		return
	}
	log.WithField(logging.KeyClass, "vtereservoir.Facility").Logf(level, msg, args...)
}

var _ VTE = (*vteAdapter)(nil)

type vteAdapter struct {
	f *Facility
	*vte.Handler
	exit bool
}

func (v *vteAdapter) SetDefaultAttributes(attr term.Attributes) {
	v.Handler.SetDefaultAttributes(attr)
}

func (v *vteAdapter) IsComplete() bool {
	return v.Component().IsComplete()
}

func (v *vteAdapter) URI() workspaceapi.URI {
	return v.Component().URI()
}

func (v *vteAdapter) Title() string {
	return v.Component().Title()
}

func (v *vteAdapter) UsedAlternateBuffer() bool {
	return v.Component().UsedAlternateBuffer()
}

func (v *vteAdapter) ClearPrimaryBuffer() bool {
	return v.Handler.ClearPrimaryBuffer()
}

func (v *vteAdapter) Handle(ev term.Event) (exit, handled bool) {
	exit, handled = v.Handler.Handle(ev)
	v.exit = exit
	return
}

func (v *vteAdapter) Close() error {
	v.f.log(log.TraceLevel, "Close called on vte %p", v)
	if v.exit || v.f.pool == nil || v.UsedAlternateBuffer() {
		return v.Handler.Close()
	}
	v.Handler.SystemCanDispatchBell(func(err error) {
		if err == nil {
			v.f.log(log.TraceLevel, "we were able to schedule a callback, caching VTE %p", v)
			if !v.f.put(v) {
				_ = v.Handler.Close()
			}
		}
	})
	return nil
}
