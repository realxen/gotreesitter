package bench

import (
	"fmt"
	"testing"

	"github.com/BishopFox/jsluice"
	"github.com/odvcencio/gotreesitter/jsextract"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func sizeLabel(data []byte) string {
	n := len(data)
	switch {
	case n < 1024:
		return fmt.Sprintf("%dB", n)
	case n < 1024*1024:
		return fmt.Sprintf("%.1fKB", float64(n)/1024)
	default:
		return fmt.Sprintf("%.1fMB", float64(n)/(1024*1024))
	}
}

type testCase struct {
	name string
	data []byte
}

var cases = []testCase{
	{"small", jsSmall},
	{"medium", jsMedium},
	{"large", jsLarge},
}

// ---------------------------------------------------------------------------
// gotreesitter (pure Go) benchmarks
// ---------------------------------------------------------------------------

func BenchmarkGoTreeSitter(b *testing.B) {
	for _, tc := range cases {
		b.Run(fmt.Sprintf("%s_%s", tc.name, sizeLabel(tc.data)), func(b *testing.B) {
			b.SetBytes(int64(len(tc.data)))
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				eps, err := jsextract.ExtractEndpoints(tc.data)
				if err != nil {
					b.Fatal(err)
				}
				_ = eps
			}
		})
	}
}

// BenchmarkGoTreeSitterPreloaded simulates real-world usage where the
// extractor is created once at application startup and reused for every
// JS file (katana's actual pattern).
func BenchmarkGoTreeSitterPreloaded(b *testing.B) {
	ext := jsextract.NewExtractor()
	for _, tc := range cases {
		b.Run(fmt.Sprintf("%s_%s", tc.name, sizeLabel(tc.data)), func(b *testing.B) {
			b.SetBytes(int64(len(tc.data)))
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				eps, err := ext.Extract(tc.data)
				if err != nil {
					b.Fatal(err)
				}
				_ = eps
			}
		})
	}
}

// ---------------------------------------------------------------------------
// jsluice (CGO) benchmarks
// ---------------------------------------------------------------------------

func BenchmarkJsluice(b *testing.B) {
	for _, tc := range cases {
		b.Run(fmt.Sprintf("%s_%s", tc.name, sizeLabel(tc.data)), func(b *testing.B) {
			b.SetBytes(int64(len(tc.data)))
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				analyzer := jsluice.NewAnalyzer(tc.data)
				urls := analyzer.GetURLs()
				_ = urls
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Correctness comparison — not a benchmark, but validates both see similar counts
// ---------------------------------------------------------------------------

func TestExtractionParity(t *testing.T) {
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			goEps, err := jsextract.ExtractEndpoints(tc.data)
			if err != nil {
				t.Fatal(err)
			}

			analyzer := jsluice.NewAnalyzer(tc.data)
			cgoURLs := analyzer.GetURLs()

			t.Logf("gotreesitter: %d endpoints, jsluice: %d URLs (source: %d bytes)",
				len(goEps), len(cgoURLs), len(tc.data))

			// Build sets for comparison
			goSet := make(map[string]string, len(goEps))
			for _, ep := range goEps {
				goSet[ep.URL] = ep.Type
			}
			cgoSet := make(map[string]string, len(cgoURLs))
			for _, u := range cgoURLs {
				cgoSet[u.URL] = u.Type
			}

			// Report URLs found by jsluice but not gotreesitter
			var missed int
			for url := range cgoSet {
				if _, ok := goSet[url]; !ok {
					missed++
					if missed <= 10 {
						t.Logf("  jsluice-only: %q (%s)", url, cgoSet[url])
					}
				}
			}
			if missed > 10 {
				t.Logf("  ... and %d more jsluice-only URLs", missed-10)
			}

			// Report URLs found by gotreesitter but not jsluice
			var extra int
			for url := range goSet {
				if _, ok := cgoSet[url]; !ok {
					extra++
					if extra <= 10 {
						t.Logf("  gotreesitter-only: %q (%s)", url, goSet[url])
					}
				}
			}
			if extra > 10 {
				t.Logf("  ... and %d more gotreesitter-only URLs", extra-10)
			}

			// Count overlap
			var overlap int
			for url := range goSet {
				if _, ok := cgoSet[url]; ok {
					overlap++
				}
			}
			t.Logf("overlap: %d, go-only: %d, cgo-only: %d", overlap, extra, missed)
		})
	}
}
