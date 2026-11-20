package gofastrtf

import (
	"runtime"
	"strings"
	"testing"

	"github.com/AOShei/go-fast-core"
)

// TestTableCutAtBudget checks that a table larger than the text budget is
// cut at the limit, keeping the rows that fit, rather than left out whole.
func TestTableCutAtBudget(t *testing.T) {
	row := `\trowd\cellx3000\cellx6000\pard\intbl ` + strings.Repeat("x", 100) + `\cell ` + strings.Repeat("y", 100) + `\cell\row`
	src := wrap(`\pard Before\par` + strings.Repeat(row, 200) + `\pard After\par`)
	doc := extract(t, src, &gofastcore.Options{Limits: gofastcore.Limits{MaxTextBytes: 8192}})
	body := doc.Parts[0]
	if !body.Has(gofastcore.WarnLimitExceeded) {
		t.Fatalf("no limit.exceeded: %+v", body.Warnings)
	}
	var tb *gofastcore.Table
	for _, b := range body.Blocks {
		if b.Kind == gofastcore.KindTable {
			tb = b.Table
		}
	}
	if tb == nil || len(tb.Rows) == 0 || len(tb.Rows) >= 200 {
		t.Fatalf("table cut at the limit should keep the rows that fit: %s", dump(doc))
	}
}

// TestHeldBlocksBounded checks that blocks held back until their
// paragraph ends (field notes, here) are bounded by the text budget while
// they are read, not only when the paragraph is finished: a paragraph of
// many small fields costs far more in blocks than in input.
func TestHeldBlocksBounded(t *testing.T) {
	if testing.Short() {
		t.Skip("allocates a few megabytes of input")
	}
	field := `{\field{\*\fldinst PAGE}{\fldrslt 1}}`
	src := wrap(`\pard ` + strings.Repeat(field, 200_000) + `\par`)
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	doc := extract(t, src, &gofastcore.Options{Limits: gofastcore.Limits{MaxTextBytes: 64 << 10}})
	runtime.ReadMemStats(&after)
	if !doc.Parts[0].Has(gofastcore.WarnLimitExceeded) {
		t.Errorf("no limit.exceeded: %+v", doc.Parts[0].Warnings)
	}
	if got := after.TotalAlloc - before.TotalAlloc; got > 48<<20 {
		t.Errorf("allocated %d MB for %d KB of input under a 64 KB text budget", got>>20, len(src)>>10)
	}
}
