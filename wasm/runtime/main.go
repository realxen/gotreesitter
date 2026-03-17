//go:build js && wasm

package main

import (
	"syscall/js"

	"github.com/odvcencio/gotreesitter"
)

func main() {
	js.Global().Set("gotreesitter", js.ValueOf(map[string]interface{}{
		"parse":     js.FuncOf(parse),
		"highlight": js.FuncOf(highlight),
		"loadBlob":  js.FuncOf(loadBlob),
		"version":   js.ValueOf("0.1.0-runtime"),
		"mode":      js.ValueOf("runtime"),
	}))
	select {}
}

var languages = map[string]*gotreesitter.Language{}
var highlighters = map[string]*gotreesitter.Highlighter{}

func loadBlob(this js.Value, args []js.Value) interface{} {
	if len(args) < 3 {
		return err("usage: loadBlob(name, blobUint8Array, highlightQuery)")
	}
	name := args[0].String()
	jsArr := args[1]
	query := args[2].String()

	length := jsArr.Get("length").Int()
	blob := make([]byte, length)
	js.CopyBytesToGo(blob, jsArr)

	lang, langErr := gotreesitter.LoadLanguage(blob)
	if langErr != nil {
		return err("load blob: " + langErr.Error())
	}
	languages[name] = lang

	if query != "" {
		hl, hlErr := gotreesitter.NewHighlighter(lang, query)
		if hlErr != nil {
			return err("highlighter: " + hlErr.Error())
		}
		highlighters[name] = hl
	}

	return ok(map[string]interface{}{"name": name})
}

func parse(this js.Value, args []js.Value) interface{} {
	if len(args) < 2 {
		return err("usage: parse(name, source)")
	}
	lang, has := languages[args[0].String()]
	if !has {
		return err("language not loaded: " + args[0].String())
	}
	parser := gotreesitter.NewParser(lang)
	tree, parseErr := parser.Parse([]byte(args[1].String()))
	if parseErr != nil {
		return err(parseErr.Error())
	}
	root := tree.RootNode()
	return ok(map[string]interface{}{
		"sexp":     root.SExpr(lang),
		"hasError": root.HasError(),
	})
}

func highlight(this js.Value, args []js.Value) interface{} {
	if len(args) < 2 {
		return err("usage: highlight(name, source)")
	}
	hl, has := highlighters[args[0].String()]
	if !has {
		return err("no highlighter for: " + args[0].String())
	}
	ranges := hl.Highlight([]byte(args[1].String()))
	jsRanges := make([]interface{}, len(ranges))
	for i, r := range ranges {
		jsRanges[i] = map[string]interface{}{
			"startByte": r.StartByte,
			"endByte":   r.EndByte,
			"capture":   r.Capture,
		}
	}
	return ok(map[string]interface{}{"ranges": jsRanges})
}

func ok(extra map[string]interface{}) interface{} {
	extra["ok"] = true
	return extra
}

func err(msg string) interface{} {
	return map[string]interface{}{"ok": false, "error": msg}
}
