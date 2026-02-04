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

package vctrlcmd

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/unstablebuild/blue/iterator"
	"github.com/unstablebuild/rune-go-sdk/api/browserapi"
	"github.com/unstablebuild/rune-go-sdk/api/textapi"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"unstable.build/go-tui/clipboard"
	"unstable.build/go-tui/ide/vctrl"
)

// extracts protocol, domain, owner and repo name from a git remote URL.
var gitRemoteRegex = regexp.MustCompile(
	`^(?:(https)://|(git)\@)([^/:]+)[:/]([^/]+)/([\w-]+)(?:\.git)?$`)

type copyRemoteURL struct {
	git              vctrl.Service
	clip             clipboard.Register
	noti             browserapi.Notifications
	providerResolver providerResolver
}

func newCopyRemoteURL(
	git vctrl.Service, clip clipboard.Register, noti browserapi.Notifications,
) *copyRemoteURL {
	ret := new(copyRemoteURL)
	ret.git = git
	ret.clip = clip
	ret.noti = noti
	ret.providerResolver = stringsContainsResolver{}
	return ret
}

// satisfy textapi.CommandHandler
func (c *copyRemoteURL) HandleCommand(ctx context.Context, cmd textapi.Command) (
	err error,
) {
	remoteName := "origin"
	if len(cmd.Args) > 1 {
		remoteName = cmd.Args[0]
	}

	line := cmd.Cursor.Content.Y + 1

	weblink, err := c.generate(ctx, cmd.URI, remoteName, line)
	if err != nil {
		return
	}

	err = c.clipboardCopy(weblink)
	return
}

// satisfy apitext.CommandHandler
func (c *copyRemoteURL) Complete(ctx context.Context, cmd textapi.Command) (
	iterator.Iterator[string], string, error,
) {
	if cmd.URI == (workspaceapi.URI{}) {
		return iterator.Empty[string](), "", nil
	}
	remotes, err := c.git.ListRemotes(ctx, cmd.URI)
	if err != nil {
		return nil, "", err
	}
	return iterator.FromSlice(remotes), "", nil
}

type remoteURLParts struct {
	domain string
	owner  string
	repo   string
	commit string
	file   string
	line   int
}

func (r remoteURLParts) ToMap() map[string]string {
	return map[string]string{
		"domain": r.domain,
		"owner":  r.owner,
		"repo":   r.repo,
		"commit": r.commit,
		"file":   r.file,
		"line":   strconv.Itoa(r.line),
	}

}

func (c *copyRemoteURL) generate(
	ctx context.Context, file workspaceapi.URI, remoteName string, line int,
) (string, error) {
	fileRelPath, err := c.git.RelPath(ctx, file.Path())
	if err != nil {
		return "", err
	}

	remoteURL, err := c.git.RemoteURL(ctx, file, remoteName)
	if err != nil {
		return "", fmt.Errorf("git remote url: %w", err)
	}
	if remoteURL == "" {
		return "", errors.New("empty parsed remote url")
	}

	parts, err := c.parseRemoteURL(remoteURL)
	if err != nil {
		return "", fmt.Errorf("git parse remote url: %w", err)
	}

	currentCommit, err := c.git.CurrentCommit(ctx, file)
	if err != nil {
		return "", fmt.Errorf("git current commit: %w", err)
	}

	parts.commit = currentCommit
	parts.file = fileRelPath
	parts.line = line

	provider, err := c.providerResolver.resolveByRemoteDomain(parts.domain)
	if err != nil {
		return "", fmt.Errorf("could not guess provider: %s", parts.domain)
	}

	weblink, err := provider.buildWeblink(parts)
	if err != nil {
		return "", fmt.Errorf("provider build web link: %w", err)
	}

	return weblink, nil
}

func (c *copyRemoteURL) parseRemoteURL(remoteURL string) (remoteURLParts, error) {
	// Match HTTPS URLs as well as git SSH URLs like the ones below:
	//
	// - git@git.unstable.build:unstablebuild/go-tui.git
	// - https://git.unstable.build/unstablebuild/go-tui.git

	matches := gitRemoteRegex.FindStringSubmatch(remoteURL)
	if len(matches) != 6 {
		return remoteURLParts{}, errors.New("parse remote url")
	}

	_ = matches[2] //  holds the protocol: "https" or "git"

	ret := remoteURLParts{}
	ret.domain = matches[3]
	ret.owner = matches[4]
	ret.repo = matches[5]

	return ret, nil
}

func (c *copyRemoteURL) clipboardCopy(text string) error {
	err := c.clip.Copy(clipboard.DefaultRegisterID, clipboard.Data{Text: text})
	if err != nil {
		return fmt.Errorf("copy web URL: %w", err)
	}

	return c.notify(browserapi.LevelSuccess, "web url copied to clipboard")
}

func (c *copyRemoteURL) notify(
	level browserapi.NotificationLevel, msg string, args ...interface{},
) error {
	_, err := c.noti.Notify(level, msg, args...)
	return err
}

// expand rewrites s to replace {k} with match[k] for each key k in match. All
// template `{placeholders}` must be filled otherwise error is returned.
func expand(s string, match map[string]string) (string, error) {
	oldNew := make([]string, 0, 2*len(match))
	for k, v := range match {
		oldNew = append(oldNew, "{"+k+"}", v)
	}
	ret := strings.NewReplacer(oldNew...).Replace(s)
	if strings.Contains(ret, "{") || strings.Contains(ret, "}") {
		return "", fmt.Errorf("template left with empty placeholders: %s", ret)
	}
	return ret, nil
}
