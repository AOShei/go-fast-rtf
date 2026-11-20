# Changelog

## Unreleased: declared, not judged (breaking)

Built on go-fast-core `05bbddb`, schema `0.2`.

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
  `input.trailing_data` with the number of bytes after the brace.
- **Command:** `-exclude-hidden` is now `-exclude-display`; `-stored`
  includes the stored part in Markdown and text. The render flags follow
  the series' `COMMAND_FLAGS.md`: `-part-markers` is new, and
  `-exclude-display=unknown` is an error (there is no such reason to ask
  for); the help text of `-furniture`, `-stored` and `-exclude-display`
  matches the other commands.
- **Kept:** every bound on budgets and limits, the custom-property limit,
  the text of Word 6/95 drawing objects; nothing is dropped.
