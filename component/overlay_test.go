package component

import (
	"testing"

	"github.com/ernestrc/go-tui/term"
	testutil "github.com/ernestrc/go-tui/util/test"
)

func TestDrawOverlay(t *testing.T) {
	cfg := SpanConfig{
		PadVertical:      -5,
		PadHorizontal:    -10,
		ContentAlignment: SpanAlignmentCentered,
	}
	background := &TestComponent{Ch: '*'}
	cover := NewFrame(StringCentered("a"))

	o := NewOverlay(background, cover, cfg)

	w := term.NewStringWriter(16, 9)

	tests := []testutil.ComponentTestCase{
		{
			func() { o.Resize(16, 9) }, `
****************
****************
***┌────────┐***
***│        │***
***│   a    │***
***│        │***
***└────────┘***
****************
****************`,
		}, {
			func() { o.Resize(4, 4) }, `
┌──┐            
│a │            
│  │            
└──┘            
                
                
                
                
                `,
		}, {
			func() { o.Resize(2, 2) }, `
a               
                
                
                
                
                
                
                
                `,
		},
	}

	testutil.TestComponent(t, o, w, tests)
}
