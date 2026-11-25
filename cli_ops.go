package ssastyle

import (
	"fmt"
	"io"
)

func runResolve(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := newFlagSet("resolve", stderr)
	var cf commonFlags
	cf.bind(fs)
	var inPath, onlyStyle string
	fs.StringVar(&inPath, "in", "", "输入文件 (.ssa/.ass/.json，- 表示标准输入)")
	fs.StringVar(&onlyStyle, "style", "", "只显示指定样式")
	if err := fs.Parse(args); err != nil {
		return failErr(stderr, err)
	}
	if inPath == "" {
		fmt.Fprintln(stderr, "resolve 需要 -in 文件")
		fs.Usage()
		return ExitError
	}
	doc, diags, err := loadPath(inPath, stdin)
	if err != nil {
		return failErr(stderr, err)
	}
	opts := cf.mergeOptions()
	cat := NewCatalog(opts, doc)
	eff := cat.ResolveAll()
	allDiags := append(append([]Diagnostic{}, diags...), cat.Diagnostics...)
	for _, es := range eff {
		allDiags = append(allDiags, es.Diagnostics...)
	}
	allDiags = DedupeDiagnostics(allDiags)

	if cf.jsonOut {
		out := struct {
			Styles      map[string]*EffectiveStyle `json:"styles"`
			Diagnostics []Diagnostic               `json:"diagnostics,omitempty"`
		}{eff, allDiags}
		writeJSON(stdout, out)
		return ExitOK
	}
	if onlyStyle != "" {
		if !cat.Has(onlyStyle) {
			fmt.Fprintf(stderr, "未找到样式 %q\n", onlyStyle)
			return ExitError
		}
		RenderEffectiveOne(stdout, onlyStyle, eff[onlyStyle])
	} else {
		RenderEffectiveText(stdout, cat, eff)
	}
	renderDiags(stderr, allDiags)
	return ExitOK
}

func runConvert(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := newFlagSet("convert", stderr)
	var cf commonFlags
	cf.bind(fs)
	var inPath string
	var format string
	fs.StringVar(&inPath, "in", "", "输入旧样式文件 (.ssa/.json，- 为标准输入)")
	fs.StringVar(&format, "format", "json", "输出格式: json | ass")
	if err := fs.Parse(args); err != nil {
		return failErr(stderr, err)
	}
	if inPath == "" {
		fmt.Fprintln(stderr, "convert 需要 -in 文件")
		fs.Usage()
		return ExitError
	}
	doc, diags, err := loadPath(inPath, stdin)
	if err != nil {
		return failErr(stderr, err)
	}
	rep := Convert(doc, ConvertOptions{Merge: cf.mergeOptions()})
	rep.Diagnostics = append(diags, rep.Diagnostics...)
	switch format {
	case "ass":
		io.WriteString(stdout, FormatASSStyles(rep))
		if len(rep.Unmapped) > 0 {
			fmt.Fprintf(stderr, "提示: %d 个不可映射属性未写入 ASS；用 -format json 查看 unmapped_properties\n", len(rep.Unmapped))
		}
	default:
		writeJSON(stdout, rep)
		if len(rep.Unmapped) > 0 {
			fmt.Fprintf(stderr, "提示: %d 个属性无法直接映射，已在 JSON 的 unmapped_properties 中单独报告\n", len(rep.Unmapped))
		}
	}
	return ExitOK
}

// RenderEffectiveOne 输出单个生效样式。
func RenderEffectiveOne(w io.Writer, name string, es *EffectiveStyle) {
	if es == nil {
		return
	}
	fmt.Fprintf(w, "样式 %q  继承链: %s\n", name, chainArrow(es.Chain))
	for _, k := range sortedFieldKeys(es.Fields) {
		f := es.Fields[k]
		if f.Reset {
			fmt.Fprintf(w, "  %-15s <reset by explicit null>  (来源 %s)\n", k, sourceOr(f.SourceStyle))
			continue
		}
		fmt.Fprintf(w, "  %-15s = %q  (来源 %s)\n", k, f.Raw, sourceOr(f.SourceStyle))
	}
}

func renderDiags(w io.Writer, diags []Diagnostic) {
	if len(diags) == 0 {
		return
	}
	fmt.Fprintln(w, "诊断:")
	for _, d := range diags {
		fmt.Fprintf(w, "  (%s/%s) %s.%s %s\n", d.Level, d.Code, d.Style, d.Field, d.Message)
	}
}
