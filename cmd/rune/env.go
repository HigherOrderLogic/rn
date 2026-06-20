// Unstable Build LLC ("COMPANY") CONFIDENTIAL
//
// Unpublished Copyright (c) 2023-2024 Unstable Build, All Rights Reserved.
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

package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/Xuanwo/go-locale"
	log "github.com/sirupsen/logrus"
	"golang.org/x/text/language"
	"unstable.build/go-tui/debug"
)

const fallbackLocale = "UTF-8"

var darwinRe = regexp.MustCompile("UserShell: (/[^ ]+)\n")

func setupRuneBinPATH(dataDir string) error {
	if err := makePkgDirs(dataDir); err != nil {
		return err
	}
	return setRuneBinPATH(dataDir, os.Getenv("PATH"))
}

func startLoginShellPATHResolve(dataDir string) <-chan error {
	done := make(chan error, 1)
	go debug.CapturePanicReport(func() {
		login, err := resolveLoginPath()
		if err != nil {
			done <- err
			return
		}
		done <- setRuneBinPATH(dataDir, login)
	})
	return done
}

func setRuneBinPATH(dataDir, base string) error {
	binDir := filepath.Join(dataDir, "bin")
	if err := os.Setenv("PATH", binDir+":"+base); err != nil {
		return fmt.Errorf("set env PATH: %w", err)
	}
	return nil
}

func resolveLoginPath() (string, error) {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/sh"
	}

	out, err := loginShellPATHCmd(shell).Output()
	if err != nil {
		return "", fmt.Errorf("shell echo PATH: %w", err)
	}

	// Interactive rc files may print banners to stdout before our echo
	// runs. Take the last non-empty line so prior chatter is ignored.
	p := lastNonEmptyLine(string(out))
	if p == "" {
		return "", fmt.Errorf("shell returned empty PATH")
	}
	return p, nil
}

// loginShellPATHCmd builds the command that prints the login shell PATH.
//
// The shell is invoked interactively (-i) so rc files that mutate PATH are
// sourced, but interactive shells touch the controlling terminal on startup
// (zsh ZLE, job control) and sit in the foreground process group. If the
// shell inherited Rune's terminal it could change its modes or absorb the
// SIGINT raised by Ctrl-C, which previously broke Ctrl-C quitting `rune`
// from the command line. Detach it from the terminal: read from /dev/null
// and run in its own process group so terminal-generated signals never reach
// it.
func loginShellPATHCmd(shell string) *exec.Cmd {
	cmd := exec.Command(shell, "-i", "-l", "-c", "echo $PATH")
	cmd.Stdin = nil
	detachFromTerminal(cmd)
	return cmd
}

func makePkgDirs(dataDir string) error {
	for _, sub := range []string{"bin", "lib"} {
		dir := filepath.Join(dataDir, sub)
		if err := os.MkdirAll(dir, 0o777); err != nil {
			return fmt.Errorf("mkdir %s: %w", dir, err)
		}
	}
	return nil
}

func lastNonEmptyLine(s string) string {
	lines := strings.Split(s, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if t := strings.TrimSpace(lines[i]); t != "" {
			return t
		}
	}
	return ""
}

func setEnvForGUI(dataPath string) {
	os.Setenv("RUNE_DATADIR", dataPath)

	// set vte vars
	os.Setenv("TERM", "xterm-256color")
	os.Setenv("COLORTERM", "truecolor")

	// https://specifications.freedesktop.org/startup-notification-spec/startup-notification-0.1.txt
	os.Unsetenv("DESKTOP_STARTUP_ID")
	// https://wayland.app/protocols/xdg-activation-v1
	os.Unsetenv("XDG_ACTIVATION_TOKEN")

	if os.Getenv("SHELL") == "" {
		shell, err := userShell()
		if err != nil {
			log.Errorf("shell detect: %v", err)
			shell = "bash"
		}
		os.Setenv("SHELL", shell)
	}

	u, err := user.Current()
	if err == nil {
		os.Setenv("USER", u.Username)
		os.Setenv("HOME", u.HomeDir)
	} else {
		log.Errorf("user detect: %v", err)
	}

	tag, err := locale.Detect()
	if err != nil {
		log.Errorf("locale detect: %v", err)
	} else {
		base, baseConfidence := tag.Base()
		region, regionConfidence := tag.Region()
		if baseConfidence != language.No && regionConfidence != language.No {
			value := fmt.Sprintf("%s_%s.UTF-8", base.String(), region.String())
			log.Debugf("setting LC_ALL to %q", value)
			os.Setenv("LC_ALL", value)
			return
		}
	}

	log.Debugf("setting LC_CTYPE to %q", fallbackLocale)
	os.Setenv("LC_CTYPE", fallbackLocale)
}

func userShell() (string, error) {
	switch runtime.GOOS {
	case "plan9":
		return plan9Shell()
	case "linux":
		return nixShell()
	case "openbsd":
		return nixShell()
	case "freebsd":
		return nixShell()
	case "darwin":
		return darwinShell()
	case "windows":
		return windowsShell()
	}
	return "", fmt.Errorf("undefined GOOS: %s", runtime.GOOS)
}

func plan9Shell() (string, error) {
	if _, err := os.Stat("/dev/osversion"); err != nil {
		if os.IsNotExist(err) {
			return "", err
		} else {
			return "", errors.New("/dev/osversion check failed")
		}
	}

	return "/bin/rc", nil
}

func nixShell() (string, error) {
	user, err := user.Current()
	if err != nil {
		return "", err
	}

	out, err := exec.Command("getent", "passwd", user.Uid).Output()
	if err != nil {
		return "", err
	}

	ent := strings.Split(strings.TrimSuffix(string(out), "\n"), ":")
	return ent[6], nil
}

func darwinShell() (string, error) {
	dir := "Local/Default/Users/" + os.Getenv("USER")
	out, err := exec.Command("dscl", "localhost", "-read", dir, "UserShell").Output()
	if err != nil {
		return "", err
	}

	matched := darwinRe.FindStringSubmatch(string(out))
	shell := matched[1]
	if shell == "" {
		return "", fmt.Errorf("invalid output: %s", string(out))
	}

	return shell, nil
}

func windowsShell() (string, error) {
	consoleApp := os.Getenv("COMSPEC")
	if consoleApp == "" {
		consoleApp = "cmd.exe"
	}

	return consoleApp, nil
}
