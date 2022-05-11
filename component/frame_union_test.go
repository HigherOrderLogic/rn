package component

import (
	"testing"

	"github.com/ernestrc/go-tui/term"
	testutil "github.com/ernestrc/go-tui/util/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDrawFrameUnionNoFrame(t *testing.T) {
	one := &TestComponent{Ch: 'X'}
	two := &TestComponent{Ch: 'B'}
	three := &TestComponent{Ch: 'b'}
	four := &TestComponent{Ch: 'x'}
	five := &TestComponent{Ch: '\''}
	main := &TestComponent{Ch: 'A'}
	f := NewFrameUnion(main)
	f.Frame = false
	f.Resize(20, 16)
	f.UnionTop(one, 1)

	w := term.NewStringWriter(20, 20)

	tests := []testutil.ComponentTestCase{
		{
			nil, `
XXXXXXXXXXXXXXXXXXXX
AAAAAAAAAAAAAAAAAAAA
AAAAAAAAAAAAAAAAAAAA
AAAAAAAAAAAAAAAAAAAA
AAAAAAAAAAAAAAAAAAAA
AAAAAAAAAAAAAAAAAAAA
AAAAAAAAAAAAAAAAAAAA
AAAAAAAAAAAAAAAAAAAA
AAAAAAAAAAAAAAAAAAAA
AAAAAAAAAAAAAAAAAAAA
AAAAAAAAAAAAAAAAAAAA
AAAAAAAAAAAAAAAAAAAA
AAAAAAAAAAAAAAAAAAAA
AAAAAAAAAAAAAAAAAAAA
AAAAAAAAAAAAAAAAAAAA
AAAAAAAAAAAAAAAAAAAA
                    
                    
                    
                    `,
		}, {
			func() {
				f.Resize(2, 3)
			}, `
XX                  
AA                  
AA                  
                    
                    
                    
                    
                    
                    
                    
                    
                    
                    
                    
                    
                    
                    
                    
                    
                    `,
		}, {
			func() {
				f.Resize(3, 3)
			}, `
XXX                 
AAA                 
AAA                 
                    
                    
                    
                    
                    
                    
                    
                    
                    
                    
                    
                    
                    
                    
                    
                    
                    `,
		}, {
			func() {
				f.Resize(2, 2)
			}, `
XX                  
AA                  
                    
                    
                    
                    
                    
                    
                    
                    
                    
                    
                    
                    
                    
                    
                    
                    
                    
                    `,
		}, {
			func() {
				f.UnionBottom(two, 1)
				f.Resize(2, 2)
			}, `
AA                  
AA                  
                    
                    
                    
                    
                    
                    
                    
                    
                    
                    
                    
                    
                    
                    
                    
                    
                    
                    `,
		}, {
			func() {
				f.Resize(20, 16)
			}, `
XXXXXXXXXXXXXXXXXXXX
AAAAAAAAAAAAAAAAAAAA
AAAAAAAAAAAAAAAAAAAA
AAAAAAAAAAAAAAAAAAAA
AAAAAAAAAAAAAAAAAAAA
AAAAAAAAAAAAAAAAAAAA
AAAAAAAAAAAAAAAAAAAA
AAAAAAAAAAAAAAAAAAAA
AAAAAAAAAAAAAAAAAAAA
AAAAAAAAAAAAAAAAAAAA
AAAAAAAAAAAAAAAAAAAA
AAAAAAAAAAAAAAAAAAAA
AAAAAAAAAAAAAAAAAAAA
AAAAAAAAAAAAAAAAAAAA
AAAAAAAAAAAAAAAAAAAA
BBBBBBBBBBBBBBBBBBBB
                    
                    
                    
                    `,
		}, {
			func() {
				f.UnionBottom(three, 1)
				f.UnionTop(four, 1)
				f.UnionBottom(five, 1)
				f.Resize(20, 20)
			}, `
XXXXXXXXXXXXXXXXXXXX
xxxxxxxxxxxxxxxxxxxx
AAAAAAAAAAAAAAAAAAAA
AAAAAAAAAAAAAAAAAAAA
AAAAAAAAAAAAAAAAAAAA
AAAAAAAAAAAAAAAAAAAA
AAAAAAAAAAAAAAAAAAAA
AAAAAAAAAAAAAAAAAAAA
AAAAAAAAAAAAAAAAAAAA
AAAAAAAAAAAAAAAAAAAA
AAAAAAAAAAAAAAAAAAAA
AAAAAAAAAAAAAAAAAAAA
AAAAAAAAAAAAAAAAAAAA
AAAAAAAAAAAAAAAAAAAA
AAAAAAAAAAAAAAAAAAAA
AAAAAAAAAAAAAAAAAAAA
AAAAAAAAAAAAAAAAAAAA
''''''''''''''''''''
bbbbbbbbbbbbbbbbbbbb
BBBBBBBBBBBBBBBBBBBB`,
		},
	}

	testutil.TestComponent(t, f, w, tests)
}

func TestDrawFrameUnionWithFrame(t *testing.T) {
	one := NewFrame(&TestComponent{Ch: 'X'})
	two := NewFrame(&TestComponent{Ch: 'B'})
	three := NewFrame(&TestComponent{Ch: 'b'})
	four := NewFrame(&TestComponent{Ch: 'x'})
	five := NewFrame(&TestComponent{Ch: '\''})
	six := NewFrame(&TestComponent{Ch: '6'})
	seven := NewFrame(&TestComponent{Ch: '7'})
	eight := NewFrame(&TestComponent{Ch: '8'})
	nine := NewFrame(&TestComponent{Ch: '9'})
	main := NewFrame(&TestComponent{Ch: 'A'})
	f := NewFrameUnion(main)
	f.Resize(20, 16)
	f.UnionTop(one, 3)

	w := term.NewStringWriter(20, 20)

	tests := []testutil.ComponentTestCase{
		{
			nil, `
┌──────────────────┐
│XXXXXXXXXXXXXXXXXX│
├──────────────────┤
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
└──────────────────┘
                    
                    
                    
                    `,
		}, {
			func() {
				f.Left = '┊'
				f.Right = '┊'
			}, `
┌──────────────────┐
│XXXXXXXXXXXXXXXXXX│
┊──────────────────┊
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
└──────────────────┘
                    
                    
                    
                    `,
		}, {
			func() {
				f.Resize(2, 3)
			}, `
AA                  
AA                  
AA                  
                    
                    
                    
                    
                    
                    
                    
                    
                    
                    
                    
                    
                    
                    
                    
                    
                    `,
		}, {
			func() {
				f.Resize(3, 3)
			}, `
┌─┐                 
│A│                 
└─┘                 
                    
                    
                    
                    
                    
                    
                    
                    
                    
                    
                    
                    
                    
                    
                    
                    
                    `,
		}, {
			func() {
				f.Resize(2, 2)
			}, `
AA                  
AA                  
                    
                    
                    
                    
                    
                    
                    
                    
                    
                    
                    
                    
                    
                    
                    
                    
                    
                    `,
		}, {
			func() {
				f.UnionBottom(two, 3)
				f.Resize(2, 2)
			}, `
AA                  
AA                  
                    
                    
                    
                    
                    
                    
                    
                    
                    
                    
                    
                    
                    
                    
                    
                    
                    
                    `,
		}, {
			func() {
				f.Resize(20, 16)
			}, `
┌──────────────────┐
│XXXXXXXXXXXXXXXXXX│
┊──────────────────┊
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
┊──────────────────┊
│BBBBBBBBBBBBBBBBBB│
└──────────────────┘
                    
                    
                    
                    `,
		}, {
			func() {
				f.UnionBottom(three, 3)
				f.UnionTop(four, 3)
				f.UnionBottom(five, 3)
				f.UnionLeft(six, 3)
				f.UnionLeft(seven, 3)
				f.UnionRight(eight, 3)
				f.UnionRight(nine, 3)
				f.Resize(20, 20)
			}, `
┌──────────────────┐
│XXXXXXXXXXXXXXXXXX│
┊──────────────────┊
│xxxxxxxxxxxxxxxxxx│
┊─┬─┬──────────┬─┬─┊
│6│7│AAAAAAAAAA│9│8│
│6│7│AAAAAAAAAA│9│8│
│6│7│AAAAAAAAAA│9│8│
│6│7│AAAAAAAAAA│9│8│
│6│7│AAAAAAAAAA│9│8│
│6│7│AAAAAAAAAA│9│8│
│6│7│AAAAAAAAAA│9│8│
│6│7│AAAAAAAAAA│9│8│
┊─┴─┴──────────┴─┴─┊
│''''''''''''''''''│
┊──────────────────┊
│bbbbbbbbbbbbbbbbbb│
┊──────────────────┊
│BBBBBBBBBBBBBBBBBB│
└──────────────────┘`,
		},
	}

	testutil.TestComponent(t, f, w, tests)

	t.Run("ComponentAt returns the component at position offset", func(t *testing.T) {
		c, ok := f.ComponentAt(term.Coordinates{})
		require.True(t, ok)
		assert.Equal(t, one, c.C)
		assert.Equal(t, term.Coordinates{}, c.Position())

		c, ok = f.ComponentAt(term.Coordinates{X: 19})
		require.True(t, ok)
		assert.Equal(t, one, c.C)
		assert.Equal(t, term.Coordinates{}, c.Position())

		c, ok = f.ComponentAt(term.Coordinates{Y: 2, X: 19})
		require.True(t, ok)
		assert.Equal(t, one, c.C)
		assert.Equal(t, term.Coordinates{}, c.Position())

		c, ok = f.ComponentAt(term.Coordinates{Y: 3, X: 19})
		require.True(t, ok)
		assert.Equal(t, four, c.C)
		assert.Equal(t, term.Coordinates{Y: 2}, c.Position())

		c, ok = f.ComponentAt(term.Coordinates{Y: 5, X: 19})
		require.True(t, ok)
		assert.Equal(t, eight, c.C)
		assert.Equal(t, term.Coordinates{Y: 4, X: 17}, c.Position())

		c, ok = f.ComponentAt(term.Coordinates{Y: 5, X: 17})
		require.True(t, ok)
		assert.Equal(t, nine, c.C)

		c, ok = f.ComponentAt(term.Coordinates{Y: 5, X: 13})
		require.True(t, ok)
		assert.Equal(t, main, c.C)

		c, ok = f.ComponentAt(term.Coordinates{Y: 12, X: 3})
		require.True(t, ok)
		assert.Equal(t, seven, c.C)

		c, ok = f.ComponentAt(term.Coordinates{Y: 15, X: 3})
		require.True(t, ok)
		assert.Equal(t, five, c.C)

		c, ok = f.ComponentAt(term.Coordinates{Y: 19, X: 3})
		require.True(t, ok)
		assert.Equal(t, two, c.C)
		assert.Equal(t, term.Coordinates{Y: 17}, c.Position())

		c, ok = f.ComponentAt(term.Coordinates{Y: 16, X: 3})
		require.True(t, ok)
		assert.Equal(t, three, c.C)
		assert.Equal(t, term.Coordinates{Y: 15}, c.Position())

		pos := f.MainPosition()
		assert.Equal(t, term.Coordinates{X: 4, Y: 4}, pos)
	})
}

func TestDrawFrameUnionWithFrameBottomOnly(t *testing.T) {
	one := NewFrame(&TestComponent{Ch: 'X'})
	main := NewFrame(&TestComponent{Ch: 'A'})
	f := NewFrameUnion(main)
	f.UnionBottom(one, 3)
	f.Resize(20, 16)

	w := term.NewStringWriter(20, 20)

	tests := []testutil.ComponentTestCase{
		{
			nil, `
┌──────────────────┐
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
├──────────────────┤
│XXXXXXXXXXXXXXXXXX│
└──────────────────┘
                    
                    
                    
                    `,
		},
	}

	testutil.TestComponent(t, f, w, tests)
}

func TestComponentAtOutOfBounds(t *testing.T) {
	f := NewFrameUnion(&TestComponent{})
	f.Resize(10, 10)

	_, ok := f.ComponentAt(term.Coordinates{X: 10})
	require.False(t, ok)
	_, ok = f.ComponentAt(term.Coordinates{Y: 10})
	require.False(t, ok)
}
