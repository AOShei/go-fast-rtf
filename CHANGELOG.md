# Changelog

## v0.1.0 (not yet tagged)

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
