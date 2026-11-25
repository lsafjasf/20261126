// Command imsc-check validates an IMSC image subtitle document and reports
// every element-level failure. Exit status:
//
//	0 - document parsed and at least one decodable image exists
//	1 - validation errors or no decodable image (ErrNoImages)
//	2 - usage error or unreadable input
package main

import (
	"errors"
	"fmt"
	"os"

	"imscloader/imsc"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintf(os.Stderr, "usage: %s <imsc-document.xml>\n", os.Args[0])
		os.Exit(2)
	}

	f, err := os.Open(os.Args[1])
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot open %s: %v\n", os.Args[1], err)
		os.Exit(2)
	}
	defer f.Close()

	doc, err := imsc.Load(f)
	if doc == nil {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("images: %d decodable\n", len(doc.Images))
	fmt.Printf("regions: %d\n", len(doc.Regions))
	fmt.Printf("cues: %d (placeholders: %d)\n", len(doc.Cues), len(doc.PlaceholderCues()))

	for _, elemErr := range doc.ElementErrors.ElementErrors() {
		fmt.Printf("ERROR %s\n", elemErr.Error())
	}
	for _, cue := range doc.Cues {
		status := "image"
		if cue.Placeholder != nil {
			status = "placeholder"
		}
		fmt.Printf("cue line %d %s begin=%v end=%v region=%q images=%d fallback=%q\n",
			cue.Line, status, cue.Begin, cueEnd(cue), cue.RegionID, len(cue.Images), cue.FallbackText)
	}

	if errors.Is(err, imsc.ErrNoImages) {
		fmt.Println("RESULT: no decodable images; text track must use placeholder/fallback content")
		os.Exit(1)
	}
	if err != nil {
		fmt.Println("RESULT: validation failed")
		os.Exit(1)
	}
	fmt.Println("RESULT: ok")
}

func cueEnd(cue *imsc.Cue) string {
	if !cue.HasEnd {
		return "open"
	}
	return cue.End.String()
}
