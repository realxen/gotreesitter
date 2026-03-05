# cgo_harness

This module contains CGo-only parity and baseline benchmark harnesses used to compare `gotreesitter` against native C tree-sitter parsers.

## Unified Harness Gate

From repo root, run the unified gate runner:

```sh
go run ./cmd/harnessgate -mode all
```

This executes:

- root correctness (`go test ./... -count=1`)
- curated cgo parity suites
- stable perf trio (optionally benchgate-compared to a baseline)

Artifacts are written under `harness_out/`.

Optional weighted confidence scoring can be enabled from `harnessgate` using
either a built-in profile (`top50`, `core90`) or a custom manifest JSON:

```sh
go run ./cmd/harnessgate -mode correctness \
  -real-corpus-dir cgo_harness/corpus_real \
  -real-corpus-langs top10 \
  -confidence-profile core90 \
  -confidence-min 0.90
```

Framework details (oracles, corpus tiers, gate policy):

- `cgo_harness/HARNESS_FRAMEWORK.md`

## Run Parity Tests

```sh
go test . -tags treesitter_c_parity \
  -run '^TestParityFreshParse$|^TestParityHasNoErrors$|^TestParityIssue3Repros$|^TestParityGLRCanaryGo$|^TestParityGLRCanarySet$|^TestParityGLRCapPressureTopLanguages$' \
  -count=1 -v

go test . -tags treesitter_c_parity \
  -run '^TestParityCorpusFreshParse$' \
  -count=1 -v
```

Optional Scala real-world structural parity probe:

```sh
go test . -tags treesitter_c_parity \
  -run '^TestParityScalaRealWorldCorpus$' \
  -count=1 -v
```

## Run Corpus Parity (`dump.v1`)

This command compares `gotreesitter` vs the native C oracle, emits `dump.v1`
artifacts for both runtimes, writes JSONL results, and updates `PARITY.md`.

```sh
go run -tags treesitter_c_parity ./cmd/corpus_parity \
  --lang top10 \
  --corpus ./corpus \
  --out ./parity_out/results.jsonl \
  --artifact-dir ./parity_out/dump_v1 \
  --scoreboard ./PARITY.md
```

Notes:

- `--lang` accepts `top10` (default), a single language (`go`), or a comma-separated list.
- For multiple languages, corpus layout is `--corpus/<language>/**`.
- For a single language (`--lang go`), `--corpus` can point directly at that language directory.

## Build Real Corpus (Lock-Pinned)

Use the corpus builder to materialize production-grade real corpus fixtures from
`grammars/languages.lock` pinned upstream commits:

```sh
go run ./cgo_harness/cmd/build_real_corpus \
  -profile cgo_harness/cmd/build_real_corpus/top50_manifest.json \
  -out cgo_harness/corpus_real
```

Notes:

- Selection is deterministic and bucketed (`small`, `medium`, `large`) per language.
- Selection targets `small`/`medium`/`large` buckets per language, with deterministic fallback when one bucket has no candidates.
- Source files are pulled from pinned upstream commits and recorded in
  `cgo_harness/corpus_real/manifest.json` with SHA256 + source path metadata.
- Validate corpus quality bar:

```sh
cd cgo_harness
GTS_REAL_CORPUS_MANIFEST=corpus_real/manifest.json \
  go test . -run TestRealCorpusManifestQuality -count=1
```

- Use this corpus with the parity runner:

```sh
go run ./cmd/harnessgate -mode correctness \
  -real-corpus-dir cgo_harness/corpus_real \
  -real-corpus-langs top50
```

## Run C Baseline Benchmarks

```sh
GOMAXPROCS=1 go test . -run '^$' -tags treesitter_c_bench \
  -bench 'BenchmarkCTreeSitterGoParseFull|BenchmarkCTreeSitterGoParseIncrementalSingleByteEdit|BenchmarkCTreeSitterGoParseIncrementalNoEdit' \
  -benchmem -count=10 -benchtime=750ms
```

These harnesses are intentionally split into a separate module so the root `gotreesitter` module remains pure-Go in dependency metadata.

## Run Pure-C Runtime Matrix (No CGo)

This compares against the tree-sitter C runtime compiled directly with `gcc`/`g++` and does not execute through Go cgo bindings.

```sh
./pure_c/run_matrix.sh
```

The matrix currently runs full-parse benchmarks for:

- `c`
- `go`
- `java`
- `html`
- `lua`
- `toml`
- `yaml`

## Run Pure-C Go Incremental Benchmark (No CGo)

This reproduces full parse, incremental single-byte edit, and incremental
random-edit incremental, and no-edit numbers against the native C runtime:

```sh
./pure_c/run_go_benchmark.sh
```

Optional arguments:

```sh
./pure_c/run_go_benchmark.sh <func_count> <full_iters> <inc_iters>
```

Example:

```sh
./pure_c/run_go_benchmark.sh 500 2000 20000
```

Optional compiler tuning flags:

```sh
CFLAGS_EXTRA="-march=native -flto" ./pure_c/run_go_benchmark.sh
```

## Run Go Head-to-Head Comparison

This runs both:

- `gotreesitter` Go benchmarks
- pure-C runtime benchmark (no cgo)

```sh
./pure_c/run_go_head_to_head.sh
```

## Run Multi-Language Head-to-Head Matrix

This runs:

- pure-C runtime matrix (`c`, `go`, `java`, `html`, `lua`, `toml`, `yaml`)
- matching `gotreesitter` benchmarks
- a combined summary table with per-language speedup ratios

```sh
./pure_c/run_matrix_head_to_head.sh
```

## Run Full Claim Suite (3-way, multi-size, repeated)

This runs repeated benchmarks across:

- `gotreesitter` (pure Go)
- tree-sitter C runtime via cgo bindings
- tree-sitter C runtime compiled directly with GCC (no cgo)

and generates a median-based report.

```sh
./pure_c/run_claim_suite.sh
```

Tunable inputs:

```sh
RUNS=7 SIZES="500 2000 10000" CFLAGS_EXTRA="-march=native -flto" ./pure_c/run_claim_suite.sh
```
