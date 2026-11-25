package ssastyle

import (
	"fmt"
	"io"
)

func runCompare(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := newFlagSet("compare", stderr)
	var cf commonFlags
	cf.bind(fs)
	var casePath, legacyPath, convertedPath string
	var strictUnmapped bool
	var noExtra bool
	fs.StringVar(&casePath, "case", "", "自包含 case JSON")
	fs.StringVar(&legacyPath, "legacy", "", "旧 SSA/ASS/JSON")
	fs.StringVar(&convertedPath, "converted", "", "转换后文件")
	fs.BoolVar(&strictUnmapped, "strict-unmapped", false, "不可映射属性也导致退出码 1")
	fs.BoolVar(&noExtra, "no-extra", false, "不报告转换侧多余字段")
	if err := fs.Parse(args); err != nil {
		return failErr(stderr, err)
	}

	var (
		legacyDoc, convertedDoc *Document
		loadDiags               []Diagnostic
		opts                    CompareOptions
	)
	opts = DefaultCompareOptions()
	opts.Merge = cf.mergeOptions()
	opts.Styles = cf.styleList()
	opts.ReportExtra = !noExtra
	strict := strictUnmapped

	if casePath != "" {
		c, diags, err := LoadCaseFile(casePath)
		if err != nil {
			return failErr(stderr, err)
		}
		loadDiags = append(loadDiags, diags...)
		legacyDoc, convertedDoc = c.Legacy, c.Converted
		caseOpts := c.CompareOptions()
		// 命令行显式参数优先于 case。
		if len(cf.styleList()) > 0 {
			caseOpts.Styles = cf.styleList()
		}
		opts = caseOpts
		opts.Merge = mergeCLIFlags(opts.Merge, cf, args)
		strict = strictUnmapped || c.StrictUnmapped
	} else {
		if legacyPath == "" || convertedPath == "" {
			fmt.Fprintln(stderr, "compare 需要 -legacy 与 -converted，或使用 -case")
			fs.Usage()
			return ExitError
		}
		ld, d1, err := loadPath(legacyPath, stdin)
		if err != nil {
			return failErr(stderr, fmt.Errorf("读取 legacy: %w", err))
		}
		cd, d2, err := loadPath(convertedPath, stdin)
		if err != nil {
			return failErr(stderr, fmt.Errorf("读取 converted: %w", err))
		}
		legacyDoc, convertedDoc = ld, cd
		loadDiags = append(loadDiags, d1...)
		loadDiags = append(loadDiags, d2...)
	}

	rep := Compare(legacyDoc, convertedDoc, opts)
	rep.Diagnostics = append(loadDiags, rep.Diagnostics...)
	rep.summarize()

	if cf.jsonOut {
		writeJSON(stdout, rep)
	} else {
		RenderTextReport(stdout, rep)
	}

	if !rep.Equal || rep.Summary.Errors > 0 {
		return ExitDifferent
	}
	if strict && len(rep.Unmapped) > 0 {
		if !cf.jsonOut {
			fmt.Fprintf(stdout, "strict-unmapped: 存在 %d 个不可映射/有损派生属性\n", len(rep.Unmapped))
		}
		return ExitDifferent
	}
	return ExitOK
}

// explicit 判断某个 flag 是否在命令行被显式设置。
func explicit(name string, args []string) bool {
	for _, a := range args {
		if a == "-"+name || a == "--"+name || len(a) > len(name)+1 &&
			(a[:len(name)+2] == "-"+name+"=" || a[:len(name)+3] == "--"+name+"=") {
			return true
		}
	}
	return false
}

// mergeCLIFlags 让命令行显式开关覆盖 case 中的合并选项。
func mergeCLIFlags(base MergeOptions, cf commonFlags, args []string) MergeOptions {
	if explicit("keep-empty", args) {
		base.EmptyIsUnset = !cf.keepEmpty
	}
	if explicit("null-reset", args) {
		base.ExplicitNullResets = cf.nullReset
	}
	if explicit("dup-error", args) {
		base.DuplicateConflictIsError = cf.dupError
	}
	return base
}
