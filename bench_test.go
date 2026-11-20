package gofastrtf

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
)

// BenchmarkExtract extracts three files of the public corpus's dev set
// (GOFAST_CORPUS, or ../.testdata/corpus), from memory. It is skipped
// without the corpus.
func BenchmarkExtract(b *testing.B) {
	root := os.Getenv("GOFAST_CORPUS")
	if root == "" {
		root = filepath.Join("..", ".testdata", "corpus")
	}
	for _, name := range []string{"tika-testRTFVarious.rtf", "tika-testRTFTIKA_2899.rtf", "tika-testRTFListMicrosoftWord.rtf"} {
		data, err := os.ReadFile(filepath.Join(root, "dev", "rtf", name))
		if err != nil {
			b.Skip("no corpus at", root)
		}
		b.Run(name, func(b *testing.B) {
			b.SetBytes(int64(len(data)))
			b.ReportAllocs()
			for b.Loop() {
				if _, err := Extract(context.Background(), bytes.NewReader(data), int64(len(data)), nil); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
