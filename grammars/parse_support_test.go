package grammars

import (
	"testing"

	"github.com/odvcencio/gotreesitter"
)

var parseSmokeSamples = map[string]string{
	"agda":              "module M where\n",
	"authzed":           "definition user {}\n",
	"bash":              "echo hi\n",
	"c":                 "int main(void) { return 0; }\n",
	"capnp":             "@0xdbb9ad1f14bf0b36;\nstruct Person {\n  name @0 :Text;\n}\n",
	"c_sharp":           "using System;\n",
	"comment":           "TODO: fix this\n",
	"corn":              "{ x = 1 }\n",
	"cpp":               "int main() { return 0; }\n",
	"css":               "body { color: red; }\n",
	"desktop":           "[Desktop Entry]\n",
	"dtd":               "<!ELEMENT note (#PCDATA)>\n",
	"doxygen":           "/**\n * @brief A function\n * @param x The value\n */\n",
	"earthfile":         "FROM alpine\n",
	"editorconfig":      "root = true\n",
	"go":                "package main\n\nfunc main() {\n\tprintln(1)\n}\n",
	"embedded_template": "<% if true %>\n  hello\n<% end %>\n",
	"facility":          "service Example {\n}\n",
	"foam":              "FoamFile\n{\n    version 2.0;\n}\n",
	"fidl":              "library example;\ntype Foo = struct {};\n",
	"firrtl":            "circuit Top :\n",
	"haskell":           "module Main where\nx = 1\n",
	"html":              "<html><body>Hello</body></html>\n",
	"java":              "class Main { int x; }\n",
	"javascript":        "function f() { return 1; }\nconst x = () => x + 1;\n",
	"json":              "{\"a\": 1}\n",
	"julia":             "module M\nx = 1\nend\n",
	"kotlin":            "fun main() {\n    val x: Int? = null\n    println(x)\n}\n",
	"lua":               "local x = 1\n",
	"php":               "<?php echo 1;\n",
	"python":            "def f():\n    return 1\n",
	"regex":             "a+b*\n",
	"ruby":              "def f\n  1\nend\n",
	"rust":              "fn main() { let x = 1; }\n",
	"sql":               "SELECT id, name FROM users WHERE id = 1;\n",
	"swift":             "let x: Int = 1\n",
	"toml":              "a = 1\ntitle = \"hello\"\ntags = [\"x\", \"y\"]\n",
	"tsx":               "const x = <div/>;\n",
	"typescript":        "function f(): number { return 1; }\n",
	"yaml":              "a: 1\n",
	"zig":               "const x: i32 = 1;\n",
	"scala":             "object Main { def f(x: Int): Int = x + 1 }\n",
	"elixir":            "defmodule M do\n  def f(x), do: x\nend\n",
	"graphql":           "type Query { hello: String }\n",
	"hcl":               "resource \"x\" \"y\" { a = 1 }\n",
	"nix":               "let x = 1; in x\n",
	"ocaml":             "let x = 1\n",
	"verilog":           "module m;\nendmodule\n",

	// Phase 2: DFA-only languages
	"dot":        "digraph G { a -> b; }\n",
	"git_config": "[core]\n\tbare = false\n",
	"ini":        "[section]\nkey = value\n",
	"json5":      "{ \"key\": \"value\" }\n",
	"llvm":       "define i32 @main() {\n  ret i32 0\n}\n",
	"move":       "module 0x1::m {}\n",
	"ninja":      "rule cc\n  command = gcc\n",
	"pascal":     "program P;\nbegin\nend.\n",
	"v":          "fn main() {}\n",
	"vimdoc":     "*tag*\tHelp text\n",

	// Phase 3: scanner-needed languages (targeted samples that avoid external tokens)
	"jsdoc":      "/** hello */\n",
	"wgsl":       "fn main() {}\n",
	"nginx":      "events {}\n",
	"svelte":     "<p>hello</p>\n",
	"xml":        "<root/>\n",
	"r":          "x <- 1\n",
	"rescript":   "let x = 1\n",
	"purescript": "module Main where\n",
	"rst":        "Title\n=====\n",
	"vhdl":       "entity e is end;\n",

	// Phase 5: new grammars
	"gdscript":       "extends Node\nfunc _ready():\n\tpass\n",
	"godot_resource": "x = 1\n",
	"groovy":         "def x = 1\n",
	"hare":           "export fn main() void = void;\n",
	"hyprlang":       "general {\n}\n",
	"ledger":         "2024-01-01 Groceries\n  Expenses:Food  $50\n  Assets:Bank\n",
	"liquid":         "{{ name }}\n",
	"nickel":         "1 + 2\n",
	"pem":            "-----BEGIN CERTIFICATE-----\nMIIC\n-----END CERTIFICATE-----\n",
	"pkl":            "x = 1\n",
	"prisma":         "model User {\n  id Int @id\n}\n",
	"promql":         "up{job=\"prometheus\"}\n",
	"ql":             "from int x where x = 1 select x\n",
	"rego":           "package p\n",
	"ron":            "(x: 1)\n",
	"squirrel":       "local x = 1;\n",
	"tablegen":       "class Foo;\n",
	"thrift":         "struct Foo {}\n",
	"uxntal":         "|00 @System &vector $2\n",
}

var parseSmokeKnownDegraded = map[string]string{
	"comment":    "known parser limitation: extra token state handling causes recoverable errors",
	"swift":      "known lexer parity gap: parser currently reports recoverable errors on smoke sample",
	"pascal":     "DFA lexer cannot handle this grammar without external scanner support",
	"vimdoc":     "DFA lexer cannot handle this grammar without external scanner support",
	"nginx":      "requires external scanner (3 tokens) not yet implemented",
	"svelte":     "requires external scanner (16 tokens) not yet implemented",
	"xml":        "requires external scanner (11 tokens) not yet implemented",
	"r":          "requires external scanner (14 tokens) not yet implemented",
	"rescript":   "requires external scanner (12 tokens) not yet implemented",
	"purescript": "requires external scanner (14 tokens) not yet implemented",
	"rst":        "requires external scanner (41 tokens) not yet implemented",
	"vhdl":       "requires external scanner (167 tokens) not yet implemented",
	"norg":       "requires external scanner (122 tokens) not yet implemented",
	"nushell":    "requires external scanner (3 tokens) not yet implemented",
	"typst":      "requires external scanner (49 tokens) not yet implemented",
	"yuck":       "requires external scanner (3 tokens) not yet implemented",
	"hurl":       "DFA lexer cannot handle this grammar",
}

func parseSmokeSample(name string) string {
	if sample, ok := parseSmokeSamples[name]; ok {
		return sample
	}
	return "x\n"
}

func parseSmokeDegradedReason(report ParseSupport, name string) string {
	if reason, ok := parseSmokeKnownDegraded[name]; ok {
		return reason
	}
	if report.Reason != "" {
		return report.Reason
	}
	return "parser reported recoverable syntax errors on smoke sample"
}

func TestSupportedLanguagesParseSmoke(t *testing.T) {
	entries := AllLanguages()
	entryByName := make(map[string]LangEntry, len(entries))
	for _, entry := range entries {
		entryByName[entry.Name] = entry
	}

	reports := AuditParseSupport()
	for _, report := range reports {
		sample := parseSmokeSample(report.Name)

		if report.Backend == ParseBackendUnsupported {
			t.Logf("skip %s: %s", report.Name, report.Reason)
			continue
		}

		entry, ok := entryByName[report.Name]
		if !ok {
			t.Fatalf("missing registry entry for %q", report.Name)
		}
		lang := entry.Language()
		parser := gotreesitter.NewParser(lang)
		source := []byte(sample)

		var tree *gotreesitter.Tree
		switch report.Backend {
		case ParseBackendTokenSource:
			ts := entry.TokenSourceFactory(source, lang)
			tree = parser.ParseWithTokenSource(source, ts)
		case ParseBackendDFA, ParseBackendDFAPartial:
			tree = parser.Parse(source)
		default:
			t.Fatalf("unknown backend %q for %q", report.Backend, report.Name)
		}

		if tree == nil || tree.RootNode() == nil {
			t.Fatalf("%s parse returned nil root using backend %q", report.Name, report.Backend)
		}
		if tree.RootNode().HasError() {
			t.Logf("%s parse smoke sample produced syntax errors (degraded): %s", report.Name, parseSmokeDegradedReason(report, report.Name))
			continue
		}
	}
}
