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

package localstorage

import (
	"context"
	"os"
	"path/filepath"
	"sync"

	log "github.com/sirupsen/logrus"
	"github.com/unstablebuild/blue/document"
	"github.com/unstablebuild/blue/document/docmarshal"
	"github.com/unstablebuild/blue/document/firstmover"
	"unstable.build/go-tui/api/config"
	"unstable.build/go-tui/api/workspaceapi"
	"unstable.build/go-tui/debug"
	"unstable.build/go-tui/localstorage/schemedoc"
	"unstable.build/go-tui/workspace"
)

// New returns a document.Service storage service that
// uses the local directory dir to setup a local filesystem-based
// multi-process safe, goroutine-safe document.Service.
func New(ctx context.Context, dir string, marshaler docmarshal.Marshaler) document.Service {
	ret := new(delayedLoadingService)
	ret.mu.Lock()
	go debug.CapturePanicReport(func() {
		defer ret.mu.Unlock()

		storageDir := filepath.Join(dir, ".db")
		err := os.MkdirAll(storageDir, 0777)
		if err != nil {
			log.Errorf("new storage: mkdir: %v", err)
			ret.service = document.NewInMemoryService()
			return
		}
		storageDirURI, err := workspaceapi.CurrentUserHostURI(storageDir)
		if err != nil {
			log.Errorf("new storage: URI: %v", err)
			ret.service = document.NewInMemoryService()
			return
		}
		scheme, err := workspace.NewFileScheme(ctx, config.NopConfig(), storageDirURI)
		if err != nil {
			log.Errorf("new storage: %v", err)
			ret.service = document.NewInMemoryService()
			return
		}
		storage, err := schemedoc.NewDocumentService(scheme, marshaler)
		if err != nil {
			log.Errorf("new storage: %v", err)
			ret.service = document.NewInMemoryService()
			return
		}

		// place lock path at parent dir of .db
		lockPath := filepath.Join(dir, ".dblock")

		cfg := firstmover.DefaultConfig()
		cfg.Marshaler = marshaler
		cfg.CloseError = schemedoc.ErrClosing
		svc := firstmover.New(storage, lockPath, cfg)

		ret.service = svc
	})
	return ret
}

type delayedLoadingService struct {
	service document.Service
	mu      sync.RWMutex
}

func (d *delayedLoadingService) Create(ctx context.Context, ID string, doc interface{}) error {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.service.Create(ctx, ID, doc)
}

func (d *delayedLoadingService) Set(ctx context.Context, ID string, doc interface{}) error {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.service.Set(ctx, ID, doc)
}

func (d *delayedLoadingService) Update(ctx context.Context, ID string,
	updates []document.Update, precond ...document.Precondition) error {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.service.Update(ctx, ID, updates, precond...)
}

func (d *delayedLoadingService) Get(ctx context.Context, ID string, doc interface{}) error {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.service.Get(ctx, ID, doc)
}

func (d *delayedLoadingService) Delete(ctx context.Context, ID string) error {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.service.Delete(ctx, ID)
}

func (d *delayedLoadingService) List(ctx context.Context, filters []document.Filter) (
	document.Iterator, error,
) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.service.List(ctx, filters)
}

func (d *delayedLoadingService) Close() error {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.service.Close()
}
