package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"os"

	"channelmap"
)

func main() {
	input := flag.String("input", "", "path to the WAV file (or pass it as the first positional argument)")
	signaturesPath := flag.String("signatures", "", "optional JSON role-feature signature file")
	referencePath := flag.String("reference", "", "optional known-good reference WAV used as per-role sample template")
	calibration := flag.Bool("calibration", false, "use the bundled wavchannel-calibration-v1 test-tone profile")
	profile := flag.String("profile", "auto", "feature profile: auto, calibration-v1, or none")
	asJSON := flag.Bool("json", false, "emit machine-readable JSON")
	sha := flag.Bool("sha256", false, "include SHA-256 of the input bytes in JSON/report metadata")
	check := flag.Bool("check", false, "exit 1 when a confidence-qualified conflict is found")
	repairOutput := flag.String("repair-output", "", "optional path for a WAV reordered into mask order")
	flag.Parse()

	path := *input
	if path == "" && flag.NArg() > 0 {
		path = flag.Arg(0)
	}
	if path == "" {
		fmt.Fprintln(os.Stderr, "usage: wavchannelcheck -input file.wav [-signatures roles.json] [-calibration] [-json] [-check]")
		os.Exit(64)
	}

	var data []byte
	var err error
	if path == "-" {
		data, err = io.ReadAll(os.Stdin)
		if err != nil {
			fmt.Fprintf(os.Stderr, "read WAV from stdin: %v\n", err)
			os.Exit(2)
		}
	} else {
		data, err = os.ReadFile(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "read WAV: %v\n", err)
			os.Exit(2)
		}
	}

	reportPath := path
	if path == "-" {
		reportPath = "stdin"
	}
	opts := channelmap.Options{Path: reportPath, Profile: *profile, UseCalibrationProfile: *calibration, IncludeSampleSHA256: *sha}
	if *signaturesPath != "" {
		if *referencePath != "" {
			fmt.Fprintln(os.Stderr, "use either -signatures or -reference, not both")
			os.Exit(64)
		}
		f, err := os.Open(*signaturesPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "read signatures: %v\n", err)
			os.Exit(2)
		}
		sig, err := channelmap.LoadSignatures(f)
		closeErr := f.Close()
		if err != nil {
			fmt.Fprintf(os.Stderr, "parse signatures: %v\n", err)
			os.Exit(2)
		}
		if closeErr != nil {
			fmt.Fprintf(os.Stderr, "close signatures: %v\n", closeErr)
			os.Exit(2)
		}
		opts.Signatures = sig
	}
	if *referencePath != "" {
		refData, err := os.ReadFile(*referencePath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "read reference WAV: %v\n", err)
			os.Exit(2)
		}
		refFile, err := channelmap.Parse(bytes.NewReader(refData))
		if err != nil {
			fmt.Fprintf(os.Stderr, "parse reference WAV: %v\n", err)
			os.Exit(2)
		}
		sig, err := channelmap.SignaturesFromReference(refFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "build reference signatures: %v\n", err)
			os.Exit(2)
		}
		opts.Signatures = sig
	}

	report, err := channelmap.AnalyzeBytes(data, opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "analyze WAV: %v\n", err)
		os.Exit(2)
	}

	if *asJSON {
		out, err := channelmap.RenderJSON(report)
		if err != nil {
			fmt.Fprintf(os.Stderr, "encode JSON: %v\n", err)
			os.Exit(2)
		}
		fmt.Println(string(out))
	} else {
		fmt.Print(channelmap.RenderText(report))
	}

	if *repairOutput != "" {
		if report.Repair == nil {
			fmt.Fprintln(os.Stderr, "cannot write repair WAV: no unique reliable mapping")
			os.Exit(3)
		}
		file, err := channelmap.Parse(bytes.NewReader(data))
		if err != nil {
			fmt.Fprintf(os.Stderr, "reparse WAV for repair: %v\n", err)
			os.Exit(2)
		}
		reordered, err := channelmap.ReorderChannels(file.Samples, report.Repair.SourceIndicesForMaskOrder)
		if err != nil {
			fmt.Fprintf(os.Stderr, "reorder channels: %v\n", err)
			os.Exit(3)
		}
		out, err := channelmap.EncodeWAV(reordered, channelmap.EncodeWAVOptions{
			SampleRate:    file.Format.SampleRate,
			BitsPerSample: file.Format.BitsPerSample,
			IEEEFloat:     file.Format.Codec.SampleKind == "ieee-float",
			ChannelMask:   file.Format.ChannelMask,
			Extensible:    file.Format.HasChannelMask,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "encode repaired WAV: %v\n", err)
			os.Exit(3)
		}
		if err := os.WriteFile(*repairOutput, out, 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "write repaired WAV: %v\n", err)
			os.Exit(2)
		}
		fmt.Fprintf(os.Stderr, "wrote repaired WAV: %s\n", *repairOutput)
	}

	if *check && report.Status == channelmap.StatusConflict {
		os.Exit(1)
	}
}
