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
| `\field` | the result (`\fldrslt`) is in the text; the instruction (`\fldinst`) is a `note` of type `field`, flagged `invisible`; a field nested in an instruction is written into it as `{ ... }` |
| `HYPERLINK` field | with `Options.Spans`, a span with `Href` over the result (`\l` gives `#anchor`); in a table cell, `Table.Cells[].Href` |
| `\pict` | `image` after its paragraph: `Image.ID` `pict-N`, MIME from `\pngblip`, `\jpegblip`, `\emfblip`, `\wmetafile`, `\macpict`, `\dibitmap`, `\wbitmap`; pixel size from the PNG or JPEG header, or `\picw`/`\pich` for bitmaps; alt text from `wzDescription` of `\picprop` or of the shape. `\nonshppict` and `\shprslt` (copies for old readers) are skipped |
| `\shptxt` | the text of a text box: its blocks follow the paragraph the shape is in |
| `\object` | the object's `\result` is read as content; the embedded data (`\objdata`, an OLE 1.0 stream) is handed to `Options.Child`, unpacked from a `Package` to the file it packages; linked objects are listed, not read |
| an empty paragraph whose only border is a bottom border with a style (`\brdrb\brdrs`, ...) | `thematic_break`, `Origin` `inferred`: RTF has no horizontal-rule control, and this is how a rule is drawn. Not when the paragraph before or after it has the same borders (a box), nor in table cells, headers, footers, notes or text boxes, nor for a paragraph that holds `\page` or `\sect`. A bordered paragraph with text, and a `* * *` paragraph, stay paragraphs |
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
come in two escapes. A pair the table does not map, and a lead byte with
no trail byte, are one U+FFFD each, and `text.unmapped_glyphs` names the
code page. Other code pages (437, 850, Johab, ...) are not decoded. The ligatures
U+FB00 to U+FB06 (Mac Roman has two) are written as their letters.

Destinations marked `\*` that this library does not know are skipped, as the
specification asks. So are theme data, the colour scheme map, the data
store, latent styles, rsid tables and the like, which hold no text.

## Hidden text

Hidden text is extracted like any other and flagged with the reason it is
not seen; the part carries one `text.hidden` warning. All checks are cheap
and always run; `Options.DeepHidden` changes nothing.
`Document.HiddenChecked` is `invisible+tiny+low_contrast+invisible_chars`.

| Reason | When |
|---|---|
| `invisible` | `\v` (hidden text), `\webhidden`; index and table-of-contents entries (`\xe`, `\tc`, `\tcn`), which are not shown where they stand; field instructions; everything in the stored part |
| `tiny` | `\fsN` below 2 points (N < 4), including `\fs0` |
| `low_contrast` | the text colour (`\cf`) against what is behind it, judged with `gofastcore.LowContrast`: the first set of `\highlight`, `\chcbpat`, `\cb`, the paragraph's `\cbpat`, the page colour (the `fillColor` of `\*\background`), else white. Text in the automatic colour is never flagged |
| `invisible_chars` | core's scan, on every block |

These properties count whether they are written on the run or set by a
style: a paragraph style (`\sN`) or a character style (`\csN`) of the
`\stylesheet`, followed through `\sbasedon`, that sets `\v`, `\webhidden`,
`\cf`, `\cb`, `\highlight`, `\chcbpat` or `\fs`. The style's properties are
applied where `\s` or `\cs` stands; properties written after it win, and
`\plain` resets them.

In a table, a cell that holds hidden text is a `Table.Hidden` region with its
reasons. A comment's or footnote's hidden passages are spans like a
paragraph's.

Tracked deletions are not flagged hidden: as in go-fast-ooxml they are
`note` blocks of type `deletion`, and the text is given as if the changes were
accepted.

What `invisible` and the others cannot see: shapes positioned off the page,
text covered by pictures, text inside pictures, and table cell shading.

### What the file stores and never shows

These go to the `stored` part (flagged `invisible`), each with
`Attrs["stored"]`:

| `stored` | What |
|---|---|
| `generator` | `\*\generator`, the program that wrote the file |
| `document_variable` | `\*\docvar {name}{value}`, as `name: value`, with `Attrs["name"]` |
| `custom_property` | `\*\userprops`: `\propname` and `\staticval`, as `name: value` |
| `trailing_data` | text after the closing brace of the document, which no program shows |

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
| `rtf.unread_content` | warning | content this version does not read: equations (`\mmath`) |
| `rtf.control_characters_removed` | info | control characters with no meaning as text were removed |

## Limits

| Limit | Default | What it bounds |
|---|---|---|
| `Limits.MaxInputBytes` | 1 GiB | the file; larger is `ErrLimitExceeded` |
| `Limits.MaxTextBytes` | 256 MiB (core) | text of blocks, cells, markers and link targets, with `BlockCost` and `CellCost`, shared with embedded documents; the body is cut off at the limit with `limit.exceeded` |
| `Limits.MaxDecodedBytes` | 1 GiB | picture bytes kept with `ImagesData` and embedded object data handed to `Options.Child`, shared with embedded documents; past it, pictures have no data and objects are listed, not read |
| `Extractor.MaxGroupDepth` | 1024 | nesting of groups; deeper groups are not read, with `limit.exceeded` |
| `Limits.MaxDepth` | 4 (core) | nesting of embedded documents |

Smaller fixed bounds: a field instruction holds 32 KiB, a metadata item
64 KiB, a document variable's value 64 KiB, the text after the document's end
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
  spans (the hidden-text properties of styles are read; see above).

## The command

```
go install github.com/AOShei/go-fast-rtf/cmd/gofastrtf@latest
gofastrtf report.rtf                       # JSON
gofastrtf -format markdown -exclude-hidden docs/
```

`-images`, `-spans`, `-max-group-depth`, `-max-decoded-bytes`,
`-max-text-bytes`, `-furniture`, `-quiet`, `-o`; `gofastrtf -h` lists them.
Messages go to standard error; the exit status is 1 if a file could not be
read.

## Layout

| Path | What |
|---|---|
| `extract.go` | `Extractor`, `Extract`, `ExtractFile`, warning codes, limits |
| `internal/token` | the tokenizer |
| `internal/codepage` | code page tables and decoder |
| `internal/reader` | groups, destinations and the blocks they become |
| `cmd/gofastrtf` | the command |

## Running the tests

```
go test -short ./...
go test -run XXX -fuzz FuzzExtract -fuzztime=20s .
go test -run XXX -fuzz FuzzLexer -fuzztime=20s ./internal/token
```

Every fixture is written by the tests themselves; there is no downloaded
corpus.

## Licence

MIT. The code page tables are the content of the Unicode Consortium's
mapping files; see [`NOTICE`](NOTICE).
