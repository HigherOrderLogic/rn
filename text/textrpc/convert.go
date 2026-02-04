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

package textrpc

import (
	"fmt"

	"github.com/unstablebuild/rune-go-sdk/api/textapi"
	"github.com/unstablebuild/rune-go-sdk/term/termrpc"
	"github.com/unstablebuild/rune-go-sdk/api/textapi/textrpc"
)

func protoTypeToModel(protoType textrpc.EditorEvent_Type) (ev textapi.EventType, err error) {
	switch protoType {
	case textrpc.EditorEvent_TypeClose:
		ev = textapi.EventTypeClose
	case textrpc.EditorEvent_TypeFlush:
		ev = textapi.EventTypeFlush
	case textrpc.EditorEvent_TypeOpen:
		ev = textapi.EventTypeOpen
	case textrpc.EditorEvent_TypeEdit:
		ev = textapi.EventTypeEdit
	case textrpc.EditorEvent_TypeScroll:
		ev = textapi.EventTypeScroll
	case textrpc.EditorEvent_TypeHidden:
		ev = textapi.EventTypeHidden
	case textrpc.EditorEvent_TypeVisible:
		ev = textapi.EventTypeVisible
	case textrpc.EditorEvent_TypeCursor:
		ev = textapi.EventTypeCursor
	case textrpc.EditorEvent_TypeSelection:
		ev = textapi.EventTypeSelection
	case textrpc.EditorEvent_TypeCreate:
		ev = textapi.EventTypeCreate
	case textrpc.EditorEvent_TypeChange:
		ev = textapi.EventTypeChange
	case textrpc.EditorEvent_TypeFocus:
		ev = textapi.EventTypeFocus
	case textrpc.EditorEvent_TypeUnfocus:
		ev = textapi.EventTypeUnfocus
	case textrpc.EditorEvent_TypeRemove:
		ev = textapi.EventTypeRemove
	case textrpc.EditorEvent_TypeRename:
		ev = textapi.EventTypeRename
	default:
		err = fmt.Errorf("failed to convert proto editor event: invalid type: %v",
			protoType)
	}

	return
}

func fromProto(e *textapi.Event, pe *textrpc.EditorEvent) (err error) {
	e.Type, err = protoTypeToModel(pe.GetType())
	if err != nil {
		return
	}
	if pe.GetResourceName().GetUri() != "" {
		e.URI, err = textrpc.NewURIFromProto(pe.GetResourceName())
		if err != nil {
			return
		}
	}
	if pe.ResourceName != nil {
		uri, err := textrpc.NewURIFromProto(pe.GetResourceName())
		if err != nil {
			return err
		}
		e.Resource = textrpc.Token{
			URI: uri,
		}
	}
	e.Start = pe.GetStart().ToModel()
	e.End = pe.GetEnd().ToModel()
	e.From = pe.GetFrom().ToModel()
	e.To = pe.GetTo().ToModel()
	e.Content = pe.GetContent()
	return nil
}

func protoType(e textapi.Event) textrpc.EditorEvent_Type {
	switch e.Type {
	case textapi.EventTypeClose:
		return textrpc.EditorEvent_TypeClose
	case textapi.EventTypeFlush:
		return textrpc.EditorEvent_TypeFlush
	case textapi.EventTypeOpen:
		return textrpc.EditorEvent_TypeOpen
	case textapi.EventTypeEdit:
		return textrpc.EditorEvent_TypeEdit
	case textapi.EventTypeScroll:
		return textrpc.EditorEvent_TypeScroll
	case textapi.EventTypeHidden:
		return textrpc.EditorEvent_TypeHidden
	case textapi.EventTypeVisible:
		return textrpc.EditorEvent_TypeVisible
	case textapi.EventTypeChange:
		return textrpc.EditorEvent_TypeChange
	case textapi.EventTypeCreate:
		return textrpc.EditorEvent_TypeCreate
	case textapi.EventTypeCursor:
		return textrpc.EditorEvent_TypeCursor
	case textapi.EventTypeSelection:
		return textrpc.EditorEvent_TypeSelection
	case textapi.EventTypeFocus:
		return textrpc.EditorEvent_TypeFocus
	case textapi.EventTypeUnfocus:
		return textrpc.EditorEvent_TypeUnfocus
	case textapi.EventTypeRename:
		return textrpc.EditorEvent_TypeRename
	case textapi.EventTypeRemove:
		return textrpc.EditorEvent_TypeRemove
	default:
		panic(fmt.Sprintf("failed to convert editor event to proto: invalid type: %v", e.Type))
	}
}

// expects ev Resource to be a browser.Token
func toProto(e textapi.Event) textrpc.EditorEvent {
	var ret textrpc.EditorEvent
	ret.Type = protoType(e)

	ret.ResourceName = NewURI(e.URI)

	var start, end, from, to termrpc.Coordinates
	start.FromModel(e.Start)
	end.FromModel(e.End)
	from.FromModel(e.From)
	to.FromModel(e.To)

	ret.Start = &start
	ret.End = &end
	ret.Content = e.Content
	ret.From = &from
	ret.To = &to

	return ret //nolint:govet
}
