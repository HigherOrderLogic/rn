// Unstable Build LLC ("COMPANY") CONFIDENTIAL
//
// Unpublished Copyright (c) 2017-2024 Unstable Build, All Rights Reserved.
//
// NOTICE: All information contained herein is, and remains the property of COMPANY.
// The intellectual and technical concepts contained herein are proprietary to
// COMPANY and may be covered by U.S. and Foreign Patents, patents in process,
// and are protected by trade secret or copyright law. Dissemination of this information
// or reproduction of this material is strictly forbidden unless prior written permission
// is obtained from COMPANY. Access to the source code contained herein is hereby
// forbidden to anyone except current COMPANY employees, managers or contractors who
// have executed Confidentiality and Non-disclosure agreements explicitly covering such access.
//
// The copyright notice above does not evidence any actual or intended publication or
// disclosure of this source code, which includes information that is confidential and/or
// proprietary, and is a trade secret, of COMPANY. ANY REPRODUCTION, MODIFICATION,
// DISTRIBUTION, PUBLIC  PERFORMANCE, OR PUBLIC DISPLAY OF OR THROUGH USE OF THIS SOURCE CODE
// WITHOUT  THE EXPRESS WRITTEN CONSENT OF COMPANY IS STRICTLY PROHIBITED, AND IN
// VIOLATION OF APPLICABLE LAWS AND INTERNATIONAL TREATIES. THE RECEIPT OR POSSESSION OF
// THIS SOURCE CODE AND/OR RELATED INFORMATION DOES NOT CONVEY OR IMPLY ANY RIGHTS TO
// REPRODUCE, DISCLOSE OR DISTRIBUTE ITS CONTENTS, OR TO MANUFACTURE, USE, OR SELL
// ANYTHING THAT IT MAY DESCRIBE, IN WHOLE OR IN PART.

package component

import (
	"fmt"
	"unsafe"

	"github.com/unstablebuild/rune-go-sdk/component"
	"github.com/unstablebuild/rune-go-sdk/term"
	"github.com/unstablebuild/rune-go-sdk/tui"
)

type splitDir uint8

const (
	noSplit splitDir = iota
	vertical
	horizontal
)

// TileTree represents the root node of a tree of TileNodes.
type TileTree struct {
	root TileNode
}

// TileNode represents a node in a tree of tiled components.
type TileNode struct {
	tree          *TileTree
	width, height int
	content       tui.Component
	children      []*component.Virtual[*TileNode]
	childSplit    splitDir
	parent        *TileNode
	fixedSize     int
	dirty         bool
}

// Init initializes a TileTree or resets it if already initialied.
func (t *TileTree) Init(content tui.Component) (n *TileNode) {
	n = new(TileNode)
	t.root.childSplit = vertical
	t.root.children = []*component.Virtual[*TileNode]{{C: n}}
	t.root.tree = t
	n.initNode(content, &t.root)
	return
}

// NewTileTree allocates storage for a new TileTree and initializes it.
// It also returns the TileNode allocated to store the given content.
func NewTileTree(content tui.Component) (t *TileTree, n *TileNode) {
	t = new(TileTree)
	n = t.Init(content)
	return
}

// Resize resizes the contents of this TileTree.
func (t *TileTree) Resize(width, height int) {
	t.root.Resize(width, height)
}

// Draw draws the contents of this TileTree.
func (t *TileTree) Draw(w term.Writer) {
	t.root.Draw(w)
}

// DrawTile draws only node on w.
func (t *TileTree) DrawTile(node *TileNode, w term.Writer) {
	t.root.drawTile(node, w)
}

func (t *TileNode) initNode(content tui.Component, parent *TileNode) {
	t.content = content
	t.children = []*component.Virtual[*TileNode]{}
	t.parent = parent
	t.tree = parent.tree
}

// Resize satisfies tui.Component.
func (t *TileNode) Resize(width, height int) {
	t.height = height
	t.width = width
	t.dirty = false

	if len(t.children) == 0 && t.content == nil {
		panic("corrupted node: non-empty children and content")
	}

	if t.content != nil {
		t.content.Resize(width, height)
		return
	}

	if t.childSplit == vertical {
		t.resizeVertical(width, height)
		return

	}

	t.resizeHorizontal(width, height)
}

// Draw satisfies tui.Component.
func (t *TileNode) Draw(w term.Writer) {
	if len(t.children) == 0 && t.content == nil {
		panic("corrupted node: non-empty children and content")
	}

	if t.dirty {
		t.Resize(t.width, t.height)
	}

	if t.content != nil {
		t.content.Draw(w)
		return
	}

	for _, ti := range t.children {
		ti.Draw(w)
	}
}

func (t *TileNode) drawTile(node *TileNode, w term.Writer) bool {
	if t.dirty {
		t.Resize(t.width, t.height)
	}

	if t == node {
		t.content.Draw(w)
		return true
	}

	for _, ti := range t.children {
		// call drawTile and use Virtual's position, width, height
		// to emulate Virtual.Draw via VirtualWriter
		vwriter := component.VirtualWriter{
			Writer: w, Offset: ti.Position(),
			Height: ti.Height(), Width: ti.Width(),
		}
		if ok := ti.C.drawTile(node, &vwriter); ok {
			return true
		}
	}
	return false
}

func (t *TileNode) childIdx(child *TileNode) int {
	for i, c := range t.children {
		if c.C == child {
			return i
		}
	}

	panic("tile is not a child of parent")
}

func (t *TileNode) addChildAtIdx(
	child *TileNode, content tui.Component, direction splitDir, idx int,
) {
	if idx > len(t.children) {
		panic(fmt.Errorf("trying to append child at index out of bounds: %d; len=%d",
			idx, len(t.children)))
	}

	v := &component.Virtual[*TileNode]{C: child}

	// transfer content to child at index 0 but do it in a way such that it
	// maintains mapping of content to TileNode.
	// This is because instances of TileNode are leaked outside of the
	// tree through various APIs.
	if len(t.children) == 0 {
		proxyNode := new(TileNode)
		proxyNode.initNode(nil, t.parent)
		proxyNode.childSplit = direction
		proxyNode.fixedSize = t.fixedSize
		proxyNode.children = append(proxyNode.children, &component.Virtual[*TileNode]{C: t}, v)

		idx := t.parent.childIdx(t)
		t.parent.children[idx] = &component.Virtual[*TileNode]{C: proxyNode}

		child.initNode(content, proxyNode)
		t.initNode(t.content, proxyNode)
		t.fixedSize = 0

		proxyNode.parent.Resize(proxyNode.parent.width, proxyNode.parent.height)
		return
	}

	child.initNode(content, t)

	t.children = append(t.children, nil)
	copy(t.children[idx+1:], t.children[idx:])
	t.children[idx] = v

	t.Resize(t.width, t.height)
}

func split(over *TileNode, direction splitDir, content tui.Component) (
	n *TileNode,
) {
	if content == nil {
		panic("empty content for tile")
	}
	if over == nil {
		panic("trying to split over a nil tile")
	}

	n = new(TileNode)

	parent := over.parent

	if parent.childSplit == direction {
		i := parent.childIdx(over)
		parent.addChildAtIdx(n, content, direction, i+1)
		return
	}

	over.addChildAtIdx(n, content, direction, 0)
	return
}

func removeChild(parent, child *TileNode) {
	i := parent.childIdx(child)
	copy(parent.children[i:], parent.children[i+1:])
	parent.children = parent.children[:len(parent.children)-1]

	child.parent = nil
	child.tree = nil
	child.children = nil

	// transfer last child to content but do it in a way such that it
	// maintains mapping of contents to TileNode.
	if len(parent.children) == 1 && parent.parent != nil {
		proxyNode := parent
		lastNode := parent.children[0]

		idx := proxyNode.parent.childIdx(proxyNode)
		proxyNode.parent.children[idx] = lastNode

		lastNode.C.parent = proxyNode.parent
		lastNode.C.fixedSize = 0

		proxyNode.parent.Resize(proxyNode.parent.width, proxyNode.parent.height)

		proxyNode.tree = nil
		proxyNode.parent = nil
		proxyNode.children = nil
		return
	}

	// be extra safe: when a child is removed from a parent, reset all fixed sizes
	parent.resetFixedSizes()
	parent.Resize(parent.width, parent.height)
}

// Close removes this node from the tree. It panics if node is last node on the tree.
func (t *TileNode) Close() {
	if t.parent == nil {
		return
	}
	if t.parent == &t.tree.root && len(t.parent.children) == 1 {
		panic("unsupported: trying to close last node")
	}

	parent := t.parent
	t.parent = nil

	if len(parent.children) != 0 {
		removeChild(parent, t)
		return
	}

	parent.Close()
}

// SplitVertical splits the given node to incorporate new content. If direction of the
// given node's split is vertical, a new node with content will be added as a sibling of node.
// Otherwise, a new node with content will become a child of the given node so
// width will be divided in half so new node can be drawn next to it.
// It panics if node is a child of this TileTree.
func (t *TileTree) SplitVertical(
	node *TileNode, content tui.Component,
) *TileNode {
	return split(node, vertical, content)
}

// SplitHorizontal splits the given node to incorporate new content. If direction of the
// given node's split is horizontal, a new node with content will be added as a sibling of node.
// Otherwise, a new node will become a child of the given node so
// height will be divided in half so new node can be drawn next to it.
// It panics if node is a child of this TileTree.
func (t *TileTree) SplitHorizontal(
	node *TileNode, content tui.Component,
) *TileNode {
	return split(node, horizontal, content)
}

// leftMostChild will return the left-most tile in the node
// if node's split is horizontal, or the top-most tile if the node's split is vertical
func (t *TileNode) leftMostChild() *TileNode {
	node := t.children[0].C
	if len(node.children) == 0 {
		return node
	}

	return node.leftMostChild()
}

// rightMostChild will return the right-most tile in the node
// if node's split is horizontal, or the bottom-most tile if the node's split is vertical
func (t *TileNode) rightMostChild() *TileNode {
	node := t.children[len(t.children)-1].C
	if len(node.children) == 0 {
		return node
	}

	return node.rightMostChild()
}

func getParentIdx(node *TileNode) (parent *TileNode, i int) {
	parent = node.parent
	if parent == nil {
		return nil, 0
	}
	i = parent.childIdx(node)
	return
}

func tileLeftDir(node *TileNode, direction splitDir) *TileNode {
	parent, i := getParentIdx(node)
	if parent == nil {
		return nil
	}
	if i == 0 || parent.childSplit != direction {
		return tileLeftDir(parent, direction)
	}

	link := parent.children[i-1].C
	if len(link.children) == 0 {
		return link
	}

	return link.rightMostChild()
}

func tileRightDir(node *TileNode, direction splitDir) *TileNode {
	parent, i := getParentIdx(node)
	if parent == nil {
		return nil
	}
	if i == len(parent.children)-1 || parent.childSplit != direction {
		return tileRightDir(parent, direction)
	}

	link := parent.children[i+1].C
	if len(link.children) == 0 {
		return link
	}

	return link.leftMostChild()
}

// TileLeft returns the tile left-adjacent to t or nil if t is the
// left-most tile in the tree.
func (t *TileNode) TileLeft() *TileNode {
	return tileLeftDir(t, vertical)
}

// TileRight returns the tile right-adjacent to t or nil if t is the
// right-most tile in the tree.
func (t *TileNode) TileRight() *TileNode {
	return tileRightDir(t, vertical)
}

// TileUp returns the tile on top of t or nil if t is the
// top-most tile in the tree.
func (t *TileNode) TileUp() *TileNode {
	return tileLeftDir(t, horizontal)
}

// TileDown returns the tile in the bottom of t or nil if t is the
// bottom-most tile in the tree.
func (t *TileNode) TileDown() *TileNode {
	return tileRightDir(t, horizontal)
}

// Size returns the total number of nodes under this TileNode.
func (t *TileNode) Size() (size int) {
	if len(t.children) == 0 {
		return 1
	}

	for _, c := range t.children {
		size += c.C.Size()
	}

	return
}

// Size returns the total number of nodes in this tree.
func (t *TileTree) Size() (size int) {
	return t.root.Size()
}

// Content returns the Component held by this TileNode in the TileTree.
func (t *TileNode) Content() tui.Component {
	if t.content == nil {
		panic("corrupted node: leaked proxy node outside of tree")
	}
	return t.content
}

func (t *TileNode) tileAt(tileOffset, pos term.Coordinates) *TileNode {
	if t.childSplit == noSplit {
		return t
	}

	switch t.childSplit {
	case vertical:
		for _, child := range t.children {
			childPos := child.Position()
			childPos.X += tileOffset.X
			childPos.Y += tileOffset.Y
			if pos.X >= childPos.X && pos.X < childPos.X+child.Width() {
				return child.C.tileAt(childPos, pos)
			}
		}
	case horizontal:
		for _, child := range t.children {
			childPos := child.Position()
			childPos.X += tileOffset.X
			childPos.Y += tileOffset.Y
			if pos.Y >= childPos.Y && pos.Y < childPos.Y+child.Height() {
				return child.C.tileAt(childPos, pos)
			}
		}
	}

	panic(fmt.Sprintf("could not find tile at %+v", pos))
}

func (t *TileNode) tilePosition(child *TileNode, currOffset term.Coordinates) (
	offset term.Coordinates, ok bool,
) {
	if t == child {
		panic("missed child on parent loop")
	}

	for _, c := range t.children {
		offset = c.Position()
		offset.X += currOffset.X
		offset.Y += currOffset.Y

		ok = (c.C == child)
		if ok {
			return
		}

		offset, ok = c.C.tilePosition(child, offset)
		if ok {
			return
		}
	}
	return
}

// TilePosition returns the given tile's position offset inside this TileTree.
// It panics if tile is not a member of this tree.
func (t *TileTree) TilePosition(tile *TileNode) term.Coordinates {
	offset, ok := t.root.tilePosition(tile, term.Coordinates{})
	if !ok {
		panic("tile does not belong to this tree")
	}
	return offset
}

// TileAt returns the tile at pos term.Coordinates.
func (t *TileTree) TileAt(pos term.Coordinates) *TileNode {
	if pos.X < 0 || pos.Y < 0 || pos.X >= t.root.width || pos.Y >= t.root.height {
		panic("Coordinates out of bounds")
	}
	return t.root.tileAt(term.Coordinates{}, pos)
}

// SetContentResize sets the content of a TileNode to c.
func (t *TileNode) SetContentResize(c tui.Component, resize bool) (prev tui.Component) {
	prev = t.content
	t.content = c
	if resize {
		t.content.Resize(t.width, t.height)
	}
	return
}

// SetFixedHeight sets the height of this node, or returns
// false if this node's height cannot be fixed.
//
// Calling this method with height=0 effectively resets the
// height to be automatically calculated based on the space available.
func (t *TileNode) SetFixedHeight(height int) bool {
	if t.parent == nil {
		panic("corrupted tile tree: exposed root node")
	}
	if height != 0 && (t.height == height || t.fixedSize == height) {
		return true
	}
	if t.parent.childSplit == horizontal {
		if !t.parent.canSetFixedSize(t.parent.height, height) {
			return false
		}
		t.parent.resetFixedSizes()
		t.fixedSize = height
		t.parent.dirty = true
		t.dirty = true
		return true
	}
	if t.parent.parent == nil {
		return false
	}
	return t.parent.SetFixedHeight(height)
}

// SetFixedWidth sets the width of this node, or returns
// false if this node's width cannot be fixed.
//
// Calling this method with width=0 effectively resets the
// width to be automatically calculated based on the space available.
func (t *TileNode) SetFixedWidth(width int) bool {
	if t.parent == nil {
		panic("corrupted tile tree: exposed root node")
	}
	if width != 0 && (t.width == width || t.fixedSize == width) {
		return true
	}
	if t.parent.childSplit == vertical {
		if !t.parent.canSetFixedSize(t.parent.width, width) {
			return false
		}
		t.parent.resetFixedSizes()
		t.fixedSize = width
		t.parent.dirty = true
		t.dirty = true
		return true
	}
	if t.parent.parent == nil {
		return false
	}
	return t.parent.SetFixedWidth(width)
}

// ID returns a unique identifier for this tile.
func (t *TileNode) ID() uint64 {
	return uint64(uintptr(unsafe.Pointer(t)))
}

// Height returns the height of this node.
func (t *TileNode) Height() int {
	return t.height
}

// Width returns the height of this node.
func (t *TileNode) Width() int {
	return t.width
}

// MaxWidth returns the max fixed width that this window can be set, based on the
// available space and siblings.
func (t *TileNode) MaxWidth() int {
	if t.parent == nil {
		panic("corrupted tile tree: exposed root node")
	}
	if t.parent.childSplit == vertical {
		return t.parent.width - (3 * (len(t.parent.children) - 1))
	}
	if t.parent.parent == nil {
		return t.parent.width
	}
	return t.parent.MaxWidth()
}

// MaxHeight returns the max fixed height that this window can be set, based on the
// available space and siblings.
func (t *TileNode) MaxHeight() int {
	if t.parent == nil {
		panic("corrupted tile tree: exposed root node")
	}
	if t.parent.childSplit == horizontal {
		return t.parent.height - (3 * (len(t.parent.children) - 1))
	}
	if t.parent.parent == nil {
		return t.parent.height
	}
	return t.parent.MaxHeight()
}

// Position returns the position of this tile node inside its TileTree.
func (t *TileNode) Position() term.Coordinates {
	return t.tree.TilePosition(t)
}

func (t *TileNode) iterate(op func(*TileNode)) {
	if t.content != nil {
		op(t)
		return
	}

	for _, ti := range t.children {
		ti.C.iterate(op)
	}
}

// Iterate applies op to the content of all nodes of this tree.
func (t *TileTree) Iterate(op func(*TileNode)) {
	t.root.iterate(op)
}

// Closed returns if this Window has been closed.
func (t *TileNode) Closed() bool {
	return t.parent == nil
}

func (t *TileNode) fixedSizeNodes() (totalFixedSize, fixedSizeNodes int) {
	for _, ti := range t.children {
		fixedSize := ti.C.fixedSize
		if fixedSize != 0 {
			totalFixedSize += fixedSize
			fixedSizeNodes++
		}
	}
	return
}

func (t *TileNode) resizeHorizontal(width, height int) {
	length := len(t.children)
	fixedHeight, fixedSizeNodes := t.fixedSizeNodes()
	cheight := (height - fixedHeight) / (length - fixedSizeNodes)
	hspare := height - fixedHeight - cheight*(length-fixedSizeNodes)

	useSpareIdx := length - hspare
	spareCell := 0

	var offset int
	for i, ti := range t.children {
		fixedSize := ti.C.fixedSize
		ti.Move(term.Coordinates{Y: offset})
		if fixedSize != 0 {
			ti.Resize(width, fixedSize)
			offset += fixedSize
		} else {
			if i == useSpareIdx {
				spareCell = 1
			}
			nheight := cheight + spareCell
			ti.Resize(width, nheight)
			offset += nheight
		}
	}
}

func (t *TileNode) resizeVertical(width, height int) {
	length := len(t.children)
	fixedWidth, fixedSizeNodes := t.fixedSizeNodes()
	cwidth := (width - fixedWidth) / (length - fixedSizeNodes)
	wspare := width - fixedWidth - cwidth*(length-fixedSizeNodes)

	useSpareIdx := length - wspare
	spareCell := 0

	var offset int
	for i, ti := range t.children {
		fixedSize := ti.C.fixedSize
		ti.Move(term.Coordinates{X: offset})
		if fixedSize != 0 {
			ti.Resize(fixedSize, height)
			offset += fixedSize
		} else {
			if i == useSpareIdx {
				spareCell = 1
			}
			nwidth := cwidth + spareCell
			ti.Resize(nwidth, height)
			offset += nwidth
		}
	}
}

func (t *TileNode) canSetFixedSize(avail, fixedSize int) bool {
	if len(t.children) <= 1 {
		return false
	}
	effective := (avail - fixedSize) / (len(t.children) - 1)
	return effective >= 3 // do not let a fixed compress to much th rest of tiles
}

func (t *TileNode) resetFixedSizes() {
	for _, c := range t.children {
		c.C.fixedSize = 0
	}
}
