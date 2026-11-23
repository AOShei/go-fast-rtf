"""Extract the text of every file in a list with one Python tool, for the
email, RTF and ODF comparisons (run_html.py does HTML).

    python -I run_doc.py TOOL LIST OUTDIR WORKERS

Each worker process imports the tool once and reads its share of the
files (in order, interleaved). For each file: the seconds the call took
(time.perf_counter, after the bytes are read), whether it raised (and the
exception's type), and the length of the text. The text is written to
OUTDIR/text/NAME.txt. A call longer than 60 s is stopped and counted as a
timeout. OUTDIR/results.jsonl gets one line a file.
"""
import json, os, signal, subprocess, sys, time
from multiprocessing import Pool

TOOL, LIST, OUT, WORKERS = sys.argv[1], sys.argv[2], sys.argv[3], int(sys.argv[4])

HEADERS = ("subject", "from", "to", "cc", "date")


def html_text(h):
    # HTML bodies of the email tools that leave them as HTML: the fastest
    # HTML text tool of the HTML comparison.
    from selectolax.lexbor import LexborHTMLParser
    t = LexborHTMLParser(h)
    for n in t.css("script, style, noscript, template"):
        n.decompose()
    return t.body.text(separator="\n") if t.body else ""


def make(tool):
    # --- email ---
    if tool == "stdlib-email":
        # Python's email package (policy.default): the headers, the body
        # it prefers (plain, else HTML through selectolax).
        import email, email.policy
        def f(b):
            m = email.message_from_bytes(b, policy=email.policy.default)
            out = [f"{h}: {m[h]}" for h in HEADERS if m[h]]
            body = m.get_body(preferencelist=("plain", "html"))
            if body is not None:
                c = body.get_content()
                out.append(html_text(c) if body.get_content_type() == "text/html" else c)
            return "\n".join(out)
        return f
    if tool == "mail-parser":
        import mailparser
        def f(b):
            m = mailparser.parse_from_bytes(b)
            out = [m.subject or "", str(m.from_ or ""), str(m.to or ""), str(m.date or "")]
            out += m.text_plain or [html_text(h) for h in m.text_html]
            return "\n".join(out)
        return f
    if tool == "eml-parser":
        import eml_parser
        p = eml_parser.EmlParser(include_raw_body=True)
        def f(b):
            d = p.decode_email_bytes(b)
            h = d.get("header", {})
            out = [str(h.get("subject", "")), str(h.get("from", "")), " ".join(map(str, h.get("to", []))), str(h.get("date", ""))]
            for part in d.get("body", []):
                c = part.get("content", "")
                out.append(html_text(c) if "html" in part.get("content_type", "") else c)
            return "\n".join(out)
        return f
    # --- RTF ---
    if tool == "striprtf":
        from striprtf.striprtf import rtf_to_text
        return lambda b: rtf_to_text(b.decode("latin-1"), errors="ignore")
    # --- ODF ---
    if tool == "odfpy":
        import io
        from odf import opendocument, teletype, text as otext, table
        def f(b):
            d = opendocument.load(io.BytesIO(b))
            out = []
            for tag in (otext.H, otext.P):
                out += [teletype.extractText(e) for e in d.getElementsByType(tag)]
            return "\n".join(out)
        return f
    if tool == "pandas-odf":
        # pandas.read_excel with the odf engine (odfpy): every sheet, as
        # data engineers load a spreadsheet.
        import io, pandas as pd
        def f(b):
            sheets = pd.read_excel(io.BytesIO(b), sheet_name=None, engine="odf", header=None)
            return "\n".join(df.to_csv(sep="\t", index=False, header=False) for df in sheets.values())
        return f
    # --- several formats ---
    if tool == "pandoc":
        # pandoc reads RTF and ODT: one process a file, as pypandoc runs it.
        fmt = {".rtf": "rtf", ".odt": "odt"}
        def f(b, ext):
            r = subprocess.run(["pandoc", "-f", fmt[ext], "-t", "plain", "--wrap=none"], input=b,
                               capture_output=True, timeout=60)
            if r.returncode:
                raise RuntimeError("pandoc " + str(r.returncode))
            return r.stdout.decode("utf-8", "replace")
        f.ext = True
        return f
    if tool == "kreuzberg":
        from kreuzberg import extract_bytes_sync
        mime = {".eml": "message/rfc822", ".rtf": "application/rtf", ".odt": "application/vnd.oasis.opendocument.text",
                ".ods": "application/vnd.oasis.opendocument.spreadsheet", ".odp": "application/vnd.oasis.opendocument.presentation"}
        def f(b, ext):
            return extract_bytes_sync(b, mime_type=mime[ext]).content
        f.ext = True
        return f
    if tool == "markitdown":
        import io
        from markitdown import MarkItDown, StreamInfo
        md = MarkItDown()
        def f(b, ext):
            return md.convert_stream(io.BytesIO(b), stream_info=StreamInfo(extension=ext)).text_content
        f.ext = True
        return f
    raise SystemExit("unknown tool " + tool)


class Timeout(Exception):
    pass


def alarm(*_):
    raise Timeout()


def work(args):
    k, files = args
    signal.signal(signal.SIGALRM, alarm)
    f = make(TOOL)
    out = []
    for path in files:
        name = os.path.basename(path)
        ext = os.path.splitext(name)[1].lower()
        b = open(path, "rb").read()
        t0 = time.perf_counter()
        err, n = None, 0
        signal.alarm(60)
        try:
            text = (f(b, ext) if getattr(f, "ext", False) else f(b)) or ""
            n = len(text)
        except Timeout:
            err = "Timeout"
        except BaseException as e:  # a tool's failure, whatever it is
            err = type(e).__name__
        finally:
            signal.alarm(0)
        dt = time.perf_counter() - t0
        if err is None:
            with open(os.path.join(OUT, "text", name + ".txt"), "w", encoding="utf-8", errors="replace") as o:
                o.write(text)
        out.append({"file": name, "s": dt, "err": err, "chars": n})
    return out


def main():
    files = [l.strip() for l in open(LIST) if l.strip()]
    os.makedirs(os.path.join(OUT, "text"), exist_ok=True)
    shares = [(k, files[k::WORKERS]) for k in range(WORKERS)]
    if WORKERS == 1:
        res = [work(shares[0])]
    else:
        with Pool(WORKERS) as p:
            res = p.map(work, shares)
    with open(os.path.join(OUT, "results.jsonl"), "w") as o:
        for part in res:
            for r in part:
                o.write(json.dumps(r) + "\n")


if __name__ == "__main__":
    main()
