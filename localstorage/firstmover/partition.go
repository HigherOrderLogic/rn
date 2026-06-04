// Unstable Build LLC ("COMPANY") CONFIDENTIAL
//
// Unpublished Copyright (c) 2018-2024 Unstable Build, All Rights Reserved.
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

package firstmover

import (
	"context"
	"errors"
	"sync"

	"github.com/unstablebuild/rune-go-sdk/api/storageapi"
)

type partitionService struct {
	root  *Service
	chain []string

	// mu guards the cached partition-chain resolution below. The cache
	// lets a long-lived partitioned view reuse one resolved handle
	// chain across operations instead of re-opening it per op (each
	// open fsyncs on a bbolt backend). It is invalidated when the
	// underlying active backend changes (leadership transition) or on
	// Close.
	mu           sync.Mutex
	cachedActive storageapi.Service
	cachedLeader bool
	cachedTarget storageapi.Service
	cachedWalked []storageapi.Service
}

func (p *partitionService) resolve(active storageapi.Service) (
	storageapi.Service, []storageapi.Service, error,
) {
	walked := make([]storageapi.Service, 0, len(p.chain))
	cur := active
	for _, name := range p.chain {
		next, err := cur.Partition(name)
		if err != nil {
			for i := len(walked) - 1; i >= 0; i-- {
				_ = walked[i].Close()
			}
			return nil, nil, err
		}
		walked = append(walked, next)
		cur = next
	}
	return cur, walked, nil
}

// resolveCached returns a partition target for the current active
// backend, reusing a previously resolved handle chain when active has
// not changed. The cache owns the walked handles and releases them on
// invalidation (when active changes) or on Close, so callers must not
// close the returned target. This avoids re-opening (and fsyncing) the
// partition chain on every operation.
//
// active and isLeader must be sampled together via pickActive so the
// leader/follower close contract is honored when an older cached chain
// is released.
func (p *partitionService) resolveCached(active storageapi.Service, isLeader bool) (
	storageapi.Service, error,
) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// cachedActive is only ever set to a non-nil active (a successful
	// resolve), so it doubles as the cache-valid signal.
	if p.cachedActive == active && active != nil {
		return p.cachedTarget, nil
	}

	p.invalidateCacheLocked()

	target, walked, err := p.resolve(active)
	if err != nil {
		return nil, err
	}
	p.cachedActive = active
	p.cachedLeader = isLeader
	p.cachedTarget = target
	p.cachedWalked = walked
	return target, nil
}

// invalidateCacheLocked releases any cached walked handles (closing
// leader-side handles exactly once, never follower-side) and clears the
// cache. Callers must hold p.mu.
func (p *partitionService) invalidateCacheLocked() {
	if p.cachedActive == nil {
		return
	}
	releaseWalked(p.cachedWalked, p.cachedLeader)
	p.cachedTarget = nil
	p.cachedWalked = nil
	p.cachedActive = nil
}

// pickActive samples root.active and reports whether it is the
// leader-local backend. The two values must be sampled under the same
// lock so a leadership transition cannot make the caller release
// walked partitions on a stale flag — or skip release on a stale flag.
func (p *partitionService) pickActive() (active storageapi.Service, isLeader bool) {
	p.root.mu.Lock()
	defer p.root.mu.Unlock()
	return p.root.active, p.root.active != nil && p.root.svc == p.root.active
}

// releaseWalked closes resolved partition handles iff they came from
// the leader-local backend (each Partition() opened a refcount on the
// underlying bbolt store). Follower-side handles are shallow
// storagerpc.Client copies sharing one gRPC connection; closing them
// would tear down that shared connection, so the caller must pass
// isLeader=false in that case.
//
// isLeader is captured at resolve time, not re-sampled here, so that
// leadership transitions between resolve and release don't drop or
// duplicate the close.
func releaseWalked(walked []storageapi.Service, isLeader bool) {
	if !isLeader {
		return
	}
	for i := len(walked) - 1; i >= 0; i-- {
		_ = walked[i].Close()
	}
}

func (p *partitionService) withActive(
	ctx context.Context,
	fn func(ctx context.Context, target storageapi.Service) error,
) error {
	if err := p.root.waitReady(ctx); err != nil {
		return err
	}
	return p.root.retryHandleDocErrs(ctx, func(ctx context.Context) (bool, error) {
		active, isLeader := p.pickActive()
		target, err := p.resolveCached(active, isLeader)
		if err != nil {
			return p.root.isRetriableError(ctx, err), err
		}
		err = fn(ctx, target)
		return p.root.isRetriableError(ctx, err), err
	})
}

func (p *partitionService) Create(ctx context.Context, ID string, doc any) error {
	return p.withActive(ctx, func(ctx context.Context, target storageapi.Service) error {
		return target.Create(ctx, ID, doc)
	})
}

func (p *partitionService) Set(ctx context.Context, ID string, doc any) error {
	return p.withActive(ctx, func(ctx context.Context, target storageapi.Service) error {
		return target.Set(ctx, ID, doc)
	})
}

func (p *partitionService) Update(
	ctx context.Context, ID string, updates []storageapi.Update,
	preconds ...storageapi.Precondition,
) error {
	return p.withActive(ctx, func(ctx context.Context, target storageapi.Service) error {
		return target.Update(ctx, ID, updates, preconds...)
	})
}

func (p *partitionService) Get(ctx context.Context, ID string, doc any) error {
	return p.withActive(ctx, func(ctx context.Context, target storageapi.Service) error {
		return target.Get(ctx, ID, doc)
	})
}

func (p *partitionService) Delete(ctx context.Context, ID string) error {
	return p.withActive(ctx, func(ctx context.Context, target storageapi.Service) error {
		return target.Delete(ctx, ID)
	})
}

func (p *partitionService) List(ctx context.Context, filters []storageapi.Filter) (
	it storageapi.Iterator, err error,
) {
	// The iterator's lifetime, not this call's, governs the inner
	// context, so List closes over its own ctx rather than the per-
	// attempt ctx withActive threads in.
	err = p.withActive(ctx, func(_ context.Context, target storageapi.Service) error {
		it, err = target.List(ctx, filters)
		return err
	})
	return
}

func (p *partitionService) Partition(name string) (storageapi.Service, error) {
	p.root.mu.Lock()
	if p.root.closed {
		p.root.mu.Unlock()
		return nil, errors.New("firstmover: Partition on closed Service")
	}
	p.root.mu.Unlock()
	chain := make([]string, len(p.chain)+1)
	copy(chain, p.chain)
	chain[len(p.chain)] = name
	return &partitionService{root: p.root, chain: chain}, nil
}

func (p *partitionService) Publish(ctx context.Context, topic string, msg []byte) error {
	return p.root.Publish(ctx, topic, msg)
}

func (p *partitionService) Subscribe(ctx context.Context, topic string) error {
	return p.root.Subscribe(ctx, topic)
}

func (p *partitionService) Receive(ctx context.Context, topic string) ([]byte, error) {
	return p.root.Receive(ctx, topic)
}

func (p *partitionService) IsLeader() bool { return p.root.IsLeader() }

func (p *partitionService) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.invalidateCacheLocked()
	return nil
}
