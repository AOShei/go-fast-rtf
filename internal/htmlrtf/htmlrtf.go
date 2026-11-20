// Package htmlrtf de-encapsulates HTML from RTF that encapsulates it, as
// Outlook stores HTML mail ([MS-OXRTFEX]): the document declares
// \fromhtml1 in its header, the HTML's markup is in {\*\htmltagN ...}
// groups, the HTML's text is between them, and \htmlrtf ... \htmlrtf0
// runs hold an RTF-only rendering that is skipped.
package htmlrtf

import (
	"context"
	"io"
	"strconv"
	"unicode/utf8"

	"github.com/AOShei/go-fast-rtf/internal/token"
)

// Result is what Extract found.
type Result struct {
	// Found: the document declares \fromhtml1 in its header.
	Found bool
	// HTML is the de-encapsulated HTML, in the code page CodePage. It is
	// empty when Found is false or when Cut is true.
	HTML     []byte
	CodePage int
	// Cut: the HTML is longer than the limit and was not kept.
	Cut bool
}

type state struct {
	htmlrtf bool // \htmlrtf is on
	tag     bool // inside an \htmltag destination
	skip    bool // inside a destination that is not HTML
	star    bool // the group began with \*
	first   bool // no control word read in the group yet
	uc      int32
}

// skipped are destinations whose content is not part of the HTML.
var skipped = map[string]bool{
	"fonttbl": true, "colortbl": true, "stylesheet": true, "info": true,
	"pict": true, "object": true, "fldinst": true, "listtable": true,
	"listoverridetable": true, "revtbl": true, "header": true, "footer": true,
	"headerl": true, "headerr": true, "headerf": true, "footerl": true,
	"footerr": true, "footerf": true, "footnote": true, "annotation": true,
	"shp": true, "shpinst": true, "nonshppict": true,
	"xmlnstbl": true, "rsidtbl": true, "generator": true, "filetbl": true,
	"themedata": true, "colorschememapping": true, "latentstyles": true,
	"datastore": true, "pgdsctbl": true, "listtext": true, "pntext": true,
}

// special are control words that stand for one character.
var special = map[string]rune{
	"lquote": 0x2018, "rquote": 0x2019, "ldblquote": 0x201C, "rdblquote": 0x201D,
	"bullet": 0x2022, "endash": 0x2013, "emdash": 0x2014, "emspace": 0x2003,
	"enspace": 0x2002,
}

// Extract reads src, an RTF document from its opening brace. When the
// header declares \fromhtml1 it returns the encapsulated HTML, as long as
// it is at most limit bytes; past that it stops and sets Cut. Groups
// nested deeper than maxDepth are not read. Reading stops, without
// Found, at the first content of a document that does not declare
// \fromhtml1.
func Extract(ctx context.Context, src io.Reader, limit int64, maxDepth int) (Result, error) {
	lex := token.New(src)
	res := Result{CodePage: 1252}
	var stack []state
	deep := 0 // groups being skipped past maxDepth
	ucLeft := int32(0)
	var out []byte
	emit := func(b ...byte) bool {
		if int64(len(out)+len(b)) > limit {
			res.Cut = true
			return false
		}
		out = append(out, b...)
		return true
	}
	// content is called at the first token that is content; it says
	// whether to go on.
	content := func() bool { return res.Found }
	n := 0
	for {
		if n++; n&4095 == 0 {
			if err := ctx.Err(); err != nil {
				return Result{}, err
			}
		}
		t := lex.Next()
		if t.Kind == token.EOF {
			break
		}
		if deep > 0 {
			switch t.Kind {
			case token.Open:
				deep++
			case token.Close:
				deep--
			}
			continue
		}
		if t.Kind == token.Open {
			if len(stack) >= maxDepth {
				deep = 1
				continue
			}
			s := state{uc: 1}
			if len(stack) > 0 {
				s = stack[len(stack)-1]
			}
			s.star, s.first = false, true
			stack = append(stack, s)
			continue
		}
		if len(stack) == 0 {
			if t.Kind == token.Close {
				break // the end of the document
			}
			continue
		}
		g := &stack[len(stack)-1]
		if t.Kind == token.Close {
			stack = stack[:len(stack)-1]
			ucLeft = 0
			if len(stack) == 0 {
				break
			}
			continue
		}
		first := g.first
		if t.Kind != token.Symbol || t.Sym != '*' {
			g.first = false
		}
		if g.skip {
			continue
		}
		on := g.tag || !g.htmlrtf
		var ok = true
		switch t.Kind {
		case token.Word:
			if first && (skipped[t.Name] || g.star && t.Name != "htmltag") {
				g.skip = true
				continue
			}
			ucLeft = 0
			switch t.Name {
			case "fromhtml":
				if len(stack) == 1 && !res.Found && t.HasParam && t.Param == 1 {
					res.Found = true
				}
			case "ansicpg":
				if len(stack) == 1 && t.Param > 0 {
					res.CodePage = int(t.Param)
				}
			case "mac":
				if len(stack) == 1 {
					res.CodePage = 10000
				}
			case "pc":
				if len(stack) == 1 {
					res.CodePage = 437
				}
			case "pca":
				if len(stack) == 1 {
					res.CodePage = 850
				}
			case "htmlrtf":
				g.htmlrtf = !t.HasParam || t.Param != 0
			case "htmltag":
				if first || g.star {
					g.tag = true
					if !content() {
						return res, nil
					}
				}
			case "uc":
				if t.Param >= 0 {
					g.uc = t.Param
				}
			case "par", "line", "tab", "u":
				if !content() {
					return res, nil
				}
				if !on {
					if t.Name == "u" {
						ucLeft = g.uc
					}
					continue
				}
				switch t.Name {
				case "tab":
					ok = emit('\t')
				case "u":
					r := rune(t.Param)
					if r < 0 {
						r += 0x10000
					}
					ok = emitRune(emit, res.CodePage, r)
					ucLeft = g.uc
				default:
					ok = emit('\r', '\n')
				}
			default:
				if r, sp := special[t.Name]; sp {
					if !content() {
						return res, nil
					}
					if on {
						ok = emitRune(emit, res.CodePage, r)
					}
				}
			}
		case token.Symbol:
			if t.Sym == '*' {
				g.star = true
				continue
			}
			switch t.Sym {
			case '{', '}', '\\', '~', '_':
				if !content() {
					return res, nil
				}
			}
			if !on {
				continue
			}
			switch t.Sym {
			case '{', '}', '\\':
				ok = emit(t.Sym)
			case '~':
				ok = emitRune(emit, res.CodePage, 0xA0)
			case '_':
				ok = emitRune(emit, res.CodePage, 0x2011)
			}
		case token.Text:
			data := t.Data
			if ucLeft > 0 {
				k := min(int(ucLeft), len(data))
				ucLeft -= int32(k)
				data = data[k:]
			}
			if len(data) == 0 {
				continue
			}
			if !content() {
				return res, nil
			}
			if on {
				ok = emit(data...)
			}
		case token.Hex:
			if ucLeft > 0 {
				ucLeft--
				continue
			}
			if !content() {
				return res, nil
			}
			if on {
				ok = emit(t.Sym)
			}
		case token.Bin:
			if ucLeft > 0 {
				ucLeft--
			}
		}
		if !ok {
			return res, nil
		}
	}
	if err := lex.Err(); err != nil && lex.Read == 0 {
		return Result{}, err
	}
	if res.Found {
		res.HTML = out
	}
	return res, nil
}

// emitRune writes r in the code page: as UTF-8 in code page 65001, as
// itself when it is ASCII, and otherwise as an HTML character reference,
// which means the character in every code page.
func emitRune(emit func(...byte) bool, cp int, r rune) bool {
	if r < 0x80 {
		return emit(byte(r))
	}
	if cp == 65001 {
		var b [utf8.UTFMax]byte
		return emit(b[:utf8.EncodeRune(b[:], r)]...)
	}
	return emit([]byte("&#" + strconv.Itoa(int(r)) + ";")...)
}

// Charset returns the WHATWG label of a code page, or "" when it has none.
func Charset(cp int) string {
	switch {
	case cp == 65001:
		return "utf-8"
	case cp >= 1250 && cp <= 1258, cp == 874:
		return "windows-" + strconv.Itoa(cp)
	case cp >= 28592 && cp <= 28606 && cp != 28600 && cp != 28601 && cp != 28602:
		return "iso-8859-" + strconv.Itoa(cp-28590)
	}
	switch cp {
	case 28591, 20127:
		return "windows-1252" // WHATWG reads both as windows-1252
	case 866:
		return "ibm866"
	case 932:
		return "shift_jis"
	case 936:
		return "gbk"
	case 949:
		return "euc-kr"
	case 950:
		return "big5"
	case 54936:
		return "gb18030"
	case 20866:
		return "koi8-r"
	case 21866:
		return "koi8-u"
	case 10000:
		return "macintosh"
	case 10007:
		return "x-mac-cyrillic"
	case 20932, 51932:
		return "euc-jp"
	case 50220:
		return "iso-2022-jp"
	}
	return ""
}
