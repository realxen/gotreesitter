//go:build grammar_subset && grammar_subset_move

package grammars

func init() {
	Register(LangEntry{
		Name:           "move",
		Extensions:     []string{".move"},
		Language:       MoveLanguage,
		GrammarSource:  GrammarSourceTS2GoBlob,
		HighlightQuery: "\"module\" @keyword\n(hex_address) @number\n(identifier) @type\n",
	})
}
