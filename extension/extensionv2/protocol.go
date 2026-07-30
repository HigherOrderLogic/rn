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

package extensionv2

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/unstablebuild/blue/auth"
	"github.com/unstablebuild/rune-go-sdk/api/config"
	"github.com/unstablebuild/rune-go-sdk/api/extensionapi"
	"golang.org/x/oauth2"
	"unstable.build/go-tui/extension"
	"unstable.build/go-tui/ide/ideauthorizer"
)

var _ io.Writer = (*protocol)(nil)
var _ io.Reader = (*protocol)(nil)

const extensionTokenExpiresIn = 10 * 24 * 365 * time.Hour

type protocol struct {
	write             bytes.Buffer
	read              bytes.Buffer
	readiness         *extensionReadiness
	grantor           extension.Grantor
	readCh            chan struct{}
	extensionID       string
	socket            string
	dataDir           string
	installDir        string
	cert              []byte
	ctx               context.Context
	cfg               map[string]any
	keys              auth.Keys
	insecureAuth      bool
	verifiedPublisher string
}

func newProtocol(
	ctx context.Context, grantor extension.Grantor,
	extensionID, socket, dataDir, installDir string,
	cert []byte, insecureAuth bool, cfg config.Config, keys auth.Keys,
	readiness *extensionReadiness, verifiedPublisher string,
) *protocol {
	mapCfg := make(map[string]any)
	cfg.Iterate(func(k string, v any) {
		mapCfg[k] = v
	})
	return &protocol{
		readCh:            make(chan struct{}),
		extensionID:       extensionID,
		dataDir:           dataDir,
		installDir:        installDir,
		socket:            socket,
		cert:              cert,
		ctx:               ctx,
		cfg:               mapCfg,
		keys:              keys,
		insecureAuth:      insecureAuth,
		grantor:           grantor,
		readiness:         readiness,
		verifiedPublisher: verifiedPublisher,
	}
}

func (p *protocol) setReady(err error) {
	if p.readiness == nil {
		return
	}
	p.readiness.Set(err)
}

func (p *protocol) Write(data []byte) (int, error) {
	n, _ := p.write.Write(data)
	if p.write.Len() == 0 || p.write.Bytes()[p.write.Len()-1] != '}' {
		return n, nil
	}

	var meta extensionapi.Metadata
	err := json.Unmarshal(p.write.Bytes(), &meta)
	if err != nil {
		err := fmt.Errorf("unmarshal json metadata: %w", err)
		p.setReady(err)
		return n, err
	}

	err = validateMetadata(p.extensionID, meta)
	if err != nil {
		err = fmt.Errorf("validate metadata: %w", err)
		p.setReady(err)
		return n, err
	}

	verifiedPublisher := p.verifiedPublisher
	if verifiedPublisher != "" && !matchesPublisherKey(meta.DeveloperKey, verifiedPublisher) {
		log.Warnf("extension %s developer key does not match verified signing key", p.extensionID)
		verifiedPublisher = ""
	}

	ok, err := p.grantor.Grant(meta, verifiedPublisher)
	if err != nil {
		err = fmt.Errorf("grant permissions: %w", err)
		p.setReady(err)
		return n, err
	}
	if !ok {
		err = errors.New("permission denied")
		p.setReady(err)
		return n, err
	}

	req := extensionapi.Config{
		Socket:      p.socket,
		Certificate: p.cert,
		DataDir:     p.dataDir,
		InstallDir:  p.installDir,
		Config:      p.cfg,
	}
	if !p.insecureAuth {
		signKey, err := p.keys.Sign(p.ctx)
		if err != nil {
			err = fmt.Errorf("get sign key: %w", err)
			p.setReady(err)
			return n, err
		}
		claimsExtra := ideauthorizer.Extension{Metadata: meta, VerifiedPublisher: verifiedPublisher}
		accessToken, err := auth.SignToken(signKey,
			meta.DeveloperID, meta.DeveloperEmail, claimsExtra, extensionTokenExpiresIn)
		if err != nil {
			p.setReady(err)
			return n, err
		}
		req.Token = &oauth2.Token{
			AccessToken: accessToken,
			TokenType:   "bearer",
		}
	}

	reqBytes, err := json.Marshal(&req)
	if err != nil {
		err = fmt.Errorf("marshal extension config: %w", err)
		p.setReady(err)
		return n, err
	}
	_, _ = p.read.Write(reqBytes)
	close(p.readCh)
	p.setReady(nil)
	return n, nil
}

func matchesPublisherKey(developerKey, fingerprint string) bool {
	developerKey = strings.TrimPrefix(strings.ToUpper(developerKey), "0X")
	fingerprint = strings.ToUpper(fingerprint)
	return developerKey == fingerprint || strings.HasSuffix(fingerprint, developerKey)
}

func (p *protocol) Read(b []byte) (int, error) {
	<-p.readCh
	if p.read.Len() == 0 {
		return 0, io.EOF
	}
	return p.read.Read(b)
}

func validateMetadata(extensionID string, meta extensionapi.Metadata) error {
	if meta.DeveloperID == "" {
		return errors.New("developer id must not be missing")
	}
	if meta.DeveloperEmail == "" {
		return errors.New("developer email must not be missing")
	}
	if meta.DeveloperKey == "" {
		return errors.New("developer key must not be missing")
	}
	if meta.ExtensionID != extensionID {
		return errors.New("extension id returned by extension must " +
			"match the extension registered by the host")
	}
	if meta.ExtensionName == "" {
		return errors.New("extension name must not be missing")
	}
	if len(meta.Permissions) == 0 {
		return errors.New("extension must request some permissions")
	}
	return nil
}
