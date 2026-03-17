package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammargen"
	"github.com/odvcencio/gotreesitter/grammars"
)

func must[T any](v T, err error) T { if err != nil { panic(err) }; return v }

func dumpNode(n *gotreesitter.Node, lang *gotreesitter.Language, src []byte, depth int) {
	if n == nil { return }
	fmt.Printf("%s%q [%d:%d] text=%q children=%d\n", strings.Repeat("  ", depth), n.Type(lang), n.StartByte(), n.EndByte(), strings.TrimSpace(string(src[n.StartByte():n.EndByte()])), n.ChildCount())
	for i := 0; i < n.ChildCount(); i++ { dumpNode(n.Child(i), lang, src, depth+1) }
}

func inspect(label string, lang *gotreesitter.Language, src []byte) {
	p := must(gotreesitter.NewParser(lang).Parse(src))
	fmt.Println("===", label)
	for i := 0; i < p.RootNode().ChildCount(); i++ { dumpNode(p.RootNode().Child(i), lang, src, 0) }
}

func main() {
	src := []byte("package p\n\nfunc f(r rune) { _ = !IsLetter(r) }\n")
	fmt.Println("builtin")
	inspect("builtin", grammars.GoLanguage(), src)

	g := must(grammargen.ImportGrammarJSON(must(os.ReadFile("/tmp/grammar_parity/go/src/grammar.json"))))
	lang := must(grammargen.GenerateLanguage(g))
	fmt.Println("generated")
	inspect("generated", lang, src)
}
