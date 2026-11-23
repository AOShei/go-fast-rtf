// Command godoc extracts the text of every file in a list with
// go-fast-rtf, as run_doc.py does with a
// Python tool: one process, WORKERS goroutines reading the files in turn,
// the time of each call (Extract and the text rendering, after the bytes
// are read), whether it failed, and the length of the text; the text goes
// to OUTDIR/text/NAME.txt, one line a file to OUTDIR/results.jsonl.
//
//	godoc rtf LIST OUTDIR WORKERS
package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	gofastcore "github.com/AOShei/go-fast-core"
	gofastrtf "github.com/AOShei/go-fast-rtf"
)

type result struct {
	File  string  `json:"file"`
	S     float64 `json:"s"`
	Err   *string `json:"err"`
	Chars int     `json:"chars"`
}

type extractor func(context.Context, io.ReaderAt, int64, *gofastcore.Options) (*gofastcore.Document, error)

func main() {
	lib, list, out := os.Args[1], os.Args[2], os.Args[3]
	workers, _ := strconv.Atoi(os.Args[4])
	var extract extractor
	switch lib {
	case "rtf":
		extract = (&gofastrtf.Extractor{}).Extract
	default:
		panic("unknown library " + lib)
	}
	f, err := os.Open(list)
	if err != nil {
		panic(err)
	}
	var files []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if sc.Text() != "" {
			files = append(files, sc.Text())
		}
	}
	os.MkdirAll(filepath.Join(out, "text"), 0o755)
	res := make([]result, len(files))
	var wg sync.WaitGroup
	for k := range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := k; i < len(files); i += workers {
				b, err := os.ReadFile(files[i])
				if err != nil {
					panic(err)
				}
				name := filepath.Base(files[i])
				t0 := time.Now()
				// One worker reads one document at a time (Workers: 1),
				// as the commands do with -j above 1.
				doc, err := extract(context.Background(), bytes.NewReader(b), int64(len(b)), &gofastcore.Options{Workers: 1, Name: name})
				var text string
				if err == nil {
					text = doc.Text()
				}
				r := result{File: name, S: time.Since(t0).Seconds(), Chars: len([]rune(text))}
				if err != nil {
					s := fmt.Sprint(err)
					r.Err = &s
				} else {
					os.WriteFile(filepath.Join(out, "text", name+".txt"), []byte(text), 0o644)
				}
				res[i] = r
			}
		}()
	}
	wg.Wait()
	w, _ := os.Create(filepath.Join(out, "results.jsonl"))
	defer w.Close()
	enc := json.NewEncoder(w)
	for _, r := range res {
		enc.Encode(r)
	}
}
