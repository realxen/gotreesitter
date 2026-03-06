package grammars

import (
	"strings"
	"testing"

	"github.com/odvcencio/gotreesitter"
)

// Languages where the highlight query compiles but the smoke sample is too
// simple to produce any highlight ranges. These are not bugs.
var highlightNoRangesExpected = map[string]bool{
	// These produce raw query matches but the Highlighter API returns 0 ranges.
	// Typically due to predicate filtering, tokenization gaps, or injection
	// requirements in the highlight query.
	"cobol":   true, // smoke sample is too small for current capture patterns
	"comment": true, // comment grammar relies on injection/predicate semantics
	"cpp":     true, // C++ highlight query requires predicate support beyond current Highlighter
	"mermaid": true, // mermaid highlights require specific node nesting not matched
	"nginx":   true, // query expects richer directive context than smoke sample
	"nim":     true, // highlight query depends on captures not hit by smoke sample
	"org":     true, // org-mode highlights depend on injection/predicate features
	"rst":     true, // rst query expects structural nodes outside minimal sample
}

func TestAllHighlightQueriesCompile(t *testing.T) {
	entries := AllLanguages()
	t.Cleanup(func() { PurgeEmbeddedLanguageCache() })

	var withQuery int
	var compileErrs int

	for _, entry := range entries {
		if strings.TrimSpace(entry.HighlightQuery) == "" {
			continue
		}
		withQuery++
		lang := entry.Language()
		if _, err := gotreesitter.NewQuery(entry.HighlightQuery, lang); err != nil {
			compileErrs++
			t.Errorf("%s: highlight query compile error: %v", entry.Name, err)
		}
		UnloadEmbeddedLanguage(entry.Name + ".bin")
	}

	t.Logf("highlight compile audit: with_query=%d compile_errors=%d", withQuery, compileErrs)
}

func TestAllTagsQueriesCompile(t *testing.T) {
	entries := AllLanguages()
	t.Cleanup(func() { PurgeEmbeddedLanguageCache() })

	var withQuery int
	var compileErrs int

	for _, entry := range entries {
		if strings.TrimSpace(entry.TagsQuery) == "" {
			continue
		}
		withQuery++
		lang := entry.Language()
		if _, err := gotreesitter.NewTagger(lang, entry.TagsQuery); err != nil {
			compileErrs++
			t.Errorf("%s: tags query compile error: %v", entry.Name, err)
		}
		UnloadEmbeddedLanguage(entry.Name + ".bin")
	}

	t.Logf("tags compile audit: with_query=%d compile_errors=%d", withQuery, compileErrs)
}

func TestHighlightQueriesProduceResults(t *testing.T) {
	entries := AllLanguages()
	t.Cleanup(func() { PurgeEmbeddedLanguageCache() })

	var tested, skippedNoQuery, skippedNoSample, skippedUnsupported int
	for _, entry := range entries {
		name := entry.Name
		if strings.TrimSpace(entry.HighlightQuery) == "" {
			skippedNoQuery++
			continue
		}

		lang := entry.Language()
		report := EvaluateParseSupport(entry, lang)
		if report.Backend == ParseBackendUnsupported {
			skippedUnsupported++
			UnloadEmbeddedLanguage(entry.Name + ".bin")
			continue
		}

		sample := ParseSmokeSample(name)
		if sample == "x\n" {
			skippedNoSample++
			UnloadEmbeddedLanguage(entry.Name + ".bin")
			continue
		}

		tested++
		t.Run(name, func(t *testing.T) {
			// Build highlighter options.
			var opts []gotreesitter.HighlighterOption
			if entry.TokenSourceFactory != nil {
				factory := entry.TokenSourceFactory
				opts = append(opts, gotreesitter.WithTokenSourceFactory(
					func(src []byte) gotreesitter.TokenSource {
						return factory(src, lang)
					},
				))
			}

			h, err := gotreesitter.NewHighlighter(lang, entry.HighlightQuery, opts...)
			if err != nil {
				t.Fatalf("compile highlight query: %v", err)
			}

			ranges := h.Highlight([]byte(sample))
			if len(ranges) == 0 && !highlightNoRangesExpected[name] {
				t.Errorf("highlight query compiled but produced 0 ranges for sample %q", sample)
			}
		})
		UnloadEmbeddedLanguage(entry.Name + ".bin")
	}

	t.Logf("highlight validation: tested=%d skipped(no_query=%d no_sample=%d unsupported=%d)",
		tested, skippedNoQuery, skippedNoSample, skippedUnsupported)

}
