# Changelog

## Unreleased

- **go-fast-core v0.1.10** (was v0.1.4). Of the core code this module
  uses, the only change since v0.1.4 is two new stored kinds: `gofastrtf
  -stored=` now also accepts `shape_data` and `linked_data`, which Visio
  drawings in go-fast-ooxml will use.

## v0.2.1 (2026-11-24)

- **go-fast-core v0.1.4** (was v0.1.2): `Document.Text` and `Markdown`
  allocate their result once. The output is unchanged.
- **Benchmark** against the RTF tools in use today (`bench/`), and
  **`GOGC` documented** (README): reading at 100, 200 and 400. The
  library keeps Go's default; trading memory for speed is the caller's
  choice.

## v0.2.0 (2026-11-23)

- **Shared warning codes** (go-fast-core v0.1.2). `rtf.unread_content`
  is now `input.unread_content`, and `rtf.control_characters_removed` is
  `text.control_characters_removed`, as every go-fast library raises
  them. The JSON code strings change; the Go names `WarnUnreadContent`
  and `WarnControlCharsRemoved` are deprecated and have the new values,
  so code that compares with them keeps working. The control-character
  message gives the count in words ("3 control characters with no
  meaning as text were removed from the text"), not the number alone;
  the unread-content message begins "content this version does not
  read:". Requires go-fast-core v0.1.2.

## v0.1.2 (2026-11-22)

- A comment date (`\atndate`) longer than 32 bytes, whose number is then
  cut, gives `limit.exceeded` on the body.
- The label of a packaged file (an OLE `Package`), which names the
  child, is cut at 4 KiB with `limit.exceeded` on the document. In
  v0.1.1 a label or original path longer than 4 KiB stopped the
  unpacking: the child was the whole package, named by `\objname`.
- A PNG's width and height are given up to the largest the PNG
  specification allows; in v0.1.1 sizes above 16,777,216 pixels were
  given as unknown.
- `\ucN` above 16 is read as it is: in v0.1.1 it was taken as 16, and
  the fallback bytes past the 16th of each `\uN` were given as text.
- README: the bounds on data used only inside the library are listed.

## v0.1.1 (2026-11-22)

- Built on go-fast-core v0.1.1: a part or document keeps at most 1,000
  warnings, and the rest are counted in one `warnings.omitted` warning.
- The fixed bounds of this library that cut or leave out content now say
  so, with one `limit.exceeded` warning (severity `error`) for each bound
  on the part that lost the content, giving how many items were cut or
  left out and, for text, how many bytes. In v0.1.0 they cut silently. On
  the body: field instructions and index entries (32 KiB), tracked
  deletions (1 MiB), comment authors (1 KiB), form field names and
  choices (4 KiB) and choices past 1,024 in one field, picture and shape
  descriptions (64 KiB), bookmark names and list markers (256 bytes),
  revision authors (512 bytes) and past 16,384 of them, fonts, styles,
  lists and list overrides past 65,536 each, fields nested deeper than
  256, table rows past 1,048,576 in one table, and cell definitions
  (`\cellx`) past 4,096 in one row (the cells' text was and is read;
  their merges are not). On the stored part, or on the document
  when there is no stored part: the generator (4 KiB), document variable
  and custom property names (1 KiB) and values (64 KiB), document
  variables and custom properties past 16,384, and the text after the
  document's end (1 MiB). On the document: metadata items (64 KiB) and
  embedded object names (1 KiB) and classes (256 bytes).
- `text.unmapped_glyphs` is one warning for the body, however many code
  pages it concerns: with more than one, the message gives the total,
  what happened in each of the first eight code pages and the number of
  the rest. In v0.1.0 there was one warning per code page, without bound.
  With one code page the message is unchanged. The first 64 code pages
  met are counted one by one and the rest together, so the count takes
  bounded memory.
- The text limit reached in the stored data (`limit.exceeded`, "the
  stored data is cut off there") is on the document when the stored part
  is not given; in v0.1.0 it was lost with the part. The body no longer
  says it is cut off when only the stored data reached the limit.
- Text buffers hold exactly their bound: in v0.1.0 they stopped up to 3
  bytes short of it.
- The value of a custom property past the 16,384th is no longer added to
  the value of the last one kept.

## v0.1.0 (2026-11-21)

The first release. Built on go-fast-core v0.1.0, schema 0.2.

- Reads RTF (specification 1.9.1) into one `body` part, and a `stored`
  part for data the file keeps but never shows.
- Decodes `\uN`, and `\'hh` and 8-bit text through code pages 874, 1250 to
  1258, Mac Roman (10000), UTF-8 (65001) and the double-byte 932, 936, 949
  and 950, with the user-defined areas of 932 and 936 as Windows maps
  them.
- Gives headings, lists, tables with merges and header rows, footnotes,
  comments, tracked deletions, field instructions as stored `field_code`
  notes, hyperlinks, pictures, text boxes, headers and footers, form-field
  choices, metadata, and Word's horizontal line as a native
  `thematic_break`.
- Reports only the display the file declares: `hidden` for `\v` (direct
  or through a style) and for a shape's `fHidden`.
- Hands embedded objects (`\objdata`, unpacked from a `Package`) to
  `Options.Child`, and the HTML of a `\fromhtml1` document as `body.html`.
- Stored data, with core's kinds: `field_code`, `document_variable`,
  `after_end`, `generator` and `custom_property` (the last two also as
  `StoredGenerator` and `StoredCustomProperty` here).
- The command `gofastrtf` prints JSON, Markdown or text (`-format`), with
  `-images`, `-spans`, `-max-group-depth`, `-max-decoded-bytes`,
  `-max-text-bytes`, `-quiet`, `-o`, `-version` and the series' render
  flags `-furniture`, `-stored[=kinds]`, `-part-markers`, `-children` and
  `-exclude-display[=reasons]`.

## Before v0.1.0

### Core v0.1.0

- go-fast-core pinned at its v0.1.0 tag. Nothing changes here: since
  `4499357` core changed documentation, and `TextBudget.Take` now takes a
  negative amount as 0, which this library never passes.

### Core 4499357: closed value sets, shared render flags

- go-fast-core pinned at `4499357`. `generator` and `custom_property` are
  now core's stored kinds (`gofastcore.StoredGenerator`,
  `StoredCustomProperty`); `StoredGenerator` and `StoredCustomProperty`
  here are the same values. `StoredKindNames` is removed: core's
  `StoredKindNames()` lists every kind.
- Command: the render flags are core's `AddRenderFlags`; their names,
  values and errors are unchanged, and `-stored`'s help text gives core's
  examples.

Development history, newest first. The core pins named are those of the
time.

### Core d4cd8e2

- go-fast-core pinned at `d4cd8e2`: `Validate` now rejects an inferred
  `thematic_break`; this library emits only declared ones and passes.

### No inferred thematic breaks

- **Removed:** the `thematic_break` (`Origin` `inferred`) given for an
  empty paragraph whose only border is a styled bottom border. A border is
  not a declared break; such a paragraph now gives no block, as any empty
  paragraph. Paragraph border words are no longer tracked.
- **Added:** Word's horizontal line, the shape property `fHorizRule` (on
  a shape, or in an inline picture's `\picprop`), gives a `thematic_break`
  with `Origin` `native` in place of the line's picture, as go-fast-ooxml
  reads Word's `o:hr`.

### Exported stored kinds

- **Added:** `StoredGenerator`, `StoredCustomProperty` and `StoredKindNames`,
  the `Attrs["stored"]` values this library writes beyond core's, for
  commands that take `-stored=kinds` (go-fast-email's renders RTF
  children). `TestStoredKindNamesComplete` checks the list against what
  the library writes.

### Field codes as stored data

Built on go-fast-core `ebecf3c`.

- **Changed:** every field note (`\fldinst` instructions and `\xe`, `\tc`,
  `\tcn` entries) carries `Attrs["stored"] = "field_code"`, as
  go-fast-ooxml does and core's `Validate` now requires. Markdown and text
  have the field results and leave the instructions out unless asked for,
  so a Word document gives the same text from .rtf as from .docx.
- **Command:** `-stored[=kinds]`: alone, `=true` or `=all` for every kind;
  `=false` or `=` off; `=k1,k2` (or `k1+k2`) for those kinds only (core's
  `StoredKindNames` plus `generator` and `custom_property`); anything else
  is an error listing the kinds, exit status 2.

### Encapsulated HTML

- **Added:** a document that declares `\fromhtml1` (HTML mail as Outlook
  stores it, [MS-OXRTFEX]) has its HTML de-encapsulated and handed to
  `Options.Child` as `body.html`, `text/html; charset=<WHATWG label of
  \ansicpg>`, first in `Children`. When the child is extracted, the body
  gives no blocks; otherwise (no `Options.Child`, or it does not read
  HTML) the RTF is read as before. The `\htmlrtf` runs no longer reach
  the output then, nor the `text.unmapped_glyphs` they raised (the
  `\'a0` bytes under `\ansicpg65001` in Outlook's files). The HTML counts
  against `Limits.MaxDecodedBytes`. `gofastrtf` reads no HTML: its
  body is the RTF reading as before, and `body.html` is listed as a child
  that was not read.

### User-defined areas of code pages 932 and 936

- **Fixed:** code page 932 decodes byte 0x80 as U+0080 and its
  user-defined area (lead bytes 0xF0 to 0xF9) as U+E000 to U+E757; code
  page 936 decodes its three user-defined areas (0xAAA1 to 0xAFFE, 0xF8A1
  to 0xFEFE, 0xA140 to 0xA7A0) as U+E000 to U+E765, as Windows does. These
  were U+FFFD with `text.unmapped_glyphs` before. 949 and 950 unchanged.

### Declared, not judged

Built on go-fast-core `787b056`, schema `0.2`.

The library now reports only what an RTF file declares about display, and
no longer judges whether text can be seen.

- **Renamed (core):** `Block.Hidden`, `Span.Hidden`, `Table.Hidden` are
  `Display`; JSON `"hidden"` is `"display"`. The one reason RTF declares is
  `hidden` (formerly `invisible`): `\v`, on the run or through a paragraph
  or character style. Also `hidden`: the text box and pictures of a shape
  whose property `fHidden` is true (`{\sp{\sn fHidden}{\sv 1}}`), and of
  the shapes in such a group.
- **Removed checks:** `tiny` (`\fs` under 2 points); `low_contrast` (the
  text colour against highlight, character and paragraph shading, table
  cell shading, the fill of a text box and the page background), with the
  colour table, shading and fill reading it needed; the invisible-character
  scan; the out-of-proportion rule for field instructions, index entries
  and stored data, with its address exemption.
- **Removed API and output:** `gofastrtf.HiddenChecked` and the document's
  `hidden_checked`; the warnings `text.hidden` and `text.unshown_outlier`.
- **Changed:** text after the document's closing brace is in the stored
  part with `Attrs["stored"] = "after_end"` (formerly `trailing_data`,
  flagged), with no reason, and the stored part carries
  `input.trailing_data` with the number of bytes after the brace. White
  space, NUL bytes and `0x1A` (Ctrl-Z) alone after the brace give neither.
- **Command:** `-exclude-hidden` is now `-exclude-display`; `-stored`
  includes the stored part in Markdown and text. The render flags follow
  the series' `COMMAND_FLAGS.md`: `-part-markers` is new, and
  `-exclude-display=unknown` is an error (there is no such reason to ask
  for); the help text of `-furniture`, `-stored` and `-exclude-display`
  matches the other commands. `-children` (default true) prints embedded
  documents in Markdown and text, each after a line naming it, through
  core's `RenderOptions.Children`; `-children=false` leaves them out.
- **Kept:** every bound on budgets and limits, the custom-property limit,
  the text of Word 6/95 drawing objects; nothing is dropped.
