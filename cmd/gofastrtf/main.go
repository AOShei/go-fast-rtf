// Command gofastrtf extracts RTF files into the shared document model of
// the go-fast libraries and prints them as JSON, Markdown or plain text.
//
//	gofastrtf [flags] file-or-directory...
//
// A directory stands for the .rtf files in it and below it. With one input
// the JSON is indented; with several, each document is one line. Only the
// documents go to standard output: everything said about them goes to
// standard error, and the exit status is 1 if any input could not be read.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/signal"
	"path/filepath"
	"runtime/debug"
	"sort"
	"strings"

	"github.com/AOShei/go-fast-core"
	"github.com/AOShei/go-fast-rtf"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	os.Exit(run(ctx, os.Args[1:], os.Stdout, os.Stderr))
}

// displayFlag is --exclude-display: given alone it stands for every reason,
// and with a value for the reasons named.
type displayFlag struct{ reasons gofastcore.Display }

func (h *displayFlag) IsBoolFlag() bool { return true }
func (h *displayFlag) String() string {
	if h.reasons == 0 {
		return ""
	}
	return h.reasons.String()
}

func (h *displayFlag) Set(v string) error {
	switch v {
	case "true", "all":
		h.reasons = gofastcore.DisplayAny
		return nil
	case "false", "":
		h.reasons = 0
		return nil
	}
	var reasons gofastcore.Display
	if err := reasons.UnmarshalText([]byte(strings.ReplaceAll(v, ",", "+"))); err != nil {
		return err
	}
	if !reasons.Valid() || reasons == 0 {
		return fmt.Errorf("unknown reason in %q; the reasons are %s", v, strings.ReplaceAll((gofastcore.DisplayAny&^gofastcore.DisplayUnknown).String(), "+", ", "))
	}
	h.reasons = reasons
	return nil
}

// run is the command: it reads args and writes documents to stdout and
// messages to stderr, and returns the exit status.
func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("gofastrtf", flag.ContinueOnError)
	flags.SetOutput(stderr)
	var (
		format   = flags.String("format", "json", "what to print: json, markdown or text")
		output   = flags.String("o", "", "write to this file instead of standard output")
		images   = flags.String("images", "meta", "pictures: meta (a block for each, with its size and alternative text), data (their bytes too) or none")
		spans    = flags.Bool("spans", false, "report bold, italic and the like, and links, as spans")
		maxDepth = flags.Int("max-group-depth", 0, "how deeply groups may nest (0: the default, 1024; -1: no limit)")
		maxBytes = flags.Int64("max-decoded-bytes", 0, "bytes of picture and object data kept (0: the default, 1 GiB; -1: no limit)")
		maxText  = flags.Int64("max-text-bytes", 0, "bytes of text a document and the files embedded in it may hold (0: the default, 256 MiB; -1: no limit)")
		quiet    = flags.Bool("quiet", false, "say nothing about files that were read; failures are still reported")
		version  = flags.Bool("version", false, "print the version and exit")
		// For Markdown and text.
		furniture = flags.Bool("furniture", false, "in Markdown and text, include running headers and footers")
		stored    = flags.Bool("stored", false, "in Markdown and text, include the stored part (document variables, custom properties, the generator, data after the end)")
	)
	var exclude displayFlag
	flags.Var(&exclude, "exclude-display", "in Markdown and text, leave out what the file declares is not shown: all of it, or with =reasons only those reasons, such as hidden (JSON always has everything, with its reasons)")
	flags.Usage = func() {
		fmt.Fprintln(stderr, "usage: gofastrtf [flags] file-or-directory...")
		fmt.Fprintln(stderr, "Extracts RTF files. A directory stands for the .rtf files in it and below it.")
		flags.PrintDefaults()
	}
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if *version {
		fmt.Fprintln(stdout, "gofastrtf", moduleVersion())
		return 0
	}
	fail := func(format string, args ...any) int {
		fmt.Fprintf(stderr, "gofastrtf: "+format+"\n", args...)
		return 2
	}
	if flags.NArg() == 0 {
		flags.Usage()
		return 2
	}
	if *format != "json" && *format != "markdown" && *format != "text" {
		return fail("unknown format %q: json, markdown or text", *format)
	}
	opts := gofastcore.Options{Spans: *spans}
	opts.Limits.MaxDecodedBytes, opts.Limits.MaxTextBytes = *maxBytes, *maxText
	switch *images {
	case "none":
		opts.Images = gofastcore.ImagesNone
	case "meta":
		opts.Images = gofastcore.ImagesMeta
	case "data":
		opts.Images = gofastcore.ImagesData
	default:
		return fail("unknown value %q for -images: none, meta or data", *images)
	}
	extractor := &gofastrtf.Extractor{MaxGroupDepth: *maxDepth}
	// An object embedded in a document is read too if it is an RTF file.
	opts.Child = func(ctx context.Context, name, mime string, r io.ReaderAt, size int64, o *gofastcore.Options) (*gofastcore.Document, error) {
		doc, err := extractor.Extract(ctx, r, size, o)
		if errors.Is(err, gofastcore.ErrMalformed) {
			return nil, fmt.Errorf("%w: not an RTF file", gofastcore.ErrUnsupported)
		}
		return doc, err
	}

	var inputs []string
	status := 0
	for _, arg := range flags.Args() {
		info, err := os.Stat(arg)
		if err != nil {
			fmt.Fprintf(stderr, "gofastrtf: %v\n", err)
			status = 1
			continue
		}
		if !info.IsDir() {
			inputs = append(inputs, arg)
			continue
		}
		var found []string
		err = filepath.WalkDir(arg, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				fmt.Fprintf(stderr, "gofastrtf: %v\n", err)
				status = 1
				return nil
			}
			if !d.IsDir() && strings.EqualFold(filepath.Ext(path), ".rtf") {
				found = append(found, path)
			}
			return nil
		})
		if err != nil {
			fmt.Fprintf(stderr, "gofastrtf: %v\n", err)
			status = 1
		}
		sort.Strings(found)
		inputs = append(inputs, found...)
	}

	out := stdout
	if *output != "" {
		file, err := os.Create(*output)
		if err != nil {
			fmt.Fprintf(stderr, "gofastrtf: %v\n", err)
			return 1
		}
		defer file.Close()
		out = file
	}
	w := bufio.NewWriter(out)
	defer w.Flush()
	render := &gofastcore.RenderOptions{Furniture: *furniture, Stored: *stored, ExcludeDisplay: exclude.reasons}

	written := 0
	for _, path := range inputs {
		if ctx.Err() != nil {
			fmt.Fprintln(stderr, "gofastrtf: interrupted")
			return 1
		}
		o := opts
		o.Name = filepath.Base(path)
		doc, err := extractor.ExtractFile(ctx, path, &o)
		if err != nil {
			fmt.Fprintf(stderr, "%s: %v\n", path, err)
			status = 1
			continue
		}
		if !*quiet {
			if note := remarks(doc); note != "" {
				fmt.Fprintf(stderr, "%s: %s\n", path, note)
			}
		}
		switch *format {
		case "json":
			enc := json.NewEncoder(w)
			enc.SetEscapeHTML(false)
			if len(inputs) == 1 {
				enc.SetIndent("", "  ")
			}
			err = enc.Encode(doc)
		case "markdown":
			if len(inputs) > 1 {
				if written > 0 {
					fmt.Fprintln(w)
				}
				fmt.Fprintf(w, "<!-- %s -->\n\n", strings.ReplaceAll(path, "--", "- -"))
			}
			if err = doc.WriteMarkdown(w, render); err == nil {
				_, err = fmt.Fprintln(w)
			}
		default:
			if len(inputs) > 1 {
				if written > 0 {
					fmt.Fprintln(w)
				}
				fmt.Fprintf(w, "==> %s <==\n", path)
			}
			if err = doc.WriteText(w, render); err == nil {
				_, err = fmt.Fprintln(w)
			}
		}
		if err == nil {
			err = w.Flush()
		}
		if err != nil {
			fmt.Fprintf(stderr, "gofastrtf: writing %s: %v\n", path, err)
			return 1
		}
		written++
	}
	return status
}

// remarks sums up what a document's warnings say that a person running the
// command would want to know without reading the JSON: anything of
// severity error.
func remarks(doc *gofastcore.Document) string {
	counts := map[string]int{}
	for _, w := range doc.AllWarnings() {
		if w.Severity == gofastcore.SeverityError {
			counts[w.Code]++
		}
	}
	if len(counts) == 0 {
		return ""
	}
	codes := make([]string, 0, len(counts))
	for code := range counts {
		codes = append(codes, code)
	}
	sort.Strings(codes)
	for i, code := range codes {
		if n := counts[code]; n > 1 {
			codes[i] = fmt.Sprintf("%s (%d)", code, n)
		}
	}
	return "warnings: " + strings.Join(codes, ", ")
}

// moduleVersion is the version this command was built from.
func moduleVersion() string {
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" {
		return info.Main.Version
	}
	return "(devel)"
}
