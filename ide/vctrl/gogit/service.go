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
	"strings"
	"syscall"
	"time"

	"github.com/ernestrc/logd-go/logging"
	"github.com/go-git/go-billy/v6"
	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing/cache"
	"github.com/go-git/go-git/v6/plumbing/object"
	"github.com/go-git/go-git/v6/storage/filesystem"
	"github.com/go-git/go-git/v6/storage/filesystem/dotgit"
	log "github.com/sirupsen/logrus"
	"github.com/unstablebuild/rune-go-sdk/api/schemeapi"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"unstable.build/go-tui/ide/vctrl"
)

// NewService attempts to locate the .git directory by traversing the
// filesystem tree, starting at the root of the workspace. If no .git is
// found, then it is assumed that there might be one or more .git
// directories under the workspace file tree.
//
// The returned service does not cache a *git.Repository: each call that
// needs one opens a fresh repository via getRepo. This avoids stale
// pack-index caches when the on-disk packs are rewritten out-of-band by
// external `git` operations (`git gc`, `git fetch`, worktree-driven
// repacks, etc.). See RUNE-133.
func NewService(
	workspace workspaceapi.URI, scheme schemeapi.Scheme,
) (vctrl.Service, error) {
	root, _ := getRoot(scheme, workspace.Path())
	ret := svc{
		scheme:    scheme,
		workspace: workspace,
		root:      root,
	}
	ret.log(log.TraceLevel, "initialized with root %s", root)
	return ret, nil
}

type svc struct {
	scheme    schemeapi.Scheme
	workspace workspaceapi.URI
	root      string
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
			// files outside any git repository are routinely
			// diffed (e.g. config files); not an error
			level := log.ErrorLevel
			if errors.Is(err, errNoRepoStorage) {
				level = log.DebugLevel
			}
			s.log(level, "diff file %s in %s: %v",
				file, time.Since(start), err)
			return
		}
		s.log(log.TraceLevel, "found %d changes in file %s in %s",
			len(diff.Hunks), file, time.Since(start))
	}()

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
	var committed string
	switch {
	case errors.Is(err, object.ErrFileNotFound):
		// File not present in HEAD (e.g. brand-new file or file added
		// on a different branch) is a valid state, not an error. Treat
		// the committed side as empty so we still produce a
		// full-additions diff.
		s.log(log.DebugLevel, "file %s not found in HEAD commit (%s)",
			path.Path(), headRef.Hash())
		err = nil
	case err != nil:
		err = fmt.Errorf("decode file: %w", err)
		return
	default:
		committed, err = committedFile.Contents()
		if err != nil {
			err = fmt.Errorf("get file content from commit: %w", err)
			return
		}
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
	return s.currentCommit(repo)
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
	return s.shortRef(repo)
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

// getRepo opens the git repository that contains the given file.
func (s svc) getRepo(file workspaceapi.URI) (*git.Repository, error) {
	root, err := s.getRoot(file.Path())
	if err != nil {
		return nil, err
	}
	shim := billyScheme{Scheme: s.scheme}
	storage, err := resolveGitStorage(shim, root)
	if err != nil {
		return nil, err
	}
	rootfs, err := shim.Chroot(root)
	if err != nil {
		return nil, fmt.Errorf("chroot %s: %w", root, err)
	}
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

var errNoRepoStorage = errors.New(
	"could not find repository storage for the given file")

func getRoot(scheme schemeapi.Scheme, path string) (string, error) {
	for {
		if path == "/" {
			return "", errNoRepoStorage
		}
		// this could cause an ENOTDIR; handled below
		info, err := scheme.Stat(filepath.Join(path, ".git"))
		if err == nil {
			// Accept both directories (regular repos) and files (worktrees)
			_ = info
			return path, nil
		}
		if !os.IsNotExist(err) && !errors.Is(err, syscall.ENOTDIR) {
			return "", fmt.Errorf("stat .git dir: %w", err)
		}
		path = filepath.Dir(path)
	}
}

// resolveGitStorage returns the filesystem.Storage for the git repository
// at the given root path. It handles both regular repos (.git directory)
// and git worktrees (.git file with gitdir pointer).
func resolveGitStorage(bs billyScheme, rootPath string) (*filesystem.Storage, error) {
	rootfs, err := bs.Chroot(rootPath)
	if err != nil {
		return nil, fmt.Errorf("chroot %s: %w", rootPath, err)
	}

	info, err := rootfs.Stat(".git")
	if err != nil {
		return nil, fmt.Errorf("stat .git: %w", err)
	}

	if info.IsDir() {
		// Regular repo: .git is a directory
		dotfs, err := rootfs.Chroot(".git")
		if err != nil {
			return nil, fmt.Errorf("chroot .git: %w", err)
		}
		return filesystem.NewStorage(dotfs, cache.NewObjectLRUDefault()), nil
	}

	// Worktree: .git is a file containing "gitdir: <path>"
	dotfs, err := dotGitFileToFilesystem(bs, rootfs, rootPath)
	if err != nil {
		return nil, err
	}

	// Check for commondir (shared objects/refs with main repo)
	commonDir, err := resolveCommonDir(bs, dotfs)
	if err != nil {
		return nil, err
	}
	if commonDir != nil {
		repoFs := dotgit.NewRepositoryFilesystem(dotfs, commonDir)
		return filesystem.NewStorage(repoFs, cache.NewObjectLRUDefault()), nil
	}

	return filesystem.NewStorage(dotfs, cache.NewObjectLRUDefault()), nil
}

// dotGitFileToFilesystem reads a .git file (as used by worktrees) and returns
// the billy.Filesystem for the gitdir it points to.
func dotGitFileToFilesystem(bs billyScheme, rootfs billy.Filesystem, rootPath string) (billy.Filesystem, error) {
	f, err := rootfs.Open(".git")
	if err != nil {
		return nil, fmt.Errorf("open .git file: %w", err)
	}
	defer f.Close()

	b, err := io.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("read .git file: %w", err)
	}

	line := string(b)
	const prefix = "gitdir: "
	if !strings.HasPrefix(line, prefix) {
		return nil, fmt.Errorf(".git file has no %q prefix", prefix)
	}

	gitdir := strings.SplitN(line[len(prefix):], "\n", 2)[0]
	gitdir = strings.TrimSpace(gitdir)

	// Resolve relative gitdir paths relative to the worktree root
	if !filepath.IsAbs(gitdir) {
		gitdir = filepath.Join(rootPath, gitdir)
	}
	gitdir = filepath.Clean(gitdir)

	dotfs, err := bs.Chroot(gitdir)
	if err != nil {
		return nil, fmt.Errorf("chroot gitdir %s: %w", gitdir, err)
	}

	return dotfs, nil
}

// resolveCommonDir reads the "commondir" file from a worktree git dir
// and returns the billy.Filesystem for the shared (common) git directory.
// Returns nil if no commondir file exists.
func resolveCommonDir(bs billyScheme, dotfs billy.Filesystem) (billy.Filesystem, error) {
	f, err := dotfs.Open("commondir")
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("open commondir: %w", err)
	}
	defer f.Close()

	b, err := io.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("read commondir: %w", err)
	}

	path := strings.TrimSpace(string(b))
	if path == "" {
		return nil, nil
	}

	// commondir is relative to the worktree gitdir
	if !filepath.IsAbs(path) {
		path = filepath.Join(dotfs.Root(), path)
	}
	path = filepath.Clean(path)

	commonFs, err := bs.Chroot(path)
	if err != nil {
		return nil, fmt.Errorf("chroot commondir %s: %w", path, err)
	}

	return commonFs, nil
}
