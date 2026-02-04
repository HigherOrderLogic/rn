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

package storagerpc

import (
	"context"
	"errors"

	"github.com/unstablebuild/blue/document"
	"github.com/unstablebuild/rune-go-sdk/api/storageapi"
	"github.com/unstablebuild/rune-go-sdk/api/storageapi/storagerpc/docmarshal"
	"github.com/unstablebuild/rune-go-sdk/api/storageapi/storagerpc/docpb"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Server wraps another storageapi.Service and exposes it through a grpc interface.
type Server struct {
	marshaler docmarshal.Marshaler
	// Use document.Service ifc so we can use all of the implmenetations
	// at github.com/unstablebuild/blue, as well as re-use testing suite.
	// The client converts all of the pb's to storageapi.Service structures.
	other document.Service
	docpb.UnimplementedDocumentStoreServer
}

// NewServer allocates storage for a new Server and initializes it.
func NewServer(other document.Service, m docmarshal.Marshaler) *Server {
	ret := new(Server)
	ret.Init(other, m)
	return ret
}

// Init initializes this server with the given underlying document.Service
// and marshaler.
func (s *Server) Init(other document.Service, m docmarshal.Marshaler) {
	s.other = other
	s.marshaler = m
}

// Create satisfies proto.DocumentStoreServer
func (s *Server) Create(
	ctx context.Context, req *docpb.CreateDocumentRequest,
) (res *docpb.CreateDocumentResponse, err error) {
	id := req.GetId()
	data := req.GetData()

	var pr map[string]interface{}
	err = storageapi.SafeDecode(s.marshaler, &pr, data)
	if err != nil {
		return
	}

	err = s.other.Create(ctx, id, pr)
	if err != nil {
		if errors.Is(err, document.ErrAlreadyExists) {
			err = nil
			res = &docpb.CreateDocumentResponse{
				AlreadyExists: true,
			}
		}
		if errors.Is(err, document.ErrPermissionDenied) {
			err = status.Error(codes.PermissionDenied, "")
		}
		return
	}

	res = &docpb.CreateDocumentResponse{}
	return
}

// Set satisfies proto.DocumentStoreServer
func (s *Server) Set(
	ctx context.Context, req *docpb.SetDocumentRequest,
) (res *docpb.DocumentResponse, err error) {
	id := req.GetId()
	data := req.GetData()

	var pr map[string]interface{}
	err = storageapi.SafeDecode(s.marshaler, &pr, data)
	if err != nil {
		return
	}

	err = s.other.Set(ctx, id, &pr)
	if errors.Is(err, document.ErrPermissionDenied) {
		err = status.Error(codes.PermissionDenied, "")
	}
	res = new(docpb.DocumentResponse)
	return
}

// Update satisfies proto.DocumentStoreServer
func (s *Server) Update(
	ctx context.Context, req *docpb.UpdateDocumentRequest,
) (*docpb.UpdateDocumentResponse, error) {
	updates, err := makeModelUpdates(s.marshaler, req.GetUpdates())
	if err != nil {
		return nil, err
	}
	preconds, err := makeModelPreconds(s.marshaler, req.GetPreconditions())
	if err != nil {
		return nil, err
	}
	// client should panic if no updates are passed
	// so the following is to avoid potential DOS from a malicious client.
	if len(updates) == 0 {
		err = errors.New("invalid request: no paths to update")
		return nil, err
	}
	err = s.other.Update(ctx, req.GetId(), updates, preconds...)
	if err != nil {
		switch err {
		case document.ErrNotFound:
			return &docpb.UpdateDocumentResponse{NotFound: true}, nil
		case document.ErrPreconditionFailed:
			return &docpb.UpdateDocumentResponse{PreconditionFailed: true}, nil
		case document.ErrPermissionDenied:
			return nil, status.Error(codes.PermissionDenied, "")
		}
		return nil, err
	}
	return new(docpb.UpdateDocumentResponse), nil
}

// Get satisfies proto.DocumentStoreServer
func (s *Server) Get(
	ctx context.Context, req *docpb.GetDocumentRequest,
) (res *docpb.GetDocumentResponse, err error) {
	id := req.GetId()

	var pr map[string]interface{}
	err = s.other.Get(ctx, id, &pr)
	if err != nil {
		if errors.Is(err, document.ErrNotFound) {
			res = &docpb.GetDocumentResponse{NotFound: true}
			err = nil
		}
		if errors.Is(err, document.ErrPermissionDenied) {
			err = status.Error(codes.PermissionDenied, "")
		}
		return
	}

	res = &docpb.GetDocumentResponse{
		Data: storageapi.Encode(s.marshaler, pr, false),
	}
	return
}

// Delete satisfies proto.DocumentStoreServer
func (s *Server) Delete(
	ctx context.Context, req *docpb.DeleteDocumentRequest,
) (res *docpb.DocumentResponse, err error) {
	id := req.GetId()

	err = s.other.Delete(ctx, id)
	if errors.Is(err, document.ErrPermissionDenied) {
		err = status.Error(codes.PermissionDenied, "")
	}
	res = new(docpb.DocumentResponse)
	return
}

func (s *Server) streamList(list docpb.DocumentStore_ListServer, it storageapi.Iterator) (err error) {
	for it.HasNext() {
		var pr map[string]interface{}
		err = it.NextTo(&pr)
		res := docpb.ListDocumentResponse{}
		if err != nil {
			res.Error = err.Error()
		} else {
			res.Data = storageapi.Encode(s.marshaler, pr, false)
		}
		err = list.SendMsg(&res)
		if err != nil {
			return
		}
		if res.Error != "" {
			return
		}
	}
	return
}

// List satisfies proto.DocumentStoreServer
func (s *Server) List(
	req *docpb.ListDocumentRequest, list docpb.DocumentStore_ListServer,
) error {
	ctx := context.Background()
	filters, err := makeModelFilters(s.marshaler, req.GetFilters())
	if err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	it, err := s.other.List(ctx, filters)
	if err != nil {
		if errors.Is(err, document.ErrPermissionDenied) {
			err = status.Error(codes.PermissionDenied, "")
		}
		return err
	}

	return s.streamList(list, it)
}
