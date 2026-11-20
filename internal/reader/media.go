package reader

import (
	"bytes"
	"encoding/binary"
	"strconv"
	"strings"

	"github.com/AOShei/go-fast-core"
)

// Shapes and their properties.

type shape struct {
	prev   *shape
	alt    string
	hidden bool // fHidden: the shape declares itself hidden
	images []gofastcore.Block
	boxed  []boxed // the blocks of its text box (\shptxt), and of shapes in it
}

// boxed is a block of a shape's text box and the flow it goes to.
type boxed struct {
	parent *flow
	b      gofastcore.Block
}

type spState struct {
	prev        *spState
	name, value *strbuf
}

func (r *reader) openShape(g *group) {
	s := &shape{prev: r.shape}
	r.shape = s
	g.onClose = func() {
		r.shape = s.prev
		for i := range s.boxed {
			if s.hidden {
				displayWhole(&s.boxed[i].b, gofastcore.DisplayHidden)
			}
			if s.prev != nil {
				// A shape in a group: the group may be hidden too.
				s.prev.boxed = append(s.prev.boxed, s.boxed[i])
			} else if p := s.boxed[i].parent; p != nil {
				p.after = append(p.after, s.boxed[i].b)
			} else {
				r.add(s.boxed[i].b)
			}
		}
		top := r.top()
		for _, b := range s.images {
			if b.Text == "" && s.alt != "" {
				b.Text = s.alt
			}
			if s.hidden {
				displayWhole(&b, gofastcore.DisplayHidden)
			}
			r.deliver(top.flow, b)
		}
	}
}

// displayWhole gives the whole of block b the reason d, taking it off the
// spans and table regions that had it for part of the block.
func displayWhole(b *gofastcore.Block, d gofastcore.Display) {
	b.Display |= d
	spans := b.Spans[:0]
	for _, s := range b.Spans {
		s.Display &^= d
		if s.Display != 0 || s.Style != 0 || s.Href != "" {
			spans = append(spans, s)
		}
	}
	if len(spans) == 0 {
		spans = nil
	}
	b.Spans = spans
	if t := b.Table; t != nil {
		regions := t.Display[:0]
		for _, c := range t.Display {
			c.Display &^= d
			if c.Display != 0 {
				regions = append(regions, c)
			}
		}
		if len(regions) == 0 {
			regions = nil
		}
		t.Display = regions
	}
}

// deliver puts a block after the paragraph being read in flow f.
func (r *reader) deliver(f *flow, b gofastcore.Block) {
	if f != nil {
		r.holdBlock(&b)
		f.after = append(f.after, b)
		return
	}
	r.add(b)
}

func (r *reader) openSp(g *group) {
	sp := &spState{prev: r.sp, name: newBuf(256), value: newBuf(64 << 10)}
	r.sp = sp
	g.dest, g.buf = dWords, nil
	g.onClose = func() {
		r.sp = sp.prev
		r.shapeProp(strings.TrimSpace(sp.name.String()), cleanText(sp.value.String()))
	}
}

// shapeProp reads one property of a shape or picture.
func (r *reader) shapeProp(name, value string) {
	switch {
	case name == "wzDescription" && value != "":
		if r.pict != nil {
			r.pict.alt = value
		} else if r.shape != nil {
			r.shape.alt = value
		}
	case name == "fHidden" && r.pict == nil && r.shape != nil:
		// A boolean: 0 is false, anything else true; the last one counts.
		n, err := strconv.Atoi(value)
		r.shape.hidden = err == nil && n != 0
	}
}

// Pictures.

type pict struct {
	depth  int
	format string
	picw   int32
	pich   int32
	data   []byte
	keep   int64
	n      int64
	half   byte
	odd    bool
	alt    string
	cut    bool // the data was more than could be kept
}

// sniffBytes is how much of a picture is kept, when its data is not
// asked for, to read its size from its header.
const sniffBytes = 64 << 10

func (r *reader) openPict(g *group) {
	p := &pict{depth: len(r.stack), keep: sniffBytes}
	if r.opts.Images == gofastcore.ImagesData {
		p.keep = max(r.cfg.Decode.Left(), 0)
	}
	prev := r.pict
	r.pict = p
	g.dest, g.buf = dPict, nil
	g.onClose = func() {
		r.pict = prev
		r.endPict(p)
	}
}

func (p *pict) room() int64 { return max(p.keep-int64(len(p.data)), 0) }

func (p *pict) word(name string, v int32) bool {
	switch name {
	case "pngblip":
		p.format = "png"
	case "jpegblip":
		p.format = "jpeg"
	case "emfblip":
		p.format = "emf"
	case "wmetafile":
		p.format = "wmf"
	case "macpict":
		p.format = "pict"
	case "dibitmap":
		p.format = "dib"
	case "wbitmap":
		p.format = "ddb"
	case "pmmetafile":
		p.format = "os2"
	case "picw":
		p.picw = v
	case "pich":
		p.pich = v
	default:
		return false
	}
	return true
}

func hexVal(c byte) (byte, bool) {
	switch {
	case c >= '0' && c <= '9':
		return c - '0', true
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10, true
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10, true
	}
	return 0, false
}

// hexData decodes hexadecimal digits into data, keeping at most keep
// bytes. It returns the number of bytes decoded.
func hexData(data []byte, out *[]byte, keep int64, half *byte, odd *bool) (n int64, cut bool) {
	for _, c := range data {
		v, ok := hexVal(c)
		if !ok {
			continue
		}
		if !*odd {
			*half, *odd = v, true
			continue
		}
		*odd = false
		n++
		if int64(len(*out)) < keep {
			*out = append(*out, *half<<4|v)
		} else {
			cut = true
		}
	}
	return n, cut
}

func (p *pict) hex(data []byte) {
	n, cut := hexData(data, &p.data, p.keep, &p.half, &p.odd)
	p.n += n
	p.cut = p.cut || cut
}

func (p *pict) raw(data []byte, n int64) {
	room := p.room()
	if int64(len(data)) > room {
		data = data[:room]
	}
	p.data = append(p.data, data...)
	p.n += n
	if n > int64(len(data)) {
		p.cut = true
	}
}

var pictMIME = map[string]string{
	"png": "image/png", "jpeg": "image/jpeg", "emf": "image/emf", "wmf": "image/wmf",
	"pict": "image/x-pict", "dib": "image/bmp", "ddb": "image/bmp", "os2": "image/x-os2-metafile",
}

func (r *reader) endPict(p *pict) {
	r.sawImage = true
	if r.opts.Images == gofastcore.ImagesNone {
		return
	}
	r.pictN++
	img := &gofastcore.Image{ID: "pict-" + strconv.Itoa(r.pictN), MIME: pictMIME[p.format]}
	switch p.format {
	case "png":
		img.Width, img.Height = pngSize(p.data)
	case "jpeg":
		img.Width, img.Height = jpegSize(p.data)
	case "dib", "ddb":
		img.Width, img.Height = int(max(p.picw, 0)), int(max(p.pich, 0))
	}
	if r.opts.Images == gofastcore.ImagesData && p.n > 0 {
		switch {
		case p.cut:
			r.decCut = true
		case p.format == "ddb" || p.format == "os2" || p.format == "":
			r.unsupported++
		case !r.cfg.Decode.Take(int64(len(p.data))):
			r.decCut = true
		case p.format == "dib":
			if bmp := dibToBMP(p.data); bmp != nil {
				img.Data = bmp
			} else {
				r.unsupported++
			}
		default:
			img.Data = p.data
		}
	}
	b := gofastcore.Block{Kind: gofastcore.KindImage, Text: cleanText(p.alt), Image: img}
	if r.shape != nil {
		r.holdBlock(&b)
		r.shape.images = append(r.shape.images, b)
		return
	}
	r.deliver(r.top().flow, b)
}

func pngSize(b []byte) (int, int) {
	if len(b) < 24 || !bytes.HasPrefix(b, []byte("\x89PNG\r\n\x1a\n")) || string(b[12:16]) != "IHDR" {
		return 0, 0
	}
	w, h := binary.BigEndian.Uint32(b[16:20]), binary.BigEndian.Uint32(b[20:24])
	if w > 1<<24 || h > 1<<24 {
		return 0, 0
	}
	return int(w), int(h)
}

func jpegSize(b []byte) (int, int) {
	if len(b) < 4 || b[0] != 0xFF || b[1] != 0xD8 {
		return 0, 0
	}
	i := 2
	for i+9 < len(b) {
		if b[i] != 0xFF {
			return 0, 0
		}
		m := b[i+1]
		if m == 0xFF {
			i++
			continue
		}
		l := int(binary.BigEndian.Uint16(b[i+2 : i+4]))
		if m >= 0xC0 && m <= 0xCF && m != 0xC4 && m != 0xC8 && m != 0xCC {
			h, w := binary.BigEndian.Uint16(b[i+5:i+7]), binary.BigEndian.Uint16(b[i+7:i+9])
			return int(w), int(h)
		}
		if l < 2 {
			return 0, 0
		}
		i += 2 + l
	}
	return 0, 0
}

// dibToBMP puts the file header of a BMP file in front of a device
// independent bitmap, which is what \dibitmap holds.
func dibToBMP(dib []byte) []byte {
	if len(dib) < 40 {
		return nil
	}
	size := binary.LittleEndian.Uint32(dib[0:4])
	if size < 12 || int64(size) > int64(len(dib)) {
		return nil
	}
	var bits, colors, compression uint32
	if size == 12 {
		bits = uint32(binary.LittleEndian.Uint16(dib[10:12]))
	} else {
		bits = uint32(binary.LittleEndian.Uint16(dib[14:16]))
		compression = binary.LittleEndian.Uint32(dib[16:20])
		colors = binary.LittleEndian.Uint32(dib[32:36])
	}
	if colors == 0 && bits <= 8 {
		colors = 1 << bits
	}
	entry := uint32(4)
	if size == 12 {
		entry = 3
	}
	off := 14 + size + colors*entry
	if size == 40 && (compression == 3 || compression == 6) {
		off += 12
	}
	if int64(off)-14 > int64(len(dib)) {
		return nil
	}
	out := make([]byte, 14, 14+len(dib))
	out[0], out[1] = 'B', 'M'
	binary.LittleEndian.PutUint32(out[2:6], uint32(14+len(dib)))
	binary.LittleEndian.PutUint32(out[10:14], off)
	return append(out, dib...)
}

// Embedded objects.

type object struct {
	kind  string
	class *strbuf
	name  *strbuf
	data  []byte
	keep  int64
	n     int64
	half  byte
	odd   bool
	cut   bool
	read  bool
}

func (r *reader) openObject(g *group) {
	o := &object{class: newBuf(256), name: newBuf(1024)}
	prev := r.obj
	r.obj = o
	g.onClose = func() {
		r.obj = prev
		r.endObject(o)
	}
}

func (o *object) start(r *reader) {
	o.read = true
	o.keep = max(r.cfg.Decode.Left(), 0)
}

func (o *object) room() int64 { return max(o.keep-int64(len(o.data)), 0) }

func (o *object) hex(data []byte) {
	n, cut := hexData(data, &o.data, o.keep, &o.half, &o.odd)
	o.n += n
	o.cut = o.cut || cut
}

func (o *object) raw(data []byte, n int64) {
	room := o.room()
	if int64(len(data)) > room {
		data = data[:room]
	}
	o.data = append(o.data, data...)
	o.n += n
	if n > int64(len(data)) {
		o.cut = true
	}
}

func (r *reader) endObject(o *object) {
	class := cleanText(o.class.String())
	name := cleanText(o.name.String())
	switch o.kind {
	case "objlink", "objautlink":
		r.external++
	case "objocx":
		r.active++
	}
	if name == "" {
		name = class
	}
	if o.kind == "objlink" || o.kind == "objautlink" || !o.read || len(o.data) == 0 {
		r.doc.Children = append(r.doc.Children, r.opts.ExtractChild(r.ctx, name, "", nil, 0))
		return
	}
	if o.cut || !r.cfg.Decode.Take(int64(len(o.data))) {
		r.decCut = true
		r.doc.Children = append(r.doc.Children, r.opts.ExtractChild(r.ctx, name, "", nil, o.n))
		return
	}
	payload, label := ole1Native(o.data)
	if label != "" {
		name = label
	}
	if payload == nil {
		payload = o.data
	}
	r.doc.Children = append(r.doc.Children, r.opts.ExtractChild(r.ctx, name, "", bytes.NewReader(payload), int64(len(payload))))
}

// ole1Native returns the native data of an OLE 1.0 object stream (the
// ObjectHeader of [MS-OLEDS] 2.2.4 followed by its data), and for an object
// of the class Package the file it packages and that file's label. It
// returns nil when the stream is not of that form.
func ole1Native(b []byte) (payload []byte, label string) {
	rd := &byteReader{b: b}
	rd.u32() // OLEVersion
	format := rd.u32()
	class := rd.lpstr()
	if rd.bad || format != 2 {
		return nil, ""
	}
	rd.lpstr() // topic
	rd.lpstr() // item
	size := rd.u32()
	if rd.bad || int64(size) > int64(len(rd.b)-rd.i) {
		return nil, ""
	}
	native := rd.b[rd.i : rd.i+int(size)]
	if !strings.EqualFold(strings.TrimRight(class, "\x00"), "Package") {
		return native, ""
	}
	// The Packager format: a signature, a label, the original path, then
	// a type, a temporary path and the file's data.
	p := &byteReader{b: native}
	p.u16()
	lbl := p.cstr()
	p.cstr() // original path
	if typ := p.u32(); typ != 0x00030000 || p.bad {
		return native, cleanText(lbl)
	}
	p.skip(int(p.u32())) // temporary path
	n := p.u32()
	if p.bad || int64(n) > int64(len(p.b)-p.i) {
		return native, cleanText(lbl)
	}
	return p.b[p.i : p.i+int(n)], cleanText(lbl)
}

type byteReader struct {
	b   []byte
	i   int
	bad bool
}

func (r *byteReader) u16() uint16 {
	if r.i+2 > len(r.b) {
		r.bad = true
		return 0
	}
	v := binary.LittleEndian.Uint16(r.b[r.i:])
	r.i += 2
	return v
}

func (r *byteReader) u32() uint32 {
	if r.i+4 > len(r.b) {
		r.bad = true
		return 0
	}
	v := binary.LittleEndian.Uint32(r.b[r.i:])
	r.i += 4
	return v
}

func (r *byteReader) skip(n int) {
	if n < 0 || r.i+n > len(r.b) {
		r.bad = true
		return
	}
	r.i += n
}

func (r *byteReader) lpstr() string {
	n := int(r.u32())
	if r.bad || n < 0 || n > 1<<16 || r.i+n > len(r.b) {
		r.bad = true
		return ""
	}
	s := string(r.b[r.i : r.i+n])
	r.i += n
	return s
}

func (r *byteReader) cstr() string {
	j := bytes.IndexByte(r.b[min(r.i, len(r.b)):], 0)
	if j < 0 || j > 4096 {
		r.bad = true
		return ""
	}
	s := string(r.b[r.i : r.i+j])
	r.i += j + 1
	return s
}
