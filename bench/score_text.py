"""Score extracted text against reference text, by words.

    python -I score_text.py REFDIR RESULTS_DIR... > scores.json

REFDIR/NAME.txt is the reference for each page (the browser's innerText);
each RESULTS_DIR has text/NAME.txt from a tool. Words are runs of letters
and digits (Unicode \\w), case-folded; scored as multisets: precision is
the share of the tool's words found in the reference, recall the share of
the reference's words the tool gives, F1 their harmonic mean. A page the
tool failed on scores 0. The scores are averaged over the pages of the
reference (macro), and the totals summed over all pages (micro).
"""
import collections, json, os, re, sys

WORD = re.compile(r"\w+", re.UNICODE)


def words(path):
    try:
        with open(path, encoding="utf-8", errors="replace") as f:
            return collections.Counter(w.casefold() for w in WORD.findall(f.read()))
    except FileNotFoundError:
        return None


ref_dir, dirs = sys.argv[1], sys.argv[2:]
names = sorted(n[:-4] for n in os.listdir(ref_dir) if n.endswith(".txt"))
refs = {n: words(os.path.join(ref_dir, n + ".txt")) for n in names}
names = [n for n in names if refs[n] and sum(refs[n].values()) >= 20]  # pages with some text
out = {"pages": len(names), "tools": {}}
for d in dirs:
    P = R = F = 0.0
    tp = tw = rw = 0
    missing = 0
    for n in names:
        got = words(os.path.join(d, "text", n + ".txt"))
        ref = refs[n]
        if got is None:
            missing += 1
            rw += sum(ref.values())
            continue
        common = sum((got & ref).values())
        g, r = sum(got.values()), sum(ref.values())
        p = common / g if g else 0.0
        rc = common / r if r else 0.0
        P += p; R += rc; F += (2 * p * rc / (p + rc)) if p + rc else 0.0
        tp += common; tw += g; rw += r
    k = len(names)
    out["tools"][os.path.basename(d.rstrip("/"))] = {
        "precision": P / k, "recall": R / k, "f1": F / k, "failed": missing,
        "micro_precision": tp / tw if tw else 0, "micro_recall": tp / rw if rw else 0}
json.dump(out, sys.stdout, indent=1)
