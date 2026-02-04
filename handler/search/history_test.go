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

package search

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/unstablebuild/blue/document"
	"unstable.build/go-tui/localstorage/bluestore"
)

func TestHistory(t *testing.T) {
	store := bluestore.AdaptTo(document.NewInMemoryService())
	history := NewHistory(store, "id", 4)
	err := history.Load()
	require.NoError(t, err)

	require.NoError(t, history.Add("jmac"))
	require.NoError(t, history.Add("jj"))

	assert.Equal(t, "jj", history.Next())
	assert.Equal(t, "jmac", history.Next())
	assert.Equal(t, "jj", history.Next())

	// adding another one resets history
	require.NoError(t, history.Add("Kom"))
	assert.Equal(t, "Kom", history.Next())
	assert.Equal(t, "jj", history.Next())

	// queries are persisted across stores
	history2 := NewHistory(store, "id", 4)
	err = history2.Load()
	require.NoError(t, err)
	assert.Equal(t, "Kom", history2.Next())
	assert.Equal(t, "jj", history2.Next())
	assert.Equal(t, "jmac", history2.Next())
	assert.Equal(t, "Kom", history2.Next())

	// history pointers are kept in isolation
	assert.Equal(t, "jmac", history.Next())
	assert.Equal(t, "jj", history2.Next())

	// test max
	require.NoError(t, history.Add("4"))
	require.NoError(t, history.Add("5"))
	assert.Equal(t, "5", history.Next())
	assert.Equal(t, "4", history.Next())
	assert.Equal(t, "Kom", history.Next())
	assert.Equal(t, "jj", history.Next())
	assert.Equal(t, "5", history.Next())

	// queries are NOT persisted across stores with diff IDs
	history3 := NewHistory(store, "id2", 4)
	err = history3.Load()
	require.NoError(t, err)
	assert.Equal(t, "", history3.Next())
	require.NoError(t, history3.Add("a"))
	assert.Equal(t, "a", history3.Next())
	assert.Equal(t, "a", history3.Next())

	// history items can be deleted
	require.NoError(t, history.Add("ToRemove"))
	assert.Equal(t, "ToRemove", history.Next())
	require.NoError(t, history.Remove("ToRemove"))
	found := false
	for _, v := range history.Slice() {
		if v == "ToRemove" {
			found = true
			break
		}
	}
	assert.False(t, found)
}
