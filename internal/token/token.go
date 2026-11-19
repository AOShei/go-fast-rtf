// Package token splits an RTF stream into tokens: group delimiters,
// control words, control symbols, runs of text, hexadecimal bytes (\'hh)
// and binary data (\binN). It reads from a bufio.Reader and holds no more
// than one token's worth of data, except for binary data, of which it keeps
// only as much as its caller asks for.
package token

import (
	"bufio"
	"io"
)

// Kind is the type of a token.
type Kind uint8

const (
	EOF    Kind = iota
	Open        // {
	Close       // }
	Word        // \name or \nameN
	Symbol      // \ followed by one character that is not a letter
	Text        // a run of text bytes
	Hex         // \'hh
	Bin         // the data of \binN
)

// Limits from the specification: a control word's name is at most 32
// letters and its parameter at most 10 digits. Longer ones are read to
// their end and cut there.
const (
	maxName   = 32
	maxDigits = 10
	// maxText bounds one Text token; longer runs come as several.
	maxText = 4096
)

// Token is one token. Data is valid until the next call of Next.
type Token struct {
	Kind     Kind
	Name     string // Word: the name
	Param    int32  // Word: the parameter, clamped to the range of int32
	HasParam bool   // Word: a parameter was given
	Sym      byte   // Symbol: the character; Hex: the byte
	Data     []byte // Text: the bytes; Bin: the bytes kept
	BinLen   int64  // Bin: the length the file declared and that was read
}

// Lexer reads tokens.
type Lexer struct {
	r    *bufio.Reader
	buf  []byte
	name []byte
	// KeepBin is how many bytes of the data of the next \binN to keep in
	// Token.Data; the rest is read and dropped. The caller sets it.
	KeepBin int64
	// Read is the number of bytes consumed so far.
	Read int64
	err  error
	dash bool // a '-' read after a control word is the next text
}

// New returns a lexer reading from r.
func New(r io.Reader) *Lexer {
	br, ok := r.(*bufio.Reader)
	if !ok {
		br = bufio.NewReaderSize(r, 64<<10)
	}
	return &Lexer{r: br, buf: make([]byte, 0, maxText), name: make([]byte, 0, maxName)}
}

// Err returns the read error that ended the stream, other than io.EOF.
func (l *Lexer) Err() error {
	if l.err == io.EOF {
		return nil
	}
	return l.err
}

func (l *Lexer) readByte() (byte, bool) {
	c, err := l.r.ReadByte()
	if err != nil {
		l.err = err
		return 0, false
	}
	l.Read++
	return c, true
}

func (l *Lexer) unread() {
	if l.r.UnreadByte() == nil {
		l.Read--
	}
}

// Next returns the next token. At the end of the input it returns a token
// of Kind EOF, and keeps doing so.
func (l *Lexer) Next() Token {
	for {
		var c byte
		if l.dash {
			l.dash, c = false, '-'
		} else {
			var ok bool
			if c, ok = l.readByte(); !ok {
				return Token{Kind: EOF}
			}
		}
		switch c {
		case '{':
			return Token{Kind: Open}
		case '}':
			return Token{Kind: Close}
		case '\\':
			return l.control()
		case '\r', '\n':
			continue // line ends in RTF source mean nothing
		}
		l.buf = append(l.buf[:0], c)
		for len(l.buf) < maxText {
			c, ok := l.readByte()
			if !ok {
				break
			}
			if c == '{' || c == '}' || c == '\\' || c == '\r' || c == '\n' {
				l.unread()
				break
			}
			l.buf = append(l.buf, c)
		}
		return Token{Kind: Text, Data: l.buf}
	}
}

func isLetter(c byte) bool { return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' }
func isDigit(c byte) bool  { return c >= '0' && c <= '9' }

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

func (l *Lexer) control() Token {
	c, ok := l.readByte()
	if !ok {
		return Token{Kind: EOF}
	}
	if !isLetter(c) {
		switch c {
		case '\'':
			var v byte
			for i := 0; i < 2; i++ {
				d, ok := l.readByte()
				if !ok {
					return Token{Kind: EOF}
				}
				h, ok := hexVal(d)
				if !ok {
					// A damaged escape: what follows is read as text.
					l.unread()
					if i == 0 {
						return l.Next()
					}
					return Token{Kind: Hex, Sym: v}
				}
				v = v<<4 | h
			}
			return Token{Kind: Hex, Sym: v}
		case '\r', '\n':
			// A backslash before a line end is \par.
			return Token{Kind: Word, Name: "par"}
		}
		return Token{Kind: Symbol, Sym: c}
	}
	l.name = append(l.name[:0], c)
	for {
		c, ok = l.readByte()
		if !ok {
			break
		}
		if !isLetter(c) {
			break
		}
		if len(l.name) < maxName {
			l.name = append(l.name, c)
		}
	}
	t := Token{Kind: Word, Name: wordName(l.name)}
	if !ok {
		return t
	}
	neg := false
	if c == '-' {
		d, ok2 := l.readByte()
		if ok2 && isDigit(d) {
			neg = true
			c = d
		} else {
			if ok2 {
				l.unread()
			}
			// The '-' is text; the reader cannot take back two bytes, so
			// it is held for the next token.
			l.dash = true
			return t
		}
	}
	if isDigit(c) {
		var v int64
		n := 0
		for {
			if n < maxDigits {
				v = v*10 + int64(c-'0')
			}
			n++
			c, ok = l.readByte()
			if !ok || !isDigit(c) {
				break
			}
		}
		if neg {
			v = -v
		}
		t.Param, t.HasParam = clamp32(v), true
		if !ok {
			return l.bin(t)
		}
	}
	if c != ' ' {
		l.unread()
	}
	return l.bin(t)
}

func clamp32(v int64) int32 {
	switch {
	case v > 1<<31-1:
		return 1<<31 - 1
	case v < -1<<31:
		return -1 << 31
	}
	return int32(v)
}

// bin reads the data of \binN, which follows the word directly.
func (l *Lexer) bin(t Token) Token {
	if t.Name != "bin" {
		return t
	}
	n := int64(max(t.Param, 0))
	keep := min(n, max(l.KeepBin, 0))
	out := Token{Kind: Bin}
	if keep > 0 {
		data := make([]byte, 0, min(keep, 1<<20))
		for int64(len(data)) < keep {
			c, ok := l.readByte()
			if !ok {
				break
			}
			data = append(data, c)
		}
		out.Data = data
	}
	read := int64(len(out.Data))
	if rest := n - read; rest > 0 && l.err == nil {
		for rest > 0 {
			step := int(min(rest, 1<<20))
			d, err := l.r.Discard(step)
			l.Read += int64(d)
			read += int64(d)
			rest -= int64(d)
			if err != nil {
				l.err = err
				break
			}
		}
	}
	out.BinLen = read
	return out
}

// words interns the control words met most often, so that reading them
// does not allocate.
var words = func() map[string]string {
	m := map[string]string{}
	for _, w := range []string{
		"par", "pard", "plain", "f", "fs", "b", "i", "ul", "ulnone", "cf", "cb", "u", "uc",
		"tab", "line", "cell", "row", "intbl", "cellx", "trowd", "s", "lang", "langfe",
		"loch", "hich", "dbch", "af", "rtlch", "ltrch", "fcs", "insrsid", "charrsid",
		"pararsid", "rsid", "sectrsid", "highlight", "v", "strike", "super", "sub",
		"nosupersub", "ls", "ilvl", "outlinelevel", "itap", "bin", "field", "fldinst",
		"fldrslt", "pict", "listtext", "pntext", "sp", "sn", "sv", "shp", "shpinst",
		"chcbpat", "cbpat", "kerning", "expnd", "expndtw", "sa", "sb", "li", "ri", "fi",
		"ql", "qr", "qc", "qj", "sl", "slmult", "widctlpar", "nowidctlpar", "wrapdefault",
		"aspalpha", "aspnum", "faauto", "adjustright", "rin", "lin", "itap", "noproof",
		"cgrid", "alang", "hyphpar", "webhidden", "deleted", "revised",
	} {
		m[w] = w
	}
	return m
}()

func wordName(b []byte) string {
	if s, ok := words[string(b)]; ok {
		return s
	}
	return string(b)
}
