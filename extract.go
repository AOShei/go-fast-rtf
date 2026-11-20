package gofastrtf

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/AOShei/go-fast-core"
	"github.com/AOShei/go-fast-rtf/internal/reader"
)

// Defaults for the limits of gofastcore.Options.Limits, used where a limit
// is zero. A negative limit means no limit.
const (
	// DefaultMaxInputBytes bounds the size of the file.
	DefaultMaxInputBytes = 1 << 30
	// DefaultMaxDecodedBytes bounds the bytes decoded from the hexadecimal
	// or binary data of pictures and embedded objects that are kept: the
	// pictures' bytes when Options.Images is ImagesData, and the embedded
	// objects handed to Options.Child.
	DefaultMaxDecodedBytes = 1 << 30
	// DefaultMaxGroupDepth bounds how deeply groups may nest. Groups
	// nested deeper are not read, and the body carries limit.exceeded.
	DefaultMaxGroupDepth = 1024
)

// Warning codes specific to this library. Each states a fact about the
// file. The codes shared by all go-fast libraries are in gofastcore, and
// this library raises these of them besides the ones about parts, limits
// and embedded documents:
//
//	input.active_content      a DDE or DDEAUTO field, or an ActiveX control
//	input.external_reference  a linked object, an INCLUDEPICTURE,
//	                          INCLUDETEXT, LINK or IMPORT field, a remote
//	                          template
//	input.repaired            the file ends before its groups are closed
//	text.tracked_changes      the document has tracked changes
//	text.unmapped_glyphs      bytes in a code page that is not decoded, or
//	                          with no character in their code page
//	text.hidden               text a reader cannot see; the blocks, spans
//	                          and table cells concerned say why
//	text.unshown_outlier      an item of routinely unshown data (a field
//	                          instruction, an index entry, a stored item)
//	                          is out of proportion for its kind, as
//	                          gofastcore.UnshownOutlier decides; it is
//	                          flagged invisible
//	image.unsupported_encoding
//	structure.heading_level_clamped
const (
	// WarnUnreadContent: the file holds content of a kind this version
	// does not read (an equation); the message names it.
	WarnUnreadContent = reader.WarnUnreadContent
	// WarnControlCharsRemoved: control characters with no meaning as text
	// were removed from extracted strings.
	WarnControlCharsRemoved = reader.WarnControlCharsRemoved
)

// HiddenChecked is the set of hidden-text reasons this library checks
// completely, recorded in every document's HiddenChecked.
const HiddenChecked = reader.Checked

// Extractor extracts RTF files. The zero value is ready to use and safe for
// concurrent use. Its fields are the settings that only this library has;
// everything else is in gofastcore.Options.
type Extractor struct {
	// MaxGroupDepth bounds how deeply groups may nest. Zero means
	// DefaultMaxGroupDepth; negative means no limit other than the
	// memory it takes.
	MaxGroupDepth int
}

var _ gofastcore.Extractor = (*Extractor)(nil)

// Formats lists the formats this library extracts, as they appear in
// gofastcore.Source.Format.
func (e *Extractor) Formats() []string { return []string{"rtf"} }

// Extract reads an RTF file with default settings. It is
// (&Extractor{}).Extract.
func Extract(ctx context.Context, r io.ReaderAt, size int64, opts *gofastcore.Options) (*gofastcore.Document, error) {
	return (&Extractor{}).Extract(ctx, r, size, opts)
}

// ExtractFile opens the named file and extracts it, with default settings.
func ExtractFile(ctx context.Context, path string, opts *gofastcore.Options) (*gofastcore.Document, error) {
	return (&Extractor{}).ExtractFile(ctx, path, opts)
}

// ExtractFile opens the named file and extracts it. The file name is
// recorded as the document's source name unless opts.Name is set.
func (e *Extractor) ExtractFile(ctx context.Context, path string, opts *gofastcore.Options) (*gofastcore.Document, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	o := gofastcore.Options{}
	if opts != nil {
		o = *opts
	}
	if o.Name == "" {
		o.Name = filepath.Base(path)
	}
	return e.Extract(ctx, f, info.Size(), &o)
}

// Extract reads size bytes from r and returns the document.
//
// The format is decided from the content: the file must begin, after any
// white space or byte order mark, with "{\rtf". An error wraps one of the
// gofastcore errors:
//
//   - ErrMalformed: the input is not an RTF file.
//   - ErrLimitExceeded: the input is larger than Limits.MaxInputBytes.
//   - ErrNoPartsSelected: Options.Parts does not select the body, which is
//     part 1.
func (e *Extractor) Extract(ctx context.Context, r io.ReaderAt, size int64, opts *gofastcore.Options) (*gofastcore.Document, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if opts == nil {
		opts = &gofastcore.Options{}
	}
	if size < 0 {
		return nil, fmt.Errorf("gofastrtf: negative size: %w", gofastcore.ErrMalformed)
	}
	if max := gofastcore.ResolveLimit(opts.Limits.MaxInputBytes, DefaultMaxInputBytes); size > max {
		return nil, fmt.Errorf("gofastrtf: input of %d bytes is larger than the limit of %d: %w", size, max, gofastcore.ErrLimitExceeded)
	}
	if !opts.Parts.Contains(1) {
		return nil, fmt.Errorf("gofastrtf: an RTF document has one part: %w", gofastcore.ErrNoPartsSelected)
	}
	head := make([]byte, min(size, 64))
	n, err := r.ReadAt(head, 0)
	if err != nil && err != io.EOF {
		return nil, fmt.Errorf("gofastrtf: %v: %w", err, gofastcore.ErrMalformed)
	}
	start, ok := sniff(head[:n])
	if !ok {
		return nil, fmt.Errorf("gofastrtf: not an RTF file: %w", gofastcore.ErrMalformed)
	}

	ctx, text := gofastcore.WithTextBudget(ctx, opts.Limits)
	defer text.Close()
	ctx, dec := gofastcore.WithDecodeBudget(ctx, opts.Limits, DefaultMaxDecodedBytes)
	defer dec.Close()

	depth := e.MaxGroupDepth
	switch {
	case depth == 0:
		depth = DefaultMaxGroupDepth
	case depth < 0:
		depth = 1 << 30
	}
	doc, err := reader.Read(ctx, io.NewSectionReader(r, start, size-start), reader.Config{
		Opts: opts, MaxGroupDepth: depth, Decode: dec, Text: text,
	})
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("gofastrtf: %v: %w", err, gofastcore.ErrMalformed)
	}
	doc.Source = gofastcore.Source{Format: "rtf", Name: opts.Name, MIME: "application/rtf", Size: size}
	return doc, nil
}

// sniff reports whether the input starts like an RTF file, and where the
// opening brace is.
func sniff(head []byte) (int64, bool) {
	i := 0
	if bytes.HasPrefix(head, []byte{0xEF, 0xBB, 0xBF}) {
		i = 3
	}
	for i < len(head) && (head[i] == ' ' || head[i] == '\t' || head[i] == '\r' || head[i] == '\n') {
		i++
	}
	if !bytes.HasPrefix(head[i:], []byte(`{\rtf`)) {
		return 0, false
	}
	return int64(i), true
}
