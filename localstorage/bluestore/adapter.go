// Unstable Build LLC ("COMPANY") CONFIDENTIAL
//
// Unpublished Copyright (c) 2017-2026 Unstable Build, All Rights Reserved.
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

package bluestore

import (
	"context"
	"errors"

	"github.com/unstablebuild/blue/document"
	"github.com/unstablebuild/rune-go-sdk/api/storageapi"
)

// AdaptTo wraps a document.Service to satisfy storageapi.Service.
func AdaptTo(svc document.Service) storageapi.Service {
	return adapter{svc}
}

// AdaptFrom wraps a storageapi.Service to satisfy document.Service.
func AdaptFrom(svc storageapi.Service) document.Service {
	return wrap{svc}
}

type adapter struct {
	document.Service
}

func (a adapter) Create(ctx context.Context, ID string, doc any) error {
	err := a.Service.Create(ctx, ID, doc)
	switch {
	case errors.Is(err, document.ErrAlreadyExists):
		return storageapi.ErrAlreadyExists
	case errors.Is(err, document.ErrPermissionDenied):
		return storageapi.ErrPermissionDenied
	}
	return err
}

func (a adapter) Set(ctx context.Context, ID string, doc any) error {
	err := a.Service.Set(ctx, ID, doc)
	switch {
	case errors.Is(err, document.ErrPermissionDenied):
		return storageapi.ErrPermissionDenied
	}
	return err
}

func (a adapter) Get(ctx context.Context, ID string, doc any) error {
	err := a.Service.Get(ctx, ID, doc)
	switch {
	case errors.Is(err, document.ErrPermissionDenied):
		return storageapi.ErrPermissionDenied
	case errors.Is(err, document.ErrNotFound):
		return storageapi.ErrNotFound
	}
	return err
}

func (a adapter) Delete(ctx context.Context, ID string) error {
	err := a.Service.Delete(ctx, ID)
	switch {
	case errors.Is(err, document.ErrPermissionDenied):
		return storageapi.ErrPermissionDenied
	}
	return err
}

func (a adapter) List(ctx context.Context, filters []storageapi.Filter) (
	storageapi.Iterator, error,
) {
	var svcFilters []document.Filter
	for _, filter := range filters {
		svcFilters = append(svcFilters, document.Filter{
			Field: document.Field{
				FieldPath: filter.FieldPath,
				Value:     filter.Value,
			},
			Op: document.Op(filter.Op),
		})
	}
	it, err := a.Service.List(ctx, svcFilters)
	if err == nil {
		return it, err
	}
	switch {
	case errors.Is(err, document.ErrPermissionDenied):
		return nil, storageapi.ErrPermissionDenied
	}
	return it, err
}

func (a adapter) Update(
	ctx context.Context, ID string,
	updates []storageapi.Update, precond ...storageapi.Precondition,
) error {
	var svcUpdates []document.Update
	for _, filter := range updates {
		svcUpdates = append(svcUpdates, document.Update{
			FieldPath: filter.FieldPath,
			Value:     filter.Value,
		})
	}
	var svcPreconds []document.Precondition
	for _, filter := range precond {
		svcPreconds = append(svcPreconds, document.Precondition{
			FieldPath: filter.FieldPath,
			Value:     filter.Value,
		})
	}
	err := a.Service.Update(ctx, ID, svcUpdates, svcPreconds...)
	switch {
	case errors.Is(err, document.ErrPermissionDenied):
		return storageapi.ErrPermissionDenied
	case errors.Is(err, document.ErrNotFound):
		return storageapi.ErrNotFound
	case errors.Is(err, document.ErrPreconditionFailed):
		return storageapi.ErrPreconditionFailed
	}
	return err
}

type wrap struct {
	storageapi.Service
}

func (a wrap) List(ctx context.Context, filters []document.Filter) (
	document.Iterator, error,
) {
	var svcFilters []storageapi.Filter
	for _, filter := range filters {
		svcFilters = append(svcFilters, storageapi.Filter{
			Field: storageapi.Field{
				FieldPath: filter.FieldPath,
				Value:     filter.Value,
			},
			Op: storageapi.Op(filter.Op),
		})
	}
	it, err := a.Service.List(ctx, svcFilters)
	if err == nil {
		return it, err
	}
	switch {
	case errors.Is(err, storageapi.ErrPermissionDenied):
		return nil, document.ErrPermissionDenied
	}
	return it, err
}

func (a wrap) Create(ctx context.Context, ID string, doc any) error {
	err := a.Service.Create(ctx, ID, doc)
	switch {
	case errors.Is(err, storageapi.ErrAlreadyExists):
		return document.ErrAlreadyExists
	case errors.Is(err, storageapi.ErrPermissionDenied):
		return document.ErrPermissionDenied
	}
	return err
}

func (a wrap) Update(
	ctx context.Context, ID string,
	updates []document.Update, precond ...document.Precondition,
) error {
	var svcUpdates []storageapi.Update
	for _, filter := range updates {
		svcUpdates = append(svcUpdates, storageapi.Update{
			FieldPath: filter.FieldPath,
			Value:     filter.Value,
		})
	}
	var svcPreconds []storageapi.Precondition
	for _, filter := range precond {
		svcPreconds = append(svcPreconds, storageapi.Precondition{
			FieldPath: filter.FieldPath,
			Value:     filter.Value,
		})
	}
	err := a.Service.Update(ctx, ID, svcUpdates, svcPreconds...)
	switch {
	case errors.Is(err, storageapi.ErrPermissionDenied):
		return document.ErrPermissionDenied
	case errors.Is(err, storageapi.ErrNotFound):
		return document.ErrNotFound
	case errors.Is(err, storageapi.ErrPreconditionFailed):
		return document.ErrPreconditionFailed
	}
	return err
}

func (a wrap) Get(ctx context.Context, ID string, doc any) error {
	err := a.Service.Get(ctx, ID, doc)
	switch {
	case errors.Is(err, storageapi.ErrPermissionDenied):
		return document.ErrPermissionDenied
	case errors.Is(err, storageapi.ErrNotFound):
		return document.ErrNotFound
	}
	return err
}

func (a wrap) Delete(ctx context.Context, ID string) error {
	err := a.Service.Delete(ctx, ID)
	switch {
	case errors.Is(err, storageapi.ErrPermissionDenied):
		return document.ErrPermissionDenied
	}
	return err
}
