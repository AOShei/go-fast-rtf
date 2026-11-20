# Declared, not judged: go-fast-rtf

The go-fast-rtf part of `HANDOFF_DECLARED_NOT_JUDGED.md` (part 7), following `DECLARED_HANDOFF_RTF.md` and `DECLARED_REPORT_CORE.md`. It belongs in the workspace root. I could not write there, so it is committed in the repository root; please move it.

## 1. Branch and core

- Branch `declared-not-judged`, local only (no remote, no pull request). The commits on top of `a8702ff` are `8a5228f` (the change), `cbc604e` (plan wording, flag type name, test tidy) and the commit adding this report.
- Pins go-fast-core **`05bbddb`** (`v0.0.0-20261120081927-05bbddb16b9f`), schema 0.2.

## 2. Declarations reported (as in the README)

| Reason | Declared by |
|---|---|
| `hidden` | `\v` (hidden text) on the run, or set by a paragraph style (`\sN`) or a character style (`\csN`) of the `\stylesheet`, followed through `\sbasedon`. `\v0` written after the style wins; `\plain` ends it |
| `no_ink` | none. RTF has no control that draws text with no paint: `\cf` is a colour-table entry with no transparency, and outline, emboss and shadow still draw the letters |

The README says that colour, size, position and overlap are not judged. Also not reported: `\webhidden` (a Web Layout condition), field instructions and `\xe`/`\tc`/`\tcn` entries (`field` notes), tracked deletions (deletion notes), stored data. Data after the closing brace goes in the stored part as `after_end`, with no reason. The stored part carries `input.trailing_data` with the byte count; when part 2 is not given, the document carries it.

## 3. Removed

- Checks: `tiny` (`\fs` under 2 pt); `low_contrast`, with the colour table, `\cf`/`\cb`/`\highlight`/`\chcbpat`, `\cbpat`, `\clcbpat`, text-box fill and page background reading; the invisible-character scan of metadata; the outlier rule (field instructions, index entries, stored items), with its address exemption.
- API and output: `gofastrtf.HiddenChecked`, `hidden_checked`, `text.hidden`, `text.unshown_outlier`. Command: `-exclude-hidden` became `-exclude-display`, and `-stored` was added.
- Kept: budget bounds, the custom-property limit, Word 6/95 drawing-object text.
- Size, `a8702ff..HEAD`: library code +137/−400 lines; tests +286/−308 (the inferred tests were replaced by `display_test.go`); docs +112/−93. A new `CHANGELOG.md` records the breaking change.

## 4. Corpus, before (`a8702ff`) and after

| Set | Before | After |
|---|---|---|
| dev (70 files, all ok) | files: invisible 2, tiny 1; blocks: invisible 2, tiny 23; `text.hidden` ×3 | no block with a reason; `input.trailing_data` ×2 |
| heldout (24 files, all ok) | files: invisible 1; blocks: invisible 1; `text.hidden` ×1 | no block with a reason; `input.trailing_data` ×2 |

The 23 `tiny` blocks in `tika-testRTFTIKA_2899` are gone. The two dev trailing-text files now have `after_end` and the warning. No corpus file has `\v` text, so no `hidden` reason appears.

In held-out, 2 files get the warning but only 1 had a flagged trailing block before. The likely cause is a tail with no text (control words or a stray brace), which gives the warning and no stored block. I have not checked this, because held-out files are not inspected.

There were no panics, Validate failures or timeouts.

## 5. Benchmark (`BenchmarkExtract`, ns/op, least of 5 runs × 200)

| File | Before | After |
|---|---|---|
| tika-testRTFVarious | 386,369 | 385,640 |
| tika-testRTFTIKA_2899 | 1,540,683 | 1,339,064 (−13%) |
| tika-testRTFListMicrosoftWord | 256,287 | 252,663 |

These are wall-clock figures; there is no instruction counter here, so there is no instr/op. Allocations fell slightly (TIKA_2899: 5712 → 5664 allocs/op).

## 6. R6

`TestCharactersKept` (`display_test.go`) covers tag characters, U+202E/U+202C, a run of zero-width characters, U+FEFF and variation selectors. They survive unchanged, with no reason and no warning, in:

- body text, a table cell and a field instruction;
- the title and a stored document variable;
- the output of the control words `\zwbo`, `\zwnbo`, `\zwj`, `\zwnj`, `\rtlmark` and `\ltrmark`.

Other tests:

- `TestDeclaredHidden`: every `\v` mechanism.
- `TestNotReported`: white on white, `\fs2`, `\fs0`, cell shading, a text-box fill, `\webhidden` and long field instructions all give no reason.
- `TestCleanDocumentNoReason`.
- `TestAfterEnd`.

## 7. For Andrew

- **Trailing-data threshold:** white space and NUL bytes after the brace do not count as data. Anything else counts, including a stray `}` or control words with no text: that gets the warning but no stored block. Please confirm.
- **Shape `fHidden`:** a drawing shape's `fHidden` property is arguably a declaration. It is not reported now and is listed as open in the plan. Should it map to `hidden`?
- **Process:** the worktree's corpus test could not be pointed at the corpus (setting an environment variable, a symlink and an edit outside the working directory were all refused). The "before" corpus and benchmark figures were taken by switching the main checkout to a detached `a8702ff` for a moment and then switching back. The worktree was created and removed as asked.
