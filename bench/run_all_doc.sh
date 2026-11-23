#!/bin/bash
# Run every tool of one format over a file list with 1 and 24 workers.
#   run_all_doc.sh LIB LIST RESULTS TOOLS...
# LIB is email, rtf or odf (the go-fast library); TOOLS are go-fast, the
# tools of run_doc.py, and rs-<tool> for those of rsdoc.
set -u
LIB=$1; LIST=$2; R=$3; shift 3
HERE=$(cd "$(dirname "$0")" && pwd)
B=${BENCH_ENV:-$(cd "$HERE/.." && pwd)}
PY=$B/.venv-text/bin/python
RS=$B/rust-target/release/rsdoc
GO=$HERE/godoc/godoc
MEASURE=${MEASURE:-$HERE/measure.py}
mkdir -p "$R"
for w in 1 24; do
  for t in "$@"; do
    out="$R/$t-$w"
    rm -rf "$out"; mkdir -p "$out"
    case $t in
      go-fast) cmd=("$GO" "$LIB" "$LIST" "$out" "$w");;
      rs-*) cmd=("$RS" "${t#rs-}" "$LIST" "$out" "$w");;
      *) cmd=("$PY" -I "$HERE/run_doc.py" "$t" "$LIST" "$out" "$w");;
    esac
    echo "== $t w=$w load $(cut -d' ' -f1 /proc/loadavg)"
    "$PY" -I "$MEASURE" "$out/measure.json" -- "${cmd[@]}" 2>&1 | grep -v -i warning | tail -1
  done
done
echo ALL DONE
