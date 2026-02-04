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

package workspacessh

import (
	"fmt"
	"time"

	multierr "github.com/ernestrc/go-multierror"
	"github.com/unstablebuild/rune-go-sdk/api/config"
)

const (
	defSSHTimeout = 5 * time.Second
)

type sshConfig struct {
	privateKeys []string
	timeout     time.Duration
	command     string
	shell       string
	insecure    bool
}

func fromConfig(cfg config.Config) (ret sshConfig, retErr error) {
	timeout, err := getTimeout(cfg)
	if err != nil && err != config.ErrNotFound {
		retErr = multierr.Append(retErr, err)
	}
	command, err := getCommand(cfg)
	if err != nil && err != config.ErrNotFound {
		retErr = multierr.Append(retErr, err)
	}
	shell, err := getShell(cfg)
	if err != nil && err != config.ErrNotFound {
		retErr = multierr.Append(retErr, err)
	}
	privateKeys, err := getPrivateKeys(cfg)
	if err != nil && err != config.ErrNotFound {
		retErr = multierr.Append(retErr, err)
	}
	insecure, err := getInsecure(cfg)
	if err != nil && err != config.ErrNotFound {
		retErr = multierr.Append(retErr, err)
	}
	if retErr != nil {
		retErr = fmt.Errorf("could not load ssh config: %s", retErr)
		return
	}

	ret.timeout = timeout
	ret.command = command
	ret.privateKeys = privateKeys
	ret.shell = shell
	ret.insecure = insecure
	return
}

func getTimeout(cfg config.Config) (ret time.Duration, err error) {
	ret = defSSHTimeout

	sshTimeout, err := config.GetDuration(cfg, "timeout", defSSHTimeout)
	if err != nil {
		return
	}

	ret = sshTimeout
	return
}

func getInsecure(cfg config.Config) (ret bool, err error) {
	ret, err = cfg.GetBool("insecure")
	return
}

func getCommand(cfg config.Config) (string, error) {
	cmd, err := cfg.GetString("command")
	if err != nil {
		return "", err
	}

	return cmd, nil
}

func getShell(cfg config.Config) (string, error) {
	cmd, err := cfg.GetString("shell")
	if err != nil {
		return "", err
	}

	return cmd, nil
}

func getPrivateKeys(cfg config.Config) (ret []string, err error) {
	keyIfcs, err := cfg.GetSlice("private_keys")
	if err != nil {
		return nil, err
	}

	for _, keyIfc := range keyIfcs {
		key, ok := keyIfc.(string)
		if !ok {
			err = multierr.Append(err, fmt.Errorf("slice of strings expected for 'private_keys' but found %v", key))
			continue
		}
		ret = append(ret, key)
	}
	if err != nil {
		return nil, err
	}
	return ret, err
}
