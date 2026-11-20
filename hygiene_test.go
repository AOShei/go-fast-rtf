package gofastrtf

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode"
)

// TestSourceHygiene checks that no Go source holds an invisible or
// look-alike character written literally: tests write them as escapes.
func TestSourceHygiene(t *testing.T) {
	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") {
			return err
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		line := 1
		for _, c := range string(b) {
			bad := false
			switch {
			case c == '\n':
				line++
			case c == '\t':
			case unicode.In(c, unicode.Cf, unicode.Co, unicode.Cc, unicode.Zl, unicode.Zp):
				bad = true
			case unicode.Is(unicode.Zs, c) && c != ' ':
				bad = true
			case c >= 0xFB00 && c <= 0xFB06:
				bad = true
			}
			if bad {
				t.Errorf("%s:%d: literal %U", path, line, c)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
