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

package idepkgtest

import (
	"context"
	"errors"
	"sync"

	"github.com/unstablebuild/blue/iterator"
	"github.com/unstablebuild/blue/release"
)

// ReleaseManager is a release.Manager for testing idepkg.Manager.
type ReleaseManager struct {
	mu                   sync.Mutex
	packages             map[string]release.Package
	versions             map[string][]release.Bundle
	err                  error
	progressUnits        string
	missProgressComplete bool
}

// NewReleaseManager returns a new instance of ReleaseManager.
func NewReleaseManager(
	packages map[string]release.Package,
	versions map[string][]release.Bundle,
) *ReleaseManager {
	return &ReleaseManager{
		packages:      packages,
		versions:      versions,
		progressUnits: "chunks",
	}
}

// ExpectReturnErr signals this ReleaseManager to return error in one of the next
// calls to its release.Manager API.
func (t *ReleaseManager) ExpectReturnErr(err error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.err = err
}

// GetPackage satisfies release.Manager.
func (t *ReleaseManager) GetPackage(
	ctx context.Context, pkgID string,
) (release.Package, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.err != nil {
		return release.Package{}, t.err
	}
	pkg, ok := t.packages[pkgID]
	if !ok {
		return release.Package{}, errors.New("not found")
	}
	return pkg, nil
}

// ListPackages satisfies release.Manager.
func (t *ReleaseManager) ListPackages(
	ctx context.Context, _ map[string]string,
) (iterator.Iterator[release.Package], error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.err != nil {
		return nil, t.err
	}
	var pkgs []release.Package
	for _, pkg := range t.packages {
		pkgs = append(pkgs, pkg)
	}
	return iterator.FromSlice(pkgs), nil
}

// List satisfies release.Manager.
func (t *ReleaseManager) List(
	ctx context.Context, pkgID string, _ map[string]string,
) (iterator.Iterator[release.Bundle], error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.err != nil {
		return nil, t.err
	}
	bundles, ok := t.versions[pkgID]
	if !ok {
		return nil, errors.New("not found")
	}
	return iterator.FromSlice(bundles), nil
}

// Get satisfies release.Manager.
func (t *ReleaseManager) Get(
	ctx context.Context, pkgID string,
	version release.Version, writer release.ProgressWriter,
) (release.Bundle, error) {
	t.mu.Lock()
	if t.err != nil {
		t.mu.Unlock()
		return release.Bundle{}, t.err
	}
	bundles, ok := t.versions[pkgID]
	if !ok {
		t.mu.Unlock()
		return release.Bundle{}, errors.New("not found")
	}

	var found *release.Bundle
	for _, bundle := range bundles {
		if bundle.Version == version {
			found = new(release.Bundle)
			*found = bundle
			break
		}
	}
	if found == nil {
		t.mu.Unlock()
		return release.Bundle{}, errors.New("version not found")
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		if t.missProgressComplete {
			return
		}
		const n = 100
		for i := int64(0); i < n-1; i++ {
			t.mu.Lock()
			writer.Progress(i, n, t.progressUnits)
			t.mu.Unlock()
		}
	}()

	var err error
	go func() {
		defer wg.Done()
		t.mu.Lock()
		defer t.mu.Unlock()
		switch pkgID {
		case "go":
			_, err = writer.Write(goTar)
		case "testpkg":
			_, err = writer.Write(testPkgTar)
		case "six":
			_, err = writer.Write(testPkgTar)
		}
	}()

	t.mu.Unlock()
	wg.Wait()
	if err != nil {
		return release.Bundle{}, err
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	if !t.missProgressComplete {
		writer.Progress(1, 1, t.progressUnits)
	}
	return *found, nil
}

// Upload is not implemented.
func (t *ReleaseManager) Upload(
	context.Context, release.Bundle, release.ProgressReader,
) error {
	panic("unimplemented")
}

// Delete is not implemented.
func (t *ReleaseManager) Delete(context.Context, string, release.Version) error {
	panic("unimplemented")
}

// Create is not implemented.
func (t *ReleaseManager) Create(context.Context, release.Package) error {
	panic("unimplemented")
}

// DeletePackage is not implemented.
func (t *ReleaseManager) DeletePackage(context.Context, string) error {
	panic("unimplemented")
}

// SetMissProgressComplete signals this ReleaseManager to not complete
// the download notification via UpdateNotificationProgress.
func (t *ReleaseManager) SetMissProgressComplete(doIt bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.missProgressComplete = doIt
}
