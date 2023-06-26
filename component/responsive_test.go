package component

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"unstable.build/go-tui"
	"unstable.build/go-tui/cell"
	"unstable.build/go-tui/term"
)

func TestResponsiveStringDraw(t *testing.T) {
	t.Run("should behave like String with single line strings and enough space", func(t *testing.T) {
		tcases := []struct {
			in  string
			out string
			cfg StringResponsiveConfig
		}{
			{
				in:  "aaaa",
				out: "aaaa \n     \n     \n     \n     ",
			},
			{
				in:  "X\nX\nX\nX\nX\nX\nX\nX\n",
				out: "X    \nX    \nX    \nX    \nX    ",
			},
			{
				in:  "a",
				out: "a    \n     \n     \n     \n     ",
			},
		}

		for _, tcase := range tcases {
			t.Run("StringResponsive", func(t *testing.T) {
				testString(t, func(in string) tui.Component {
					return StringResponsive(in, tcase.cfg)
				}, 5, 5, tcase.in, tcase.out)
			})

			t.Run("BufferResponsive", func(t *testing.T) {
				testString(t, func(in string) tui.Component {
					b := cell.NewBuffer()
					b.WriteString(in)
					return Buffer(b, tcase.cfg)
				}, 5, 5, tcase.in, tcase.out)
			})
		}
	})

	t.Run("should wrap lines around", func(t *testing.T) {
		tcases := []struct {
			in     string
			out    string
			cfg    StringResponsiveConfig
			height int
		}{
			{
				in:  "XXXXXXXXXX\nBBBBBBBBBB\nCCCCCCCCCCCC\nDDDDDDDDDD\nEEEEEEEEEEE\nFFFFFFFFFF\n",
				out: "XXXXX\nXXXXX\nBBBBB\nBBBBB\nCCCCC",
			},
			{
				in:  "XXXXXXXXXXXXXXXX\nYYYYYYYYYnZZZZZZZZZ",
				out: "XXXXX\nXXXXX\nXXXXX\nX    \nYYYYY",
			},
			{
				in:  "X\n1111 \n222222222222222222222222222222222",
				out: "X    \n1111 \n22222\n22222\n22222",
			},
			{
				in: "X\n111\n222222222222222222222222222222222",
				out: `┌───┐
│X  │
│111│
│222│
└───┘`,
				cfg: StringResponsiveConfig{StringConfig: StringConfig{FrameCharSet: FrameCharSetDefault()}},
			},
			{
				in: "XXXX\n111\n22",
				out: `┌───┐
│XXX│
│X  │
│111│
└───┘`,
				cfg: StringResponsiveConfig{StringConfig: StringConfig{FrameCharSet: FrameCharSetDefault()}},
			},
			{
				in: "XXXXXXXXXX\nBBBBBBBBBB\nCCCCCCCCCCCC\nDDDDDDDDDD\nEEEEEEEEEEE\nFFFFFFFFFF",
				cfg: StringResponsiveConfig{
					StringConfig: StringConfig{
						Alignment:         SpanAlignmentCentered,
						FrameCharSet:      FrameCharSetDefault(),
						PaddingVertical:   2,
						PaddingHorizontal: 2,
					},
				},
				height: 67,
				out: `┌───┐
│   │
│ X │
│ X │
│ X │
│ X │
│ X │
│ X │
│ X │
│ X │
│ X │
│ X │
│ B │
│ B │
│ B │
│ B │
│ B │
│ B │
│ B │
│ B │
│ B │
│ B │
│ C │
│ C │
│ C │
│ C │
│ C │
│ C │
│ C │
│ C │
│ C │
│ C │
│ C │
│ C │
│ D │
│ D │
│ D │
│ D │
│ D │
│ D │
│ D │
│ D │
│ D │
│ D │
│ E │
│ E │
│ E │
│ E │
│ E │
│ E │
│ E │
│ E │
│ E │
│ E │
│ E │
│ F │
│ F │
│ F │
│ F │
│ F │
│ F │
│ F │
│ F │
│ F │
│ F │
│   │
└───┘`,
			},
		}

		for _, tcase := range tcases {
			t.Run("StringResponsive", func(t *testing.T) {
				height := 5
				if tcase.height != 0 {
					height = tcase.height
				}
				testString(t, func(in string) tui.Component {
					return StringResponsive(in, tcase.cfg)
				}, 5, height, tcase.in, tcase.out)
			})

			t.Run("BufferResponsive", func(t *testing.T) {
				height := 5
				if tcase.height != 0 {
					height = tcase.height
				}
				testString(t, func(in string) tui.Component {
					b := cell.NewBuffer()
					b.WriteString(in)
					return Buffer(b, tcase.cfg)
				}, 5, height, tcase.in, tcase.out)
			})
		}
	})

	t.Run("should not split words in half", func(t *testing.T) {
		tcases := []struct {
			in  string
			out string
			cfg StringResponsiveConfig
		}{
			{
				in:  "XX XX XX\nYYYYY YYYY\nZZZZZZZZZZZZ ZZZZZZZZZZZZ ZZZZZZZZZZZZ",
				out: "XX   \nXX XX\nYYYYY\n YYYY\nZZZZZ",
				cfg: StringResponsiveConfig{NoSplitWords: true},
			},
		}

		for _, tcase := range tcases {
			t.Run("StringResponsive", func(t *testing.T) {
				testString(t, func(in string) tui.Component {
					return StringResponsive(in, tcase.cfg)
				}, 5, 5, tcase.in, tcase.out)
			})

			t.Run("BufferResponsive", func(t *testing.T) {
				testString(t, func(in string) tui.Component {
					b := cell.NewBuffer()
					b.WriteString(in)
					return Buffer(b, tcase.cfg)
				}, 5, 5, tcase.in, tcase.out)
			})
		}
	})
}

func TestResponsiveHeight(t *testing.T) {
	tcases := []struct {
		in    string
		width int
		out   int
		cfg   StringResponsiveConfig
	}{
		{
			in:    "XXXXXXXXXX\nBBBBBBBBBB\nCCCCCCCCCCCC\nDDDDDDDDDD\nEEEEEEEEEEE\nFFFFFFFFFF\n",
			width: 5,
			out:   15,
		},
		{
			in:    "X",
			width: 5,
			out:   1,
		},
		{
			in:    "X\nX\nX\nX\n",
			width: 10,
			out:   5,
		},
		{
			in:    "XXXXXXXXXX",
			width: 10,
			out:   1,
		},
		{
			in:    "XXXXXXXXXX",
			width: -1,
			out:   0,
		},
		{
			in:    "XXXXXXXXXX",
			width: 0, // could trigger division by zero
			out:   0,
		},
		{
			in:    "X\nX\nX\nX\n",
			width: 10,
			out:   7,
			cfg:   StringResponsiveConfig{StringConfig: StringConfig{FrameCharSet: FrameCharSetDefault()}},
		},
		{
			in:    "X",
			width: 5,
			out:   3,
			cfg:   StringResponsiveConfig{StringConfig: StringConfig{FrameCharSet: FrameCharSetDefault()}},
		},
		{
			in:    "XXXXXXXXXX",
			width: 10,
			out:   4,
			cfg:   StringResponsiveConfig{StringConfig: StringConfig{FrameCharSet: FrameCharSetDefault()}},
		},
		{
			in:    "XXXXXXXXXX\nBBBBBBBBBB\nCCCCCCCCCCCC\nDDDDDDDDDD\nEEEEEEEEEEE\nFFFFFFFFFF\n",
			width: 5,
			out:   27,
			cfg:   StringResponsiveConfig{StringConfig: StringConfig{FrameCharSet: FrameCharSetDefault()}},
		},
		{
			in:    "XXXXXXXXXX\nBBBBBBBBBB\nCCCCCCCCCCCC\nDDDDDDDDDD\nEEEEEEEEEEE\nFFFFFFFFFF\n",
			width: 5,
			out:   68,
			cfg: StringResponsiveConfig{
				StringConfig: StringConfig{
					Alignment:         SpanAlignmentCentered,
					FrameCharSet:      FrameCharSetDefault(),
					PaddingVertical:   2,
					PaddingHorizontal: 2,
				},
			},
		},
	}

	for _, tcase := range tcases {
		t.Run("StringResponsive", func(t *testing.T) {
			s := StringResponsive(tcase.in, tcase.cfg)
			out := s.Height(tcase.width)
			assert.Equal(t, tcase.out, out)
		})
		t.Run("BufferResponsive", func(t *testing.T) {
			b := cell.CellsToBuffer(nil, 4)
			b.WriteString(tcase.in)
			s := Buffer(b, tcase.cfg)
			out := s.Height(tcase.width)
			assert.Equal(t, tcase.out, out)
		})
	}
}

func TestBufferWithEdits(t *testing.T) {
	t.Run("Height", func(t *testing.T) {
		b := cell.CellsToBuffer(nil, 4)
		b.WriteString("aa")
		s := Buffer(b, StringResponsiveConfig{})

		height := s.Height(1)
		assert.Equal(t, 2, height)

		b.WriteString("a")

		height = s.Height(1)
		assert.Equal(t, 3, height)
	})

	t.Run("Draw", func(t *testing.T) {
		w := term.NewStringWriter(5, 5)
		b := cell.CellsToBuffer(nil, 4)
		b.WriteString("a\nb\nc")
		s := Buffer(b, StringResponsiveConfig{})
		s.Resize(4, 4)

		s.Draw(w)
		require.NoError(t, w.Flush())
		assert.Equal(t, "a    \nb    \nc    \n     \n     ", w.String())

		b.WriteString("xyz")

		w = term.NewStringWriter(5, 5)
		s.Draw(w)
		require.NoError(t, w.Flush())
		assert.Equal(t, "a    \nb    \ncxyz \n     \n     ", w.String())
	})
}

func TestNopResponsive(t *testing.T) {
	t.Run("does not panic", func(t *testing.T) {
		w := term.NewStringWriter(5, 5)
		s := NopResponsive()
		s.Resize(4, 4)

		s.Draw(w)
		require.NoError(t, w.Flush())
		assert.Equal(t, "     \n     \n     \n     \n     ", w.String())
	})
}
