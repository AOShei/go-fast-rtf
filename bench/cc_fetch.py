"""Fetch documents found by cc_find.py out of Common Crawl's WARC files.

    python -I cc_fetch.py FOUND_DIR OUT_DIR PER_TYPE PER_HOST

For each media type, up to PER_TYPE documents at random (seed 1), at
most PER_HOST from one host, so that no site makes up the batch. Each is
one HTTP range request for its WARC record; the body is written to
OUT_DIR/<type>/<n>.<ext>, and OUT_DIR/manifest.tsv gets file, type, size,
SHA-256 and URL. Duplicates (same SHA-256) are kept once. Three requests
at a time, with a pause after a refusal.
"""
import collections, concurrent.futures as cf, glob, hashlib, io, os, random, sys, threading, time, urllib.request
from warcio.archiveiterator import ArchiveIterator

found, out, per_type, per_host = sys.argv[1], sys.argv[2], int(sys.argv[3]), int(sys.argv[4])
EXT = {"application/rtf": "rtf", "text/rtf": "rtf",
       "application/vnd.oasis.opendocument.text": "odt",
       "application/vnd.oasis.opendocument.spreadsheet": "ods",
       "application/vnd.oasis.opendocument.presentation": "odp"}
rows = []
for f in sorted(glob.glob(os.path.join(found, "*.tsv"))):
    lines = open(f, encoding="utf-8").read().splitlines()
    head = lines[0].split("\t")
    for l in lines[1:]:
        rows.append(dict(zip(head, l.split("\t"))))
random.Random(1).shuffle(rows)
picked, hosts, per = [], collections.Counter(), collections.Counter()
for r in rows:
    ext = EXT[r["content_mime_detected"]]
    host = r["url"].split("/")[2] if "//" in r["url"] else r["url"]
    if per[ext] >= per_type or hosts[host] >= per_host:
        continue
    per[ext] += 1
    hosts[host] += 1
    picked.append((ext, r))
print("picked", dict(per), file=sys.stderr)
lock, seen, manifest = threading.Lock(), set(), []


def fetch(item):
    i, (ext, r) = item
    off, n = int(r["warc_record_offset"]), int(r["warc_record_length"])
    req = urllib.request.Request("https://data.commoncrawl.org/" + r["warc_filename"],
                                 headers={"Range": f"bytes={off}-{off + n - 1}"})
    for attempt in range(5):
        try:
            data = urllib.request.urlopen(req, timeout=60).read()
            break
        except Exception as e:
            time.sleep(20 * (attempt + 1))
    else:
        return
    for rec in ArchiveIterator(io.BytesIO(data)):
        if rec.rec_type != "response":
            continue
        body = rec.content_stream().read()
        h = hashlib.sha256(body).hexdigest()
        with lock:
            if h in seen or not body:
                return
            seen.add(h)
        os.makedirs(os.path.join(out, ext), exist_ok=True)
        name = f"{ext}/{i:05d}.{ext}"
        open(os.path.join(out, name), "wb").write(body)
        with lock:
            manifest.append((name, ext, len(body), h, r["url"]))
        return


with cf.ThreadPoolExecutor(3) as ex:
    for k, _ in enumerate(ex.map(fetch, enumerate(picked))):
        if k % 500 == 0:
            print(k, file=sys.stderr, flush=True)
with open(os.path.join(out, "manifest.tsv"), "w", encoding="utf-8") as m:
    for row in sorted(manifest):
        m.write("\t".join(map(str, row)) + "\n")
print(len(manifest), "documents written", file=sys.stderr)
