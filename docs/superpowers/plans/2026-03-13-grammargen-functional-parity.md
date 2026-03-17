# Grammargen Functional Parity Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Achieve 100% functional parity between grammargen-compiled blobs and C tree-sitter across all 49 grammars in the real corpus suite.

**Architecture:** Add a `functionalParity` comparison mode that checks named-node skeleton + ERROR gate + field assignments. Enable LR splitting by default. Improve R/R conflict resolution and DFA priorities. All work in `/tmp/gts-grammargen-rebase` worktree, all testing in Docker.

**Tech Stack:** Go 1.24, Docker (8-16g containers), existing grammargen pipeline + parity test infrastructure.

**Commit command:** Always use `buckley commit --yes --minimal-output` — never `git commit` directly.

**Spec:** `docs/superpowers/specs/2026-03-13-grammargen-functional-parity-design.md`

---

## Chunk 1: Functional Parity Metric (Phase 1a)

### Task 1: Add `compareFunctional` function

**Files:**
- Modify: `/tmp/gts-grammargen-rebase/grammargen/parity_test.go` (add after `compareTreesDeep` at ~line 60)

- [ ] **Step 1: Write the skeleton extraction helper**

Add `extractNamedSkeleton` after the existing `compareTreesDeep` function (~line 60). This extracts the named-node tree from a parse tree, making supertypes/invisible nodes transparent.

```go
// skeletonNode represents a named node in the structural skeleton.
// Intentionally omits byte ranges — functional parity cares about
// structure (types, nesting, fields), not exact byte positions.
type skeletonNode struct {
	Type     string
	Field    string // field label from parent, or ""
	Children []skeletonNode
}

// extractNamedSkeleton extracts the named-node skeleton from a parse tree.
// Invisible named nodes (supertypes, _-prefixed hidden rules) are transparent:
// their children are promoted to the parent level, matching SExpr behavior.
func extractNamedSkeleton(n *gotreesitter.Node, lang *gotreesitter.Language, depth int) []skeletonNode {
	if n == nil || depth > 2000 {
		return nil
	}
	var result []skeletonNode
	for i := 0; i < n.ChildCount(); i++ {
		child := n.Child(i)
		if child == nil {
			continue
		}
		if !child.IsNamed() {
			continue
		}
		typ := child.Type(lang)
		field := n.FieldNameForChild(i, lang)
		// Invisible named nodes (supertypes): transparent, promote children
		if strings.HasPrefix(typ, "_") {
			promoted := extractNamedSkeleton(child, lang, depth+1)
			// Propagate field label from parent to promoted children if they lack one
			if field != "" {
				for j := range promoted {
					if promoted[j].Field == "" {
						promoted[j].Field = field
					}
				}
			}
			result = append(result, promoted...)
			continue
		}
		sn := skeletonNode{
			Type:     unescapeUnicodeInType(typ),
			Field:    field,
			Children: extractNamedSkeleton(child, lang, depth+1),
		}
		result = append(result, sn)
	}
	return result
}
```

- [ ] **Step 2: Write the ERROR-counting helper**

Add `countErrorNodes` right after `extractNamedSkeleton`:

```go
// countErrorNodes counts ERROR and MISSING nodes in a parse tree.
func countErrorNodes(n *gotreesitter.Node, depth int) int {
	if n == nil || depth > 2000 {
		return 0
	}
	count := 0
	if n.IsError() || n.IsMissing() {
		count++
	}
	for i := 0; i < n.ChildCount(); i++ {
		count += countErrorNodes(n.Child(i), depth+1)
	}
	return count
}
```

- [ ] **Step 3: Write the skeleton comparison function**

Add `compareSkeletons` after the helpers:

```go
// compareFunctional checks functional parity between two parse trees.
// Returns nil if functionally equivalent, or a slice of divergences.
// Functional equivalence means: no spurious ERROR nodes + named-node
// skeleton matches (types, nesting, fields).
func compareFunctional(
	genRoot *gotreesitter.Node, genLang *gotreesitter.Language,
	refRoot *gotreesitter.Node, refLang *gotreesitter.Language,
) []parityDivergence {
	// Gate 1: ERROR check. If C has no errors but Go does, hard fail.
	refErrors := countErrorNodes(refRoot, 0)
	genErrors := countErrorNodes(genRoot, 0)
	if refErrors == 0 && genErrors > 0 {
		return []parityDivergence{{
			Path:     "root",
			Category: "error",
			GenValue: fmt.Sprintf("%d ERROR nodes", genErrors),
			RefValue: "0 ERROR nodes",
		}}
	}
	// If ref has errors too, skip (sample not eligible — caller handles this)
	if refErrors > 0 {
		return nil
	}

	// Gate 2: Named-node skeleton comparison
	genSkel := extractNamedSkeleton(genRoot, genLang, 0)
	refSkel := extractNamedSkeleton(refRoot, refLang, 0)

	// Wrap root: the root itself is named, include it
	genRootType := unescapeUnicodeInType(genRoot.Type(genLang))
	refRootType := unescapeUnicodeInType(refRoot.Type(refLang))

	genTree := skeletonNode{Type: genRootType, Children: genSkel}
	refTree := skeletonNode{Type: refRootType, Children: refSkel}

	var divs []parityDivergence
	compareSkeletonsRec(&genTree, &refTree, "root", 0, &divs)
	return divs
}

func compareSkeletonsRec(gen, ref *skeletonNode, path string, depth int, divs *[]parityDivergence) {
	if len(*divs) >= 10 || depth > 2000 {
		return
	}

	// Type check (with unicode normalization already applied during extraction)
	if gen.Type != ref.Type {
		// Root-level lenience: empty ref type is a ts2go metadata issue
		if !(depth == 0 && ref.Type == "") {
			*divs = append(*divs, parityDivergence{
				Path: path, Category: "type",
				GenValue: gen.Type, RefValue: ref.Type,
			})
			return
		}
	}

	// Field label check
	if gen.Field != ref.Field {
		*divs = append(*divs, parityDivergence{
			Path: path, Category: "field",
			GenValue: fmt.Sprintf("field=%q", gen.Field),
			RefValue: fmt.Sprintf("field=%q", ref.Field),
		})
	}

	// Child count check
	if len(gen.Children) != len(ref.Children) {
		*divs = append(*divs, parityDivergence{
			Path: path, Category: "childCount",
			GenValue: fmt.Sprintf("%d named children", len(gen.Children)),
			RefValue: fmt.Sprintf("%d named children", len(ref.Children)),
		})
		return
	}

	// Recurse into children
	for i := range gen.Children {
		childPath := fmt.Sprintf("%s/%s", path, gen.Children[i].Type)
		if i > 0 && i < len(gen.Children) && gen.Children[i].Type == gen.Children[i-1].Type {
			childPath = fmt.Sprintf("%s/%s[%d]", path, gen.Children[i].Type, i)
		}
		compareSkeletonsRec(&gen.Children[i], &ref.Children[i], childPath, depth+1, divs)
	}
}
```

- [ ] **Step 4: Verify it compiles**

```bash
cd /tmp/gts-grammargen-rebase && go test -run '^$' -count=1 ./grammargen
```

Expected: clean build, no errors.

- [ ] **Step 5: Commit**

```bash
cd /tmp/gts-grammargen-rebase && buckley commit --yes --minimal-output
```

Message: `grammargen(metric): add compareFunctional skeleton-based parity check`

---

### Task 2: Wire functional metric into real corpus test

**Files:**
- Modify: `/tmp/gts-grammargen-rebase/grammargen/parity_real_corpus_test.go`

- [ ] **Step 1: Bump floor file version constant**

At line 37 of `parity_real_corpus_test.go`, the code has `realCorpusFloorsFileVersion = 3` but the on-disk floor file is at version 14. Bump to 15 to reflect the schema change (adding FunctionalParity):

```go
// Before:
realCorpusFloorsFileVersion = 3

// After:
realCorpusFloorsFileVersion = 15
```

- [ ] **Step 2: Add FunctionalParity field to realCorpusMetrics**

At line 41-46, add the new field:

```go
type realCorpusMetrics struct {
	Eligible         int `json:"eligible"`
	NoError          int `json:"no_error"`
	SExprParity      int `json:"sexpr_parity"`
	DeepParity       int `json:"deep_parity"`
	FunctionalParity int `json:"functional_parity"`
}
```

- [ ] **Step 3: Add totals to realCorpusFloorFile**

At lines 56-72, add `TotalFunctional`:

```go
TotalFunctional int `json:"total_functional_parity"`
```

Add after the existing `TotalDeep` field.

- [ ] **Step 4: Add functional comparison to the sample loop**

Insert after line 328 (after the entire `if len(divs) == 0 { metrics.DeepParity++ ... } else { divCategoryCounts... }` block). Functional parity is an independent metric — measure it for every eligible no-error sample, regardless of deep parity result:

```go
			funcDivs := compareFunctional(genRoot, genLang, refRoot, refLang)
			if len(funcDivs) == 0 {
				metrics.FunctionalParity++
			}
```

- [ ] **Step 5: Add functional tracking to totals accumulation**

At ~line 370, after `totalDeepParity += metrics.DeepParity`, add:

```go
			totalFunctionalParity += metrics.FunctionalParity
```

And declare `totalFunctionalParity int` alongside the existing total vars (around line 168).

- [ ] **Step 6: Add functional parity to ratchet enforcement**

In `enforceRealCorpusRatchet` (~line 468), add after the DeepParity checks:

```go
	if cur.FunctionalParity < floor.FunctionalParity {
		t.Errorf("ratchet regression functional parity: %d < floor %d", cur.FunctionalParity, floor.FunctionalParity)
	}
```

And in the ratio checks:

```go
		if cur.FunctionalParity*floor.Eligible < floor.FunctionalParity*cur.Eligible {
			t.Errorf("ratchet regression functional parity ratio: %d/%d < floor %d/%d", cur.FunctionalParity, cur.Eligible, floor.FunctionalParity, floor.Eligible)
		}
```

- [ ] **Step 7: Add functional parity to ratchet update logic**

In the update ratio regression check (~line 436-438), add:

```go
					cur.FunctionalParity*prev.Eligible < prev.FunctionalParity*cur.Eligible {
```

And in the floor file write section (~line 455-456), add:

```go
		floorFile.TotalFunctional = totalFunctionalParity
```

- [ ] **Step 8: Add functional parity to summary log**

Update the summary log line (~line 463). Find the existing format string and append `functional_parity=%d` with `totalFunctionalParity` as the corresponding arg. The full replacement:

```go
	t.Logf("REAL CORPUS SUMMARY: profile=%s grammars=%d eligible=%d no-error=%d sexpr_parity=%d deep_parity=%d functional_parity=%d requireParity=%v ratchetUpdate=%v ratchetRebase=%v maxCases=%d maxSampleBytes=%d",
		profile, testedGrammars, totalEligible, totalNoError, totalSExprParity, totalDeepParity, totalFunctionalParity,
		requireParity, updateRatchet, rebaseRatchet, maxCases, maxSampleBytes)
```

- [ ] **Step 9: Verify it compiles**

```bash
cd /tmp/gts-grammargen-rebase && go test -run '^$' -count=1 ./grammargen
```

- [ ] **Step 10: Commit**

```bash
cd /tmp/gts-grammargen-rebase && buckley commit --yes --minimal-output
```

Message: `grammargen(metric): wire functional parity into real corpus test + ratchet`

---

### Task 3: Smoke test the functional metric locally

**Files:**
- No new files — uses existing test infrastructure

- [ ] **Step 1: Run a minimal local test with JSON (small, fast, safe on host)**

JSON is tiny and safe to run on host — it's the Tier 1 grammar:

```bash
cd /tmp/gts-grammargen-rebase && go test ./grammargen -run 'TestJSONParityWithExistingBlob' -v -count=1 2>&1 | head -50
```

Expected: PASS. This validates the new code compiles and doesn't panic.

- [ ] **Step 2: Commit if any fixes were needed**

```bash
cd /tmp/gts-grammargen-rebase && buckley commit --yes --minimal-output
```

Message: `grammargen(metric): fix functional parity smoke test issues`

---

## Chunk 2: LR Splitting Default + Docker Baseline (Phase 1b)

### Task 4: Enable LR splitting by default

**Files:**
- Modify: `/tmp/gts-grammargen-rebase/grammargen/parity_real_corpus_test.go:202-204`

- [ ] **Step 1: Change LR splitting from opt-in to opt-out**

Replace the env-var check at line 202-204:

```go
// Before:
if getenvBool("GTS_GRAMMARGEN_LR_SPLIT") {
    gram.EnableLRSplitting = true
}

// After:
if !getenvBool("GTS_GRAMMARGEN_NO_LR_SPLIT") {
    gram.EnableLRSplitting = true
}
```

This enables LR splitting by default for parity tests. Set `GTS_GRAMMARGEN_NO_LR_SPLIT=1` to disable (rollback escape hatch).

**Note:** The spec says "flip default in `grammar.go`" but we deliberately change it only in the test file. This is safer — it doesn't affect external callers of `Grammar.Generate()`. The Grammar struct's zero-value remains `EnableLRSplitting=false`; callers opt in explicitly. The test default-on ensures all parity validation uses splitting without forcing it on the public API.

- [ ] **Step 2: Verify it compiles**

```bash
cd /tmp/gts-grammargen-rebase && go test -run '^$' -count=1 ./grammargen
```

- [ ] **Step 3: Commit**

```bash
cd /tmp/gts-grammargen-rebase && buckley commit --yes --minimal-output
```

Message: `grammargen(split): enable LR splitting by default (opt-out via NO_LR_SPLIT)`

---

### Task 5: Docker baseline run — establish functional parity floor

**Files:**
- Modify: `/tmp/gts-grammargen-rebase/cgo_harness/docker/run_grammargen_real_corpus_in_docker.sh` (add LR_SPLIT env passthrough)
- Modify: `/tmp/gts-grammargen-rebase/grammargen/testdata/real_corpus_parity_floors.json` (updated by ratchet)

- [ ] **Step 1: Update Docker script to pass through LR split env var**

The Docker script sets env vars at ~line 293. Since we flipped to default-on, no change needed — but verify the `GTS_GRAMMARGEN_NO_LR_SPLIT` var is NOT set in the script. It shouldn't be. Confirm with:

```bash
grep -n "LR_SPLIT" /tmp/gts-grammargen-rebase/cgo_harness/docker/run_grammargen_real_corpus_in_docker.sh
```

Expected: no matches (LR splitting is now default-on in test code, no env var needed).

- [ ] **Step 2: Run Docker batch for non-OOM grammars with ratchet rebase**

```bash
cd /tmp/gts-grammargen-rebase && bash cgo_harness/docker/run_grammargen_real_corpus_in_docker.sh \
  --profile aggressive \
  --max-cases 25 \
  --max-grammars 0 \
  --memory 12g \
  --seed-dir /home/draco/work/gotreesitter/.claude/worktrees/grammargen-pr9-resume/cgo_harness/grammar_seed \
  --offline 2>&1 | tee /tmp/functional-baseline-batch.log
```

This runs all non-skipped grammars (~36 of 49). Capture the functional_parity numbers from the log output.

- [ ] **Step 3: Run Docker individual runs for OOM grammars**

The batch Docker script has no `--only` flag. For the 7 OOM grammars with floor entries (css, scala, go_lang, c_lang, python, javascript, dockerfile), use `docker run` directly with the `GTS_GRAMMARGEN_REAL_CORPUS_ONLY` env var:

```bash
SEED_DIR="/home/draco/work/gotreesitter/.claude/worktrees/grammargen-pr9-resume/cgo_harness/grammar_seed"
GRAMMAR="python"  # repeat for each OOM grammar

docker run --rm \
  --memory 16g --cpus 4 \
  -v /tmp/gts-grammargen-rebase:/workspace:ro \
  -v "$SEED_DIR":/tmp/grammar_parity:ro \
  -w /workspace \
  -e GTS_GRAMMARGEN_REAL_CORPUS_ENABLE=1 \
  -e GTS_GRAMMARGEN_REAL_CORPUS_ROOT=/tmp/grammar_parity \
  -e GTS_GRAMMARGEN_REAL_CORPUS_PROFILE=aggressive \
  -e GTS_GRAMMARGEN_REAL_CORPUS_MAX_CASES=25 \
  -e GTS_GRAMMARGEN_REAL_CORPUS_ALLOW_PARTIAL=1 \
  -e GTS_GRAMMARGEN_REAL_CORPUS_ONLY=$GRAMMAR \
  -e GTS_GRAMMARGEN_REAL_CORPUS_FLOORS_PATH=/workspace/grammargen/testdata/real_corpus_parity_floors.json \
  golang:1.24-bookworm \
  go test ./grammargen -run '^TestMultiGrammarImportRealCorpusParity$' -count=1 -v -timeout 90m \
  2>&1 | tee /tmp/functional-baseline-$GRAMMAR.log
```

Repeat for each OOM grammar. These must run individually due to memory constraints.

- [ ] **Step 4: Record baseline**

Collect functional_parity counts from all Docker runs. Update the floor file with rebase mode:

Add `GTS_GRAMMARGEN_REAL_CORPUS_RATCHET_REBASE=1` to the Docker env vars for a rebase run, or manually aggregate results into the floor file.

- [ ] **Step 5: Commit updated floor file**

```bash
cd /tmp/gts-grammargen-rebase && buckley commit --yes --minimal-output
```

Message: `grammargen(split): baseline functional parity floor with LR splitting enabled`

---

## Chunk 3: R/R Conflict Resolution (Phase 2)

### Task 6: Analyze R/R conflict resolution gap

**Files:**
- Read: `/tmp/gts-grammargen-rebase/grammargen/lr.go:1884-1944` (`resolveReduceReduceLegacy`)
- Read: C tree-sitter's `ts_parse_table.c` conflict resolution (reference material)

- [ ] **Step 1: Identify specific R/R resolution failures**

Run the Phase 1 baseline and examine the divergence logs for the 6 target grammars (scala, yaml, haskell, javascript, python, php). For each, categorize whether divergences are:
- Type mismatches (wrong production chosen → wrong node type)
- ChildCount mismatches (wrong flatten/structure → different child counts)
- ERROR nodes (parser dead-end from wrong table entry)

Use the Docker logs from Task 5 to extract divergence categories:

```bash
grep "deep mismatch\|divs=" /tmp/functional-baseline-*.log | grep -E "scala|yaml|haskell|javascript|python|php"
```

- [ ] **Step 2: Examine C tree-sitter's R/R resolution for specific cases**

For each target grammar, compare the parse tables:
- grammargen: dump action table states with R/R conflicts via diagnostic logging
- ts2go reference: compare action table entries for the same states

Add temporary diagnostic logging to `resolveReduceReduceLegacy()` in `lr.go`:

```go
// Temporary: log R/R conflicts for target grammars
if len(reduces) > 1 {
	fmt.Fprintf(os.Stderr, "R/R conflict state=%d lookahead=%d reduces=%v\n",
		stateIdx, lookaheadSym, reduces)
}
```

Run for a single target grammar (e.g., python) and compare with ts2go blob's action table.

This step is investigative — the exact fix depends on what the analysis reveals. The spec identifies two likely gaps:
1. Accumulated dynamic precedence across the reduce path (not just production-level)
2. "Consider all actions" pre-pass before GLR fallback

- [ ] **Step 3: Implement improved R/R resolution**

Based on Step 2 findings, modify `resolveReduceReduceLegacy()` in `/tmp/gts-grammargen-rebase/grammargen/lr.go:1884`.

The fix will likely involve:

```go
func resolveReduceReduceLegacy(lookaheadSym int, reduces []lrAction, ng *NormalizedGrammar, cache *conflictResolutionCache) ([]lrAction, error) {
	// Existing declared-conflict and annotation checks stay...
	if allInDeclaredConflict(reduces, ng, cache) {
		return reduces, nil
	}
	if shouldKeepRepeatedAnnotationReduces(lookaheadSym, reduces, ng) {
		return reduces, nil
	}

	// NEW: "Consider all actions" pre-pass.
	// Evaluate whether the conflict can be resolved by considering
	// the full action set for this state (shifts + all reduces).
	// If only one reduce survives after considering shift priorities,
	// use it deterministically instead of falling through to GLR.
	// [Implementation depends on analysis from Step 2]

	// Existing precedence tiebreaker hierarchy with FIX:
	// Consider accumulated dynamic precedence of the reduce path,
	// not just the production being reduced.
	// [Implementation depends on analysis from Step 2]

	best := reduces[0]
	bestProd := &ng.Productions[best.prodIdx]
	for _, r := range reduces[1:] {
		rProd := &ng.Productions[r.prodIdx]
		// ... improved comparison logic ...
	}
	return []lrAction{best}, nil
}
```

**Note:** The exact code depends on the analysis in Steps 1-2. This is the most investigative task in the plan — budget extra time for it.

- [ ] **Step 4: Run Docker parity for each target grammar**

Test each of the 6 target grammars individually using `docker run` with `GTS_GRAMMARGEN_REAL_CORPUS_ONLY` (same pattern as Task 5 Step 3):

```bash
SEED_DIR="/home/draco/work/gotreesitter/.claude/worktrees/grammargen-pr9-resume/cgo_harness/grammar_seed"
GRAMMAR="scala"  # repeat for each target grammar

docker run --rm \
  --memory 16g --cpus 4 \
  -v /tmp/gts-grammargen-rebase:/workspace:ro \
  -v "$SEED_DIR":/tmp/grammar_parity:ro \
  -w /workspace \
  -e GTS_GRAMMARGEN_REAL_CORPUS_ENABLE=1 \
  -e GTS_GRAMMARGEN_REAL_CORPUS_ROOT=/tmp/grammar_parity \
  -e GTS_GRAMMARGEN_REAL_CORPUS_PROFILE=aggressive \
  -e GTS_GRAMMARGEN_REAL_CORPUS_MAX_CASES=25 \
  -e GTS_GRAMMARGEN_REAL_CORPUS_ALLOW_PARTIAL=1 \
  -e GTS_GRAMMARGEN_REAL_CORPUS_ONLY=$GRAMMAR \
  -e GTS_GRAMMARGEN_REAL_CORPUS_FLOORS_PATH=/workspace/grammargen/testdata/real_corpus_parity_floors.json \
  golang:1.24-bookworm \
  go test ./grammargen -run '^TestMultiGrammarImportRealCorpusParity$' -count=1 -v -timeout 90m \
  2>&1 | tee /tmp/rr-fix-$GRAMMAR.log
```

Compare functional_parity counts against the baseline from Task 5.

- [ ] **Step 5: Ratchet floor file after confirmed improvements**

For each grammar that improved, do a ratchet update run:

```bash
# Add GTS_GRAMMARGEN_REAL_CORPUS_RATCHET_UPDATE=1 to the Docker env
```

- [ ] **Step 6: Commit after each confirmed improvement**

```bash
cd /tmp/gts-grammargen-rebase && buckley commit --yes --minimal-output
```

Message: `grammargen(rr): improve R/R conflict resolution for <grammar> (+N functional parity)`

Repeat Steps 4-6 for each target grammar. Each grammar improvement is a separate commit — bank wins incrementally.

---

## Chunk 4: DFA Priority + Flattening + Final Gate (Phases 3-4)

### Task 7: DFA terminal priority fixes

**Files:**
- Modify: `/tmp/gts-grammargen-rebase/grammargen/dfa.go` (priority handling)
- Modify: `/tmp/gts-grammargen-rebase/grammargen/normalize.go` (terminal extraction)

- [ ] **Step 1: Identify DFA priority divergences**

From the Docker logs after Phase 2, filter for remaining type-category divergences:

```bash
grep "deep mismatch.*type" /tmp/rr-fix-*.log
```

DFA priority issues manifest as type mismatches where one side produces a keyword and the other an identifier (e.g., `context` vs `identifier`).

- [ ] **Step 2: Fix identified priority issues**

The specific fix depends on analysis. Common patterns:
- Keyword/identifier priority inversion: ensure keyword terminals have lower (better) AcceptPriority than catch-all identifier patterns
- String literal priority: ensure longer literals beat shorter ones when both match
- Immediate token pruning edge cases: verify `pruneImmediateTransitions()` in `dfa.go` handles all cases

- [ ] **Step 3: Run Docker parity for affected grammars**

```bash
cd /tmp/gts-grammargen-rebase && bash cgo_harness/docker/run_grammargen_real_corpus_in_docker.sh \
  --profile aggressive --max-cases 25 --max-grammars 0 --memory 12g \
  --seed-dir /home/draco/work/gotreesitter/.claude/worktrees/grammargen-pr9-resume/cgo_harness/grammar_seed \
  --offline 2>&1 | tee /tmp/dfa-fix-batch.log
```

- [ ] **Step 4: Commit after confirmed improvements**

```bash
cd /tmp/gts-grammargen-rebase && buckley commit --yes --minimal-output
```

Message: `grammargen(dfa): fix terminal priority for <pattern> (+N functional parity)`

---

### Task 8: Production flattening gap fixes

**Files:**
- Modify: `/tmp/gts-grammargen-rebase/grammargen/normalize.go` (hidden rule flattening)

- [ ] **Step 1: Identify remaining flattening divergences**

From Docker logs, look for childCount divergences that indicate structural differences from incomplete flattening:

```bash
grep "deep mismatch.*childCount" /tmp/dfa-fix-*.log
```

- [ ] **Step 2: Fix identified flattening gaps**

Common patterns:
- Recursive hidden rules not flattened (guard in `isSingleSymRef` too strict)
- Deeply nested precedence wrappers not unwrapped

- [ ] **Step 3: Docker parity run + ratchet**

Same pattern as previous tasks.

- [ ] **Step 4: Commit**

```bash
cd /tmp/gts-grammargen-rebase && buckley commit --yes --minimal-output
```

Message: `grammargen(flatten): fix hidden rule flattening for <pattern> (+N functional parity)`

---

### Task 9: Final cleanup + PR gate

**Files:**
- Modify: `/tmp/gts-grammargen-rebase/grammargen/testdata/real_corpus_parity_floors.json` (final floor)

- [ ] **Step 1: Run full Docker suite — all 49 grammars**

Batch run for non-OOM grammars:

```bash
cd /tmp/gts-grammargen-rebase && bash cgo_harness/docker/run_grammargen_real_corpus_in_docker.sh \
  --profile aggressive --max-cases 25 --max-grammars 0 --memory 12g \
  --seed-dir /home/draco/work/gotreesitter/.claude/worktrees/grammargen-pr9-resume/cgo_harness/grammar_seed \
  --offline 2>&1 | tee /tmp/final-gate-batch.log
```

Individual runs for OOM grammars (python, javascript, scala, c_lang, go_lang, css, dockerfile) — use `docker run` directly with `GTS_GRAMMARGEN_REAL_CORPUS_ONLY=$GRAMMAR` (same pattern as Task 5 Step 3).

- [ ] **Step 2: Verify 100% functional parity**

Check logs for all 49 grammars. Every grammar must have `functional_parity == eligible`:

```bash
grep "functional_parity" /tmp/final-gate-*.log
```

If any grammar is below 100%, identify the remaining divergences and fix (loop back to relevant task).

- [ ] **Step 3: Final ratchet rebase**

Run with `GTS_GRAMMARGEN_REAL_CORPUS_RATCHET_REBASE=1` to generate the final floor file with all `functional_parity` fields at ceiling.

- [ ] **Step 4: Commit final floor**

```bash
cd /tmp/gts-grammargen-rebase && buckley commit --yes --minimal-output
```

Message: `grammargen(gate): 100% functional parity on 49 grammars — PR ready`

- [ ] **Step 5: Create PR**

From the worktree:

```bash
cd /tmp/gts-grammargen-rebase && gh pr create \
  --base fix/top50-parity-burndown \
  --title "grammargen: 100% functional parity on 49 grammars" \
  --body "$(cat <<'EOF'
## Summary
- Adds functional parity metric (named-node skeleton + ERROR gate + field check)
- Enables LR(1) state splitting by default
- Improves R/R conflict resolution to match C tree-sitter behavior
- Fixes DFA terminal priority and production flattening gaps
- All 49 real corpus grammars at 100% functional parity

## Test plan
- [ ] Full Docker suite passes: batch (36 grammars) + individual OOM grammars (7)
- [ ] Floor file ratchet enforces no regressions
- [ ] `GTS_GRAMMARGEN_NO_LR_SPLIT=1` rollback tested
- [ ] Existing sexpr/deep parity floors not regressed

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

---

## Key Constraints

1. **Docker-only testing.** Never run parity tests on host. OOM kills WSL.
2. **Bank wins with `buckley commit --yes --minimal-output`.** Never use `git commit` directly. Commit after every confirmed improvement.
3. **Floor ratchet.** Floors only go up. Any regression = investigate before proceeding.
4. **Parser-locked lane.** No changes to `parser.go` or any parser runtime file. All fixes are in `grammargen/`.
5. **Plan documents stay on disk.** Never commit this plan.
6. **Phase 2 is investigative.** The R/R fix requires analysis before coding. Budget accordingly — the exact implementation depends on what the diagnostic logging reveals.
