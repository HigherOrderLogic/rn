package markdown

import (
	"github.com/ernestrc/go-tui/component"
	md "github.com/russross/blackfriday/v2"
)

// Parse parses the markdown input and returns a component.Scroll
// which renders the parsed input.
func Parse(input []byte) *component.Scroll {
	parser := md.New()
	ast := parser.Parse(input)
	return renderScroll(ast)
}
