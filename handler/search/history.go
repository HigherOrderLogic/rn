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
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/unstablebuild/blue/document"
	"github.com/unstablebuild/blue/retry"
	"github.com/unstablebuild/rune-go-sdk/api/storageapi"
	"unstable.build/go-tui/localstorage/bluestore"
)

var retryStrategy = retry.SequentialStrategy(30 * time.Millisecond)

const (
	defaultStoreTimeout = 5 * time.Second
)

// History acts as a persisted stack of queries. It provides operations
// to push queries and retrieve previously persisted queries.
// A pop operation is not provided explicitly although calling Add
// when the number of stored queries is equal to the maximum allowed
// will remove the oldest query.
type History struct {
	store   document.Service
	timeout time.Duration
	docID   string
	idx     int
	max     int
	doc     historyDocument
}

// NewHistory allocates storage for a new instance of History and initializes it.
func NewHistory(
	store storageapi.Service, documentID string, maxHistory int,
) *History {
	ret := new(History)
	ret.Init(store, documentID, maxHistory)
	return ret
}

// Init initializes this history with the given store, documentID and maximum history.
func (h *History) Init(
	store storageapi.Service, documentID string, maxHistory int,
) {
	if documentID == "" || store == nil || maxHistory == 0 {
		err := fmt.Sprintf("invalid Init args: documentID=%q, store=%v, max=%d",
			documentID, store, maxHistory)
		panic(err)
	}
	h.store = bluestore.AdaptFrom(store)
	h.docID = documentID
	h.timeout = defaultStoreTimeout
	h.max = maxHistory
	h.doc.Version = 1
}

// Load fetches any queries persisted in store and populates
// this instance of History.
func (h *History) Load() error {
	ctx, cancel := context.WithTimeout(context.Background(), defaultStoreTimeout)
	defer cancel()

	err := h.store.Get(ctx, h.docID, &h.doc)
	if errors.Is(err, document.ErrNotFound) {
		err = h.store.Create(ctx, h.docID, &h.doc)
	}
	if err != nil {
		return fmt.Errorf("failed to load search history from store: %s", err)
	}
	return nil
}

// Add adds query to the history. If the number of queries persisted
// is greater than the max permitted, then the first query is dropped.
func (h *History) Add(query string) error {
	// next Next should return query
	h.idx = 0

	if query == "" {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), defaultStoreTimeout)
	defer cancel()

	err := document.ConsistentUpdate(ctx, h.store, h.docID, &h.doc, retryStrategy,
		func() ([]document.Update, []document.Precondition) {
			h.doc.Queries = append(h.doc.Queries, "")
			copy(h.doc.Queries[1:], h.doc.Queries)
			h.doc.Queries[0] = query

			if len(h.doc.Queries) > h.max {
				h.doc.Queries = h.doc.Queries[:h.max]
			}

			return []document.Update{
					{FieldPath: []string{"Queries"}, Value: h.doc.Queries},
					{FieldPath: []string{"Version"}, Value: h.doc.Version + 1},
				}, []document.Precondition{
					{FieldPath: []string{"Version"}, Value: h.doc.Version},
				}
		})
	if err != nil {
		return fmt.Errorf("could not set command history: %v", err)
	}
	return nil
}

// Remove removes all entries in history that match `{baseCmd} {cmd}`, e.g. `!
// echo 2` (baseCmd: `!`; cmd: `echo 2`).
func (h *History) Remove(cmd string) error {
	ctx, cancel := context.WithTimeout(context.Background(), defaultStoreTimeout)
	defer cancel()

	// anticipate capacity, since the resulting list will be roughly of similar length
	newQueries := make([]string, 0, len(h.doc.Queries))

	err := document.ConsistentUpdate(ctx, h.store, h.docID, &h.doc, retryStrategy,
		func() ([]document.Update, []document.Precondition) {
			newQueries = newQueries[:0]

			// this would be more efficient if the results were sorted but they are not
			for _, q := range h.doc.Queries {
				if q != cmd {
					newQueries = append(newQueries, q)
				}
			}

			h.doc.Queries = newQueries

			return []document.Update{
					{FieldPath: []string{"Queries"}, Value: h.doc.Queries},
					{FieldPath: []string{"Version"}, Value: h.doc.Version + 1},
				}, []document.Precondition{
					{FieldPath: []string{"Version"}, Value: h.doc.Version},
				}
		})
	if err != nil {
		return fmt.Errorf("could not remove item from command history: %v", err)
	}
	return nil
}

// Next returns the next query and updates History
// such that the next call to Next would return the query after.
// If this method is called after the last query has been returned
// the first query is returned instead.
func (h *History) Next() string {
	if len(h.doc.Queries) == 0 {
		return ""
	}
	search := h.doc.Queries[h.idx]
	h.idx++
	if h.idx == len(h.doc.Queries) {
		h.idx = 0
	}
	return search
}

// Slice returns all queries as a slice.
func (h *History) Slice() []string {
	return h.doc.Queries
}

type historyDocument struct {
	Queries []string
	Version int64
}
