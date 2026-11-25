package sdh

import "io"

// Options bundles the editable policies for Process / ProcessSRT.
type Options struct {
	Plain  PlainOptions
	Layout LayoutOptions
}

// DefaultOptions returns default SDH-off and overlap policies.
func DefaultOptions() Options {
	return Options{
		Plain:  DefaultPlainOptions(),
		Layout: DefaultLayoutOptions(),
	}
}

// Result is the complete output of processing one SDH subtitle file.
type Result struct {
	Cues         []Cue
	Project      *Project
	Plain        []Cue
	Verification *CompletenessReport
	Report       *Report
}

// ProcessCues splits cues into semantic layers, resolves overlaps and builds
// the SDH-off plain track plus the completeness report.
func ProcessCues(cues []Cue, opts Options) *Result {
	project := SplitWithOptions(cues, opts.Layout)
	plain := project.Plain(opts.Plain)
	verification := VerifyCompletenessWith(cues, plain, VerifyOptions{PlainOptions: opts.Plain})
	return &Result{
		Cues:         project.Cues,
		Project:      project,
		Plain:        plain,
		Verification: verification,
		Report:       BuildReport(project, plain, verification),
	}
}

// ProcessSRT parses SRT bytes and runs ProcessCues.
func ProcessSRT(data []byte, opts Options) (*Result, error) {
	cues, err := ParseSRTBytes(data)
	if err != nil {
		return nil, err
	}
	return ProcessCues(cues, opts), nil
}

// ProcessReader parses SRT from r and runs ProcessCues.
func ProcessReader(r io.Reader, opts Options) (*Result, error) {
	cues, err := ParseSRT(r)
	if err != nil {
		return nil, err
	}
	return ProcessCues(cues, opts), nil
}
