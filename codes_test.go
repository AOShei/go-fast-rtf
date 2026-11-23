package gofastrtf

import (
	"testing"

	gofastcore "github.com/AOShei/go-fast-core"
)

// TestSharedCodes: the events go-fast-core has codes for are raised with
// them; raw line breaks in the source are not control characters removed.
func TestSharedCodes(t *testing.T) {
	doc := extract(t, "{\\rtf1\\ansi\r\n{\\info{\\title T\\'01itle}}\r\nHello\r\nworld\\'01x \\u7? y {\\*\\mmath x}\\par\r\n}", nil)
	want := map[string]string{
		gofastcore.WarnTextControlCharactersRemoved: "3 control characters with no meaning as text were removed from the text",
		gofastcore.WarnInputUnreadContent:           "content this version does not read: equation (1)",
	}
	got := map[string]string{}
	for _, w := range doc.Parts[0].Warnings {
		got[w.Code] = w.Message
	}
	for code, msg := range want {
		if got[code] != msg {
			t.Errorf("%s: %q, want %q (all: %v)", code, got[code], msg, doc.Parts[0].Warnings)
		}
	}
	if WarnControlCharsRemoved != gofastcore.WarnTextControlCharactersRemoved || WarnUnreadContent != gofastcore.WarnInputUnreadContent {
		t.Error("the deprecated names do not have the core values")
	}
}
