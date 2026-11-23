"""Make reference text for RTF, ODT and ODS files with LibreOffice.

    python -I reference_doc.py LIST OUTDIR N SCRATCH [shown]

Takes N files of the list at random (seed 1) and converts each with
LibreOffice headless: text for RTF and ODT, CSV of every sheet for ODS,
with each cell's value (a number as 42956178) or, given "shown", its
text as displayed (42,956,178), which is what the file stores beside it.
OUTDIR/NAME.txt gets the text. LibreOffice is contained (it has looped
and filled a disk before): its own session and process group, killed as
a group after 60 s and after every call, a file-size limit of 256 MB,
and a private profile and output directory under SCRATCH, removed at
the end.
"""
import os, random, resource, shutil, signal, subprocess, sys

LIST, OUT, N, SCRATCH = sys.argv[1], sys.argv[2], int(sys.argv[3]), sys.argv[4]
SHOWN = sys.argv[5:] == ["shown"]
files = [l.strip() for l in open(LIST) if l.strip()]
random.Random(1).shuffle(files)
os.makedirs(OUT, exist_ok=True)
profile = os.path.join(SCRATCH, "lo-profile")
work = os.path.join(SCRATCH, "lo-out")
FILTER = {".rtf": "txt:Text (encoded):UTF8", ".odt": "txt:Text (encoded):UTF8",
          ".ods": 'csv:Text - txt - csv (StarCalc):9,34,76,1,,0,false,true,%s,false,false,-1' % ("true" if SHOWN else "false")}


def limit():
    resource.setrlimit(resource.RLIMIT_FSIZE, (256 << 20, 256 << 20))


done = failed = 0
try:
    for path in files[:N]:
        name = os.path.basename(path)
        ext = os.path.splitext(name)[1].lower()
        shutil.rmtree(work, ignore_errors=True)
        os.makedirs(work)
        p = subprocess.Popen(["soffice", "--headless", "--norestore", "-env:UserInstallation=file://" + profile,
                              "--convert-to", FILTER[ext], "--outdir", work, path],
                             stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL,
                             start_new_session=True, preexec_fn=limit)
        try:
            p.wait(timeout=60)
        except subprocess.TimeoutExpired:
            pass
        try:
            os.killpg(p.pid, signal.SIGKILL)
        except ProcessLookupError:
            pass
        p.wait()
        outs = sorted(os.listdir(work))
        if not outs:
            failed += 1
            continue
        text = "\n".join(open(os.path.join(work, o), encoding="utf-8", errors="replace").read() for o in outs)
        open(os.path.join(OUT, name + ".txt"), "w", encoding="utf-8").write(text)
        done += 1
finally:
    shutil.rmtree(work, ignore_errors=True)
    shutil.rmtree(profile, ignore_errors=True)
print(done, "references,", failed, "failed", file=sys.stderr)
