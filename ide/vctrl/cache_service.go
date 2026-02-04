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
	"sync"

	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
)

var _ Service = (*Cache)(nil)

// Cache is a Service that adds the ability
// to cache the first response to methods of the given Service, until Purge is called.
type Cache struct {
	mu       sync.Mutex
	root     Service
	diffs    map[workspaceapi.URI]FileDiff
	commits  map[workspaceapi.URI]string
	refs     map[workspaceapi.URI]string
	urls     map[string]string
	remotes  map[workspaceapi.URI][]string
	relpaths map[string]string
}

// NewCache allocates storage for a new Service Cache
// and initializes it with the give svc as the underlying service.
func NewCache(root Service) *Cache {
	ret := new(Cache)
	ret.Init(root)
	return ret
}

// Init initializes this cache with the given service.
func (c *Cache) Init(root Service) {
	c.root = root
	c.diffs = make(map[workspaceapi.URI]FileDiff)
	c.commits = make(map[workspaceapi.URI]string)
	c.refs = make(map[workspaceapi.URI]string)
	c.urls = make(map[string]string)
	c.remotes = make(map[workspaceapi.URI][]string)
	c.relpaths = make(map[string]string)
}

// Purge purges all contents of the cache.
func (c *Cache) Purge() {
	c.mu.Lock()
	defer c.mu.Unlock()
	clear(c.diffs)
	clear(c.commits)
	clear(c.refs)
	clear(c.remotes)
	clear(c.relpaths)
	clear(c.urls)
}

// Diff returns the differences between the given file in the worktree and HEAD.
func (c *Cache) Diff(ctx context.Context, file workspaceapi.URI) (FileDiff, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	diff, ok := c.diffs[file]
	if ok {
		return diff, nil
	}
	diff, err := c.root.Diff(ctx, file)
	c.diffs[file] = diff
	return diff, err
}

// CurrentCommit returns the current commit hash.
func (c *Cache) CurrentCommit(ctx context.Context, file workspaceapi.URI) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	commit, ok := c.commits[file]
	if ok {
		return commit, nil
	}
	commit, err := c.root.CurrentCommit(ctx, file)
	c.commits[file] = commit
	return commit, err
}

// ShortRef returns the current branch or short reference that HEAD points to.
func (c *Cache) ShortRef(ctx context.Context, file workspaceapi.URI) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	ref, ok := c.refs[file]
	if ok {
		return ref, nil
	}
	ref, err := c.root.ShortRef(ctx, file)
	c.refs[file] = ref
	return ref, err
}

// RemoteURL returns the remote URL given a remote name.
func (c *Cache) RemoteURL(ctx context.Context, file workspaceapi.URI, remoteName string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	url, ok := c.urls[remoteName]
	if ok {
		return url, nil
	}
	url, err := c.root.RemoteURL(ctx, file, remoteName)
	c.urls[remoteName] = url
	return url, err
}

// ListRemotes returns a list of remote servers.
func (c *Cache) ListRemotes(ctx context.Context, file workspaceapi.URI) ([]string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	remotes, ok := c.remotes[file]
	if ok {
		return remotes, nil
	}
	remotes, err := c.root.ListRemotes(ctx, file)
	c.remotes[file] = remotes
	return remotes, err
}

// RelPath extracts the path relative to the git repository.
func (c *Cache) RelPath(ctx context.Context, file string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	path, ok := c.relpaths[file]
	if ok {
		return path, nil
	}
	path, err := c.root.RelPath(ctx, file)
	c.relpaths[file] = path
	return path, err
}
