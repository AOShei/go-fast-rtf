"""Find documents of some media types in Common Crawl's columnar index.

    python -I cc_find.py PATHS_GZ OUTDIR [PREFIX...]

Reads, of the index files (those whose name starts with one of the
PREFIXes, else all), one at a time, the rows whose detected media type is
RTF or OpenDocument, fetched whole (status 200, not truncated), and writes
url, mime, warc file, offset and length to OUTDIR/<index file>.tsv. A file
already written is skipped, so the script can be run again after a
failure; a refused request is retried after a pause (Common Crawl asks
clients to slow down rather than run in parallel).
"""
import gzip, os, sys, time, polars as pl

paths = [l.strip() for l in gzip.open(sys.argv[1], "rt") if "subset=warc" in l]
out = sys.argv[2]
prefixes = sys.argv[3:]
os.makedirs(out, exist_ok=True)
mimes = ["application/rtf", "text/rtf",
         "application/vnd.oasis.opendocument.text",
         "application/vnd.oasis.opendocument.spreadsheet",
         "application/vnd.oasis.opendocument.presentation"]
for p in paths:
    name = p.rsplit("/", 1)[1]
    if prefixes and not any(name.startswith(x) for x in prefixes):
        continue
    dest = os.path.join(out, name[:10] + ".tsv")
    if os.path.exists(dest):
        continue
    for attempt in range(6):
        try:
            df = (pl.scan_parquet("https://data.commoncrawl.org/" + p)
                    .filter(pl.col("content_mime_detected").is_in(mimes))
                    .select("url", "content_mime_detected", "fetch_status", "content_truncated",
                            "warc_filename", "warc_record_offset", "warc_record_length")
                    .collect())
            break
        except OSError as e:
            print(name[:10], "retrying:", str(e)[:80], file=sys.stderr, flush=True)
            time.sleep(60 * (attempt + 1))
    else:
        continue
    df = df.filter((pl.col("fetch_status") == 200) & pl.col("content_truncated").is_null())
    df.write_csv(dest + ".tmp", separator="\t")
    os.rename(dest + ".tmp", dest)
    print(name[:10], df.group_by("content_mime_detected").len().sort("len").rows(), file=sys.stderr, flush=True)
