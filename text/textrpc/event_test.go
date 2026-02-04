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
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/unstablebuild/rune-go-sdk/api/textapi"
	"github.com/unstablebuild/rune-go-sdk/api/textapi/textrpc"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"github.com/unstablebuild/rune-go-sdk/term"
	"github.com/unstablebuild/rune-go-sdk/term/termrpc"
	"unstable.build/go-tui/cell"
	"unstable.build/go-tui/component"
	"unstable.build/go-tui/text"
	"unstable.build/go-tui/text/texttest"
)

var (
	uri      workspaceapi.URI
	protoURI textrpc.URI
)

func init() {
	var err error
	uri, err = workspaceapi.ParseURI("file:///test")
	if err != nil {
		panic(err)
	}

	protoURI = *NewURI(uri)
}

func makeEventIntegrationCase(content string) (in, out *cell.Buffer, cursor *text.Cursor) {
	in, out = cell.NewBuffer(), cell.NewBuffer()
	scroll := component.NewScroll(in)
	scroll.Resize(100, 100)
	cursor = text.NewCursor(scroll, nil)
	in.WriteString(content)
	out.WriteString(content)
	return
}

func TestIntegrationInsert(t *testing.T) {
	in, out, cursor := makeEventIntegrationCase("")
	in.Subscribe(text.CellSubscriber(uri, texttest.NewTestHandler(),
		text.FuncEventHandler(func(ctx context.Context, ev textapi.Event) bool {
			out.InsertString(ev.Start, ev.Content)
			return false
		})))

	content := `package main
func main() {
}`
	in.WriteString(content)
	assert.Equal(t, out.String(), content)

	require.True(t, cursor.MoveDown())
	cursor.InsertLineBelow()
	cursor.Insert('\t')
	cursor.Insert('f')
	cursor.Insert('m')
	cursor.Insert('t')
	cursor.Insert('.')

	assert.Equal(t, `package main
func main() {
	fmt.
}`, out.String())

	require.True(t, cursor.MoveLastLine())
	require.True(t, cursor.MoveEndLine())

	cursor.Insert('\n')
	cursor.Insert('\n')
	cursor.Insert('i')
	cursor.Insert('f')
	cursor.Insert('{')
	cursor.Insert('\n')
	cursor.Insert('\t')
	cursor.Insert('X')
	cursor.Insert('\n')
	cursor.Insert('}')

	assert.Equal(t, `package main
func main() {
	fmt.
}

if{
	X
}`, out.String())

	// one of the newlines is "absorbed" by the unix file reader
	in.InsertString(term.Coordinates{Y: 10}, "boom\n\n")

	assert.Equal(t, `package main
func main() {
	fmt.
}

if{
	X
}


boom

`, out.String())
}

func TestIntegrationDelete(t *testing.T) {
	content := `package main
func main() {
	for {
		fmt.Println("Six")
	}
}`
	in, out, cursor := makeEventIntegrationCase(content)

	in.Subscribe(text.CellSubscriber(uri, texttest.NewTestHandler(),
		text.FuncEventHandler(func(ctx context.Context, ev textapi.Event) bool {
			out.Delete(ev.Start, ev.End)
			return false
		})))

	require.True(t, cursor.MoveDown())
	require.True(t, cursor.MoveDown())
	assert.True(t, cursor.SelectLine())
	require.True(t, cursor.MoveDown())
	require.True(t, cursor.MoveDown())
	assert.True(t, cursor.DeleteSelection())

	assert.Equal(t, `package main
func main() {
}`, out.String())
}

func TestIntegrationUndoer(t *testing.T) {
	content := `package main
func main() {
	for {
		fmt.Println("Six")
	}
}`
	in, out, cursor := makeEventIntegrationCase(content)

	in.Subscribe(text.CellSubscriber(uri, texttest.NewTestHandler(),
		text.FuncEventHandler(func(ctx context.Context, ev textapi.Event) bool {
			switch ev.Type {
			case textapi.EventTypeEdit:
				out.Edit(ctx, ev.Start, ev.End, ev.Content)
			}
			return false
		})))

	require.True(t, cursor.MoveDown())
	require.True(t, cursor.MoveDown())
	assert.True(t, cursor.Select())
	require.True(t, cursor.MoveDown())
	require.True(t, cursor.MoveDown())
	require.True(t, cursor.MoveEndLine())
	assert.True(t, cursor.DeleteSelection())

	modified := `package main
func main() {

}`
	assert.Equal(t, modified, out.String())

	cursor.Undo()
	assert.Equal(t, content, out.String())

	cursor.Redo()
	assert.Equal(t, modified, out.String())
}

func TestEventProto(t *testing.T) {
	tsuite := []struct {
		in  textapi.Event
		out textrpc.EditorEvent
	}{
		{
			in: textapi.Event{
				Type:    textapi.EventTypeOpen,
				Content: "Ageispolis",
			},
			out: textrpc.EditorEvent{
				Type:    textrpc.EditorEvent_TypeOpen,
				Content: "Ageispolis",
			},
		},
		{
			in: textapi.Event{
				Type:  textapi.EventTypeHidden,
				Start: term.Coordinates{Y: 8},
				End:   term.Coordinates{Y: 9},
			},
			out: textrpc.EditorEvent{
				Type:  textrpc.EditorEvent_TypeHidden,
				Start: &termrpc.Coordinates{Y: 8},
				End:   &termrpc.Coordinates{Y: 9},
			},
		},
		{
			in: textapi.Event{
				Type:  textapi.EventTypeVisible,
				Start: term.Coordinates{Y: 8},
			},
			out: textrpc.EditorEvent{
				Type:  textrpc.EditorEvent_TypeVisible,
				Start: &termrpc.Coordinates{Y: 8},
			},
		},
		{
			in: textapi.Event{
				Type: textapi.EventTypeFocus,
				URI:  uri,
				Resource: textrpc.Token{
					URI: uri,
				},
			},
			out: textrpc.EditorEvent{
				Type:         textrpc.EditorEvent_TypeFocus,
				ResourceName: &protoURI,
			},
		},
		{
			in: textapi.Event{
				Type:     textapi.EventTypeUnfocus,
				URI:      uri,
				Resource: textrpc.Token{URI: uri},
			},
			out: textrpc.EditorEvent{
				Type:         textrpc.EditorEvent_TypeUnfocus,
				ResourceName: &protoURI,
			},
		},
		{
			in: textapi.Event{
				Type:  textapi.EventTypeScroll,
				Start: term.Coordinates{X: 1, Y: 2},
				End:   term.Coordinates{X: 3, Y: 4},
				From:  term.Coordinates{X: 5, Y: 6},
				To:    term.Coordinates{X: 7, Y: 8},
			},
			out: textrpc.EditorEvent{
				Type:  textrpc.EditorEvent_TypeScroll,
				Start: &termrpc.Coordinates{X: 1, Y: 2},
				End:   &termrpc.Coordinates{X: 3, Y: 4},
				From:  &termrpc.Coordinates{X: 5, Y: 6},
				To:    &termrpc.Coordinates{X: 7, Y: 8},
			},
		},
	}

	t.Run("editor.Event -> EditorEvent", func(t *testing.T) {
		for _, tcase := range tsuite {
			actual := toProto(tcase.in)
			assertEqualProto(t, tcase.out, actual)
		}
	})

	t.Run("EditorEvent -> editor.Event ", func(t *testing.T) {
		for _, tcase := range tsuite {
			actual := textapi.Event{}
			fromProto(&actual, &tcase.out)
			assertEqualEvent(t, tcase.in, actual)
		}
	})
}

func assertEqualProto(t *testing.T, expected, actual textrpc.EditorEvent) {
	assert.Equal(t, expected.Type, actual.Type)
	assert.Equal(t, expected.ResourceName.GetUri(), actual.ResourceName.GetUri())
	assert.Equal(t, expected.Start.GetX(), actual.Start.GetX())
	assert.Equal(t, expected.Start.GetY(), actual.Start.GetY())
	assert.Equal(t, expected.End.GetX(), actual.End.GetX())
	assert.Equal(t, expected.End.GetY(), actual.End.GetY())
	assert.Equal(t, expected.From.GetX(), actual.From.GetX())
	assert.Equal(t, expected.From.GetY(), actual.From.GetY())
	assert.Equal(t, expected.To.GetX(), actual.To.GetX())
	assert.Equal(t, expected.To.GetY(), actual.To.GetY())
	assert.Equal(t, expected.Content, actual.Content)
}

func assertEqualEvent(t *testing.T, expected, actual textapi.Event) {
	assert.Equal(t, expected.Type, actual.Type)
	assert.Equal(t, expected.URI.String(), actual.URI.String())
	assert.Equal(t, expected.Resource, actual.Resource)
	assert.Equal(t, expected.Start, actual.Start)
	assert.Equal(t, expected.From, actual.From)
	assert.Equal(t, expected.Content, actual.Content)
}
