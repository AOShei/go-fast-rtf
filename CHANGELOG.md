# Changelog

## Unreleased: encapsulated HTML

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

## Unreleased: user-defined areas of code pages 932 and 936

- **Fixed:** code page 932 decodes byte 0x80 as U+0080 and its
  user-defined area (lead bytes 0xF0 to 0xF9) as U+E000 to U+E757; code
  page 936 decodes its three user-defined areas (0xAAA1 to 0xAFFE, 0xF8A1
  to 0xFEFE, 0xA140 to 0xA7A0) as U+E000 to U+E765, as Windows does. These
  were U+FFFD with `text.unmapped_glyphs` before. 949 and 950 unchanged.

## Unreleased: declared, not judged (breaking)

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
