package grammars

import (
	"testing"

	ts "github.com/odvcencio/gotreesitter"
)

func TestPHPMixedGroupedUseRetainsNamespaceUseDeclaration(t *testing.T) {
	src := []byte("<?php\nuse Foo\\Baz\\{\n  Bar as Barr,\n  function foo as fooo,\n  const FOO as FOOO,\n};\n")
	parser := ts.NewParser(PhpLanguage())
	tree, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	root := tree.RootNode()
	if root == nil {
		t.Fatal("missing root node")
	}
	if tree.ParseStopReason() != ts.ParseStopAccepted {
		t.Fatalf("stop=%s runtime=%s", tree.ParseStopReason(), tree.ParseRuntime().Summary())
	}
	if got := root.EndByte(); got != uint32(len(src)) {
		t.Fatalf("root end = %d, want %d; tree=%s", got, len(src), root.SExpr(PhpLanguage()))
	}
	if got := root.ChildCount(); got != 2 {
		t.Fatalf("root child count = %d, want 2; tree=%s", got, root.SExpr(PhpLanguage()))
	}
	if decl := root.Child(1); decl == nil || decl.Type(PhpLanguage()) != "namespace_use_declaration" {
		t.Fatalf("second child = %v, want namespace_use_declaration; tree=%s", decl, root.SExpr(PhpLanguage()))
	} else if !decl.HasError() {
		t.Fatalf("grouped use should retain error flag for trailing comma recovery; tree=%s", root.SExpr(PhpLanguage()))
	}
}

func TestPHPGroupedUseRecoveryPreservesFollowingFunction(t *testing.T) {
	src := []byte("<?php\nnamespace A;\n\nuse Foo\\Baz as Baaz;\n\nuse Foo\\Baz\\{\n  const FOO,\n};\n\nfunction a() {}\n")
	parser := ts.NewParser(PhpLanguage())
	tree, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	root := tree.RootNode()
	if root == nil {
		t.Fatal("missing root node")
	}
	if tree.ParseStopReason() != ts.ParseStopAccepted {
		t.Fatalf("stop=%s runtime=%s", tree.ParseStopReason(), tree.ParseRuntime().Summary())
	}
	if got := root.EndByte(); got != uint32(len(src)) {
		t.Fatalf("root end = %d, want %d; tree=%s", got, len(src), root.SExpr(PhpLanguage()))
	}
	if got := root.ChildCount(); got != 5 {
		t.Fatalf("root child count = %d, want 5; tree=%s", got, root.SExpr(PhpLanguage()))
	}
	if fn := root.Child(4); fn == nil || fn.Type(PhpLanguage()) != "function_definition" {
		t.Fatalf("last child = %v, want function_definition; tree=%s", fn, root.SExpr(PhpLanguage()))
	}
}
