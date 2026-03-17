# Grammargen Functional Parity: Design Spec

**Date:** 2026-03-13
**Branch:** `grammargen-pr9-on-pr8` (worktree at `/tmp/gts-grammargen-rebase`)
**Target:** PR into `fix/top50-parity-burndown`
**Goal:** Functional parity = 100% on 49 grammars in real corpus suite

## Problem

grammargen (pure-Go grammar compiler) produces parse blobs that diverge from C tree-sitter on 230/915 eligible test cases (74.8% sexpr parity). The current comparison mode is too strict — it fails on anonymous token differences that don't affect semantic correctness. Meanwhile, real structural divergences (ERROR nodes, wrong named-node trees) are mixed in with noise.

We need a metric that captures what matters (correct named-node structure, no spurious errors) and tolerates what doesn't (anonymous token grouping, byte-range rounding). Then we need to systematically fix the compiler until that metric hits 100%.

## Functional Parity Definition

A grammargen parse is **functionally equivalent** to the C oracle when:

1. **No spurious ERROR nodes.** If C produces a clean parse (zero ERROR nodes), grammargen must too. Any ERROR node on valid input is a hard failure.

2. **Named-node skeleton matches.** Extract only named nodes (where `IsNamed() == true`) from both trees. Compare: node types, nesting structure (parent-child relationships), and ordering.

3. **Field assignments match.** Named children accessible via field names must return the same node types in both trees.

4. **Existing tolerances carry forward:**
   - Anonymous token count/type differences — tolerated
   - Byte-range differences +/-2 on non-root nodes — tolerated
   - Unicode normalization (`\uXXXX` vs UTF-8) — tolerated
   - Named-child-only fallback when total child counts differ but named children agree — tolerated

### Functional Comparison Algorithm

```
functionalCompare(goTree, cTree, lang):
  1. ERROR gate: walk cTree. If zero ERROR nodes in C parse but goTree has
     any ERROR node → FAIL immediately.
  2. Extract named skeletons from both trees:
     - Recursive DFS; at each node, if node.IsNamed() AND node is not a
       supertype/invisible node (i.e., not _-prefixed hidden), emit it.
     - Invisible named nodes (supertypes) are transparent: recurse into
       their children without emitting the wrapper (matches SExpr behavior).
     - Each emitted node records: (type string, [field label if parent
       assigned one], [ordered children]).
  3. Compare skeletons recursively:
     - At each level: same count of named children, same types in order,
       same field labels on field-bearing children.
     - Unicode normalization (\uXXXX vs UTF-8) carries forward.
     - Byte ranges are NOT compared — functional parity is about
       structure (types, nesting, fields), not exact byte positions.
  4. Return PASS or FAIL with first divergence path.
```

**Tolerances removed from `deep` mode:** The existing `compareTreesDeep` has three permissive tolerances that functional mode drops:
- Leaf-vs-populated tolerance (one side has 0 children, other has some) — removed, this masks real structural divergence
- Prefix-match tolerance (one side's named children are a prefix of the other's) — removed, truncated parses are real failures
- Error-children "better parse" tolerance (grammargen has fewer errors) — removed, any ERROR on valid input is a hard fail regardless

### What Changes From Today

| Current mode | Behavior | Problem |
|---|---|---|
| `sexpr` | Full S-expression string match | Too strict — fails on anonymous token noise |
| `deep` | Recursive structural comparison with tolerances | Too loose — tolerates leaf-vs-populated, prefix-match, error-child differences |
| `functional` (new) | Named-skeleton + ERROR check + field check | Right granularity for behavioral equivalence |

Expected baseline jump from ~75% to ~85-90% just by switching metrics, since many current failures are anonymous-token-only differences.

## Compiler Improvement Phases

### Phase 1: Functional Metric + LR Splitting Default

**Metric implementation:**
- New `functionalParity` comparison mode in `grammargen/parity_real_corpus_test.go`
- Extracts named-node skeleton from both Go and C parse trees
- ERROR-node check as hard gate before skeleton comparison
- Field-assignment comparison on named children
- New `functional` field in floor file (`real_corpus_parity_floors.json`)

**LR splitting:**
- Flip `EnableLRSplitting = true` as default in `grammargen/grammar.go`
- Per-grammar experiments showed: python -87% GLR, c -65%, javascript -31%, haskell -13%, scala -12%
- These numbers have not been validated at full-corpus level — Phase 1 includes a validation step
- Fewer GLR conflicts means fewer wrong-fork parses means fewer ERROR nodes

**Phase 1 validation step:** Run full 49-grammar corpus in Docker with LR splitting enabled + functional metric. Generate new floor file. This establishes the real baseline before Phase 2 work begins.

**Expected outcome:** Functional parity baseline of 85-90% (extrapolated; validated by the Phase 1 run).

### Phase 2: Reduce/Reduce Conflict Resolution

**Target grammars:** scala (0/25), yaml (2/25), haskell (3/25), javascript (8/25), python (9/25), php (9/25). These 6 grammars account for ~50% of remaining divergences (119/229 failing cases).

**Root cause:** `resolveReduceReduceLegacy()` in `grammargen/lr.go` handles static precedence, dynamic precedence, and production-index tiebreaking at the individual production level. However, C tree-sitter's `ts_parse_table.c` evaluates R/R conflicts in the context of the full parse state — it considers the accumulated dynamic precedence across the entire reduce path, not just the production being reduced. grammargen also lacks C's "consider all actions" phase where conflicting reduce actions are evaluated against the full set of possible reductions in the state before committing to GLR. This causes grammargen to either pick the wrong winner or unnecessarily create GLR entries where C resolves deterministically.

**Fix:** Implement state-context-aware R/R resolution: evaluate accumulated dynamic precedence of the reduce path, and add a "consider all actions" pre-pass before falling back to GLR. Each grammar gets its own Docker parity run; floor ratchets per grammar.

**Expected outcome:** Functional parity 90-95%.

### Phase 3: DFA Priority + Flattening Gaps

**DFA terminal priority:** Some grammars have keyword/identifier priority inversions where grammargen's DFA picks the wrong token. The `AcceptPriority` fix (already landed) addressed the most common case, but edge cases remain.

**Production flattening:** Hidden-rule pass-through flattening covers 80 rules across 16 grammars. Remaining gaps: recursive hidden rules, deeply nested precedence wrappers.

**These are the long tail** — individually small but collectively ~25% of remaining divergences after Phase 2.

**Expected outcome:** Functional parity 95-99%.

### Phase 4: Final Cleanup + PR Gate

- Grammar-by-grammar mop-up of remaining functional divergences
- External scanner symbol alignment edge cases (if any surface)
- Floor file updated to 100% functional parity on all 49 grammars
- PR submitted

## Testing & Safety

### Docker-Only Execution

Every parity test runs in Docker. No exceptions.

- Default: 8g memory, 4 CPUs
- `-timeout 90m` on all test runs
- Host machine never runs parity tests (WSL OOM incident, 2026-03-11)

**OOM-prone grammars:** The batch Docker script currently skips 13 grammars (rust, c_sharp, java, ruby, cpp, kotlin, css, scala, go_lang, c_lang, python, javascript, dockerfile). Seven of these have floor entries and must be tested for the 100% goal. Strategy:
- Per-grammar Docker runs with `--langs <grammar>` and `--memory 16g` for large grammars
- The existing `run_grammargen_c_parity.sh --langs python --memory 16g` already supports this
- Batch script remains as the fast-feedback loop for non-OOM grammars; individual runs cover the rest
- CI runs both: batch for bulk + individual for the 7 OOM grammars

### Floor Ratchet

- `real_corpus_parity_floors.json` gains a `functional` field per grammar
- Floors only increase — any regression below floor is a hard fail
- Version bump on every floor update
- Docker CI enforces no regressions

### Per-Grammar Isolation

- Each grammar testable independently: `--langs python`
- Enables parallel Docker runs for independent grammars
- Enables focused iteration: fix mechanism, run grammar, ratchet, move on

### Branch Strategy

- All work on `grammargen-pr9-on-pr8` branch in `/tmp/gts-grammargen-rebase` worktree
- Commit prefixes: `grammargen(metric):`, `grammargen(split):`, `grammargen(rr):`, `grammargen(dfa):`
- PR when functional parity hits 100%

## Success Criteria

**PR ships when:**
- Functional parity = 100% on all 49 grammars (all eligible test cases)
- Floor file at version N with all `functional` fields at ceiling
- LR splitting enabled by default
- Docker test scripts run functional parity mode

**100% functional parity does NOT mean:**
- Byte-identical SExpr output
- Identical parse table sizes or GLR behavior
- Coverage of all 206 grammars (follow-up work)

## What This Unlocks

- grammargen becomes the default blob source — no CGo dependency for grammar compilation
- New grammars added by: `grammar.json` -> grammargen -> embed blob
- The ouroboros: tree-sitter grammars compiled and executed entirely in Go
- Path to self-hosting: grammargen consuming its own grammar DSL directly

## Out of Scope

- Parser runtime changes (parser-locked lane)
- New grammar additions beyond existing 206
- grammargen compilation speed optimization
- Grammar DSL improvements
