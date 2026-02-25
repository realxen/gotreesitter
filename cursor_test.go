package gotreesitter

import (
	"testing"
)

func TestTreeCursorNew(t *testing.T) {
	lang := queryTestLanguage()
	tree := buildSimpleTree(lang)
	root := tree.RootNode()

	c := NewTreeCursor(root, tree)
	if c.CurrentNode() != root {
		t.Fatal("CurrentNode should be root")
	}
	if c.Depth() != 0 {
		t.Fatalf("Depth at root should be 0, got %d", c.Depth())
	}
}

func TestTreeCursorGotoFirstChild(t *testing.T) {
	lang := queryTestLanguage()
	tree := buildSimpleTree(lang)

	c := NewTreeCursor(tree.RootNode(), tree)

	if !c.GotoFirstChild() {
		t.Fatal("GotoFirstChild should succeed on program node")
	}
	// First child of program is function_declaration
	if c.CurrentNode().Symbol() != Symbol(5) {
		t.Fatalf("expected function_declaration (5), got %d", c.CurrentNode().Symbol())
	}
	if c.Depth() != 1 {
		t.Fatalf("Depth should be 1, got %d", c.Depth())
	}

	// Descend again: first child of function_declaration is "func" keyword
	if !c.GotoFirstChild() {
		t.Fatal("GotoFirstChild should succeed on function_declaration")
	}
	if c.CurrentNode().Symbol() != Symbol(8) {
		t.Fatalf("expected func keyword (8), got %d", c.CurrentNode().Symbol())
	}
	if c.Depth() != 2 {
		t.Fatalf("Depth should be 2, got %d", c.Depth())
	}
}

func TestTreeCursorGotoNextSibling(t *testing.T) {
	lang := queryTestLanguage()
	tree := buildSimpleTree(lang)

	c := NewTreeCursor(tree.RootNode(), tree)
	c.GotoFirstChild() // function_declaration
	c.GotoFirstChild() // "func" keyword

	// Traverse siblings: func -> identifier -> parameter_list -> block
	expected := []Symbol{1, 13, 14}
	for i, sym := range expected {
		if !c.GotoNextSibling() {
			t.Fatalf("GotoNextSibling should succeed at step %d", i)
		}
		if c.CurrentNode().Symbol() != sym {
			t.Fatalf("step %d: expected symbol %d, got %d", i, sym, c.CurrentNode().Symbol())
		}
	}

	// No more siblings
	if c.GotoNextSibling() {
		t.Fatal("GotoNextSibling should return false at last sibling")
	}
}

func TestTreeCursorGotoPrevSibling(t *testing.T) {
	lang := queryTestLanguage()
	tree := buildSimpleTree(lang)

	c := NewTreeCursor(tree.RootNode(), tree)
	c.GotoFirstChild() // function_declaration
	c.GotoFirstChild() // "func" keyword

	// Move to last sibling
	for c.GotoNextSibling() {
	}
	// Now at block (14)
	if c.CurrentNode().Symbol() != Symbol(14) {
		t.Fatalf("expected block (14), got %d", c.CurrentNode().Symbol())
	}

	// Go back: block -> parameter_list -> identifier -> func
	expected := []Symbol{13, 1, 8}
	for i, sym := range expected {
		if !c.GotoPrevSibling() {
			t.Fatalf("GotoPrevSibling should succeed at step %d", i)
		}
		if c.CurrentNode().Symbol() != sym {
			t.Fatalf("step %d: expected symbol %d, got %d", i, sym, c.CurrentNode().Symbol())
		}
	}

	// No more prev siblings
	if c.GotoPrevSibling() {
		t.Fatal("GotoPrevSibling should return false at first sibling")
	}
}

func TestTreeCursorGotoParent(t *testing.T) {
	lang := queryTestLanguage()
	tree := buildSimpleTree(lang)

	c := NewTreeCursor(tree.RootNode(), tree)
	c.GotoFirstChild() // function_declaration
	c.GotoFirstChild() // "func" keyword

	if !c.GotoParent() {
		t.Fatal("GotoParent should succeed")
	}
	if c.CurrentNode().Symbol() != Symbol(5) {
		t.Fatalf("expected function_declaration (5), got %d", c.CurrentNode().Symbol())
	}
	if c.Depth() != 1 {
		t.Fatalf("Depth should be 1, got %d", c.Depth())
	}

	if !c.GotoParent() {
		t.Fatal("GotoParent should succeed to root")
	}
	if c.CurrentNode().Symbol() != Symbol(7) {
		t.Fatalf("expected program (7), got %d", c.CurrentNode().Symbol())
	}
	if c.Depth() != 0 {
		t.Fatalf("Depth should be 0, got %d", c.Depth())
	}
}

func TestTreeCursorGotoLastChild(t *testing.T) {
	lang := queryTestLanguage()
	tree := buildSimpleTree(lang)

	c := NewTreeCursor(tree.RootNode(), tree)
	c.GotoFirstChild() // function_declaration

	if !c.GotoLastChild() {
		t.Fatal("GotoLastChild should succeed")
	}
	// Last child of function_declaration is block (14)
	if c.CurrentNode().Symbol() != Symbol(14) {
		t.Fatalf("expected block (14), got %d", c.CurrentNode().Symbol())
	}
}

func TestTreeCursorAtLeaf(t *testing.T) {
	lang := queryTestLanguage()
	tree := buildSimpleTree(lang)

	c := NewTreeCursor(tree.RootNode(), tree)
	c.GotoFirstChild() // function_declaration
	c.GotoFirstChild() // "func" keyword (leaf)

	if c.GotoFirstChild() {
		t.Fatal("GotoFirstChild should return false on leaf node")
	}
	if c.GotoLastChild() {
		t.Fatal("GotoLastChild should return false on leaf node")
	}
}

func TestTreeCursorBoundary(t *testing.T) {
	lang := queryTestLanguage()
	tree := buildSimpleTree(lang)

	c := NewTreeCursor(tree.RootNode(), tree)

	// At root: parent should fail
	if c.GotoParent() {
		t.Fatal("GotoParent at root should return false")
	}

	// At root: siblings should fail
	if c.GotoNextSibling() {
		t.Fatal("GotoNextSibling at root should return false")
	}
	if c.GotoPrevSibling() {
		t.Fatal("GotoPrevSibling at root should return false")
	}
}

func TestTreeCursorFromTree(t *testing.T) {
	lang := queryTestLanguage()
	tree := buildSimpleTree(lang)

	c := NewTreeCursorFromTree(tree)
	if c.CurrentNode() != tree.RootNode() {
		t.Fatal("NewTreeCursorFromTree should start at root")
	}
	if c.Depth() != 0 {
		t.Fatalf("Depth should be 0, got %d", c.Depth())
	}
}
