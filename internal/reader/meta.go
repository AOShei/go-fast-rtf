package reader

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/AOShei/go-fast-core"
)

// hiddenOf returns the reasons text with properties c in a paragraph with
// properties p cannot be seen.
func (r *reader) hiddenOf(c *chr, p *para) gofastcore.Hidden {
	var h gofastcore.Hidden
	if c.hidden || c.webHidden {
		h |= gofastcore.HiddenInvisible
	}
	if float64(c.fs)/2 < gofastcore.HiddenTinySize {
		h |= gofastcore.HiddenTiny
	}
	if r.lowContrast(c, p) {
		h |= gofastcore.HiddenLowContrast
	}
	return h
}

// color returns the colour table entry i, if it is set and not auto.
func (r *reader) colorAt(i int32) (rgb, bool) {
	if i <= 0 || int(i) >= len(r.colors) || r.colors[i].auto {
		return rgb{}, false
	}
	return r.colors[i], true
}

// lowContrast compares the text colour with what is behind the text: its
// highlight, its shading, \cb, the paragraph's shading, the page colour,
// white. Text in the automatic colour is drawn to contrast with what is
// behind it, so it is never low contrast.
func (r *reader) lowContrast(c *chr, p *para) bool {
	fg, ok := r.colorAt(c.cf)
	if !ok {
		return false
	}
	key := [2]int32{c.cf, -2}
	back := rgb{r: 255, g: 255, b: 255}
	found := false
	for _, i := range []int32{c.hl, c.shd, c.cb, p.shd} {
		if bg, ok := r.colorAt(i); ok {
			back, key[1], found = bg, i, true
			break
		}
	}
	if !found && r.page != nil {
		back, key[1] = *r.page, -1
	}
	if v, ok := r.contrast[key]; ok {
		return v
	}
	v := gofastcore.LowContrast(fg.r, fg.g, fg.b, back.r, back.g, back.b)
	if len(r.contrast) < 1<<12 {
		r.contrast[key] = v
	}
	return v
}

// revAuthor names the author of a revision from the revision table.
func (r *reader) revAuthor(i int32) string {
	if i < 0 || int(i) >= len(r.revs) {
		return ""
	}
	return r.revs[i]
}

// dttm turns a date in the packed DTTM form RTF uses for revisions and
// comments into ISO 8601.
func dttm(v int32) string {
	if v == 0 {
		return ""
	}
	u := uint32(v)
	minute, hour, day := u&0x3F, (u>>6)&0x1F, (u>>11)&0x1F
	month, year := (u>>16)&0xF, 1900+(u>>20)&0x1FF
	if month < 1 || month > 12 || day < 1 || day > 31 || hour > 23 || minute > 59 {
		return ""
	}
	return fmt.Sprintf("%04d-%02d-%02dT%02d:%02d:00", year, month, day, hour, minute)
}

func parseInt(s string) int32 {
	n, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	if err != nil {
		return 0
	}
	return int32(n)
}

// formatNumber writes a note number in the document's numbering format.
func formatNumber(n int, format string) string {
	switch format {
	case "roman", "ROMAN":
		s := roman(n)
		if format == "ROMAN" {
			return strings.ToUpper(s)
		}
		return s
	case "alpha", "ALPHA":
		s := ""
		for m := n; m > 0; m = (m - 1) / 26 {
			s = string(rune('a'+(m-1)%26)) + s
		}
		if format == "ALPHA" {
			return strings.ToUpper(s)
		}
		return s
	}
	return strconv.Itoa(n)
}

func roman(n int) string {
	if n <= 0 || n >= 4000 {
		return strconv.Itoa(n)
	}
	vals := []int{1000, 900, 500, 400, 100, 90, 50, 40, 10, 9, 5, 4, 1}
	syms := []string{"m", "cm", "d", "cd", "c", "xc", "l", "xl", "x", "ix", "v", "iv", "i"}
	var b strings.Builder
	for i, v := range vals {
		for n >= v {
			b.WriteString(syms[i])
			n -= v
		}
	}
	return b.String()
}

// Metadata.

type dateBuf struct{ yr, mo, dy, hr, min, sec int32 }

func (d *dateBuf) set(name string, v int32) {
	switch name {
	case "yr":
		d.yr = v
	case "mo":
		d.mo = v
	case "dy":
		d.dy = v
	case "hr":
		d.hr = v
	case "min":
		d.min = v
	case "sec":
		d.sec = v
	}
}

func (d *dateBuf) time() (time.Time, bool) {
	if d == nil || d.yr < 1 || d.yr > 9999 || d.mo < 1 || d.mo > 12 || d.dy < 1 || d.dy > 31 ||
		d.hr < 0 || d.hr > 23 || d.min < 0 || d.min > 59 || d.sec < 0 || d.sec > 59 {
		return time.Time{}, false
	}
	return time.Date(int(d.yr), time.Month(d.mo), int(d.dy), int(d.hr), int(d.min), int(d.sec), 0, time.UTC), true
}

func (r *reader) infoBuf(name string) *strbuf {
	b, ok := r.info[name]
	if !ok {
		b = newBuf(64 << 10)
		r.info[name] = b
	}
	return b
}

// languages maps the Windows language identifiers met most often to BCP 47.
var languages = map[int32]string{
	1025: "ar-SA", 1026: "bg-BG", 1027: "ca-ES", 1028: "zh-TW", 1029: "cs-CZ", 1030: "da-DK",
	1031: "de-DE", 1032: "el-GR", 1033: "en-US", 1034: "es-ES", 1035: "fi-FI", 1036: "fr-FR",
	1037: "he-IL", 1038: "hu-HU", 1039: "is-IS", 1040: "it-IT", 1041: "ja-JP", 1042: "ko-KR",
	1043: "nl-NL", 1044: "nb-NO", 1045: "pl-PL", 1046: "pt-BR", 1048: "ro-RO", 1049: "ru-RU",
	1050: "hr-HR", 1051: "sk-SK", 1053: "sv-SE", 1054: "th-TH", 1055: "tr-TR", 1057: "id-ID",
	1058: "uk-UA", 1060: "sl-SI", 1061: "et-EE", 1062: "lv-LV", 1063: "lt-LT", 1066: "vi-VN",
	2052: "zh-CN", 2057: "en-GB", 2058: "es-MX", 2067: "nl-BE", 2070: "pt-PT", 3079: "de-AT",
	3081: "en-AU", 3082: "es-ES", 3084: "fr-CA", 4105: "en-CA", 2055: "de-CH", 4108: "fr-CH",
}

// metadata fills in the document's metadata from what \info held.
func (r *reader) metadata() {
	m := &r.doc.Metadata
	get := func(name string) string {
		if b, ok := r.info[name]; ok {
			return cleanText(strings.ReplaceAll(b.String(), "\n", " "))
		}
		return ""
	}
	m.Title, m.Subject = get("title"), get("subject")
	if a := get("author"); a != "" {
		m.Authors = []string{a}
	}
	if k := get("keywords"); k != "" {
		m.Keywords = []string{k}
	}
	if t, ok := r.dates["creatim"].time(); ok {
		m.Created = t
	}
	if t, ok := r.dates["revtim"].time(); ok {
		m.Modified = t
	}
	if lang, ok := languages[r.lang]; ok {
		m.Language = lang
	}
	extra := map[string]string{}
	for _, k := range []string{"operator", "company", "manager", "category", "comment", "doccomm", "hlinkbase"} {
		if v := get(k); v != "" {
			extra[k] = v
		}
	}
	for k, name := range map[string]string{"printim": "printed", "buptim": "backed_up"} {
		if t, ok := r.dates[k].time(); ok {
			extra[name] = t.Format("2006-01-02T15:04:05")
		}
	}
	if r.generator != nil {
		gen := strings.TrimSpace(strings.TrimSuffix(cleanText(r.generator.String()), ";"))
		m.Application = gen
		r.addStored(gen, "generator", "")
	}
	for _, dv := range r.docvars {
		r.addStored(dv[0], gofastcore.StoredDocumentVariable, dv[1])
	}
	for _, p := range r.userprops {
		r.addStored(p[0], "custom_property", p[1])
	}
	if len(extra) > 0 {
		m.Extra = extra
	}
	if hiddenInMetadata(m) {
		r.doc.Warn(gofastcore.SeverityWarning, gofastcore.WarnTextHidden, "the document properties contain invisible characters")
	}
}

func hiddenInMetadata(m *gofastcore.Metadata) bool {
	found := func(s string) bool { return gofastcore.InvisibleChars(s) != nil }
	if found(m.Title) || found(m.Subject) || found(m.Language) || found(m.Application) {
		return true
	}
	for _, s := range m.Authors {
		if found(s) {
			return true
		}
	}
	for _, s := range m.Keywords {
		if found(s) {
			return true
		}
	}
	for k, v := range m.Extra {
		if found(k) || found(v) {
			return true
		}
	}
	return false
}

// Fields.

type field struct {
	instr   *strbuf
	href    string
	inInstr bool // the field is inside another field's instruction
	parent  *field
}

// maxFieldCode bounds the instruction of one field.
const maxFieldCode = 32 << 10

func (r *reader) openField(g *group) {
	f := &field{instr: newBuf(maxFieldCode)}
	for i := len(r.fields) - 1; i >= 0; i-- {
		if r.fields[i].instr == g.buf && g.buf != nil {
			f.inInstr, f.parent = true, r.fields[i]
			break
		}
	}
	if len(r.fields) >= 256 {
		r.skipGroup()
		return
	}
	r.fields = append(r.fields, f)
	g.onClose = func() {
		r.fields = r.fields[:len(r.fields)-1]
		code := cleanText(strings.ReplaceAll(f.instr.String(), "\n", " "))
		if code == "" {
			return
		}
		if f.inInstr && f.parent != nil {
			for _, c := range "{ " + code + " }" {
				f.parent.instr.addRune(c)
			}
			return
		}
		r.fieldWarnings(code)
		top := r.top()
		if top.flow == nil {
			return
		}
		top.flow.after = append(top.flow.after, r.unshownNote(code))
	}
}

// OutlierWords is how many words an item of the data RTF files routinely
// keep and never show (a field instruction, an index entry, a document
// variable) may hold before it is out of proportion for its kind. Such
// data is not flagged hidden, since nearly every file has some; an item
// longer than this is flagged invisible, with WarnUnshownOutlier.
const OutlierWords = 30

// isOutlier reports whether unshown text is out of proportion.
func isOutlier(text string) bool {
	n := 0
	for range strings.FieldsSeq(text) {
		if n++; n > OutlierWords {
			return true
		}
	}
	return false
}

// unshownNote makes the note of a field instruction or an index or
// table-of-contents entry: unflagged, unless it is an outlier. The words
// counted are those of the instruction or of the entry, without the name
// of the field an entry stands for.
func (r *reader) unshownNote(text string) gofastcore.Block {
	return r.unshownNoteOf(text, text)
}

func (r *reader) unshownNoteOf(text, counted string) gofastcore.Block {
	b := gofastcore.Block{Kind: gofastcore.KindNote, Text: text,
		Attrs: map[string]string{gofastcore.AttrNoteType: gofastcore.NoteField}}
	if isOutlier(counted) {
		b.Hidden = gofastcore.HiddenInvisible
		r.outliers++
	}
	return b
}

// entryWords are the destinations of index and table-of-contents entries,
// with the name of the Word field each stands for.
var entryWords = map[string]string{"xe": "XE", "tc": "TC", "tcn": "TC"}

// openEntry reads an index or table-of-contents entry, which is not shown
// where it stands. Its text becomes a field note after the paragraph, as
// Word writes it, "XE entry".
func (r *reader) openEntry(g *group, name string) {
	if g.buf != nil {
		return // an entry inside another destination's text
	}
	b := newBuf(maxFieldCode)
	f := g.flow
	g.buf = b
	g.onClose = func() {
		text := cleanText(strings.ReplaceAll(b.String(), "\n", " "))
		if text == "" || f == nil {
			return
		}
		f.after = append(f.after, r.unshownNoteOf(entryWords[name]+" "+text, text))
	}
}

// fieldWarnings counts fields that refer outside the file or run
// something.
func (r *reader) fieldWarnings(code string) {
	words := fieldWords(code)
	if len(words) == 0 {
		return
	}
	switch strings.ToUpper(words[0]) {
	case "INCLUDEPICTURE", "INCLUDETEXT", "INCLUDE", "LINK", "IMPORT":
		r.external++
	case "DDE", "DDEAUTO":
		r.active++
	}
}

// fieldWords splits a field instruction into its words; a quoted string
// is one word, without its quotes, and a doubled backslash in it is one.
func fieldWords(code string) []string {
	var out []string
	var cur strings.Builder
	in, any := false, false
	for i := 0; i < len(code); i++ {
		c := code[i]
		switch {
		case c == '"':
			in, any = !in, true
		case c == '\\' && in && i+1 < len(code) && code[i+1] == '\\':
			cur.WriteByte('\\')
			i++
		case (c == ' ' || c == '\t' || c == '\n') && !in:
			if any {
				out = append(out, cur.String())
				cur.Reset()
				any = false
			}
		default:
			cur.WriteByte(c)
			any = true
		}
		if len(out) > 64 {
			break
		}
	}
	if any {
		out = append(out, cur.String())
	}
	return out
}

// fieldHref is where a HYPERLINK field links to.
func fieldHref(code string) string {
	words := fieldWords(code)
	if len(words) == 0 || !strings.EqualFold(words[0], "HYPERLINK") {
		return ""
	}
	url, anchor := "", ""
	for i := 1; i < len(words); i++ {
		w := words[i]
		switch {
		case w == `\l`:
			if i+1 < len(words) {
				anchor = words[i+1]
				i++
			}
		case w == `\o` || w == `\t`:
			i++
		case strings.HasPrefix(w, `\`) && len(w) == 2:
		case url == "":
			url = w
		}
	}
	if anchor != "" {
		url += "#" + anchor
	}
	return cleanText(url)
}
