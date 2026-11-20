package gofastrtf

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/AOShei/go-fast-core"
)

// encapsulated is an RTF document that encapsulates HTML ([MS-OXRTFEX]).
const encapsulated = `{\rtf1\ansi\ansicpg1252\fromhtml1 \deff0{\fonttbl{\f0\fswiss Arial;}}` + "\r\n" +
	`{\*\htmltag19 <html>}{\*\htmltag2 \par }{\*\htmltag50 <body>}` +
	`\htmlrtf {\b \htmlrtf0 Caf\'e9 {\*\htmltag84 &nbsp;}\htmlrtf \'a0\htmlrtf0 x\par\tab y\{\}\\ ` + uw + `8364?\htmlrtf}RTFONLY\htmlrtf0 ` + "\r\n" +
	`{\*\htmltag58 </body>}{\*\htmltag27 </html>}}`

// uw is the control word \u, written so that no tool reads it as an escape.
const uw = "\\" + "u"

const encapsulatedHTML = "<html>\r\n<body>Caf\xe9 &nbsp;x\r\n\ty{}\\ &#8364;</body></html>"

// capture is a child dispatcher that records the HTML handed to it and
// returns a document with one part, or ErrUnsupported when unsupported.
type capture struct {
	name, mime  string
	html        []byte
	unsupported bool
}

func (c *capture) child(ctx context.Context, name, mime string, r io.ReaderAt, size int64, o *gofastcore.Options) (*gofastcore.Document, error) {
	if !strings.HasPrefix(mime, "text/html") {
		return nil, gofastcore.ErrUnsupported
	}
	c.name, c.mime = name, mime
	c.html = make([]byte, size)
	r.ReadAt(c.html, 0)
	if c.unsupported {
		return nil, gofastcore.ErrUnsupported
	}
	d := gofastcore.New("html")
	p := gofastcore.Part{Kind: gofastcore.PartBody, Number: 1}
	p.Blocks = append(p.Blocks, gofastcore.Block{Kind: gofastcore.KindParagraph, Text: "from html"})
	p.Finish()
	d.Parts = append(d.Parts, p)
	return d, nil
}

func TestEncapsulatedHTML(t *testing.T) {
	c := &capture{}
	doc := extract(t, encapsulated, &gofastcore.Options{Child: c.child})
	if string(c.html) != encapsulatedHTML {
		t.Errorf("html = %q, want %q", c.html, encapsulatedHTML)
	}
	if c.name != "body.html" || c.mime != "text/html; charset=windows-1252" {
		t.Errorf("name %q mime %q", c.name, c.mime)
	}
	if n := len(doc.Parts[0].Blocks); n != 0 {
		t.Errorf("body has %d blocks, want none: %+v", n, doc.Parts[0].Blocks)
	}
	if len(doc.Children) != 1 || doc.Children[0].Source.Name != "body.html" || len(doc.Children[0].Parts) != 1 {
		t.Fatalf("children = %+v", doc.Children)
	}
	for _, w := range doc.Parts[0].Warnings {
		if w.Code == gofastcore.WarnTextUnmappedGlyphs {
			t.Errorf("warning about text not read: %+v", w)
		}
	}
}

func TestEncapsulatedHTMLFallback(t *testing.T) {
	for _, opts := range []*gofastcore.Options{nil, {Child: (&capture{unsupported: true}).child}} {
		doc := extract(t, encapsulated, opts)
		var sb strings.Builder
		for _, b := range doc.Parts[0].Blocks {
			sb.WriteString(b.Text + "\n")
		}
		if !strings.Contains(sb.String(), "Caf\xc3\xa9") || !strings.Contains(sb.String(), "RTFONLY") {
			t.Errorf("fallback blocks = %q", sb.String())
		}
		if len(doc.Children) != 1 || len(doc.Children[0].Parts) != 0 {
			t.Errorf("children = %+v", doc.Children)
		}
	}
}

func TestEncapsulatedUTF8(t *testing.T) {
	c := &capture{}
	extract(t, `{\rtf1\ansi\ansicpg65001\fromhtml1 {\*\htmltag0 <p>}`+uw+`233?\'c3\'a9{\*\htmltag0 </p>}}`, &gofastcore.Options{Child: c.child})
	if want := "<p>\xc3\xa9\xc3\xa9</p>"; string(c.html) != want || c.mime != "text/html; charset=utf-8" {
		t.Errorf("html %q mime %q", c.html, c.mime)
	}
}

func TestNotEncapsulated(t *testing.T) {
	c := &capture{}
	doc := extract(t, `{\rtf1\ansi {\*\htmltag0 <p>}text\par}`, &gofastcore.Options{Child: c.child})
	if c.html != nil || len(doc.Children) != 0 || len(doc.Parts[0].Blocks) != 1 {
		t.Errorf("html %q children %d blocks %+v", c.html, len(doc.Children), doc.Parts[0].Blocks)
	}
}

func TestEncapsulatedHTMLBudget(t *testing.T) {
	// HTML tags nested in each other, and more HTML than the decode
	// budget allows: the HTML is not handed over, and the RTF is read.
	src := `{\rtf1\ansi\fromhtml1 ` + strings.Repeat(`{\*\htmltag0 <b>`, 50) + strings.Repeat(`}`, 50) +
		strings.Repeat(`{\*\htmltag0 <i>}`, 2000) + ` text\par}`
	c := &capture{}
	doc := extract(t, src, &gofastcore.Options{Child: c.child, Limits: gofastcore.Limits{MaxDecodedBytes: 1000}})
	if c.html != nil || len(doc.Parts[0].Blocks) != 1 || len(doc.Children) != 1 {
		t.Errorf("html %d bytes, blocks %+v, children %d", len(c.html), doc.Parts[0].Blocks, len(doc.Children))
	}
}

// TestEncapsulatedFiles reads RTF bodies of Outlook messages from the
// public development corpus, when present.
func TestEncapsulatedFiles(t *testing.T) {
	dir := filepath.Join("..", ".testdata", "extracted", "rtf-from-msg")
	for _, name := range []string{"poi-message_1979.1.rtf", "poi-message_1980.1.rtf", "poi-bug66335.1.rtf"} {
		path := filepath.Join(dir, name)
		if _, err := os.Stat(path); err != nil {
			t.Skipf("%s absent", path)
		}
		c := &capture{}
		doc, err := ExtractFile(context.Background(), path, &gofastcore.Options{Child: c.child})
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if err := doc.Validate(); err != nil {
			t.Errorf("%s: Validate: %v", name, err)
		}
		if len(c.html) == 0 || len(doc.Parts[0].Blocks) != 0 || !bytes.Contains(bytes.ToLower(c.html), []byte("</html>")) {
			t.Errorf("%s: %d bytes of html, %d blocks", name, len(c.html), len(doc.Parts[0].Blocks))
		}
		if name == "poi-message_1979.1.rtf" {
			h := string(c.html)
			if !strings.Contains(h, "Alec") || strings.Count(h, "&nbsp;") != 4 || strings.ContainsRune(h, utf8.RuneError) {
				t.Errorf("%s: html = %q", name, h)
			}
		}
	}
}

// emptyChild returns a document whose body part has no blocks.
func emptyChild(ctx context.Context, name, mime string, r io.ReaderAt, size int64, o *gofastcore.Options) (*gofastcore.Document, error) {
	d := gofastcore.New("html")
	p := gofastcore.Part{Kind: gofastcore.PartBody, Number: 1}
	p.Finish()
	d.Parts = append(d.Parts, p)
	return d, nil
}

func TestEncapsulatedEmptyChild(t *testing.T) {
	// The HTML gives no blocks; the text of the RTF is kept.
	src := `{\rtf1\ansi\fromhtml1 {\*\htmltag0 <html>}\htmlrtf Hidden text\par\htmlrtf0 {\*\htmltag0 </html>}}`
	doc := extract(t, src, &gofastcore.Options{Child: emptyChild})
	if len(doc.Parts[0].Blocks) != 1 || doc.Parts[0].Blocks[0].Text != "Hidden text" {
		t.Errorf("blocks %+v", doc.Parts[0].Blocks)
	}
}
