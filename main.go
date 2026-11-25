package main

import (
	"fmt"
	"io"
	"os"
)

func main() {
	var r io.Reader
	if len(os.Args) < 2 || os.Args[1] == "-" {
		r = os.Stdin
	} else {
		f, err := os.Open(os.Args[1])
		if err != nil {
			fmt.Fprintln(os.Stderr, "打开文件失败:", err)
			os.Exit(2)
		}
		defer f.Close()
		r = f
	}
	edl, err := ParseEDL(r)
	if err != nil {
		fmt.Fprintln(os.Stderr, "解析失败:", err)
		os.Exit(1)
	}
	fmt.Print(Report(edl))
}
