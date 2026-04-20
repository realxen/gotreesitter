package gotreesitter_test

import (
	"testing"

	ts "github.com/odvcencio/gotreesitter"
	gr "github.com/odvcencio/gotreesitter/grammars"
)

func TestScalaTrailingCommentAttachesToIndentedFunctionBody(t *testing.T) {
	tests := []struct {
		name        string
		src         string
		commentType string
		bodyCount   int
	}{
		{
			name: "line_comment",
			src: `object Outer {
  private def search =
    foo

  // env
  def source = bar
}
`,
			commentType: "comment",
			bodyCount:   2,
		},
		{
			name: "block_comment",
			src: `object Outer {
  private def search =
    foo

  /** env */
  def source = bar
}
`,
			commentType: "block_comment",
			bodyCount:   2,
		},
		{
			name: "line_comment_run",
			src: `object Outer {
  private def search =
    foo

  // env one
  // env two
  // env three
  def source = bar
}
`,
			commentType: "comment",
			bodyCount:   4,
		},
	}

	lang := gr.ScalaLanguage()
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := ts.NewParser(lang)
			tree, err := p.Parse([]byte(tc.src))
			if err != nil {
				t.Fatalf("parse failed: %v", err)
			}
			root := tree.RootNode()
			if root == nil {
				t.Fatal("nil root")
			}
			if root.HasError() {
				t.Fatalf("unexpected parse error tree: %s", root.SExpr(lang))
			}

			obj := root.Child(0)
			if obj == nil || obj.Type(lang) != "object_definition" {
				t.Fatalf("root child = %v, want object_definition", obj)
			}
			template := obj.Child(2)
			if template == nil || template.Type(lang) != "template_body" {
				t.Fatalf("template body = %v, want template_body", template)
			}
			if got, want := template.ChildCount(), 4; got != want {
				t.Fatalf("template child count = %d, want %d: %s", got, want, root.SExpr(lang))
			}

			firstFn := template.Child(1)
			secondFn := template.Child(2)
			if firstFn == nil || firstFn.Type(lang) != "function_definition" {
				t.Fatalf("first child = %v, want function_definition", firstFn)
			}
			if secondFn == nil || secondFn.Type(lang) != "function_definition" {
				t.Fatalf("second child = %v, want function_definition", secondFn)
			}

			body := firstFn.Child(firstFn.ChildCount() - 1)
			if body == nil || body.Type(lang) != "indented_block" {
				t.Fatalf("body = %v, want indented_block", body)
			}
			if got, want := int(body.ChildCount()), tc.bodyCount; got != want {
				t.Fatalf("indented body child count = %d, want %d: %s", got, want, root.SExpr(lang))
			}
			comment := body.Child(1)
			if comment == nil || comment.Type(lang) != tc.commentType {
				t.Fatalf("body comment = %v, want %s", comment, tc.commentType)
			}
			for i := 0; i < int(firstFn.ChildCount())-1; i++ {
				if got := firstFn.Child(i).Type(lang); got == "comment" || got == "block_comment" {
					t.Fatalf("unexpected direct comment child %q at slot %d: %s", got, i, root.SExpr(lang))
				}
			}
			if got, want := firstFn.EndByte(), secondFn.StartByte(); got != want {
				t.Fatalf("first function end = %d, want %d", got, want)
			}
		})
	}
}

func TestScalaFunctionModifiersDoNotCarryReturnTypeField(t *testing.T) {
	tests := []struct {
		name               string
		src                string
		wantReturnTypeSlot int
	}{
		{
			name: "no_return_type",
			src: `object Outer {
  private def search =
    foo
}
`,
			wantReturnTypeSlot: -1,
		},
		{
			name: "explicit_return_type",
			src: `object Outer {
  private def search: Int =
    foo
}
`,
			wantReturnTypeSlot: 4,
		},
	}

	lang := gr.ScalaLanguage()
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := ts.NewParser(lang)
			tree, err := p.Parse([]byte(tc.src))
			if err != nil {
				t.Fatalf("parse failed: %v", err)
			}
			root := tree.RootNode()
			if root == nil || root.HasError() {
				t.Fatalf("unexpected parse error tree: %s", root.SExpr(lang))
			}
			fn := root.Child(0).Child(2).Child(1)
			if fn == nil || fn.Type(lang) != "function_definition" {
				t.Fatalf("function node = %v, want function_definition", fn)
			}
			if got := fn.FieldNameForChild(0, lang); got != "" {
				t.Fatalf("modifiers field = %q, want empty", got)
			}
			if tc.wantReturnTypeSlot >= 0 {
				if got := fn.FieldNameForChild(tc.wantReturnTypeSlot, lang); got != "return_type" {
					t.Fatalf("return type field at slot %d = %q, want return_type", tc.wantReturnTypeSlot, got)
				}
			}
		})
	}
}

func TestScalaInterpolatedStringCarriesTrailingMultilineClose(t *testing.T) {
	src := `object Outer {
  override def toString = s"""
    |a = ${x}
    |}""".asLines
}
`

	lang := gr.ScalaLanguage()
	p := ts.NewParser(lang)
	tree, err := p.Parse([]byte(src))
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	root := tree.RootNode()
	if root == nil || root.HasError() {
		t.Fatalf("unexpected parse error tree: %s", root.SExpr(lang))
	}

	fn := root.Child(0).Child(2).Child(1)
	fieldExpr := fn.Child(4)
	if fieldExpr == nil || fieldExpr.Type(lang) != "field_expression" {
		t.Fatalf("field expr = %v, want field_expression", fieldExpr)
	}
	stringExpr := fieldExpr.Child(0)
	dot := fieldExpr.Child(1)
	if stringExpr == nil || stringExpr.Type(lang) != "interpolated_string_expression" {
		t.Fatalf("string expr = %v, want interpolated_string_expression", stringExpr)
	}
	if dot == nil || dot.Type(lang) != "." {
		t.Fatalf("dot child = %v, want .", dot)
	}
	if got, want := stringExpr.EndByte(), dot.StartByte(); got != want {
		t.Fatalf("string expr end = %d, want %d", got, want)
	}
	inner := stringExpr.Child(1)
	if inner == nil || inner.Type(lang) != "interpolated_string" {
		t.Fatalf("inner string = %v, want interpolated_string", inner)
	}
	if got, want := inner.EndByte(), dot.StartByte(); got != want {
		t.Fatalf("inner string end = %d, want %d", got, want)
	}
}

func TestScalaInterpolatedStringCarriesTrailingSingleLineTail(t *testing.T) {
	src := `object Outer {
  def x = {
    Console print f"Classpath built from ${settings.toConciseString} %n"
  }
}
`

	lang := gr.ScalaLanguage()
	p := ts.NewParser(lang)
	tree, err := p.Parse([]byte(src))
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	root := tree.RootNode()
	if root == nil || root.HasError() {
		t.Fatalf("unexpected parse error tree: %s", root.SExpr(lang))
	}

	fn := root.Child(0).Child(2).Child(1)
	block := fn.Child(fn.ChildCount() - 1)
	if block == nil || block.Type(lang) != "block" {
		t.Fatalf("block = %v, want block", block)
	}
	infix := block.Child(1)
	if infix == nil || infix.Type(lang) != "infix_expression" {
		t.Fatalf("infix = %v, want infix_expression", infix)
	}
	stringExpr := infix.Child(2)
	if stringExpr == nil || stringExpr.Type(lang) != "interpolated_string_expression" {
		t.Fatalf("string expr = %v, want interpolated_string_expression", stringExpr)
	}
	if got, want := stringExpr.EndByte(), infix.EndByte(); got != want {
		t.Fatalf("string expr end = %d, want %d", got, want)
	}
	inner := stringExpr.Child(1)
	if inner == nil || inner.Type(lang) != "interpolated_string" {
		t.Fatalf("inner string = %v, want interpolated_string", inner)
	}
	if got, want := inner.EndByte(), infix.EndByte(); got != want {
		t.Fatalf("inner string end = %d, want %d", got, want)
	}
}

func TestScalaObjectTemplateBodyRecoveredFromRootFragments(t *testing.T) {
	src := `object PathResolver {
  // Imports property/environment functions which suppress security exceptions.
  import AccessControl._
  import java.security.{AccessControlException, AccessController, PrivilegedAction, PrivilegedExceptionAction}
}
`

	tree, lang := parseByLanguageName(t, "scala", src)
	root := tree.RootNode()
	if root == nil || root.HasError() {
		t.Fatalf("unexpected scala parse error: %s", root.SExpr(lang))
	}
	if got := root.Type(lang); got != "compilation_unit" {
		t.Fatalf("root type = %q, want compilation_unit", got)
	}
	obj := root.Child(0)
	if obj == nil || obj.Type(lang) != "object_definition" {
		t.Fatalf("root child = %v, want object_definition", obj)
	}
	template := obj.Child(2)
	if template == nil || template.Type(lang) != "template_body" {
		t.Fatalf("template body = %v, want template_body: %s", template, root.SExpr(lang))
	}
	if got := template.Child(0).Type(lang); got != "{" {
		t.Fatalf("template child[0] = %q, want {", got)
	}
	if got := template.Child(template.ChildCount() - 1).Type(lang); got != "}" {
		t.Fatalf("template last child = %q, want }", got)
	}
	if found := firstNode(template, func(n *ts.Node) bool { return n.Type(lang) == "import_declaration" }); found == nil {
		t.Fatalf("template body missing import_declaration: %s", template.SExpr(lang))
	}
}

func TestScalaTraitTemplateBodyOwnsTrailingCommentSibling(t *testing.T) {
	src := `trait Fruit:
//    ^definition.interface
  val color: Color
//    ^definition.variable

object Fruit:
  val color = Color.Yellow
`

	tree, lang := parseByLanguageName(t, "scala", src)
	root := tree.RootNode()
	if root == nil || root.HasError() {
		t.Fatalf("unexpected scala parse error: %s", root.SExpr(lang))
	}
	traitNode := root.Child(0)
	if traitNode == nil || traitNode.Type(lang) != "trait_definition" {
		t.Fatalf("trait node = %v, want trait_definition", traitNode)
	}
	next := root.Child(1)
	if next == nil || next.Type(lang) != "object_definition" {
		t.Fatalf("next node = %v, want object_definition", next)
	}
	template := traitNode.Child(2)
	if template == nil || template.Type(lang) != "template_body" {
		t.Fatalf("template body = %v, want template_body", template)
	}
	last := template.Child(template.ChildCount() - 1)
	if last == nil || last.Type(lang) != "comment" {
		t.Fatalf("template last child = %v, want trailing comment", last)
	}
	if got, want := traitNode.EndByte(), next.StartByte(); got != want {
		t.Fatalf("trait end = %d, want %d", got, want)
	}
}

func TestScalaLargeBlockCommentDoesNotTruncateParse(t *testing.T) {
	// Regression test: Scala files with large block comments (e.g. license
	// headers >=7 lines) used to exhaust the default iteration budget
	// (sourceLen*30) because block_comment is a parser-level production
	// parsed character-by-character. The language-specific iteration scaling
	// (3x for Scala) prevents premature truncation.
	src := `/*
 * Copyright 2011-2026 GatlingCorp (https://gatling.io)
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *  http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package io.gatling.core.action

import com.typesafe.scalalogging.StrictLogging

/**
 * Top level abstraction in charge of executing concrete actions.
 */
trait Action extends StrictLogging {
  def name: String
  override def toString: String = name

  def !(session: Session): Unit = {
    val eventLoop = session.eventLoop
    if (eventLoop.inEventLoop) {
      execute(session)
    }
  }

  /**
   * Core method executed when the Action received a Session message
   *
   * @param session
   *   the session of the virtual user
   * @return
   *   Nothing
   */
  protected def execute(session: Session): Unit
}

/**
 * An Action that is to be chained with another.
 */
trait ChainableAction extends Action {
  def next: Action
}
`
	lang := gr.ScalaLanguage()
	p := ts.NewParser(lang)
	tree, err := p.Parse([]byte(src))
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	root := tree.RootNode()
	if root == nil {
		t.Fatal("nil root")
	}
	rt := tree.ParseRuntime()
	if rt.StopReason != "accepted" {
		t.Fatalf("parse truncated: stopReason=%s iterations=%d/%d endByte=%d/%d",
			rt.StopReason, rt.Iterations, rt.IterationLimit, root.EndByte(), len(src))
	}
	if got, want := root.EndByte(), uint32(len(src)); got != want {
		t.Fatalf("root endByte = %d, want %d (parse did not consume full input)", got, want)
	}

	// Verify block comments are properly structured (no leaked repeat1 nodes)
	var repeat1Count int
	var walk func(n *ts.Node)
	walk = func(n *ts.Node) {
		if n == nil {
			return
		}
		if n.Type(lang) == "block_comment_repeat1" {
			repeat1Count++
			return
		}
		for i := 0; i < n.ChildCount(); i++ {
			walk(n.Child(i))
		}
	}
	walk(root)
	if repeat1Count > 0 {
		t.Fatalf("found %d block_comment_repeat1 nodes leaked to root tree", repeat1Count)
	}

	// Verify key structural nodes exist
	traitAction := firstNode(root, func(n *ts.Node) bool {
		return n.Type(lang) == "trait" || n.Type(lang) == "trait_definition"
	})
	if traitAction == nil {
		t.Fatalf("missing trait definition in parsed output: %s", root.SExpr(lang))
	}
}
