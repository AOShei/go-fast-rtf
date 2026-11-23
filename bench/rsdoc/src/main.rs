// Extracts the text of every file in a list with a Rust crate, as
// run_doc.py does with a Python tool: one process, WORKERS threads reading
// the files in turn, the time of each call after the bytes are read,
// whether it failed (an error or a panic), and the length of the text;
// the text goes to OUTDIR/text/NAME.txt, a line a file to
// OUTDIR/results.jsonl.
//
//     rsdoc TOOL LIST OUTDIR WORKERS
//
// TOOL: mail-parser (email: the headers and every text body, HTML ones
// converted to text by the crate), rtf-parser (RTF), calamine (ODS: every
// cell of every sheet).
use std::io::Write;
use std::time::Instant;

fn mail(b: &[u8]) -> Result<String, String> {
    let m = mail_parser::MessageParser::default().parse(b).ok_or("not a message")?;
    let mut s = String::new();
    for h in [m.subject(), m.date().map(|d| d.to_rfc3339()).as_deref()] {
        if let Some(h) = h {
            s.push_str(h);
            s.push('\n');
        }
    }
    for a in [m.from(), m.to(), m.cc()].into_iter().flatten() {
        for x in a.iter() {
            if let Some(n) = x.name() { s.push_str(n); s.push(' '); }
            if let Some(e) = x.address() { s.push_str(e); }
            s.push('\n');
        }
    }
    let mut i = 0;
    while let Some(t) = m.body_text(i) {
        s.push_str(&t);
        s.push('\n');
        i += 1;
    }
    Ok(s)
}

fn rtf(b: &[u8]) -> Result<String, String> {
    let s = String::from_utf8_lossy(b);
    let doc = rtf_parser::RtfDocument::try_from(s.as_ref()).map_err(|e| format!("{e:?}"))?;
    Ok(doc.get_text())
}

fn ods(b: &[u8]) -> Result<String, String> {
    use calamine::Reader;
    let mut wb = calamine::Ods::new(std::io::Cursor::new(b.to_vec())).map_err(|e| format!("{e}"))?;
    let mut s = String::new();
    for name in wb.sheet_names().to_owned() {
        let r = wb.worksheet_range(&name).map_err(|e| format!("{e}"))?;
        for row in r.rows() {
            let cells: Vec<String> = row.iter().map(|c| c.to_string()).collect();
            s.push_str(&cells.join("\t"));
            s.push('\n');
        }
    }
    Ok(s)
}

fn main() {
    let a: Vec<String> = std::env::args().collect();
    let (tool, list, out, workers) = (a[1].clone(), a[2].clone(), a[3].clone(), a[4].parse::<usize>().unwrap());
    let f: fn(&[u8]) -> Result<String, String> = match tool.as_str() {
        "mail-parser" => mail,
        "rtf-parser" => rtf,
        "calamine" => ods,
        _ => panic!("unknown tool"),
    };
    let files: Vec<String> = std::fs::read_to_string(&list).unwrap().lines().filter(|l| !l.is_empty()).map(String::from).collect();
    std::fs::create_dir_all(format!("{}/text", out)).unwrap();
    let mut res: Vec<String> = vec![String::new(); files.len()];
    std::thread::scope(|s| {
        let chunks: Vec<_> = (0..workers)
            .map(|k| {
                let files = &files;
                let out = &out;
                s.spawn(move || {
                    let mut mine = Vec::new();
                    let mut i = k;
                    while i < files.len() {
                        let b = std::fs::read(&files[i]).unwrap();
                        let name = std::path::Path::new(&files[i]).file_name().unwrap().to_string_lossy().to_string();
                        let t0 = Instant::now();
                        let r = std::panic::catch_unwind(|| f(&b));
                        let dt = t0.elapsed().as_secs_f64();
                        let (err, n) = match r {
                            Ok(Ok(text)) => {
                                std::fs::write(format!("{}/text/{}.txt", out, name), &text).unwrap();
                                (serde_json::Value::Null, text.chars().count())
                            }
                            Ok(Err(e)) => (serde_json::Value::String(e), 0),
                            Err(_) => (serde_json::Value::String("panic".into()), 0),
                        };
                        mine.push((i, serde_json::json!({"file": name, "s": dt, "err": err, "chars": n}).to_string()));
                        i += workers;
                    }
                    mine
                })
            })
            .collect();
        for c in chunks {
            for (i, line) in c.join().unwrap() {
                res[i] = line;
            }
        }
    });
    let mut w = std::fs::File::create(format!("{}/results.jsonl", out)).unwrap();
    for l in res {
        writeln!(w, "{}", l).unwrap();
    }
}
