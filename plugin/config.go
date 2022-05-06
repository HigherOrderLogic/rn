package plugin

import (
	"encoding"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/ernestrc/go-tui/component"
	"github.com/ernestrc/go-tui/term"
)

// ErrNotFound is returned when config property is not in config.
var (
	ErrNotFound              = errors.New("key not found")
	ErrInvalidType           = errors.New("type is invalid")
	ErrInvalidAttributeValue = errors.New("attribute value is invalid")
)

// Config is the interface implemented by a plugin configuration provider.
type Config interface {
	GetInt(string) (int, error)
	GetFloat(string) (float64, error)
	GetString(string) (string, error)
	GetBool(string) (bool, error)
	GetConfig(string) (Config, error)
	GetMap(string) (map[string]interface{}, error)
	GetAttribute(string) (term.Attribute, error)
	GetRune(string) (rune, error)
	GetSlice(string) ([]interface{}, error)
}

type internalConfig interface {
	encoding.TextMarshaler
	encoding.TextUnmarshaler
	Config
}

type jsonMap struct {
	mapConfig
}

// satisfies Config but not internalConfig
type mapConfig map[string]interface{}

func (c *jsonMap) MarshalText() ([]byte, error) {
	text, err := json.Marshal(c.mapConfig)
	return text, err
}

func (c *jsonMap) UnmarshalText(text []byte) error {
	c.mapConfig = make(map[string]interface{})
	err := json.Unmarshal(text, &c.mapConfig)
	return err
}

func (c mapConfig) GetInt(key string) (int, error) {
	v, ok := c[key]
	if !ok {
		return 0, ErrNotFound
	}

	switch vt := v.(type) {
	case int:
		return vt, nil
	case float64:
		return int(vt), nil
	case float32:
		return int(vt), nil
	case int64:
		return int(vt), nil
	case int32:
		return int(vt), nil
	default:
		return 0, ErrInvalidType
	}
}

func (c mapConfig) GetFloat(key string) (float64, error) {
	v, ok := c[key]
	if !ok {
		return 0, ErrNotFound
	}

	switch vt := v.(type) {
	case float64:
		return vt, nil
	case int:
		return float64(vt), nil
	case float32:
		return float64(vt), nil
	case int64:
		return float64(vt), nil
	case int32:
		return float64(vt), nil
	default:
		return 0, ErrInvalidType
	}
}

func (c mapConfig) GetString(key string) (string, error) {
	v, ok := c[key]
	if !ok {
		return "", ErrNotFound
	}
	switch vt := v.(type) {
	case string:
		return vt, nil
	case []byte:
		return string(vt), nil
	case []rune:
		return string(vt), nil
	case rune:
		return string(vt), nil
	case byte:
		return string(vt), nil
	default:
		return "", ErrInvalidType
	}
}

func (c mapConfig) GetBool(key string) (bool, error) {
	v, ok := c[key]
	if !ok {
		return false, ErrNotFound
	}
	vt, ok := v.(bool)
	if !ok {
		return false, ErrInvalidType
	}
	return vt, nil
}

func (c mapConfig) GetConfig(key string) (Config, error) {
	m, err := c.GetMap(key)
	if err != nil {
		return nil, err
	}
	return mapConfig(m), nil
}

func (c mapConfig) GetMap(key string) (map[string]interface{}, error) {
	v, ok := c[key]
	if !ok {
		return nil, ErrNotFound
	}
	vt, ok := v.(map[string]interface{})
	if !ok {
		return nil, ErrInvalidType
	}

	return vt, nil
}

func (c mapConfig) GetSlice(key string) ([]interface{}, error) {
	v, ok := c[key]
	if !ok {
		return nil, ErrNotFound
	}
	vt, ok := v.([]interface{})
	if !ok {
		return nil, ErrInvalidType
	}

	return vt, nil
}

func strToAttr(str string) (attr term.Attribute, err error) {
	switch str {
	case "bold":
		attr = term.AttrBold
	case "underline":
		attr = term.AttrUnderline
	case "reverse":
		attr = term.AttrReverse
	case "default":
		attr = term.ColorDefault
	case "black":
		attr = term.ColorBlack
	case "red":
		attr = term.ColorRed
	case "green":
		attr = term.ColorGreen
	case "yellow":
		attr = term.ColorYellow
	case "blue":
		attr = term.ColorBlue
	case "magenta":
		attr = term.ColorMagenta
	case "cyan":
		attr = term.ColorCyan
	case "white":
		attr = term.ColorWhite
	default:
		err = ErrInvalidAttributeValue
	}
	return
}

func (c mapConfig) GetAttribute(key string) (term.Attribute, error) {
	v, ok := c[key]
	if !ok {
		return 0, ErrNotFound
	}
	vt, ok := v.(string)
	if ok {
		return strToAttr(vt)
	}
	i, err := c.GetInt(key)
	if err == nil {
		return term.Attribute(i), nil
	}

	avt, ok := v.([]interface{})
	if !ok {
		return 0, ErrInvalidType
	}

	var attr term.Attribute
	for _, vtv := range avt {
		if vtvs, ok := vtv.(string); ok {
			vtvattr, err := strToAttr(vtvs)
			if err != nil {
				return 0, err
			}
			attr |= vtvattr
		}
	}

	return attr, nil
}

func (c mapConfig) GetRune(key string) (rune, error) {
	v, ok := c[key]
	if !ok {
		return 0, ErrNotFound
	}
	vt, ok := v.(string)
	if ok {
		return []rune(vt)[0], nil
	}

	vtr, ok := v.(rune)
	if ok {
		return vtr, nil
	}

	vtb, ok := v.(byte)
	if ok {
		return rune(vtb), nil
	}

	i, err := c.GetInt(key)
	if err != nil {
		return 0, err
	}

	return rune(i), nil
}

// MapConfig wraps m to satisfy plugin.Config.
func MapConfig(m map[string]interface{}) Config {
	if m == nil {
		panic("invalid argument: map cannot be nil")
	}
	return mapConfig(m)
}

// NOTE(ernestrc): whatever impl we return in NewConfig,
// should be the same one cast here. There should not
// be any other Config castings in the codebase.
func toInternalConfig(c Config) mapConfig {
	return c.(mapConfig)
}

// GetAttributes is a helper which extracts and parses a term.Attributes as a map
// of fg, bg string keys to an attribute. See GetAttribute for more details.
func GetAttributes(c Config, key string) (term.Attributes, error) {
	cfg, err := c.GetConfig(key)
	if err != nil {
		return term.Attributes{}, err
	}

	var attr term.Attributes
	attr.Fg, _ = cfg.GetAttribute("fg")
	attr.Bg, _ = cfg.GetAttribute("bg")
	return attr, nil
}

// GetDuration is a helper which extracts and parses a time.Duration as a string
// from a Config.
func GetDuration(
	pconfig Config, key string, def time.Duration,
) (time.Duration, error) {
	durStr, err := pconfig.GetString(key)
	if err != nil {
		if err != ErrNotFound {
			err = fmt.Errorf("Error getting '%s' from plugin config: %v", key, err)
			return 0, err
		}
		return def, nil
	}

	duration, err := time.ParseDuration(durStr)
	if err != nil {
		err = fmt.Errorf("Error parsing duration '%s' from plugin config: %v", key, err)
		return 0, err
	}

	return duration, nil
}

// GetEvent is a helper which extracts and parses a component.FrameCharSet
//  as a map of string to runes from a Config.
func GetFrameCharset(c Config, key string, def component.FrameCharSet) (
	component.FrameCharSet, error,
) {
	cfg, err := c.GetConfig(key)
	if err != nil {
		return def, err
	}

	cs := def
	r, err := cfg.GetRune("topleft")
	if err == nil {
		cs.TopLeft = r
	}
	r, err = cfg.GetRune("topright")
	if err == nil {
		cs.TopRight = r
	}
	r, err = cfg.GetRune("bottomleft")
	if err == nil {
		cs.BottomLeft = r
	}
	r, err = cfg.GetRune("bottomright")
	if err == nil {
		cs.BottomRight = r
	}
	r, err = cfg.GetRune("horizontaltop")
	if err == nil {
		cs.HorizontalTop = r
	}
	r, err = cfg.GetRune("horizontalbottom")
	if err == nil {
		cs.HorizontalBottom = r
	}
	r, err = cfg.GetRune("verticalleft")
	if err == nil {
		cs.VerticalLeft = r
	}
	r, err = cfg.GetRune("verticalright")
	if err == nil {
		cs.VerticalRight = r
	}
	return cs, nil
}

// GetKey is a helper which extracts and parses a term.KeyComb as a string
// from a Config.
func GetKey(c Config, key string) (term.KeyComb, error) {
	s, err := c.GetString(key)
	if err != nil {
		return term.KeyComb{}, err
	}
	return term.ParseKey(s)
}
