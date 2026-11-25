package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"cea608"
)

func main() {
	asJSON := flag.Bool("json", false, "emit the full parse result as JSON")
	showXDS := flag.Bool("xds", true, "print XDS packet summaries in text mode")
	flag.Parse()

	var data []byte
	var err error
	if flag.NArg() > 0 {
		data, err = os.ReadFile(flag.Arg(0))
	} else {
		data, err = io.ReadAll(os.Stdin)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "cea608-dump: read input: %v\n", err)
		os.Exit(2)
	}

	result := cea608.Parse(data)
	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(result); err != nil {
			fmt.Fprintf(os.Stderr, "cea608-dump: json: %v\n", err)
			os.Exit(1)
		}
		return
	}

	if result.DisplayText != "" {
		fmt.Println(result.DisplayText)
	}
	if *showXDS {
		for i, packet := range result.XDSPackets {
			fmt.Fprintf(os.Stderr, "xds[%d] class=%d type=%d bytes=%d checksum_ok=%v\n",
				i, packet.Class, packet.Type, len(packet.Content), packet.ChecksumOK)
		}
	}
	if bad := result.FirstBadControl; bad != nil {
		fmt.Fprintf(os.Stderr, "first bad control: code=%04x offset=%d reason=%s\n",
			bad.Code, bad.Offset, bad.Reason)
	}
	if result.TruncatedPairs > 0 {
		fmt.Fprintf(os.Stderr, "truncated pairs: %d; recoveries: %d\n",
			result.TruncatedPairs, result.Recoveries)
	}
}
