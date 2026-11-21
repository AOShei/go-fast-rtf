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
// deletions and field instructions are note blocks; Word's horizontal line
// (the shape property fHorizRule) is a thematic_break with Origin native.
//
// Everything the file contains is extracted. RTF's display declarations,
// hidden text (\v, on the run or through a style) and a shape's fHidden,
// are reported as gofastcore.DisplayHidden; nothing else about visibility
// is judged.
//
// A document that declares \fromhtml1 (HTML mail as Outlook stores it,
// [MS-OXRTFEX]) has its HTML de-encapsulated and handed to Options.Child
// as HTMLName (body.html), with the MIME type text/html and, as its
// charset, the WHATWG label of the document's code page (\ansicpg, or
// 1252 when none is named) when that code page has one. It is the first
// of the document's children. When the child comes back with blocks, the
// body gives none; otherwise the RTF is read as usual. This library does
// not read HTML itself.
package gofastrtf
