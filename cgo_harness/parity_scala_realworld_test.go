//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const (
	scalaRealWorldRepoURL = "https://github.com/scala/scala.git"
	// Pinned for reproducible corpus parity.
	scalaRealWorldCommit = "9ca90550490028efc7a75ebb6ccac51b599d6689"
)

type scalaRealWorldCase struct {
	name string
	path string
}

var scalaRealWorldCases = []scalaRealWorldCase{
	{name: "small-tailrec", path: "src/library/scala/annotation/tailrec.scala"},
	{name: "medium-try", path: "src/library/scala/util/Try.scala"},
	{name: "large-list", path: "src/library/scala/collection/immutable/List.scala"},
	{name: "xlarge-future", path: "src/library/scala/concurrent/Future.scala"},
}

func TestParityScalaRealWorldCorpus(t *testing.T) {
	repoDir := checkoutRealWorldRepo(t, scalaRealWorldRepoURL, scalaRealWorldCommit)

	for _, tc := range scalaRealWorldCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			absPath := filepath.Join(repoDir, filepath.FromSlash(tc.path))
			src, err := os.ReadFile(absPath)
			if err != nil {
				t.Fatalf("read scala corpus file %q: %v", tc.path, err)
			}
			if len(src) == 0 {
				t.Fatalf("empty scala corpus file %q", tc.path)
			}
			normalized := normalizedSource("scala", string(src))
			parityCase := parityCase{name: "scala", source: string(normalized)}
			runParityCase(t, parityCase, "scala-realworld/"+tc.name, normalized)
		})
	}
}

func checkoutRealWorldRepo(t *testing.T, repoURL, commit string) string {
	t.Helper()

	repoDir := filepath.Join(t.TempDir(), "repo")
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			t.Fatalf("git %s: %v", strings.Join(args, " "), err)
		}
	}

	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", repoDir, err)
	}
	run("-C", repoDir, "init")
	run("-C", repoDir, "remote", "add", "origin", repoURL)
	run("-C", repoDir, "fetch", "--depth=1", "origin", commit)
	run("-C", repoDir, "checkout", "--detach", "FETCH_HEAD")

	head := gitOutput(t, repoDir, "rev-parse", "HEAD")
	if !strings.HasPrefix(head, commit[:12]) {
		t.Fatalf("repo HEAD mismatch: got=%s want_prefix=%s", head, commit[:12])
	}
	return repoDir
}

func gitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git %s output: %v", strings.Join(args, " "), err)
	}
	return strings.TrimSpace(string(out))
}

func TestParityScalaRealWorldCorpusMetadata(t *testing.T) {
	// Guard against accidental drift in the pinned corpus source.
	if !strings.HasPrefix(scalaRealWorldCommit, "9ca90550") {
		t.Fatalf("unexpected scala real-world commit pin: %s", scalaRealWorldCommit)
	}
	if len(scalaRealWorldCases) < 4 {
		t.Fatalf("insufficient scala real-world cases: %d", len(scalaRealWorldCases))
	}
	for _, c := range scalaRealWorldCases {
		if strings.TrimSpace(c.name) == "" || strings.TrimSpace(c.path) == "" {
			t.Fatalf("invalid scala real-world case: %+v", c)
		}
		if strings.Contains(c.path, "..") {
			t.Fatalf("invalid path traversal in scala case %q: %s", c.name, c.path)
		}
	}
	t.Logf("scala real-world corpus: repo=%s commit=%s files=%d",
		scalaRealWorldRepoURL, scalaRealWorldCommit, len(scalaRealWorldCases))
}

func Example_scalaRealWorldCorpus() {
	fmt.Println("scala real-world structural parity corpus is pinned and reproducible")
	// Output: scala real-world structural parity corpus is pinned and reproducible
}
