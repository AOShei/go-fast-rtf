// Package gofastrtf extracts RTF documents into the shared document model
// of the go-fast libraries (github.com/AOShei/go-fast-core).
//
// A document has one part, the body (part 1), and after it, when the file
// stores data it never shows (document variables, custom properties, the
// name of the program that wrote it, text after the document's closing
// brace, as after_end), a stored part. Stored data carries no display
// reason.
//
//	doc, err := gofastrtf.Extract(ctx, f, size, nil)
//	if err != nil {
//		return err
//	}
//	fmt.Println(doc.Markdown())
//
// Headings come from \outlinelevel and from styles named "heading N" or with
// an outline level; list items from \listtext and \pntext, whose text is the
// marker, and \ls; tables from \trowd, \cell and \row, with merges; footnotes
// and endnotes are footnote blocks after their paragraph; comments, tracked
// deletions and field instructions are note blocks.
//
// Everything the file contains is extracted. The one display declaration
// RTF has, hidden text (\v, on the run or through a style), is reported as
// gofastcore.DisplayHidden; nothing else about visibility is judged.
package gofastrtf
