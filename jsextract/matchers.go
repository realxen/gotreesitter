package jsextract

import (
	gotreesitter "github.com/odvcencio/gotreesitter"
)

// matchCallExpression checks a call_expression node against known patterns and
// returns any extracted endpoints. Returns nil if the node doesn't match.
func matchCallExpression(node *gotreesitter.Node, lang *gotreesitter.Language, src []byte) []Endpoint {
	if node.NamedChildCount() < 2 {
		return nil
	}
	fn := node.NamedChild(0)
	args := node.NamedChild(1)

	fnType := fn.Type(lang)
	argsType := args.Type(lang)
	if argsType != "arguments" {
		return nil
	}

	switch fnType {
	case "identifier":
		return matchSimpleCall(fn, args, lang, src)
	case "import":
		// import("/module") is parsed with an `import` node, not `identifier`
		url := firstStringArg(args, lang, src)
		if url == "" {
			return nil
		}
		return []Endpoint{{URL: url, Type: "import", Source: node.Text(src)}}
	case "member_expression":
		return matchMemberCall(fn, args, lang, src)
	}
	return nil
}

// matchSimpleCall handles: fetch("/url"), import("/url")
func matchSimpleCall(fn, args *gotreesitter.Node, lang *gotreesitter.Language, src []byte) []Endpoint {
	name := fn.Text(src)
	switch name {
	case "fetch", "import":
		url := firstStringArg(args, lang, src)
		if url == "" {
			return nil
		}
		return []Endpoint{{
			URL:    url,
			Type:   name,
			Source: fn.Parent().Text(src),
		}}
	}
	return nil
}

// matchMemberCall handles XHR, jQuery, window.open, location methods.
func matchMemberCall(fn, args *gotreesitter.Node, lang *gotreesitter.Language, src []byte) []Endpoint {
	obj, method := memberParts(fn, lang, src)

	switch {
	// window.open("/url") — must be checked before generic .open()
	case obj == "window" && method == "open":
		url := firstStringArg(args, lang, src)
		if url == "" {
			return nil
		}
		return []Endpoint{{URL: url, Type: "window.open", Source: fn.Parent().Text(src)}}

	// xhr.open("GET", "/url")
	case method == "open":
		url := nthStringArg(args, 1, lang, src) // URL is the second argument
		if url == "" {
			return nil
		}
		return []Endpoint{{URL: url, Type: "xhr", Source: fn.Parent().Text(src)}}

	// $.get("/url"), jQuery.post("/url"), $.ajax("/url"), $.getJSON("/url")
	case (obj == "$" || obj == "jQuery") && isJQueryMethod(method):
		url := firstStringArg(args, lang, src)
		if url == "" {
			return nil
		}
		return []Endpoint{{URL: url, Type: "jquery", Source: fn.Parent().Text(src)}}

	// location.replace("/url"), location.assign("/url")
	case (obj == "location" || obj == "window") && (method == "replace" || method == "assign"):
		url := firstStringArg(args, lang, src)
		if url == "" {
			return nil
		}
		return []Endpoint{{URL: url, Type: "location", Source: fn.Parent().Text(src)}}
	}
	return nil
}

// matchAssignmentExpression handles: location.href = "/url", window.location.pathname = "/url"
func matchAssignmentExpression(node *gotreesitter.Node, lang *gotreesitter.Language, src []byte) []Endpoint {
	if node.NamedChildCount() < 2 {
		return nil
	}
	left := node.NamedChild(0)
	right := node.NamedChild(1)

	if left.Type(lang) != "member_expression" {
		return nil
	}

	obj, prop := memberParts(left, lang, src)
	if !isLocationAssignment(obj, prop) {
		return nil
	}

	url := extractStringValue(right, lang, src)
	if url == "" {
		return nil
	}
	return []Endpoint{{
		URL:    url,
		Type:   "location",
		Source: node.Text(src),
	}}
}

func isLocationAssignment(obj, prop string) bool {
	return (obj == "location" || obj == "window") &&
		(prop == "href" || prop == "pathname" || prop == "search")
}

func isJQueryMethod(method string) bool {
	switch method {
	case "get", "post", "ajax", "getJSON":
		return true
	}
	return false
}

// memberParts extracts the object and property names from a member_expression.
func memberParts(node *gotreesitter.Node, lang *gotreesitter.Language, src []byte) (obj, prop string) {
	for i := 0; i < node.NamedChildCount(); i++ {
		child := node.NamedChild(i)
		switch child.Type(lang) {
		case "identifier":
			obj = child.Text(src)
		case "property_identifier":
			prop = child.Text(src)
		}
	}
	return
}

// firstStringArg extracts the first string argument from an arguments node.
func firstStringArg(args *gotreesitter.Node, lang *gotreesitter.Language, src []byte) string {
	return nthStringArg(args, 0, lang, src)
}

// nthStringArg extracts the nth named child's string value from an arguments node.
func nthStringArg(args *gotreesitter.Node, n int, lang *gotreesitter.Language, src []byte) string {
	if args.NamedChildCount() <= n {
		return ""
	}
	child := args.NamedChild(n)
	return extractStringValue(child, lang, src)
}

// extractStringValue extracts a plain string value from a string node.
// Returns "" for non-string nodes.
func extractStringValue(node *gotreesitter.Node, lang *gotreesitter.Language, src []byte) string {
	nodeType := node.Type(lang)
	switch nodeType {
	case "string_fragment":
		return node.Text(src)
	case "string":
		for i := 0; i < node.NamedChildCount(); i++ {
			child := node.NamedChild(i)
			if child.Type(lang) == "string_fragment" {
				return child.Text(src)
			}
		}
	case "binary_expression", "template_string":
		// Delegate to resolveExpr which handles dynamic parts.
		return resolveExpr(node, lang, src)
	}
	return ""
}

// resolveExpr walks a binary_expression or template_string, building a
// resolved URL string. String fragments are kept as-is; dynamic parts
// (identifiers, call expressions, template substitutions) become "EXPR".
// This matches jsluice's behaviour: fetch("/api/" + id) → "/api/EXPR".
func resolveExpr(node *gotreesitter.Node, lang *gotreesitter.Language, src []byte) string {
	nodeType := node.Type(lang)
	switch nodeType {
	case "string_fragment":
		return node.Text(src)
	case "string":
		for i := 0; i < node.NamedChildCount(); i++ {
			child := node.NamedChild(i)
			if child.Type(lang) == "string_fragment" {
				return child.Text(src)
			}
		}
		return ""
	case "binary_expression":
		if node.NamedChildCount() < 2 {
			return ""
		}
		left := resolveExpr(node.NamedChild(0), lang, src)
		right := resolveExpr(node.NamedChild(1), lang, src)
		return left + right
	case "template_string":
		return resolveTemplateString(node, lang, src)
	case "template_substitution":
		return "EXPR"
	default:
		// Any non-string expression (identifier, call_expression, etc.)
		return "EXPR"
	}
}

// resolveTemplateString resolves a template_string node by extracting the
// literal text between substitutions and replacing each ${...} with "EXPR".
// Template literal text parts aren't separate child nodes — they're embedded
// in the raw source between the backtick and substitution byte ranges.
func resolveTemplateString(node *gotreesitter.Node, lang *gotreesitter.Language, src []byte) string {
	start := node.StartByte() + 1 // skip opening backtick
	end := node.EndByte() - 1     // skip closing backtick

	if node.NamedChildCount() == 0 {
		// Plain template string with no substitutions
		if start < end && int(end) <= len(src) {
			return string(src[start:end])
		}
		return ""
	}

	var result string
	cursor := start
	for i := 0; i < node.NamedChildCount(); i++ {
		child := node.NamedChild(i)
		if child.Type(lang) != "template_substitution" {
			continue
		}
		// Text between cursor and this substitution
		if child.StartByte() > cursor {
			result += string(src[cursor:child.StartByte()])
		}
		result += "EXPR"
		cursor = child.EndByte()
	}
	// Trailing text after last substitution
	if cursor < end && int(end) <= len(src) {
		result += string(src[cursor:end])
	}
	return result
}
