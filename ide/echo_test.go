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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/unstablebuild/rune-go-sdk/term"
)

func TestEchoParse(t *testing.T) {
	suite := []struct {
		input          string
		expectedOutput []echoKey
		expectedErr    error
	}{
		{},
		{
			input:       "><",
			expectedErr: errors.New("invalid escape sequence: unescaped, starting '>' character"),
		},
		{
			input:          "a",
			expectedOutput: []echoKey{{KeyComb: term.KeyComb{Ch: 'a'}}},
		},
		{
			input:          "<space>",
			expectedOutput: []echoKey{{KeyComb: term.KeyComb{Key: term.KeySpace}}},
		},
		{
			input:       "a{WAIT}",
			expectedErr: errors.New("invalid instruction: {WAIT}"),
		},
		{
			input:       "a{{WAIT}",
			expectedErr: errors.New("invalid instruction: {{WAIT}"),
		},
		{
			input:       "a{}WAIT}",
			expectedErr: errors.New("invalid instruction: {}"),
		},
		{
			input:       "a<{wait}space>",
			expectedErr: errors.New("unterminated key: '<' found but no matching '>' found"),
		},
		{
			input: "a{wait}",
			expectedOutput: []echoKey{
				{KeyComb: term.KeyComb{Ch: 'a'}},
				{instructWait: true},
			},
		},
		{
			input: "{wait}a",
			expectedOutput: []echoKey{
				{instructWait: true},
				{KeyComb: term.KeyComb{Ch: 'a'}},
			},
		},
		{
			input: "<space>{wait}",
			expectedOutput: []echoKey{
				{KeyComb: term.KeyComb{Key: term.KeySpace}},
				{instructWait: true},
			},
		},
		{
			input: "{wait}<space>",
			expectedOutput: []echoKey{
				{instructWait: true},
				{KeyComb: term.KeyComb{Key: term.KeySpace}},
			},
		},
		{
			input: "<space>{wait}<space>",
			expectedOutput: []echoKey{
				{KeyComb: term.KeyComb{Key: term.KeySpace}},
				{instructWait: true},
				{KeyComb: term.KeyComb{Key: term.KeySpace}},
			},
		},
		{
			input: "{wait}<space>{wait}",
			expectedOutput: []echoKey{
				{instructWait: true},
				{KeyComb: term.KeyComb{Key: term.KeySpace}},
				{instructWait: true},
			},
		},
		{
			input: "<space>{wait}<space>{wait}",
			expectedOutput: []echoKey{
				{KeyComb: term.KeyComb{Key: term.KeySpace}},
				{instructWait: true},
				{KeyComb: term.KeyComb{Key: term.KeySpace}},
				{instructWait: true},
			},
		},
		{
			input: "{wait}<space>{wait}<space>",
			expectedOutput: []echoKey{
				{instructWait: true},
				{KeyComb: term.KeyComb{Key: term.KeySpace}},
				{instructWait: true},
				{KeyComb: term.KeyComb{Key: term.KeySpace}},
			},
		},
		{
			input: "{prompt}<space>{prompt}<space>",
			expectedOutput: []echoKey{
				{instructPrompt: true},
				{KeyComb: term.KeyComb{Key: term.KeySpace}},
				{instructPrompt: true},
				{KeyComb: term.KeyComb{Key: term.KeySpace}},
			},
		},
	}

	for i, test := range suite {
		t.Run(fmt.Sprintf("test case %d", i), func(t *testing.T) {
			actualOutput, actualErr := parseEchoKeys(test.input)
			assert.Equal(t, test.expectedOutput, actualOutput)
			assert.Equal(t, test.expectedErr, actualErr)
		})
	}
}
