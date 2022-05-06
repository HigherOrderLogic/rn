package term

import "fmt"

type (
	InputMode  int
	OutputMode int
	EventType  uint8
	Modifier   uint8
	Key        uint16
	Attribute  uint16
)

// Attributes represents a cell background and foreground attributes.
type Attributes struct {
	Bg, Fg Attribute
}

// Coordinates represent a point in a 2-D space.
type Coordinates struct {
	X, Y int
}

// Cell represents a location with content on a terminal screen.
// 'Ch' is a unicode character, 'Fg' and 'Bg' are foreground
// and background attributes respectively.
type Cell struct {
	Ch     rune
	Bg, Fg Attribute
}

// KeyComb represents is a key combination. See event for more details.
type KeyComb struct {
	Mod Modifier
	Key Key
	Ch  rune
}

// Event represents a terminal event. The 'Mod', 'Key' and 'Ch' fields are
// valid if 'Type' is EventKey. The 'Width' and 'Height' fields are valid if
// 'Type' is EventResize. The 'Err' field is valid if 'Type' is EventError.
type Event struct {
	Type   EventType // one of Event* constants
	Mod    Modifier  // one of Mod* constants or 0
	Key    Key       // one of Key* constants, invalid if 'Ch' is not 0
	Ch     rune      // a unicode character
	Width  int       // width of the screen
	Height int       // height of the screen
	Err    error     // error in case if input failed
	MouseX int       // x coord of mouse
	MouseY int       // y coord of mouse
}

func (e Event) KeyComb() KeyComb {
	return KeyComb{
		Key: e.Key,
		Mod: e.Mod,
		Ch:  e.Ch,
	}
}

// Writer abstracts termbox write functionality to decouple components from
// termbox, so they're easier to test.
type Writer interface {
	SetCell(Coordinates, Cell)
	Flush() error
	Clear(Attributes) error
	SetCursor(Coordinates)
}

func (e EventType) String() string {
	switch e {
	case EventKey:
		return "Key"
	case EventResize:
		return "Resize"
	case EventMouse:
		return "Mouse"
	case EventError:
		return "Error"
	case EventInterrupt:
		return "Interrupt"
	case EventRaw:
		return "Raw"
	case EventNone:
		return "None"
	default:
		panic(fmt.Sprintf("Not a valid EventType: %d", e))
	}
}
