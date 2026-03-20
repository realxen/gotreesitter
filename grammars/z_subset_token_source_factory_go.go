//go:build grammar_subset && grammar_subset_go

package grammars

func init() {
	registerTokenSourceFactory("go", NewGoTokenSourceOrEOF)
}
