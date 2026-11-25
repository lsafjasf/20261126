package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	eac3 "eac3switch"
)

func main() {
	format := flag.String("format", "markdown", "输出格式：markdown|mermaid|json")
	outPath := flag.String("out", "", "输出文件；为空时写到 stdout")
	flag.Parse()

	graph, _, err := eac3.RunDemo()
	if err != nil {
		fmt.Fprintf(os.Stderr, "eac3switch: %v\n", err)
		os.Exit(1)
	}

	var output []byte
	switch *format {
	case "markdown", "md":
		output = []byte(graph.Markdown())
	case "mermaid":
		output = []byte(graph.Mermaid())
	case "json":
		output, err = json.MarshalIndent(graph, "", "  ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "eac3switch: json: %v\n", err)
			os.Exit(1)
		}
		output = append(output, '\n')
	default:
		fmt.Fprintf(os.Stderr, "eac3switch: unsupported format %q\n", *format)
		os.Exit(2)
	}

	var w io.Writer = os.Stdout
	if *outPath != "" {
		file, err := os.Create(*outPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "eac3switch: create %s: %v\n", *outPath, err)
			os.Exit(2)
		}
		defer file.Close()
		w = file
	}
	if _, err := w.Write(output); err != nil {
		fmt.Fprintf(os.Stderr, "eac3switch: write: %v\n", err)
		os.Exit(1)
	}
}
