// Package jsextract extracts URLs and API endpoints from JavaScript source code
// using gotreesitter's pure-Go tree-sitter runtime.
package jsextract

import (
	"sync"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// Endpoint represents an extracted URL/API endpoint.
type Endpoint struct {
	URL    string // the extracted URL or path
	Type   string // "fetch", "xhr", "jquery", "location", "window.open", "string", "import"
	Source string // source context (e.g. "fetch('/api/users')")
}

// JSLuiceEndpoint matches katana's expected contract exactly.
type JSLuiceEndpoint struct {
	Endpoint string
	Type     string
}

var (
	jsLang     *gotreesitter.Language
	jsLangOnce sync.Once
)

func getJSLang() *gotreesitter.Language {
	jsLangOnce.Do(func() {
		jsLang = grammars.JavascriptLanguage()
	})
	return jsLang
}

// ExtractEndpoints parses JavaScript source and returns all detected endpoints.
func ExtractEndpoints(source []byte) ([]Endpoint, error) {
	lang := getJSLang()
	parser := gotreesitter.NewParser(lang)
	return extractWithParser(parser, lang, source)
}

// Extractor holds a reusable parser for repeated extraction calls.
// Use this in long-lived applications to avoid per-call parser allocation.
type Extractor struct {
	lang   *gotreesitter.Language
	parser *gotreesitter.Parser
}

// NewExtractor creates a reusable extractor. The grammar is loaded once;
// subsequent Extract calls reuse the parser.
func NewExtractor() *Extractor {
	lang := getJSLang()
	return &Extractor{lang: lang, parser: gotreesitter.NewParser(lang)}
}

// Extract parses JavaScript source and returns all detected endpoints.
func (e *Extractor) Extract(source []byte) ([]Endpoint, error) {
	return extractWithParser(e.parser, e.lang, source)
}

func extractWithParser(parser *gotreesitter.Parser, lang *gotreesitter.Language, source []byte) ([]Endpoint, error) {
	tree, err := parser.Parse(source)
	if err != nil {
		return nil, err
	}
	defer tree.Release()

	seen := make(map[string]struct{})
	var results []Endpoint

	// Phase 1: Walk the tree for structured patterns (fetch, XHR, jQuery, etc.)
	structuredURLs := make(map[string]struct{})
	walkStructured(tree.RootNode(), lang, source, &results, seen, structuredURLs)

	// Phase 2: Walk for URL-like string literals not already captured
	walkStrings(tree.RootNode(), lang, source, &results, seen, structuredURLs)

	return results, nil
}

// walkStructured does a DFS looking for call_expression and assignment_expression nodes.
func walkStructured(node *gotreesitter.Node, lang *gotreesitter.Language, src []byte, results *[]Endpoint, seen, structuredURLs map[string]struct{}) {
	nodeType := node.Type(lang)

	switch nodeType {
	case "call_expression":
		if eps := matchCallExpression(node, lang, src); len(eps) > 0 {
			for _, ep := range eps {
				if _, dup := seen[ep.URL]; !dup {
					seen[ep.URL] = struct{}{}
					structuredURLs[ep.URL] = struct{}{}
					*results = append(*results, ep)
				}
			}
		}
	case "assignment_expression":
		if eps := matchAssignmentExpression(node, lang, src); len(eps) > 0 {
			for _, ep := range eps {
				if _, dup := seen[ep.URL]; !dup {
					seen[ep.URL] = struct{}{}
					structuredURLs[ep.URL] = struct{}{}
					*results = append(*results, ep)
				}
			}
		}
	}

	for i := 0; i < node.NamedChildCount(); i++ {
		walkStructured(node.NamedChild(i), lang, src, results, seen, structuredURLs)
	}
}

// walkStrings does a DFS looking for string-like nodes with URL-like content.
// It handles plain string_fragment, template_string (with EXPR substitutions),
// and binary_expression (string concatenation with EXPR).
func walkStrings(node *gotreesitter.Node, lang *gotreesitter.Language, src []byte, results *[]Endpoint, seen, structuredURLs map[string]struct{}) {
	nodeType := node.Type(lang)
	switch nodeType {
	case "string_fragment":
		tryAddStringURL(node.Text(src), results, seen, structuredURLs)
	case "template_string":
		// Resolve template literals: `/path/${x}/more` → "/path/EXPR/more"
		// Always skip recursion into children — they are fragments of the
		// resolved URL and should not be extracted separately.
		if node.NamedChildCount() > 0 {
			resolved := resolveExpr(node, lang, src)
			tryAddStringURL(resolved, results, seen, structuredURLs)
			return
		}
	case "binary_expression":
		// Resolve string concatenation: "/path/" + x → "/path/EXPR"
		// Skip recursion — children are fragments.
		resolved := resolveExpr(node, lang, src)
		tryAddStringURL(resolved, results, seen, structuredURLs)
		return
	}

	for i := 0; i < node.NamedChildCount(); i++ {
		walkStrings(node.NamedChild(i), lang, src, results, seen, structuredURLs)
	}
}

func tryAddStringURL(text string, results *[]Endpoint, seen, structuredURLs map[string]struct{}) bool {
	if _, already := structuredURLs[text]; already {
		return false
	}
	if _, dup := seen[text]; dup || !MaybeURL(text) {
		return false
	}
	seen[text] = struct{}{}
	*results = append(*results, Endpoint{
		URL:  text,
		Type: "string",
	})
	return true
}

// ExtractJsluiceEndpoints is a drop-in replacement for katana's jsluice integration.
func ExtractJsluiceEndpoints(data string) []JSLuiceEndpoint {
	eps, err := ExtractEndpoints([]byte(data))
	if err != nil {
		return nil
	}
	out := make([]JSLuiceEndpoint, len(eps))
	for i, ep := range eps {
		out[i] = JSLuiceEndpoint{
			Endpoint: ep.URL,
			Type:     ep.Type,
		}
	}
	return out
}
