package bench

import (
	"fmt"
	"strings"
)

// Small JS (~430B) — a handful of typical patterns.
var jsSmall = []byte(`
fetch("/api/users");
fetch("https://api.example.com/v2/data");
var xhr = new XMLHttpRequest();
xhr.open("GET", "/api/items?page=1");
$.post("/api/submit", {name: "test"});
jQuery.ajax("/api/resource");
location.href = "/dashboard";
window.open("/popup", "_blank");
location.replace("/new-page");
var config = { endpoint: "https://config.example.com/v1/settings" };
var ws = "//ws.example.com/socket";
import("/modules/lazy.js");
`)

// Medium JS (~2KB) — multiple functions with mixed patterns.
var jsMedium = generateMedium()

// Large JS — generated as N independent ~2KB chunks concatenated.
// Each chunk is parseable, simulating a bundled production file.
// We test how both tools scale with input size.
var jsLarge = generateLarge()

func generateMedium() []byte {
	var b strings.Builder

	// API functions (6 × ~50B = ~300B)
	for i := range 6 {
		b.WriteString(fmt.Sprintf("function api%d(id) { return fetch(\"/api/v1/resource%d/\" + id); }\n", i, i))
	}

	// XHR functions (3 × ~100B = ~300B)
	for i := range 3 {
		b.WriteString(fmt.Sprintf("function load%d() { var x = new XMLHttpRequest(); x.open(\"GET\", \"/data/coll%d\"); x.send(); }\n", i, i))
	}

	// jQuery (3 × ~50B = ~150B)
	for i := range 3 {
		b.WriteString(fmt.Sprintf("function submit%d(d) { $.post(\"/api/form%d\", d); }\n", i, i))
	}

	// Config objects with URL strings (~400B)
	b.WriteString("var endpoints = {\n")
	for i := range 6 {
		b.WriteString(fmt.Sprintf("  svc%d: \"https://api%d.example.com/v2/service\",\n", i, i))
	}
	b.WriteString("};\n")

	// Navigation
	b.WriteString("function nav() { location.href = \"/dashboard\"; }\n")
	b.WriteString("function redir() { location.replace(\"/auth/login\"); }\n")
	b.WriteString("function popup() { window.open(\"/report/pdf\", \"_blank\"); }\n")
	b.WriteString("function ext() { window.open(\"https://ext.example.com/share\"); }\n")

	// String noise (~200B)
	for i := range 8 {
		b.WriteString(fmt.Sprintf("var msg%d = \"Processing item %d\";\n", i, i))
	}

	return []byte(b.String())
}

func generateLarge() []byte {
	// Generate multiple independent chunks, each under the parse limit.
	// This simulates a bundled file with many modules.
	chunk := generateMedium()
	// Repeat 5x with unique names to avoid conflicts
	var b strings.Builder
	for c := range 5 {
		// Rename all identifiers to make each chunk unique
		s := string(chunk)
		s = strings.ReplaceAll(s, "api", fmt.Sprintf("api%d_", c))
		s = strings.ReplaceAll(s, "load", fmt.Sprintf("load%d_", c))
		s = strings.ReplaceAll(s, "submit", fmt.Sprintf("submit%d_", c))
		s = strings.ReplaceAll(s, "nav", fmt.Sprintf("nav%d_", c))
		s = strings.ReplaceAll(s, "redir", fmt.Sprintf("redir%d_", c))
		s = strings.ReplaceAll(s, "popup", fmt.Sprintf("popup%d_", c))
		s = strings.ReplaceAll(s, "ext", fmt.Sprintf("ext%d_", c))
		s = strings.ReplaceAll(s, "endpoints", fmt.Sprintf("endpoints%d", c))
		s = strings.ReplaceAll(s, "msg", fmt.Sprintf("msg%d_", c))
		b.WriteString(s)
	}
	return []byte(b.String())
}
