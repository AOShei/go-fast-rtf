# go-fast-rtf

Extracts RTF documents (Microsoft's Rich Text Format, specification 1.9.1)
into the shared go-fast document model,
[`gofastcore.Document`](https://github.com/AOShei/go-fast-core). Standard
library and go-fast modules only.

**Status: first implementation (phase 1), not tagged.** See
[`IMPLEMENTATION_PLAN.md`](IMPLEMENTATION_PLAN.md) for what is done and what
is next.

## Using it

```go
import (
	"github.com/AOShei/go-fast-core" // package gofastcore
	"github.com/AOShei/go-fast-rtf"  // package gofastrtf
)

doc, err := gofastrtf.ExtractFile(ctx, "letter.rtf", nil)
if err != nil {
	return err // wraps gofastcore.ErrMalformed, ErrLimitExceeded, ...
}
fmt.Println(doc.Markdown())
```

`gofastrtf.Extract(ctx, r, size, opts)` reads from an `io.ReaderAt`.
`gofastrtf.Extractor` implements `gofastcore.Extractor`; its one field,
`MaxGroupDepth`, bounds how deeply groups may nest. The file is read as a
stream: only the document being built, and the picture or object being
read, are held in memory.

The format is decided from the content: the file must begin, after white
space or a byte order mark, with `{\rtf`. Anything else is `ErrMalformed`.

## How it maps

A document has one part of kind `body`, part 1. When the file stores data it
never shows, a second part of kind `stored` follows it (see below);
`Options.Parts` selects it only with the whole document.

| RTF | Block |
|---|---|
| `\par` | ends a `paragraph`; `\line` is `\n`, `\tab` a tab, both kept in the text |
| `\outlinelevelN`, a paragraph style named `heading N` or with an outline level (followed through `\sbasedon`) | `heading`, level N+1 for `\outlinelevelN`; levels 7 to 9 are clamped to 6 with `Attrs["level"]` and `structure.heading_level_clamped` |
| `\listtext`, `\pntext` | the `Marker` of a `list_item` (or of a numbered heading); level from `\ilvl`, or `\pnlvlN`; `Ordered` from the list table's `\levelnfc` for `\ls`, from `\pn` settings, or from the marker |
| `\ls` without `\listtext` | `list_item` with no marker (numbers are not computed) |
| `\trowd`, `\cellx`, `\cell`, `\row` | `table`; `\clmgf`/`\clmrg` and `\clvmgf`/`\clvmrg` become `Merges`; leading `\trhdr` rows are `HeaderRows`; several paragraphs in a cell are joined by `\n` |
| `\itapN`, `\nestcell`, `\nestrow` | a nested table is written into the cell that holds it, a line per row, cells separated by tabs |
| `\footnote` | `footnote` after its paragraph, paragraphs joined by `\n`; `Marker` is its number (`\chftn`) in the document's format (`\ftnnar`, `\aftnnrlc`, ...): arabic for footnotes, lower-case roman for endnotes (`\ftnalt`) unless the document says otherwise |
| `\annotation` with `\atnauthor`/`\atnid`, `\atndate` | `note` of type `comment`, with `author` and `date` |
| `\deleted` | left out of the text and kept as a `note` of type `deletion` with `author` (`\revauthdel` and the revision table) and `date` (`\revdttmdel`); `text.tracked_changes` |
| `\field` | the result (`\fldrslt`) is in the text; the instruction (`\fldinst`) is a `note` of type `field`, with no display reason and `Attrs["stored"] = "field_code"` (Markdown and text leave it out unless `-stored` or `-stored=field_code`, as for .docx); `\xe`, `\tc` and `\tcn` entries are such notes too (`XE entry`, `TC entry`); a field nested in an instruction is written into it as `{ ... }` |
| `HYPERLINK` field | with `Options.Spans`, a span with `Href` over the result (`\l` gives `#anchor`); in a table cell, `Table.Cells[].Href` |
| `\pict` | `image` after its paragraph: `Image.ID` `pict-N`, MIME from `\pngblip`, `\jpegblip`, `\emfblip`, `\wmetafile`, `\macpict`, `\dibitmap`, `\wbitmap`; pixel size from the PNG or JPEG header, or `\picw`/`\pich` for bitmaps; alt text from `wzDescription` of `\picprop` or of the shape. `\nonshppict` and `\shprslt` (copies for old readers) are skipped |
| `\shptxt`, `\dptxbxtext` | the text of a text box (of a shape, or of a Word 6/95 drawing object `\do`): its blocks follow the paragraph the shape is in |
| `\object` | the object's `\result` is read as content; the embedded data (`\objdata`, an OLE 1.0 stream) is handed to `Options.Child`, unpacked from a `Package` to the file it packages; linked objects are listed, not read |
| an empty paragraph whose only border is a bottom border with a style (`\brdrb\brdrs`, ...) | `thematic_break`, `Origin` `inferred`: RTF has no horizontal-rule control, and this is how a rule is drawn. Not when the paragraph before or after it has the same borders (a box), nor in table cells, headers, footers, notes or text boxes, nor for a paragraph that holds `\page` or `\sect`. A bordered paragraph with text, and a `* * *` paragraph, stay paragraphs |
| `\fromhtml1` (HTML encapsulated by Outlook, [MS-OXRTFEX]) | the HTML is de-encapsulated (the `\*\htmltag` groups and the text outside `\htmlrtf` runs; `\par` and `\line` as CRLF, `\tab` as a tab, `\'hh` as that byte, `\uN` as UTF-8 under `\ansicpg65001` and as `&#N;` otherwise) and handed to `Options.Child` as `body.html`, `text/html; charset=` the WHATWG label of `\ansicpg`; it is the first of `Children`. When the child comes back extracted, the body gives no blocks (the HTML is the message; the RTF only repeats it); otherwise the RTF is read as usual. This library does not read HTML: the caller's `Options.Child` must, as go-fast-email's command does; `gofastrtf` does not, so it shows the RTF reading. `\fromtext` is read as RTF |
| `\header*`, `\footer*` | `header`, `footer` blocks where they stand |
| `\*\formfield` with `\*\ffl` | `note` of type `choices`, one value per line, `Attrs["field"]` from `\*\ffname`; it follows the paragraph |
| `\*\bkmkstart` at the start of a paragraph | `Loc.Anchor` |
| `\info` | `Metadata`: `title`, `subject`, `author` (one entry), `keywords` (one entry), `creatim`, `revtim`; `operator`, `company`, `manager`, `category`, `comment`, `doccomm`, `hlinkbase`, `printim` (`printed`), `buptim` (`backed_up`) in `Extra`; `\deflang` gives `Language` for common languages |
| `\*\generator` | `Metadata.Application`, and a stored block |

Text is decoded through `\uN` (with the `\ucN` fallback bytes skipped,
surrogate pairs joined) and `\'hh` through the code page of the current font
(`\cpgN`, else `\fcharsetN`) or of the document (`\ansicpgN`, `\ansi`,
`\mac`). Single-byte code pages 874, 1250 to 1258 and Mac Roman (10000) are
decoded, UTF-8 (65001), and the double-byte code pages 932 (Japanese), 936
(Simplified Chinese), 949 (Korean) and 950 (Traditional Chinese), in `\'hh`
escapes and in raw 8-bit bytes alike; a lead byte and its trail byte may
come in two escapes. The user-defined areas, which the tables leave out,
are computed as Windows maps them: in 932, lead bytes 0xF0 to 0xF9 to
U+E000 to U+E757, and byte 0x80 alone is U+0080; in 936, 0xAAA1 to 0xAFFE,
0xF8A1 to 0xFEFE and 0xA140 to 0xA7A0 to U+E000 to U+E765. A pair the table does not map, and a lead byte with
no trail byte, are one U+FFFD each, and `text.unmapped_glyphs` names the
code page. Other code pages (437, 850, Johab, ...) are not decoded. The ligatures
U+FB00 to U+FB06 (Mac Roman has two) are written as their letters.

Destinations marked `\*` that this library does not know are skipped, as the
specification asks. So are theme data, the colour scheme map, the data
store, latent styles, rsid tables and the like, which hold no text.

## What the file declares

Everything the file contains is extracted. A block, span or table region
carries a display reason (`Block.Display`, JSON `display`) only where the
file states outright that its text is not shown. RTF declares this:

| Reason | Declared by |
|---|---|
| `hidden` | `\v` (hidden text) on the run, or set by a paragraph style (`\sN`) or a character style (`\csN`) of the `\stylesheet`, followed through `\sbasedon`. The style's `\v` applies where `\s` or `\cs` stands; `\v0` written after it wins, and `\plain` ends it. Also the shape property `fHidden` set to true (`{\sp{\sn fHidden}{\sv 1}}`, wherever it stands in the shape): every block of the shape's text box (`\shptxt`) and its pictures carry `hidden` on the whole block, and so do those of the shapes in a hidden group (`\shpgrp`). `{\sv 0}` gives nothing |
| `no_ink` | none. RTF has no control that draws text with no paint: character colour (`\cf`) is a colour-table entry with no transparency, and outline, emboss and shadow effects still draw the letters |

A hidden passage of a paragraph is a span with `display` set; a paragraph
hidden throughout carries the reason on the block. In a table, a cell that
holds hidden text is a `Table.Display` region. A comment's or footnote's
hidden passages are spans like a paragraph's. `RenderOptions.ExcludeDisplay`
(the command's `-exclude-display`) leaves such text out of Markdown and
plain text.

Not reported, because the file does not declare them: the colour of text
against what is behind it, the size of text (`\fs0` included), where a
shape is placed and what it covers. Colour, size, position and overlap are
not judged. Nor is `\webhidden`, which hides text only in Word's Web Layout
and on web pages, a condition: Word puts it on the page numbers of every
table of contents, which the document shows.

Data the file keeps without showing it has no reason either, and stays
where its provenance puts it: field instructions, and index and
table-of-contents entries (`\xe`, `\tc`, `\tcn`), are `note` blocks of type
`field` after their paragraph, written as Word writes the field
(`XE entry`, `TC entry`), each with `Attrs["stored"] = "field_code"`, so
that Markdown and text leave them out unless stored data of that kind is
asked for, as go-fast-ooxml does for .docx; tracked deletions are `note` blocks of type
`deletion`, the text being given as if the changes were accepted; document
properties are metadata; the rest is in the stored part.

Characters that display as nothing (zero-width characters, Unicode tag
characters, bidirectional controls, variation selectors) are kept in the
text as the file has them.

The library reports what the file declares about its content; it does not
judge whether text is visible or harmful, and the absence of a reason says
nothing about either. Threat detection is outside this library.

### The stored part

What belongs to the file and is shown on no page goes to the `stored` part,
which carries no reason (the renderers leave it out unless
`RenderOptions.Stored` or `RenderOptions.StoredKinds`, the command's
`-stored[=kinds]`), each block with `Attrs["stored"]`. Field notes in the
body carry `Attrs["stored"] = "field_code"` and are left out the same way.
In the stored part:

| `stored` | What |
|---|---|
| `generator` | `\*\generator`, the program that wrote the file |
| `document_variable` | `\*\docvar {name}{value}`, as `name: value`, with `Attrs["name"]` |
| `custom_property` | `\*\userprops`: `\propname` and `\staticval`, as `name: value` |
| `after_end` | text after the closing brace of the document. The part carries `input.trailing_data`, whose message gives the number of bytes after the brace. White space, NUL bytes and `0x1A` (Ctrl-Z, the DOS end-of-file mark) alone there are not data: they give neither a block nor the warning. Any other byte counts, however few |

## Warnings

Besides the shared codes (`limit.exceeded`, `part.no_text`,
`part.image_only`, `child.*`):

| Code | Severity | What happened |
|---|---|---|
| `input.repaired` | info | the file ends inside open groups; it was read as far as it goes |
| `input.external_reference` | warning | a linked object, an `INCLUDEPICTURE`, `INCLUDETEXT`, `INCLUDE`, `LINK` or `IMPORT` field, a remote `\*\template`; nothing was fetched |
| `input.active_content` | warning | a `DDE` or `DDEAUTO` field, an ActiveX control (`\objocx`); nothing was run |
| `text.tracked_changes` | info | the document has insertions (`\revised`) or deletions (`\deleted`) |
| `text.unmapped_glyphs` | warning | bytes in a code page that is not decoded, bytes their code page leaves undefined, unpaired surrogates |
| `image.unsupported_encoding` | warning | with `ImagesData`, a picture in a format whose bytes cannot be given as a file (device-dependent bitmap, OS/2 metafile, no format named) |
| `input.trailing_data` | info | data follows the document's closing brace; the message gives how many bytes. Its text is in the stored part as `after_end`. On the document when there is no stored part to carry it |
| `rtf.unread_content` | warning | content this version does not read: equations (`\mmath`) |
| `rtf.control_characters_removed` | info | control characters with no meaning as text were removed |

## Limits

| Limit | Default | What it bounds |
|---|---|---|
| `Limits.MaxInputBytes` | 1 GiB | the file; larger is `ErrLimitExceeded` |
| `Limits.MaxTextBytes` | 256 MiB (core) | text of blocks, cells, markers and link targets, with `BlockCost` and `CellCost`, shared with embedded documents; the body is cut off at the limit with `limit.exceeded` |
| `Limits.MaxDecodedBytes` | 1 GiB | picture bytes kept with `ImagesData`, embedded object data and encapsulated HTML handed to `Options.Child`, shared with embedded documents; past it, pictures have no data and objects are listed, not read |
| `Extractor.MaxGroupDepth` | 1024 | nesting of groups; deeper groups are not read, with `limit.exceeded` |
| `Limits.MaxDepth` | 4 (core) | nesting of embedded documents |

Smaller fixed bounds: a field instruction holds 32 KiB, a metadata item
64 KiB, a document variable's value 64 KiB, 16,384 document variables and
as many custom properties, the text after the document's end
1 MiB; a table has at most 1,048,576 rows and a row definition 4096 cells.
Without `ImagesData`, at most 64 KiB of a picture is decoded, to read its
size.

## What is not read

- Code pages other than those listed above (437, 850, Johab 1361, ...).
- List numbers are not computed: the marker is what `\listtext` or
  `\pntext` says, which Word always writes.
- Equations (`\mmath`), the default text and help text of form fields,
  drawing objects other than text boxes and pictures, `\pmmetafile` data.
- Bold, italic and other inline styles set only by a style are not given as
  spans (a style's `\v` is read; see above).

## The command

```
go install github.com/AOShei/go-fast-rtf/cmd/gofastrtf@latest
gofastrtf report.rtf                       # JSON
gofastrtf -format markdown -exclude-display docs/
```

`-images`, `-spans`, `-max-group-depth`, `-max-decoded-bytes`,
`-max-text-bytes`, `-quiet`, `-o`; `gofastrtf -h` lists them. The render
flags are the same in every go-fast command and affect Markdown and text
only; JSON always has everything:

| Flag | Effect |
|---|---|
| `-furniture` | in Markdown and text, include running headers and footers |
| `-stored[=kinds]` | in Markdown and text, include stored data (data the file keeps but shows nowhere): all of it (`-stored`, `=true`, `=all`), or with =kinds only those kinds, comma- or `+`-separated (core's: `field_code`, `document_variable`, `after_end`, ...; this library's own: `generator`, `custom_property`). `=false` or `=` turn it off; an unknown kind is an error listing the kinds, exit status 2 |
| `-part-markers` | in Markdown and text, put a line before each part saying which part it is |
| `-children` (default true) | in Markdown and text, include embedded documents (here, objects the command reads: embedded RTF files), each after a line naming it (`[child 1: name]` in text, `<!-- child 1: name -->` in Markdown); a child that was not read gives only that line; `-children=false` leaves them out |
| `-exclude-display[=reasons]` | in Markdown and text, leave out what the file declares not displayed: all of it (alone, `=true` or `=all`), or with `=reasons` only those reasons (`hidden`, `no_ink`, joined by `,` or `+`); `=false` or `=` turns it off, and any other value, `unknown` included, is an error |

Messages go to standard error; the exit status is 1 if a file could not be
read.

## Layout

| Path | What |
|---|---|
| `extract.go` | `Extractor`, `Extract`, `ExtractFile`, warning codes, limits |
| `internal/token` | the tokenizer |
| `internal/codepage` | code page tables and decoder |
| `internal/htmlrtf` | de-encapsulation of HTML (`\fromhtml1`) |
| `internal/reader` | groups, destinations and the blocks they become |
| `cmd/gofastrtf` | the command |

## Running the tests

```
go test -short ./...
go test -run XXX -fuzz FuzzExtract -fuzztime=20s .
go test -run XXX -fuzz FuzzLexer -fuzztime=20s ./internal/token
go test -run TestCorpus -v .                  # with the public corpus
go test -run XXX -bench BenchmarkExtract .    # with the public corpus
```

Every fixture is written by the tests themselves. `TestCorpus` and
`BenchmarkExtract` read the series' public corpus (`GOFAST_CORPUS`, or
`../.testdata/corpus`) and are skipped without it; no corpus file is in
this repository.

## Licence

MIT. The code page tables are the content of the Unicode Consortium's
mapping files; see [`NOTICE`](NOTICE).
