package gofastrtf

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/AOShei/go-fast-core"
)

// FuzzExtract checks that no input makes the extractor panic, run away or
// break the contract of the shared model.
func FuzzExtract(f *testing.F) {
	for _, s := range []string{
		wrap(`\pard Hello {\b world}\par`),
		wrap(`\trowd\clmgf\cellx100\clmrg\cellx200\pard\intbl a\cell b\cell\row\pard\intbl\itap2 x\nestcell{\*\nesttableprops\trowd\cellx50\nestrow}\pard\par`),
		wrap(`{\field{\*\fldinst HYPERLINK "x"{\field{\*\fldinst PAGE}{\fldrslt 1}}}{\fldrslt y}}\par`),
		wrap(`{\footnote\pard{\super\chftn} n}{\*\annotation c}{\header h}\par`),
		wrap(`{\pict\pngblip\bin8 12345678}{\object\objemb{\*\objdata 0105000002000000}}`),
		wrap(`{\v hidden}{\cf2 white}{\fs1 tiny}{\deleted gone}\u-10179\'3f\u-8704\'3f`),
		`{\rtf1\ansicpg932{\fonttbl{\f0\fcharset128 M;}}\'82\'a0\'82`,
		`{\rtf1{\*\docvar {a}{b}}{\*\userprops{\propname p}{\staticval v}}{\*\generator g;}}tail`,
		`{\rtf1\ansicpg936{\fonttbl{\f0\fcharset134 S;}{\f1\fcharset136 M;}{\f2\fcharset129 B;}}\'d6\'d0{\f1\'a4\'40}{\f2\'b0\'a1}` + "\xd6\xd0" + `\uc2` + "\\" + "u" + `20013\'d6\'d0\'d6}`,
		wrap(`\pard A\par\pard\brdrb\brdrs\brdrw10 \par\pard\box\brdrs B\par\pard\brdrb\brdrs\page\par`),
		`{\rtf1{\stylesheet{\s1\v\sbasedon2 a;}{\s2\cf1\sbasedon1 b;}{\*\cs3\fs1\sbasedon3 c;}}\pard\s1\cs3 x\par}`,
	} {
		f.Add([]byte(s))
	}
	f.Fuzz(func(t *testing.T, b []byte) {
		// An input that takes this long is a bug: crash, so that the
		// fuzzer keeps it.
		timer := time.AfterFunc(5*time.Second, func() { panic(fmt.Sprintf("slow input: %q", b)) })
		defer timer.Stop()
		for _, opts := range []*gofastcore.Options{
			{Spans: true, Images: gofastcore.ImagesData, Limits: gofastcore.Limits{MaxTextBytes: 1 << 20, MaxDecodedBytes: 1 << 20}},
			{Images: gofastcore.ImagesNone, Child: func(ctx context.Context, name, mime string, r io.ReaderAt, size int64, o *gofastcore.Options) (*gofastcore.Document, error) {
				return nil, gofastcore.ErrUnsupported
			}},
		} {
			doc, err := (&Extractor{MaxGroupDepth: 64}).Extract(context.Background(), bytes.NewReader(b), int64(len(b)), opts)
			if err != nil {
				continue
			}
			if err := doc.Validate(); err != nil {
				t.Fatalf("Validate: %v", err)
			}
		}
	})
}
