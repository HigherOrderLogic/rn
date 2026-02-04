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

package ide

import (
	"errors"
	"fmt"
	"strings"

	"github.com/unstablebuild/rune-go-sdk/term"
	tterm "unstable.build/go-tui/term"
)

type echoKey struct {
	term.KeyComb
	instructWait   bool
	instructPrompt bool
}

// parseEchoKeys recursively parses key combinations combined with instructions,
// encoded between {} characters.
func parseEchoKeys(sequence string) (ret []echoKey, err error) {
	idxOpen := strings.IndexRune(sequence, '{')
	if idxOpen < 0 {
		idxOpen = len(sequence)
	}

	// parse up until first instruction
	var keys []term.KeyComb
	keys, err = tterm.ParseKeys(sequence[0:idxOpen])
	for _, key := range keys {
		ret = append(ret, echoKey{KeyComb: key})
	}
	if err != nil {
		ret = nil
		return
	}

	remainder := sequence[idxOpen:]
	if remainder == "" {
		return
	}

	idxClose := strings.IndexRune(remainder, '}')
	if idxClose < 0 {
		err = errors.New("unterminated key: '{' found but no matching '}' found")
		ret = nil
		return
	}
	instruction := remainder[0 : idxClose+1]
	switch instruction {
	case "{wait}":
		ret = append(ret, echoKey{instructWait: true})
	case "{prompt}":
		ret = append(ret, echoKey{instructPrompt: true})
	default:
		err = fmt.Errorf("invalid instruction: %s", instruction)
	}
	if err != nil {
		ret = nil
		return
	}

	var recKeys []echoKey
	recKeys, err = parseEchoKeys(remainder[idxClose+1:])
	if err != nil {
		ret = nil
		return
	}
	ret = append(ret, recKeys...)
	return
}
