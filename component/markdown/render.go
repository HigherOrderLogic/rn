package markdown

import (
	"github.com/ernestrc/go-tui/cell"
	"github.com/ernestrc/go-tui/component"
	"github.com/ernestrc/go-tui/term"
	md "github.com/russross/blackfriday/v2"
)

var (
	// TODO
	codeAttr        = term.Attributes{}
	htmlLiteralAttr = term.Attributes{}
)

type renderState struct {
	attr term.Attributes
}

func (r *renderState) clearAttr() {
	r.attr = term.Attributes{}
}

func (r *renderState) setAttr(t md.NodeType) {
	// TODO r.attr = term.Attributes{}
}

func (r *renderState) enterList() {
	// TODO
}

func (r *renderState) clearList() {
	// TODO
}

// renderNode is the main rendering method. It will be called once for
// every leaf node and twice for every non-leaf node (first with
// entering=true, then with entering=false).
func (r *renderState) renderNode(
	buf *cell.Buffer, node *md.Node, entering bool,
) md.WalkStatus {
	switch node.Type {

	case md.Text, md.HTMLSpan:
		buf.WriteStringWithAttr(string(node.Literal), r.attr)

	case md.Softbreak, md.Hardbreak:
		buf.WriteString("\n")

	case md.Emph, md.Strong, md.Del, md.Heading:
		if entering {
			r.setAttr(node.Type)
		} else {
			r.clearAttr()
		}

	case md.Link:
		if entering {
			r.setAttr(node.Type)
			buf.WriteStringWithAttr(string(node.LinkData.Title), r.attr)
			buf.WriteStringWithAttr("(", r.attr)
			buf.WriteStringWithAttr(string(node.LinkData.Destination), r.attr)
			buf.WriteStringWithAttr(")", r.attr)
		} else {
			r.clearAttr()
		}

	case md.Image, md.Paragraph, md.BlockQuote, md.List:
		/* nop */

	case md.Code, md.HTMLBlock, md.CodeBlock:
		buf.WriteStringWithAttr(string(node.Literal), codeAttr)

	case md.Document:
		break

	case md.HorizontalRule:
		buf.WriteString("\n\n")

	case md.Item:
		if entering {
			r.enterList()
		} else {
			r.clearList()
		}
	case md.Table:
		//if entering {
		//	r.cr(w)
		//	r.out(w, tableTag)
		//} else {
		//	r.out(w, tableCloseTag)
		//	r.cr(w)
		//}
	case md.TableCell:
		//openTag := tdTag
		//closeTag := tdCloseTag
		//if node.IsHeader {
		//	openTag = thTag
		//	closeTag = thCloseTag
		//}
		//if entering {
		//	align := cellAlignment(node.Align)
		//	if align != "" {
		//		attrs = append(attrs, fmt.Sprintf(`align="%s"`, align))
		//	}
		//	if node.Prev == nil {
		//		r.cr(w)
		//	}
		//	r.tag(w, openTag, attrs)
		//} else {
		//	r.out(w, closeTag)
		//	r.cr(w)
		//}
	case md.TableHead:
		//if entering {
		//	r.cr(w)
		//	r.out(w, theadTag)
		//} else {
		//	r.out(w, theadCloseTag)
		//	r.cr(w)
		//}
	case md.TableBody:
		//if entering {
		//	r.cr(w)
		//	r.out(w, tbodyTag)
		//	// XXX: this is to adhere to a rather silly test. Should fix test.
		//	if node.FirstChild == nil {
		//		r.cr(w)
		//	}
		//} else {
		//	r.out(w, tbodyCloseTag)
		//	r.cr(w)
		//}
	case md.TableRow:
		//if entering {
		//	r.cr(w)
		//	r.out(w, trTag)
		//} else {
		//	r.out(w, trCloseTag)
		//	r.cr(w)
		//}
	default:
	}
	return md.GoToNext
}

func renderScroll(ast *md.Node) *component.Scroll {
	scroll := new(component.Scroll)
	buf := cell.NewBuffer()
	scroll.InitWithBuffer(buf)

	var r renderState
	ast.Walk(func(node *md.Node, entering bool) md.WalkStatus {
		return r.renderNode(buf, node, entering)
	})

	return scroll
}
