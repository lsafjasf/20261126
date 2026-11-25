package ssastyle

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
)

// Exit code 约定：0 成功/一致；1 发现差异（或 strict 下有不可映射项）；2 用法或 IO 错误。
const (
	ExitOK        = 0
	ExitDifferent = 1
	ExitError     = 2
)

// RunCLI 运行 ssastyle 命令行。args 不含程序名。
func RunCLI(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printUsage(stderr)
		return ExitError
	}
	switch args[0] {
	case "compare", "diff":
		return runCompare(args[1:], stdin, stdout, stderr)
	case "resolve":
		return runResolve(args[1:], stdin, stdout, stderr)
	case "convert":
		return runConvert(args[1:], stdin, stdout, stderr)
	case "-h", "--help", "help":
		printUsage(stdout)
		return ExitOK
	default:
		fmt.Fprintf(stderr, "未知子命令: %s\n", args[0])
		printUsage(stderr)
		return ExitError
	}
}

func printUsage(w io.Writer) {
	fmt.Fprint(w, `ssastyle — 旧 SSA 样式继承链 / 空值语义 / 逐字段生效值比对工具（纯 Go 标准库）

用法:
  ssastyle compare -legacy OLD.(ssa|ass|json) -converted NEW.(ass|ssa|json) [选项]
  ssastyle compare -case case.json [选项]
  ssastyle resolve -in FILE [--style Name] [--json]
  ssastyle convert -in FILE [--json]

compare 选项:
  -case FILE          自包含比对任务（可内联两侧文档或引用文件）
  -legacy FILE        旧 SSA/ASS/JSON 样式源
  -converted FILE     转换结果样式（ASS/SSA/JSON）
  -styles A,B         只比对指定样式（默认两侧并集）
  -json               输出机器可读 JSON 报告
  -strict-unmapped    存在不可映射属性时也以退出码 1 结束
  -no-extra           不报告转换侧多出的字段
  -keep-empty         空白字符串视为显式值（默认空白等同 null，不覆盖）
  -null-reset         显式 null 语义为“重置/切断继承”（默认 null=未提供，继续继承）
  -dup-error          同层重复字段取值冲突按错误处理

退出码: 0 一致；1 有差异/strict 命中；2 用法或读取错误。
`)
}

type commonFlags struct {
	jsonOut   bool
	keepEmpty bool
	nullReset bool
	dupError  bool
	styles    string
}

func (cf *commonFlags) bind(fs *flag.FlagSet) {
	fs.BoolVar(&cf.jsonOut, "json", false, "输出 JSON")
	fs.BoolVar(&cf.keepEmpty, "keep-empty", false, "空白串是显式值，不按空值处理")
	fs.BoolVar(&cf.nullReset, "null-reset", false, "显式 null 重置继承而非继续继承")
	fs.BoolVar(&cf.dupError, "dup-error", false, "重复字段冲突视为错误")
	fs.StringVar(&cf.styles, "styles", "", "逗号分隔的样式名过滤")
}

func (cf *commonFlags) mergeOptions() MergeOptions {
	m := DefaultMergeOptions()
	if cf.keepEmpty {
		m.EmptyIsUnset = false
	}
	m.ExplicitNullResets = cf.nullReset
	m.DuplicateConflictIsError = cf.dupError
	return m
}

func (cf *commonFlags) styleList() []string {
	if strings.TrimSpace(cf.styles) == "" {
		return nil
	}
	parts := strings.Split(cf.styles, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func newFlagSet(name string, stderr io.Writer) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	return fs
}

func loadPath(path string, stdin io.Reader) (*Document, []Diagnostic, error) {
	if path == "-" {
		data, err := io.ReadAll(stdin)
		if err != nil {
			return nil, nil, err
		}
		return ParseDocumentBytes(data, "stdin")
	}
	return LoadDocumentFile(path)
}

func writeJSON(w io.Writer, v any) {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}

func failErr(stderr io.Writer, err error) int {
	if errors.Is(err, flag.ErrHelp) {
		return ExitOK
	}
	fmt.Fprintf(stderr, "错误: %v\n", err)
	return ExitError
}
