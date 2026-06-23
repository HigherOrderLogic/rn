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

//go:build !windows

package main

import (
	"context"
	"testing"
)

// TestLoginShellPATHCmdDetachesFromTerminal is a regression for two terminal
// interactions: the Ctrl-C quitting breakage and the SIGTTOU stop that froze
// the GUI. The interactive login shell spawned to resolve PATH must not
// inherit Rune's terminal and must run in a new session (Setsid) with no
// controlling terminal, so its startup tcsetpgrp/tcsetattr calls cannot raise
// SIGTTOU (which would STOP it in state T and block the probe forever) and
// terminal-generated signals (the SIGINT raised by Ctrl-C) never reach it.
func TestLoginShellPATHCmdDetachesFromTerminal(t *testing.T) {
	cmd := loginShellPATHCmd(context.Background(), "/bin/sh")

	if cmd.Stdin != nil {
		t.Fatalf("login shell stdin must not be inherited from the terminal")
	}
	if cmd.SysProcAttr == nil || !cmd.SysProcAttr.Setsid {
		t.Fatalf("login shell must run in a new session (Setsid)")
	}
}
