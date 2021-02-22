package editor

//go:generate mockgen -destination=./editor_gomock.go -package editor -self_package editor -source editor.go

import (
	"github.com/ernestrc/go-tui"
	"github.com/ernestrc/go-tui/cell"
	"github.com/ernestrc/go-tui/term"
)

// Handler just wraps a tui.Handler to indicate that this API's handlers might
// not be compatible with other APIs.
type Handler interface {
	tui.Handler
}

// Writer is a cell.Writer that can fail.
type Writer interface {
	Insert(at term.Coordinates, str string) (from, to term.Coordinates, err error)
	Delete(from, to term.Coordinates) (start, end term.Coordinates, str string, err error)
}

// Reader wraps a subset of cell.Reader behaviour with an API that can fail.
type Reader interface {
	RawCells() ([][]term.Cell, error)
}

// Command represents a command issued by the user.
type Command struct {
	Name         string
	ResourceName string
	Resource     Handler
	Cursor       term.Coordinates
}

// CommandHandler is a callback interface that wraps the basic method Command.
type CommandHandler interface {
	// Handle is called when user issued a command previously registered via Register.
	HandleCommand(Command) (exit bool)
}

// Editor is the interface that wraps an API to manage a text editor.
type Editor interface {
	// Edit opens a file and returns a tui.Handler to edit it or an error
	// if there was an error opening it.
	Edit(name string, buf *cell.Buffer) (Handler, error)

	// SubscribeEditor subscribes EventHandler to events of type EventType.
	// Note that it's suffixed with Editor so implementors
	// can also implement browser.Subscriber.
	SubscribeEditor(EventType, EventHandler) error

	// Register registers command to be dispatched to CommandHandler.
	Register(string, CommandHandler) error

	// SetLocationList sets the Handler's location list for users to
	// navigate the code. See LocationList for more details.
	// In order to remove a location list, SetLocationList must be called
	// with an empty (or nil) LocationList.
	SetLocationList(Handler, string, LocationList) error

	// Moves cursor to the next location on list with ID.
	MoveToNextLocation(h Handler, ID string) error

	// Moves cursor to the previous location on list with ID.
	MoveToPrevLocation(h Handler, ID string) error

	// Cursor gets the position of Handler's cursor in the underlying
	// content buffer.
	Cursor(Handler) (term.Coordinates, error)

	// SetCursor sets the cursor of Handler to the given Coordinates.
	SetCursor(Handler, term.Coordinates) error

	// Reader returns a Reader which allows to read the editor's internal buffer.
	Reader(Handler) Reader

	// Writer returns a cell.Writer which allows for direct write access
	// to the editor's internal buffer.
	Writer(Handler) Writer
}

type cellWriter struct {
	c cell.Writer
}

type cellReader struct {
	c cell.Reader
}

func (w cellWriter) Insert(
	at term.Coordinates, str string,
) (from, to term.Coordinates, err error) {
	from, to = w.c.Insert(at, str)
	return
}

func (w cellWriter) Delete(
	from, to term.Coordinates,
) (start, end term.Coordinates, str string, err error) {
	start, end, str = w.c.Delete(from, to)
	return
}

func (r cellReader) RawCells() ([][]term.Cell, error) {
	return r.c.RawCells(), nil
}

// CellWriter wraps a cell.Writer with a Writer that returns no errors.
func CellWriter(c cell.Writer) Writer {
	return cellWriter{c}
}

// CellReader wraps a cell.Reder with a Reader that returns no errors.
func CellReader(c cell.Reader) Reader {
	return cellReader{c}
}

type fnCommandHandler struct {
	cb func(Command) bool
}

func (f fnCommandHandler) HandleCommand(c Command) bool {
	return f.cb(c)
}

// FuncCommandHandler returns an CommandHandler that calls fn
// every time Handle is invoked.
func FuncCommandHandler(fn func(Command) bool) CommandHandler {
	return fnCommandHandler{
		cb: fn,
	}
}
