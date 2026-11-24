# Benchmarks against the tools in use today

go-fast-rtf is measured against the RTF text extractors that data and AI pipelines use:
- from Python, striprtf and Kreuzberg (a Rust core);
- pandoc, run once per file as pypandoc runs it;
- from Rust, the `rtf-parser` crate.

The tables are in [`comparison.md`](comparison.md). This file says what they show and how to make them again.

## What the comparison shows

The input is 1,250 RTF files, 241 MB, found in Common Crawl and kept as served. The go-fast-rtf figures are for v0.2.1 (go-fast-core v0.1.4), measured on 2026-11-24; the other tools' are from 2026-11-23. Every file gives the same text, or the same error, as with v0.2.0.

- **Speed.** With one worker, go-fast-rtf reads them all in 2.02 s.
  - The Rust `rtf-parser` crate is faster (0.94 s), but it fails on 45 files and its text scores far lower (F1 0.670).
  - Every other tool is slower: Kreuzberg 3.3 times, striprtf 9 times, pandoc (one process per file) about 70 times.
  - With 24 workers, go-fast-rtf takes 0.30 s. `rtf-parser` takes 0.17 s, Kreuzberg 0.98 s, striprtf 2.3 s and pandoc 22 s.
- **The slowest file.** go-fast-rtf's slowest file took 35 ms with one worker and `rtf-parser`'s took 29 ms. Kreuzberg and striprtf each had a file taking 459 ms, and pandoc one taking 2.1 s.
- **Memory.** With one worker, go-fast-rtf peaks at 25 MB, the least of all. With 24 workers it peaks at 116 MB. The others peak at:
  - `rtf-parser`: 374 MB;
  - striprtf: 477 MB;
  - Kreuzberg: 1.6 GB;
  - pandoc: 6.2 GB.
- **Accuracy.** The reference is LibreOffice's text export of 283 files chosen at random. go-fast-rtf has the highest F1 of all tools:

  | Tool | F1 |
  |---|---|
  | go-fast-rtf | 0.983 |
  | striprtf | 0.969 |
  | Kreuzberg | 0.940 |
  | pandoc | 0.778 |
  | `rtf-parser` | 0.670 |

- **Failures.** go-fast-rtf refused 4 files. None of them is RTF: one is plain UTF-8 text and three are PHP warning pages, served as RTF. It refuses them as not RTF, while the other tools give their bytes as text. pandoc failed on 15 files and `rtf-parser` on 45.
- **Where the time goes.** go-fast-rtf allocates little: 0.56 GB for the 241 MB of the batch (v0.2.0). Its time goes to the lexer, which reads one byte at a time and looks up control words in a map. That is where any further speed would come from.

## `GOGC`

Go's garbage collector can be given more memory for less work, with `GOGC` in the environment or `debug.SetGCPercent` in a program. The library keeps Go's default, 100. go-fast-rtf allocates little, so it gains little from more memory:

| `GOGC` | One worker | Peak | 24 workers | Peak |
|---|---|---|---|---|
| 100 (default) | 2.02 s | 22 MB | 0.29 s | 132 MB |
| 200 | 1.97 s | 26 MB | 0.27 s | 216 MB |
| 400 | 1.89 s | 38 MB | 0.27 s | 217 MB |

## How it is measured

- Each tool reads every file in one long-lived process, as a pipeline does, first with one worker and then with 24.
  - The time of a file is the call alone, timed after its bytes are read.
  - Wall time, CPU time and peak memory are those of the whole process tree.
- The machine is shared, so `measure.py` waits for it to be quiet before each run.
  - A run is repeated if other processes used it meanwhile.
  - The tables mark a run that stayed disturbed after five attempts.
- Accuracy is scored by words, compared as multisets (`score_text.py`, the same as for go-fast-html).
  - Precision is the share of a tool's words that are in the reference. Recall is the share of the reference's words that the tool gives.
  - Scores are averaged over files. A file a tool failed on scores 0.

## Reproducing

The Python tools are pinned in `requirements-text.txt` (the same environment as go-fast-html's benchmark), and pandoc must be on the `PATH`. Set `DATA` to a directory outside the repository and `SCRATCH` to a directory for LibreOffice's profile:

```sh
# Files: RTF found in 61 of the 300 files of the crawl's columnar index,
# then at most 2,500 a type, at most 25 from one host, kept once by SHA-256.
curl -O https://data.commoncrawl.org/crawl-data/CC-MAIN-2025-43/cc-index-table.paths.gz
python -I bench/cc_find.py cc-index-table.paths.gz $DATA/found \
  $( (seq 0 14; seq 30 35; seq 60 64; seq 90 95; seq 120 136; seq 240 251) | xargs printf 'part-%05d ')
python -I bench/cc_fetch.py $DATA/found $DATA 2500 25     # $DATA/rtf/, manifest.tsv
ls $DATA/rtf/*.rtf > $DATA/rtf.list

# Tools.
(cd bench/godoc && go build -o godoc .)
(cd bench/rsdoc && CARGO_TARGET_DIR=../../../bench-env/rust-target cargo build --release)
BENCH_ENV=../bench-env bench/run_all_doc.sh rtf $DATA/rtf.list $DATA/results-rtf \
  go-fast striprtf pandoc kreuzberg rs-rtf-parser > $DATA/run.log
for g in 100 200 400; do for w in 1 24; do
  GOGC=$g python -I bench/measure.py $DATA/gogc/rtf/gogc$g-$w/measure.json -- \
    bench/godoc/godoc rtf $DATA/rtf.list $DATA/gogc/rtf/gogc$g-$w $w
done; done

# Accuracy, and the report.
python -I bench/reference_doc.py $DATA/rtf.list $DATA/reference-rtf 300 $SCRATCH
python -I bench/score_text.py $DATA/reference-rtf $DATA/results-rtf/*-1 > $DATA/scores-rtf.json
python -I bench/report_doc.py rtf $DATA $DATA/gogc > bench/comparison.md
```

`reference_doc.py` runs LibreOffice inside limits:
- its own session and process group, killed as a group;
- a file-size limit of 256 MB;
- a private profile under `$SCRATCH`, removed at the end.

`run_doc.py`, `run_all_doc.sh`, `report_doc.py` and `rsdoc` also serve the go-fast-email and go-fast-odf benchmarks; `godoc` here runs go-fast-rtf alone.
