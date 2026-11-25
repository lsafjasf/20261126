package cafmarker

import (
	"bytes"
	"io"
)

// ParseBytes parses a complete CAF file held in memory.
func ParseBytes(data []byte) (*File, error) {
	return ParseReadSeeker(bytes.NewReader(data), int64(len(data)))
}

// ParseReader reads a complete CAF stream into memory and parses it.
func ParseReader(r io.Reader) (*File, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	return ParseBytes(data)
}

// DiagnoseBytes parses data and runs marker diagnosis.
func DiagnoseBytes(data []byte, opts Options) (*File, *Report, error) {
	f, err := ParseBytes(data)
	if f == nil {
		return nil, nil, err
	}
	report := Diagnose(f, opts)
	if err != nil {
		report.Findings = append(report.Findings, Issue{Code: "parse_error", Message: err.Error()})
	}
	return f, report, err
}
