package gotreesitter_test

import (
	"testing"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func parseByLanguageName(t *testing.T, name, src string) (*gotreesitter.Tree, *gotreesitter.Language) {
	t.Helper()

	var entry grammars.LangEntry
	found := false
	for _, e := range grammars.AllLanguages() {
		if e.Name == name {
			entry = e
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("missing language entry %q", name)
	}

	var backend grammars.ParseBackend
	found = false
	for _, report := range grammars.AuditParseSupport() {
		if report.Name == name {
			backend = report.Backend
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("missing parse support report %q", name)
	}

	lang := entry.Language()
	parser := gotreesitter.NewParser(lang)
	srcBytes := []byte(src)

	var (
		tree *gotreesitter.Tree
		err  error
	)
	switch backend {
	case grammars.ParseBackendTokenSource:
		if entry.TokenSourceFactory == nil {
			t.Fatalf("%s: token source backend without factory", name)
		}
		tree, err = parser.ParseWithTokenSource(srcBytes, entry.TokenSourceFactory(srcBytes, lang))
	case grammars.ParseBackendDFA, grammars.ParseBackendDFAPartial:
		tree, err = parser.Parse(srcBytes)
	default:
		t.Fatalf("%s: unsupported backend %q", name, backend)
	}
	if err != nil {
		t.Fatalf("%s parse failed: %v", name, err)
	}
	if tree == nil || tree.RootNode() == nil {
		t.Fatalf("%s parse returned nil tree/root", name)
	}
	return tree, lang
}

func TestPugTopLevelTagCarriesTrailingNewlineSpan(t *testing.T) {
	const src = "p hello\n"
	tree, lang := parseByLanguageName(t, "pug", src)
	root := tree.RootNode()
	if root.HasError() {
		t.Fatalf("unexpected pug parse error: %s", root.SExpr(lang))
	}
	if root.ChildCount() != 1 {
		t.Fatalf("pug root childCount=%d, want 1", root.ChildCount())
	}
	tag := root.Child(0)
	if tag == nil || tag.Type(lang) != "tag" {
		t.Fatalf("pug child=%v, want tag", tag)
	}
	if got, want := tag.EndByte(), root.EndByte(); got != want {
		t.Fatalf("pug tag.EndByte=%d, want root.EndByte=%d", got, want)
	}
}

func TestCaddyTopLevelServerCarriesTrailingNewlineSpan(t *testing.T) {
	const src = ":8080 {\n}\n"
	tree, lang := parseByLanguageName(t, "caddy", src)
	root := tree.RootNode()
	if root.HasError() {
		t.Fatalf("unexpected caddy parse error: %s", root.SExpr(lang))
	}
	if root.ChildCount() != 1 {
		t.Fatalf("caddy root childCount=%d, want 1", root.ChildCount())
	}
	server := root.Child(0)
	if server == nil || server.Type(lang) != "server" {
		t.Fatalf("caddy child=%v, want server", server)
	}
	if got, want := server.EndByte(), root.EndByte(); got != want {
		t.Fatalf("caddy server.EndByte=%d, want root.EndByte=%d", got, want)
	}
}

func TestCooklangStepCarriesTerminalPunctuationAndRootNewline(t *testing.T) {
	const src = "Add @salt{1%tsp}.\n"
	tree, lang := parseByLanguageName(t, "cooklang", src)
	root := tree.RootNode()
	if root.HasError() {
		t.Fatalf("unexpected cooklang parse error: %s", root.SExpr(lang))
	}
	if root.ChildCount() != 1 {
		t.Fatalf("cooklang root childCount=%d, want 1", root.ChildCount())
	}
	step := root.Child(0)
	if step == nil || step.Type(lang) != "step" {
		t.Fatalf("cooklang child=%v, want step", step)
	}
	if got, want := step.EndByte(), uint32(len(src)-1); got != want {
		t.Fatalf("cooklang step.EndByte=%d, want %d", got, want)
	}
	if got, want := root.EndByte(), uint32(len(src)); got != want {
		t.Fatalf("cooklang root.EndByte=%d, want %d", got, want)
	}
}

func TestFortranProgramCarriesLineBreaks(t *testing.T) {
	const src = "program hello\n  implicit none\nend program hello\n"
	tree, lang := parseByLanguageName(t, "fortran", src)
	root := tree.RootNode()
	if root.HasError() {
		t.Fatalf("unexpected fortran parse error: %s", root.SExpr(lang))
	}
	if root.ChildCount() != 1 {
		t.Fatalf("fortran root childCount=%d, want 1", root.ChildCount())
	}
	program := root.Child(0)
	if program == nil || program.Type(lang) != "program" {
		t.Fatalf("fortran child=%v, want program", program)
	}
	if got, want := program.EndByte(), root.EndByte(); got != want {
		t.Fatalf("fortran program.EndByte=%d, want root.EndByte=%d", got, want)
	}
	stmt := program.Child(0)
	if stmt == nil || stmt.Type(lang) != "program_statement" {
		t.Fatalf("fortran first child=%v, want program_statement", stmt)
	}
	if got, want := stmt.EndByte(), uint32(14); got != want {
		t.Fatalf("fortran program_statement.EndByte=%d, want %d", got, want)
	}
}
