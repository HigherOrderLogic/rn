package component

import (
	"testing"

	"unstable.build/go-tui/term"
	testutil "unstable.build/go-tui/util/test"
)

func TestDrawDivider(t *testing.T) {
	f := Divider(0.5, StringConfig{
		Alignment:    SpanAlignmentCentered,
		FrameCharSet: FrameCharSetDefault(),
	})

	f.Resize(8, 4)

	w := term.NewStringWriter(9, 5)

	tests := []testutil.ComponentTestCase{
		{
			func() { f.Resize(4, 4) }, `
         
         
 ──      
         
         `,
		}, {
			func() { f.Resize(2, 2) }, `
         
─        
         
         
         `,
		}, {
			func() { f.Resize(8, 5) }, `
         
         
  ────   
         
         `,
		},
	}

	testutil.TestComponent(t, f, w, tests)
}
