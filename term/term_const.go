//go:build !js

package term

import "github.com/ernestrc/tcell/v3/termbox"

// Cell colors, you can combine a color with multiple attributes using bitwise
// OR ('|').
const (
	ColorDefault Attribute = Attribute(termbox.ColorDefault)
	ColorBlack             = Attribute(termbox.ColorBlack)
	ColorRed               = Attribute(termbox.ColorRed)
	ColorGreen             = Attribute(termbox.ColorGreen)
	ColorYellow            = Attribute(termbox.ColorYellow)
	ColorBlue              = Attribute(termbox.ColorBlue)
	ColorMagenta           = Attribute(termbox.ColorMagenta)
	ColorCyan              = Attribute(termbox.ColorCyan)
	ColorWhite             = Attribute(termbox.ColorWhite)
)

// Cell attributes, it is possible to use multiple attributes by combining them
// using bitwise OR ('|'). Although, colors cannot be combined. But you can
// combine attributes and a single color.
//
// It's worth mentioning that some platforms don't support certain attributes.
// For example windows console doesn't support AttrUnderline. And on some
// terminals applying AttrBold to background may result in blinking text. Use
// them with caution and test your code on various terminals.
const (
	AttrBold      Attribute = Attribute(termbox.AttrBold)
	AttrUnderline           = Attribute(termbox.AttrUnderline)
	AttrReverse             = Attribute(termbox.AttrReverse)
)

// Event type. See Event.Type field.
const (
	EventKey       EventType = EventType(termbox.EventKey)
	EventResize              = EventType(termbox.EventResize)
	EventMouse               = EventType(termbox.EventMouse)
	EventError               = EventType(termbox.EventError)
	EventInterrupt           = EventType(termbox.EventInterrupt)
	EventRaw                 = EventType(termbox.EventRaw)
	EventNone                = EventType(termbox.EventNone)
)

const (
	KeyF1             Key = Key(termbox.KeyF1)
	KeyF2                 = Key(termbox.KeyF2)
	KeyF3                 = Key(termbox.KeyF3)
	KeyF4                 = Key(termbox.KeyF4)
	KeyF5                 = Key(termbox.KeyF5)
	KeyF6                 = Key(termbox.KeyF6)
	KeyF7                 = Key(termbox.KeyF7)
	KeyF8                 = Key(termbox.KeyF8)
	KeyF9                 = Key(termbox.KeyF9)
	KeyF10                = Key(termbox.KeyF10)
	KeyF11                = Key(termbox.KeyF11)
	KeyF12                = Key(termbox.KeyF12)
	KeyInsert             = Key(termbox.KeyInsert)
	KeyDelete             = Key(termbox.KeyDelete)
	KeyHome               = Key(termbox.KeyHome)
	KeyEnd                = Key(termbox.KeyEnd)
	KeyPgup               = Key(termbox.KeyPgup)
	KeyPgdn               = Key(termbox.KeyPgdn)
	KeyArrowUp            = Key(termbox.KeyArrowUp)
	KeyArrowDown          = Key(termbox.KeyArrowDown)
	KeyArrowLeft          = Key(termbox.KeyArrowLeft)
	KeyArrowRight         = Key(termbox.KeyArrowRight)
	MouseLeft             = Key(termbox.MouseLeft)
	MouseMiddle           = Key(termbox.MouseMiddle)
	MouseRight            = Key(termbox.MouseRight)
	MouseRelease          = Key(termbox.MouseRelease)
	MouseWheelUp          = Key(termbox.MouseWheelUp)
	MouseWheelDown        = Key(termbox.MouseWheelDown)
	KeyCtrlTilde          = Key(termbox.KeyCtrlTilde)
	KeyCtrl2              = Key(termbox.KeyCtrl2)
	KeyCtrlSpace          = Key(termbox.KeyCtrlSpace)
	KeyCtrlA              = Key(termbox.KeyCtrlA)
	KeyCtrlB              = Key(termbox.KeyCtrlB)
	KeyCtrlC              = Key(termbox.KeyCtrlC)
	KeyCtrlD              = Key(termbox.KeyCtrlD)
	KeyCtrlE              = Key(termbox.KeyCtrlE)
	KeyCtrlF              = Key(termbox.KeyCtrlF)
	KeyCtrlG              = Key(termbox.KeyCtrlG)
	KeyBackspace          = Key(termbox.KeyBackspace)
	KeyCtrlH              = Key(termbox.KeyCtrlH)
	KeyTab                = Key(termbox.KeyTab)
	KeyCtrlI              = Key(termbox.KeyCtrlI)
	KeyCtrlJ              = Key(termbox.KeyCtrlJ)
	KeyCtrlK              = Key(termbox.KeyCtrlK)
	KeyCtrlL              = Key(termbox.KeyCtrlL)
	KeyEnter              = Key(termbox.KeyEnter)
	KeyCtrlM              = Key(termbox.KeyCtrlM)
	KeyCtrlN              = Key(termbox.KeyCtrlN)
	KeyCtrlO              = Key(termbox.KeyCtrlO)
	KeyCtrlP              = Key(termbox.KeyCtrlP)
	KeyCtrlQ              = Key(termbox.KeyCtrlQ)
	KeyCtrlR              = Key(termbox.KeyCtrlR)
	KeyCtrlS              = Key(termbox.KeyCtrlS)
	KeyCtrlT              = Key(termbox.KeyCtrlT)
	KeyCtrlU              = Key(termbox.KeyCtrlU)
	KeyCtrlV              = Key(termbox.KeyCtrlV)
	KeyCtrlW              = Key(termbox.KeyCtrlW)
	KeyCtrlX              = Key(termbox.KeyCtrlX)
	KeyCtrlY              = Key(termbox.KeyCtrlY)
	KeyCtrlZ              = Key(termbox.KeyCtrlZ)
	KeyEsc                = Key(termbox.KeyEsc)
	KeyCtrlLsqBracket     = Key(termbox.KeyCtrlLsqBracket)
	KeyCtrl3              = Key(termbox.KeyCtrl3)
	KeyCtrl4              = Key(termbox.KeyCtrl4)
	KeyCtrlBackslash      = Key(termbox.KeyCtrlBackslash)
	KeyCtrl5              = Key(termbox.KeyCtrl5)
	KeyCtrlRsqBracket     = Key(termbox.KeyCtrlRsqBracket)
	KeyCtrl6              = Key(termbox.KeyCtrl6)
	KeyCtrl7              = Key(termbox.KeyCtrl7)
	KeyCtrlSlash          = Key(termbox.KeyCtrlSlash)
	KeyCtrlUnderscore     = Key(termbox.KeyCtrlUnderscore)
	KeySpace              = Key(termbox.KeySpace)
	KeyBackspace2         = Key(termbox.KeyBackspace2)
	KeyCtrl8              = Key(termbox.KeyCtrl8)
)

// Input mode. See SetInputMode function.
const (
	InputEsc     InputMode = InputMode(termbox.InputEsc)
	InputAlt               = InputMode(termbox.InputAlt)
	InputMouse             = InputMode(termbox.InputMouse)
	InputCurrent           = InputMode(termbox.InputCurrent)
)

// Output mode. See SetOutputMode function.
const (
	OutputCurrent   OutputMode = OutputMode(termbox.OutputCurrent)
	OutputNormal               = OutputMode(termbox.OutputNormal)
	Output256                  = OutputMode(termbox.Output256)
	Output216                  = OutputMode(termbox.Output216)
	OutputGrayscale            = OutputMode(termbox.OutputGrayscale)
)

// Alt modifier constant, see Event.Mod field and SetInputMode function.
const (
	ModAlt Modifier = Modifier(termbox.ModAlt)
	// TODO remove
	ModMotion = Modifier(0x22)
	modCtrl   = Modifier(0x11)
	modShift  = Modifier(0x10)
)
