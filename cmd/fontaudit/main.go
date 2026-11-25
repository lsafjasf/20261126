package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"fontaudit/internal/audit"
	"fontaudit/internal/fontscan"
)

func main() {
	var (
		fontPath  = flag.String("font", "", "primary burn-in font file (TTF/OTF/TTC/OTC)")
		faceIndex = flag.Int("face", 0, "face index when the font file is a TTC/OTC collection")
		scanDirs  = flag.String("fallback-dirs", "", "comma-separated font directories to scan for fallback candidates (recommendations only)")
		proofDir  = flag.String("proof-dir", "", "directory for tofu proof PNGs (default: no rendering)")
		maxProofs = flag.Int("max-proofs", 12, "maximum number of proof images")
		asJSON    = flag.Bool("json", false, "emit machine-readable JSON instead of the text report")
		outFile   = flag.String("out", "", "write report to this file as well")
	)
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "fontaudit: subtitle burn-in glyph coverage audit (Go)\n\n")
		fmt.Fprintf(os.Stderr, "Usage: fontaudit -font primary.ttf [-face N] [flags] subtitle.srt|ass|vtt\n\n")
		flag.PrintDefaults()
		fmt.Fprintln(os.Stderr, "\nThe tool never modifies the subtitle or silently swaps fonts.")
	}
	flag.Parse()
	if flag.NArg() != 1 || *fontPath == "" {
		flag.Usage()
		os.Exit(2)
	}

	subPath := flag.Arg(0)
	raw, err := os.ReadFile(subPath)
	if err != nil {
		fatal("read subtitle: %v", err)
	}
	ff, err := fontscan.Open(*fontPath)
	if err != nil {
		fatal("open font: %v", err)
	}
	if *faceIndex < 0 || *faceIndex >= len(ff.Faces) {
		fatal("face index %d out of range; %s contains %d face(s)", *faceIndex, *fontPath, len(ff.Faces))
	}
	primary := ff.Faces[*faceIndex]

	var dirs []string
	if *scanDirs != "" {
		for _, d := range strings.Split(*scanDirs, ",") {
			if d = strings.TrimSpace(d); d != "" {
				dirs = append(dirs, d)
			}
		}
	}

	rep, err := audit.Run(audit.Options{
		SubtitlePath: subPath,
		SubtitleRaw:  raw,
		Primary:      primary,
		ScanDirs:     dirs,
		ProofDir:     *proofDir,
		MaxProofs:    *maxProofs,
	})
	if err != nil {
		fatal("audit: %v", err)
	}

	var payload []byte
	if *asJSON {
		payload, err = json.MarshalIndent(rep, "", "  ")
	} else {
		payload = []byte(renderText(rep))
	}
	if err != nil {
		fatal("render report: %v", err)
	}
	if *outFile != "" {
		if err := os.WriteFile(*outFile, append(payload, '\n'), 0o644); err != nil {
			fatal("write report: %v", err)
		}
	}
	os.Stdout.Write(append(payload, '\n'))

	// Non-zero on any evidence of tofu or encoding corruption so CI can gate.
	if rep.DistinctMissing > 0 || rep.Encoding.ReplacementRunes > 0 || rep.Encoding.Mojibake {
		os.Exit(1)
	}
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "fontaudit: "+format+"\n", args...)
	os.Exit(2)
}
