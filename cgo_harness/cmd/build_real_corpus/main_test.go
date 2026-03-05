package main

import "testing"

func TestSelectFilesByBucketFillsToTarget(t *testing.T) {
	candidates := []corpusFile{
		{RelPath: "a.txt", Size: 400},
		{RelPath: "b.txt", Size: 800},
		{RelPath: "c.txt", Size: 2500},
		{RelPath: "d.txt", Size: 4200},
	}

	selected := selectFilesByBucket(candidates, 1, 256, 2000, 16000)
	if len(selected) != 3 {
		t.Fatalf("expected 3 selected files, got %d", len(selected))
	}

	seen := map[string]struct{}{}
	for _, sf := range selected {
		if _, ok := seen[sf.RelPath]; ok {
			t.Fatalf("duplicate selected path: %s", sf.RelPath)
		}
		seen[sf.RelPath] = struct{}{}
		if sf.Bucket == "" {
			t.Fatalf("empty bucket for %s", sf.RelPath)
		}
	}
}

func TestSelectFilesByBucketKeepsSmallMediumLargeWhenAvailable(t *testing.T) {
	candidates := []corpusFile{
		{RelPath: "small.go", Size: 512},
		{RelPath: "medium.go", Size: 4096},
		{RelPath: "large.go", Size: 65536},
	}

	selected := selectFilesByBucket(candidates, 1, 256, 2000, 16000)
	if len(selected) != 3 {
		t.Fatalf("expected 3 selected files, got %d", len(selected))
	}

	buckets := map[string]bool{}
	for _, sf := range selected {
		buckets[sf.Bucket] = true
	}
	for _, bucket := range []string{"small", "medium", "large"} {
		if !buckets[bucket] {
			t.Fatalf("missing bucket %q in selection: %#v", bucket, selected)
		}
	}
}
