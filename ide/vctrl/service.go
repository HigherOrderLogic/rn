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

package vctrl

import (
	"context"
	"errors"
	"sync"

	"github.com/unstablebuild/rune-go-sdk/api/textapi"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"github.com/unstablebuild/rune-go-sdk/term"
)

// Service abstracts methods to perform Git operations.
type Service interface {
	// Diff returns the differences between the given file in the worktree and HEAD.
	Diff(ctx context.Context, file workspaceapi.URI) (FileDiff, error)

	// CurrentCommit returns the current commit hash.
	CurrentCommit(ctx context.Context, file workspaceapi.URI) (string, error)

	// ShortRef returns the current branch or short reference that HEAD points to.
	ShortRef(ctx context.Context, file workspaceapi.URI) (string, error)

	// RemoteURL returns the remote URL given a remote name.
	RemoteURL(ctx context.Context, file workspaceapi.URI, remoteName string) (string, error)

	// ListRemotes returns a list of remote servers.
	ListRemotes(ctx context.Context, file workspaceapi.URI) ([]string, error)

	// RelPath extracts the path relative to the git repository.
	RelPath(ctx context.Context, file string) (string, error)
}

// A FileDiff represents a unified diff for a single file.
type FileDiff struct {
	// the original name of the file
	OrigName string
	// the new name of the file (often same as OrigName)
	NewName string
	// hunks that were changed from orig to new
	Hunks []Hunk
}

// LocationList converts this FileDiff's hunks into a LocationList.
func (d FileDiff) LocationList(
	delLocAttr, addLocAttr term.Attributes,
) textapi.LocationList {
	var locs []textapi.Location
	for _, hunk := range d.Hunks {
		if hunk.NewLines == 0 {
			from := term.Coordinates{Y: int(hunk.OrigStartLine) - 1}
			locs = append(locs, textapi.Location{
				From: from,
				To:   term.Coordinates{Y: from.Y, X: 1},
				Attr: delLocAttr,
			})
			continue
		}
		from := term.Coordinates{Y: int(hunk.NewStartLine) - 1}
		to := term.Coordinates{Y: int(hunk.NewStartLine+hunk.NewLines) - 1}
		locs = append(locs, textapi.Location{
			From: term.Coordinates{Y: from.Y},
			To:   term.Coordinates{Y: to.Y},
			Attr: addLocAttr,
		})
	}
	return textapi.LocationSlice(locs)
}

// A Hunk represents a series of changes (additions or deletions) in a file's
// unified diff.
type Hunk struct {
	// starting line number in original file
	OrigStartLine int32
	// number of lines the hunk applies to in the original file
	OrigLines int32
	// starting line number in new file
	NewStartLine int32
	// number of lines the hunk applies to in the new file
	NewLines int32
	// hunk body (lines prefixed with '-', '+', or ' ')
	Body string
}

var (
	// ErrDiffNoChanges is returned when diff is run and returns no changes.
	ErrDiffNoChanges = errors.New("diff no changes")
)

// NopService returns an implementation of Service that does nothing.
func NopService() Service {
	return nopService{}
}

// SyncService wraps a service to enable access from multiple goroutines.
func SyncService(root Service, mu sync.Locker) Service {
	return syncService{root: root, mu: mu}
}

type nopService struct {
}

func (c nopService) ListRemotes(
	ctx context.Context, path workspaceapi.URI,
) ([]string, error) {
	return []string{}, nil
}

func (n nopService) Diff(ctx context.Context, file workspaceapi.URI) (FileDiff, error) {
	return FileDiff{}, nil
}

func (n nopService) CurrentCommit(ctx context.Context, file workspaceapi.URI) (string, error) {
	return "", nil
}

func (n nopService) ShortRef(ctx context.Context, file workspaceapi.URI) (string, error) {
	return "", nil
}

func (n nopService) RemoteURL(
	ctx context.Context, file workspaceapi.URI, remoteName string,
) (string, error) {
	return "", nil
}

func (n nopService) RelPath(ctx context.Context, file string) (string, error) {
	return file, nil
}

type syncService struct {
	root Service
	mu   sync.Locker
}

func (s syncService) ListRemotes(
	ctx context.Context, path workspaceapi.URI,
) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.root.ListRemotes(ctx, path)
}

func (s syncService) Diff(ctx context.Context, file workspaceapi.URI) (FileDiff, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.root.Diff(ctx, file)
}

func (s syncService) CurrentCommit(ctx context.Context, file workspaceapi.URI) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.root.CurrentCommit(ctx, file)
}

func (s syncService) ShortRef(ctx context.Context, file workspaceapi.URI) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.root.ShortRef(ctx, file)
}

func (s syncService) RemoteURL(
	ctx context.Context, file workspaceapi.URI, remoteName string,
) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.root.RemoteURL(ctx, file, remoteName)
}

func (s syncService) RelPath(ctx context.Context, file string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.root.RelPath(ctx, file)
}
