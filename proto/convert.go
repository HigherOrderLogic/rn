package proto

import (
	"fmt"

	"github.com/ernestrc/go-tui"
	"github.com/ernestrc/go-tui/term"
)

// ToModel maps this Cell into a term.Cell.
func (c *Cell) ToModel() term.Cell {
	if c == nil {
		return term.Cell{}
	}
	return term.Cell{
		Bg: term.Attribute(c.Background),
		Fg: term.Attribute(c.Foreground),
		// TODO test full UTF-8
		Ch: rune(c.Character),
	}
}

// ToModel maps this Attributes into the corresponding term.Attributes.
func (a *Attributes) ToModel() term.Attributes {
	return term.Attributes{
		Bg: term.Attribute(a.Background),
		Fg: term.Attribute(a.Foreground),
	}
}

// FromModel sets this Attributes from attr term.Attributes.
func (a *Attributes) FromModel(attr term.Attributes) {
	a.Background = uint32(attr.Bg)
	a.Foreground = uint32(attr.Fg)
}

// FromModel takes cc and maps it into this Cell.
func (c *Cell) FromModel(cc term.Cell) {
	c.Background = uint32(cc.Bg)
	c.Foreground = uint32(cc.Fg)
	c.Character = uint32(cc.Ch)
}

// FromModel takes ev and maps it into this Event.
func (e *Event) FromModel(ev term.Event) error {
	switch ev.Type {
	case term.EventKey:
		e.Type = Event_TypeKey
	case term.EventMouse:
		e.Type = Event_TypeMouse
	case term.EventNone:
		e.Type = Event_TypeNone
	case term.EventInterrupt:
		e.Type = Event_TypeInterrupt
	default:
		return fmt.Errorf("serialization error: unknown event type: %+v", ev.Type)
	}

	switch ev.Mod {
	case term.ModAlt:
		e.Mod = Event_Alt
	case term.ModMotion:
		e.Mod = Event_Motion
	case term.Modifier(0):
		e.Mod = Event_None
	default:
		return fmt.Errorf("serialization error: unknown event Mod: %+v", ev.Mod)
	}

	switch ev.Key {
	case term.KeyF1:
		e.Key = Event_F1
	case term.KeyF2:
		e.Key = Event_F2
	case term.KeyF3:
		e.Key = Event_F3
	case term.KeyF4:
		e.Key = Event_F4
	case term.KeyF5:
		e.Key = Event_F5
	case term.KeyF6:
		e.Key = Event_F6
	case term.KeyF7:
		e.Key = Event_F7
	case term.KeyF8:
		e.Key = Event_F8
	case term.KeyF9:
		e.Key = Event_F9
	case term.KeyF10:
		e.Key = Event_F10
	case term.KeyF11:
		e.Key = Event_F11
	case term.KeyF12:
		e.Key = Event_F12
	case term.KeyInsert:
		e.Key = Event_Insert
	case term.KeyDelete:
		e.Key = Event_Delete
	case term.KeyHome:
		e.Key = Event_Home
	case term.KeyEnd:
		e.Key = Event_End
	case term.KeyPgup:
		e.Key = Event_Pgup
	case term.KeyPgdn:
		e.Key = Event_Pgdn
	case term.KeyArrowUp:
		e.Key = Event_ArrowUp
	case term.KeyArrowDown:
		e.Key = Event_ArrowDown
	case term.KeyArrowLeft:
		e.Key = Event_ArrowLeft
	case term.KeyArrowRight:
		e.Key = Event_ArrowRight
	case term.MouseLeft:
		e.Key = Event_MouseLeft
	case term.MouseMiddle:
		e.Key = Event_MouseMiddle
	case term.MouseRight:
		e.Key = Event_MouseRight
	case term.MouseRelease:
		e.Key = Event_MouseRelease
	case term.MouseWheelUp:
		e.Key = Event_MouseWheelUp
	case term.MouseWheelDown:
		e.Key = Event_MouseWheelDown
	case term.KeyCtrlTilde:
		e.Key = Event_CtrlTilde
	// case term.KeyCtrl2:
	// case term.KeyCtrlSpace:
	case term.KeyCtrlA:
		e.Key = Event_CtrlA
	case term.KeyCtrlB:
		e.Key = Event_CtrlB
	case term.KeyCtrlC:
		e.Key = Event_CtrlC
	case term.KeyCtrlD:
		e.Key = Event_CtrlD
	case term.KeyCtrlE:
		e.Key = Event_CtrlE
	case term.KeyCtrlF:
		e.Key = Event_CtrlF
	case term.KeyCtrlG:
		e.Key = Event_CtrlG
	// case term.KeyBackspace:
	case term.KeyCtrlH:
		e.Key = Event_CtrlH
	case term.KeyTab:
		e.Key = Event_Tab
	// case term.KeyCtrlI:
	case term.KeyCtrlJ:
		e.Key = Event_CtrlJ
	case term.KeyCtrlK:
		e.Key = Event_CtrlK
	case term.KeyCtrlL:
		e.Key = Event_CtrlL
	case term.KeyEnter:
		e.Key = Event_Enter
	// case term.KeyCtrlM:
	case term.KeyCtrlN:
		e.Key = Event_CtrlN
	case term.KeyCtrlO:
		e.Key = Event_CtrlO
	case term.KeyCtrlP:
		e.Key = Event_CtrlP
	case term.KeyCtrlQ:
		e.Key = Event_CtrlQ
	case term.KeyCtrlR:
		e.Key = Event_CtrlR
	case term.KeyCtrlS:
		e.Key = Event_CtrlS
	case term.KeyCtrlT:
		e.Key = Event_CtrlT
	case term.KeyCtrlU:
		e.Key = Event_CtrlU
	case term.KeyCtrlV:
		e.Key = Event_CtrlV
	case term.KeyCtrlW:
		e.Key = Event_CtrlW
	case term.KeyCtrlX:
		e.Key = Event_CtrlX
	case term.KeyCtrlY:
		e.Key = Event_CtrlY
	case term.KeyCtrlZ:
		e.Key = Event_CtrlZ
	case term.KeyEsc:
		e.Key = Event_Esc
	//case term.KeyCtrlLsqBracket:
	//case term.KeyCtrl3:
	case term.KeyCtrl4:
		e.Key = Event_Ctrl4
	// case term.KeyCtrlBackslash:
	case term.KeyCtrl5:
		e.Key = Event_Ctrl5
	// case term.KeyCtrlRsqBracket:
	case term.KeyCtrl6:
		e.Key = Event_Ctrl6
	case term.KeyCtrl7:
		e.Key = Event_Ctrl7
	// case term.KeyCtrlSlash:
	// case term.KeyCtrlUnderscore:
	case term.KeySpace:
		e.Key = Event_Space
	// case term.KeyBackspace2:
	case term.KeyCtrl8:
		e.Key = Event_Ctrl8
	default:
		return fmt.Errorf("serialization error: unknown event key: %+v", ev.Key)
	}

	e.Char = uint32(ev.Ch)
	e.MouseX = int32(ev.MouseX)
	e.MouseY = int32(ev.MouseY)

	return nil
}

// ToModel maps this Event into a term.Event.
func (e *Event) ToModel() (ev term.Event, err error) {
	switch e.Type {
	case Event_TypeKey:
		ev.Type = term.EventKey
	case Event_TypeMouse:
		ev.Type = term.EventMouse
	case Event_TypeNone:
		ev.Type = term.EventNone
	case Event_TypeInterrupt:
		ev.Type = term.EventInterrupt
	default:
		return term.Event{},
			fmt.Errorf("serialization error: unknown event type: %s", e.Type)
	}

	switch e.Mod {
	case Event_Alt:
		ev.Mod = term.ModAlt
	case Event_Motion:
		ev.Mod = term.ModMotion
	case Event_None:
		ev.Mod = term.Modifier(0)
	default:
		return term.Event{},
			fmt.Errorf("serialization error: unknown event Mod: %s", e.Mod)
	}

	switch e.Key {
	case Event_F1:
		ev.Key = term.KeyF1
	case Event_F2:
		ev.Key = term.KeyF2
	case Event_F3:
		ev.Key = term.KeyF3
	case Event_F4:
		ev.Key = term.KeyF4
	case Event_F5:
		ev.Key = term.KeyF5
	case Event_F6:
		ev.Key = term.KeyF6
	case Event_F7:
		ev.Key = term.KeyF7
	case Event_F8:
		ev.Key = term.KeyF8
	case Event_F9:
		ev.Key = term.KeyF9
	case Event_F10:
		ev.Key = term.KeyF10
	case Event_F11:
		ev.Key = term.KeyF11
	case Event_F12:
		ev.Key = term.KeyF12
	case Event_Insert:
		ev.Key = term.KeyInsert
	case Event_Delete:
		ev.Key = term.KeyDelete
	case Event_Home:
		ev.Key = term.KeyHome
	case Event_End:
		ev.Key = term.KeyEnd
	case Event_Pgup:
		ev.Key = term.KeyPgup
	case Event_Pgdn:
		ev.Key = term.KeyPgdn
	case Event_ArrowUp:
		ev.Key = term.KeyArrowUp
	case Event_ArrowDown:
		ev.Key = term.KeyArrowDown
	case Event_ArrowLeft:
		ev.Key = term.KeyArrowLeft
	case Event_ArrowRight:
		ev.Key = term.KeyArrowRight
	case Event_MouseLeft:
		ev.Key = term.MouseLeft
	case Event_MouseMiddle:
		ev.Key = term.MouseMiddle
	case Event_MouseRight:
		ev.Key = term.MouseRight
	case Event_MouseRelease:
		ev.Key = term.MouseRelease
	case Event_MouseWheelUp:
		ev.Key = term.MouseWheelUp
	case Event_MouseWheelDown:
		ev.Key = term.MouseWheelDown
	case Event_CtrlTilde:
		ev.Key = term.KeyCtrlTilde
	// case term.KeyCtrl2:
	// case term.KeyCtrlSpace:
	case Event_CtrlA:
		ev.Key = term.KeyCtrlA
	case Event_CtrlB:
		ev.Key = term.KeyCtrlB
	case Event_CtrlC:
		ev.Key = term.KeyCtrlC
	case Event_CtrlD:
		ev.Key = term.KeyCtrlD
	case Event_CtrlE:
		ev.Key = term.KeyCtrlE
	case Event_CtrlF:
		ev.Key = term.KeyCtrlF
	case Event_CtrlG:
		ev.Key = term.KeyCtrlG
	// case term.KeyBackspace:
	case Event_CtrlH:
		ev.Key = term.KeyCtrlH
	case Event_Tab:
		ev.Key = term.KeyTab
	// case term.KeyCtrlI:
	case Event_CtrlJ:
		ev.Key = term.KeyCtrlJ
	case Event_CtrlK:
		ev.Key = term.KeyCtrlK
	case Event_CtrlL:
		ev.Key = term.KeyCtrlL
	case Event_Enter:
		ev.Key = term.KeyEnter
	// case term.KeyCtrlM:
	case Event_CtrlN:
		ev.Key = term.KeyCtrlN
	case Event_CtrlO:
		ev.Key = term.KeyCtrlO
	case Event_CtrlP:
		ev.Key = term.KeyCtrlP
	case Event_CtrlQ:
		ev.Key = term.KeyCtrlQ
	case Event_CtrlR:
		ev.Key = term.KeyCtrlR
	case Event_CtrlS:
		ev.Key = term.KeyCtrlS
	case Event_CtrlT:
		ev.Key = term.KeyCtrlT
	case Event_CtrlU:
		ev.Key = term.KeyCtrlU
	case Event_CtrlV:
		ev.Key = term.KeyCtrlV
	case Event_CtrlW:
		ev.Key = term.KeyCtrlW
	case Event_CtrlX:
		ev.Key = term.KeyCtrlX
	case Event_CtrlY:
		ev.Key = term.KeyCtrlY
	case Event_CtrlZ:
		ev.Key = term.KeyCtrlZ
	case Event_Esc:
		ev.Key = term.KeyEsc
	//case term.KeyCtrlLsqBracket:
	//case term.KeyCtrl3:
	case Event_Ctrl4:
		ev.Key = term.KeyCtrl4
	// case term.KeyCtrlBackslash:
	case Event_Ctrl5:
		ev.Key = term.KeyCtrl5
	// case term.KeyCtrlRsqBracket:
	case Event_Ctrl6:
		ev.Key = term.KeyCtrl6
	case Event_Ctrl7:
		ev.Key = term.KeyCtrl7
	// case term.KeyCtrlSlash:
	// case term.KeyCtrlUnderscore:
	case Event_Space:
		ev.Key = term.KeySpace
	// case term.KeyBackspace2:
	case Event_Ctrl8:
		ev.Key = term.KeyCtrl8
	default:
		return term.Event{},
			fmt.Errorf("serialization error: unknown event key: %s", e.Key)
	}

	ev.Ch = rune(e.Char)
	ev.MouseX = int(e.GetMouseX())
	ev.MouseY = int(e.GetMouseY())

	return ev, nil
}

// ToModel maps this Coordinates into term.Coordinates.
func (c *Coordinates) ToModel() term.Coordinates {
	return term.Coordinates{
		X: int(c.GetX()),
		Y: int(c.GetY()),
	}
}

// FromModel maps this Coordinates from term.Coordinates.
func (c *Coordinates) FromModel(pos term.Coordinates) {
	c.X = int32(pos.X)
	c.Y = int32(pos.Y)
}

// ToModel maps this Manual into a tui.Manual.
func (m *Manual) ToModel() (tui.Manual, error) {
	ret := tui.Manual{
		Summary: m.Summary,
		Keys:    make(tui.KeyMap),
	}

	for _, key := range m.GetKeys() {
		protoEv := Event{Key: key.GetKey(), Char: key.GetChar(), Mod: key.GetMod()}
		ev, err := protoEv.ToModel()
		if err != nil {
			return tui.Manual{}, err
		}
		tkey := term.KeyComb{Key: ev.Key, Mod: ev.Mod, Ch: ev.Ch}
		ret.Keys[tkey] = tui.EventDesc{
			ID:          key.GetId(),
			Description: key.GetDescription(),
		}
	}

	return ret, nil
}

// FromModel maps t into this Manual.
func (m *Manual) FromModel(t tui.Manual) {
	m.Summary = t.Summary

	for kk, desc := range t.Keys {
		protoEv := new(Event)
		protoEv.FromModel(term.Event{Key: kk.Key, Mod: kk.Mod, Ch: kk.Ch})

		m.Keys = append(m.Keys, &Key{
			Id:          desc.ID,
			Description: desc.Description,
			Char:        protoEv.Char,
			Mod:         protoEv.Mod,
			Key:         protoEv.Key,
		})
	}
}
