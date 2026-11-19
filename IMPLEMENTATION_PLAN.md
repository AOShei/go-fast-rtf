# go-fast-rtf implementation plan

Module `github.com/AOShei/go-fast-rtf`, package `gofastrtf`. Extracts RTF
(Microsoft RTF 1.9.1) into the go-fast-core model: one `body` part, and a
`stored` part when the file holds data it never shows. Core pinned at
031432a (v0.0.0-20261119191115-031432aa57b0).

## Layout

| Path | What |
|---|---|
| `extract.go`, `doc.go` | Public API: `Extractor`, `Extract`, `ExtractFile`, warning codes |
| `internal/token` | Tokenizer: groups, control words/symbols, text, `\'hh`, bounded `\binN` |
| `internal/codepage` | Single-byte code page tables (1250-1258, 874, 10000) and decoder with DBCS lead-byte handling |
| `internal/reader` | The group/destination state machine that builds the document |
| `cmd/gofastrtf` | CLI printing JSON, Markdown or text |

## Phases and status

1. **Tokenizer, groups, destinations, code pages** — tokenizer with depth
   limit, `\bin` bounded by the decode budget, `\*` unknown destinations
   skipped, `\uN`/`\ucN`, `\'hh` through `\ansicpg`/`\fcharset`/`\cpg`.
   Double-byte pages (932, 936, 949, 950) are not decoded: U+FFFD per
   character with `text.unmapped_glyphs` naming the code page. Status: done.
2. **Structure** — paragraphs, headings (`\outlinelevel`, heading styles),
   lists (`\listtext`, `\pntext`, `\ls`/`\ilvl`, list table for ordered vs
   bullet), tables (`\trowd`/`\cell`/`\row`, `\clmgf`/`\clmrg`,
   `\clvmgf`/`\clvmrg`, nested tables flattened into their cell), `\line`,
   `\tab`. Status: done.
3. **Notes, fields, metadata, pictures, objects** — `\info` metadata;
   footnotes; annotations as `note` comments; field results in text,
   instructions as invisible `note` blocks; hyperlinks as spans; `\pict`
   image blocks; `\object`/`\objdata` handed to `ExtractChild`. Status: done.
4. **Hidden text** — `\v`, `\webhidden` (invisible), `\fsN` below 2 pt
   (tiny), `\cf` against `\highlight`/`\chcbpat`/`\cb`/paragraph shading/page
   background (low contrast), `\deleted` as deletion notes; `\*\generator`,
   `\docvar`, `\*\userprops` and bytes after the document's closing brace in
   the stored part. Status: done.
5. **CLI, README, fuzzing** — `cmd/gofastrtf`, README, `FuzzExtract` and
   `FuzzLexer`. Status: done. Also done: drop-down form fields as `choices`
   notes.

Phases 1 to 4: done (phase-1-rtf branch), tested in `extract_test.go`,
`word_test.go` and the internal packages.

## Next, in order

1. Verify `internal/codepage/tables.go` byte by byte against the Unicode
   mapping files (transcribed without network access; injectivity and spot
   values are tested).
2. Double-byte code pages 932/936/949/950 from tables generated from the
   Unicode/Microsoft mapping files (needs the files and a generator under
   `internal/codepage/gen`); code pages 437 and 850.
3. Equations (`\mmath`): at least their linear text as `formula` blocks.
4. Computing list numbers when `\listtext` is absent.
5. Character properties from the style sheet (`\v`, `\cf`, `\fs` in a style),
   table cell shading (`\clcbpat`) for the contrast check.
6. A small corpus of RTF written by Word, LibreOffice and WordPad, made by
   the user, to compare against.
