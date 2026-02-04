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

package gogit

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/ernestrc/logd-go/logging"
	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing/cache"
	"github.com/go-git/go-git/v6/storage/filesystem"
	log "github.com/sirupsen/logrus"
	"github.com/unstablebuild/rune-go-sdk/api/schemeapi"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"unstable.build/go-tui/ide/vctrl"
)

// NewService attempts to locate the .git directory
// by traversing the filesystem tree, starting at the root
// of the workspace. If no .git is found, then it is assumed
// that there might be one or more .git directories under the
// workspace file tree.
func NewService(
	workspace workspaceapi.URI, scheme schemeapi.Scheme,
) (vctrl.Service, error) {
	root, err := getRoot(scheme, workspace.Path())
	if err != nil {
		return NewServiceWithStorage(workspace, "", nil, scheme), nil
	}
	shim := billyScheme{Scheme: scheme}
	rootfs, err := shim.Chroot(root)
	if err != nil {
		return nil, fmt.Errorf("chroot %s: %v", root, err)
	}
	dotfs, err := rootfs.Chroot(".git")
	if err != nil {
		return nil, fmt.Errorf("chroot .git: %v", err)
	}
	storage := filesystem.NewStorage(dotfs, cache.NewObjectLRUDefault())
	repo, err := git.Open(storage, shim)
	if err != nil {
		return nil, fmt.Errorf("open repository at %s: %w", root, err)
	}
	return NewServiceWithStorage(workspace, root, repo, scheme), nil
}

// NewServiceWithStorage opens a git repository with the given storage and worktree.
// If root and repo are empty, then the returned service will lazily attempt to locate
// the .git directories on a per request basis.
func NewServiceWithStorage(
	workspace workspaceapi.URI,
	root string,
	repo *git.Repository,
	scheme schemeapi.Scheme,
) vctrl.Service {
	ret := svc{
		scheme:    scheme,
		workspace: workspace,
		root:      root,
		repo:      repo,
	}
	ret.log(log.TraceLevel, "initialized with root %s", root)
	return ret
}

type svc struct {
	scheme    schemeapi.Scheme
	workspace workspaceapi.URI
	root      string
	repo      *git.Repository
}

func (s svc) ListRemotes(ctx context.Context, path workspaceapi.URI) ([]string, error) {
	// never use assumed repo here, as it might or might
	// not contain the given file, and we wouldn't know that
	repo, err := s.getRepo(path)
	if err != nil {
		return nil, err
	}
	remotes, err := repo.Remotes()
	if err != nil {
		return nil, fmt.Errorf("repository remotes: %w", err)
	}

	var ret []string
	for _, remote := range remotes {
		ret = append(ret, remote.Config().Name)
	}
	return ret, nil
}

func (s svc) Diff(ctx context.Context, file workspaceapi.URI) (
	diff vctrl.FileDiff, err error,
) {
	start := time.Now()
	s.log(log.TraceLevel, "diffing %s", file)
	defer func() {
		if err != nil {
			s.log(log.ErrorLevel, "diff file %s in %s: %v",
				file, time.Since(start), err)
			return
		}
		s.log(log.TraceLevel, "found %d changes in file %s in %s",
			len(diff.Hunks), file, time.Since(start))
	}()

	if s.repo != nil {
		diff, err = s.diff(ctx, file, s.repo)
		if err == nil {
			return diff, err
		}
	}
	repo, err := s.getRepo(file)
	if err != nil {
		return
	}

	return s.diff(ctx, file, repo)
}

func (s svc) diff(ctx context.Context, path workspaceapi.URI, repo *git.Repository) (
	diff vctrl.FileDiff, err error,
) {
	headRef, err := repo.Head()
	if err != nil {
		return
	}
	head, err := repo.CommitObject(headRef.Hash())
	if err != nil {
		err = fmt.Errorf("get HEAD commit object: %w", err)
		return
	}

	relpath, err := s.RelPath(ctx, path.Path())
	if err != nil {
		err = fmt.Errorf("relative path: %w", err)
		return
	}
	committedFile, err := head.File(relpath)
	if err != nil {
		s.log(log.DebugLevel, "file %s not found in HEAD commit (%s)",
			path.Path(), headRef.Hash())
		err = fmt.Errorf("decode file: %w", err)
		return
	}

	committed, err := committedFile.Contents()
	if err != nil {
		err = fmt.Errorf("get file content from commit: %w", err)
		return
	}

	worktreeFile, werr := s.scheme.Open(path.Path())
	if werr != nil {
		err = fmt.Errorf("open worktree file: %w", werr)
		return
	}
	defer worktreeFile.Close()

	current, err := io.ReadAll(worktreeFile)
	if err != nil {
		err = fmt.Errorf("read file: %w", err)
		return
	}

	changes := vctrl.Diff(ctx, committed, string(current))
	diff = vctrl.ConvertChangesToFileDiff(path, changes)
	return
}

func (s svc) CurrentCommit(ctx context.Context, path workspaceapi.URI) (
	ret string, err error,
) {
	repo, err := s.getRepo(path)
	if err != nil {
		return
	}
	ret, err = s.currentCommit(repo)
	if err == nil || s.repo == nil {
		return
	}
	return s.currentCommit(s.repo)
}

func (s svc) currentCommit(repo *git.Repository) (ret string, err error) {
	headRef, err := repo.Head()
	if err != nil {
		return
	}
	head, err := repo.CommitObject(headRef.Hash())
	if err != nil {
		err = fmt.Errorf("get HEAD commit object: %w", err)
		return
	}
	ret = head.ID().String()
	return
}

func (s svc) ShortRef(ctx context.Context, path workspaceapi.URI) (
	ret string, err error,
) {
	repo, err := s.getRepo(path)
	if err != nil {
		return
	}
	ret, err = s.shortRef(repo)
	if err == nil || s.repo == nil {
		return
	}
	return s.currentCommit(s.repo)
}

func (s svc) shortRef(repo *git.Repository) (ret string, err error) {
	headRef, err := repo.Head()
	if err != nil {
		return
	}
	name := headRef.Name()
	ret = name.Short()
	return
}

func (s svc) RemoteURL(
	ctx context.Context, path workspaceapi.URI, remoteName string,
) (ret string, err error) {
	// never use assumed repo here, as it might or might
	// not contain the given file, and we wouldn't know that
	repo, err := s.getRepo(path)
	if err != nil {
		return
	}
	r, err := repo.Remote(remoteName)
	if err != nil {
		return
	}
	urls := r.Config().URLs
	if len(urls) == 0 {
		err = fmt.Errorf("remote has no urls")
		return
	}

	// fetch always uses the first one
	// whilst push will use all of them
	ret = urls[0]
	return
}

func (s svc) RelPath(ctx context.Context, path string) (string, error) {
	root, err := s.getRoot(path)
	if err != nil {
		return "", fmt.Errorf("repo path: %w", err)
	}
	relFile, err := filepath.Rel(root, path)
	if err != nil {
		return "", fmt.Errorf("file path rel: %w", err)
	}
	return relFile, nil
}

func (s svc) getRoot(file string) (string, error) {
	return getRoot(s.scheme, file)
}

// use pre-loaded repo if workspace is within a repository, or at the same
// level as the repository. Otherwise, look for the given file's repo.
func (s svc) getRepo(file workspaceapi.URI) (*git.Repository, error) {
	root, err := s.getRoot(file.Path())
	if err != nil {
		return nil, err
	}
	rootfs, err := billyScheme{Scheme: s.scheme}.Chroot(root)
	if err != nil {
		return nil, err
	}
	dotfs, err := rootfs.Chroot(".git")
	if err != nil {
		return nil, err
	}
	storage := filesystem.NewStorage(dotfs, cache.NewObjectLRUDefault())
	repo, err := git.Open(storage, rootfs)
	if err != nil {
		return nil, fmt.Errorf("open repository: %w", err)
	}
	return repo, nil
}

func (s svc) log(level log.Level, msg string, args ...any) {
	if !log.IsLevelEnabled(level) {
		return
	}
	log.WithField(logging.KeyClass, "gogit.svc").Logf(level, msg, args...)
}

func getRoot(scheme schemeapi.Scheme, path string) (string, error) {
	for {
		if path == "/" {
			return "", errors.New("could not find repository storage for the given file")
		}
		// this could cause an ENOTDIR; handled below
		info, err := scheme.Stat(filepath.Join(path, ".git"))
		if err == nil {
			if !info.IsDir() {
				return "", fmt.Errorf("invalid .git directory")
			}
			return path, nil
		}
		if !os.IsNotExist(err) && !errors.Is(err, syscall.ENOTDIR) {
			return "", fmt.Errorf("stat .git dir: %w", err)
		}
		path = filepath.Dir(path)
	}
}
