package ssastyle

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func runCLI(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var out, errb bytes.Buffer
	code := RunCLI(args, strings.NewReader(""), &out, &errb)
	return code, out.String(), errb.String()
}

func TestCLIHelpAndNoArgs(t *testing.T) {
	if code, out, _ := runCLI(t, "--help"); code != ExitOK || !strings.Contains(out, "compare") {
		t.Fatalf("help code=%d out=%q", code, out)
	}
	if code, _, _ := runCLI(t); code != ExitError {
		t.Fatalf("无参数应返回 %d, got %d", ExitError, code)
	}
	if code, _, _ := runCLI(t, "bogus"); code != ExitError {
		t.Fatalf("未知子命令应返回 %d", ExitError)
	}
}

func TestCLICompareOKCase(t *testing.T) {
	code, out, errb := runCLI(t, "compare", "-case", "testdata/case_ok.json", "-json")
	if code != ExitOK {
		t.Fatalf("应一致并返回 0, code=%d stderr=%s out=%s", code, errb, out)
	}
	var rep Report
	if err := json.Unmarshal([]byte(out), &rep); err != nil {
		t.Fatalf("JSON 输出无法解析: %v\n%s", err, out)
	}
	if !rep.Equal || rep.Summary.Different != 0 || rep.Summary.MissingInConverted != 0 {
		t.Fatalf("case_ok 应完全一致: %+v", rep.Summary)
	}
}

func TestCLICompareBuggyCase(t *testing.T) {
	code, out, _ := runCLI(t, "compare", "-case", "testdata/case_buggy.json", "-json")
	if code != ExitDifferent {
		t.Fatalf("有差异应返回 %d, got %d", ExitDifferent, code)
	}
	var rep Report
	if err := json.Unmarshal([]byte(out), &rep); err != nil {
		t.Fatal(err)
	}
	if rep.Equal {
		t.Fatal("buggy case 不应 equal")
	}
	if rep.Summary.MissingInConverted == 0 && rep.Summary.Different == 0 {
		t.Fatal("应至少有 missing/diff")
	}
	if rep.Summary.Unmapped == 0 {
		t.Fatal("应单独报告不可映射属性")
	}
}

func TestCLIStrictUnmapped(t *testing.T) {
	// 不加 strict：字段一致 -> 0。
	if code, out, _ := runCLI(t, "compare", "-case", "testdata/case_unmapped.json", "-json"); code != ExitOK {
		t.Fatalf("普通模式下仅不可映射不应失败, code=%d out=%s", code, out)
	}
	// 加 strict：存在 unmapped -> 1。
	code, out, _ := runCLI(t, "compare", "-case", "testdata/case_unmapped.json", "-strict-unmapped")
	if code != ExitDifferent {
		t.Fatalf("strict-unmapped 应返回 %d, got %d", ExitDifferent, code)
	}
	if !strings.Contains(out, "不可映射") && !strings.Contains(out, "strict") {
		t.Fatalf("文本输出应提示 strict/unmapped:\n%s", out)
	}
}

func TestCLIResolveSSA(t *testing.T) {
	code, out, errb := runCLI(t, "resolve", "-in", "testdata/legacy.ssa", "-json")
	if code != ExitOK {
		t.Fatalf("resolve 失败 code=%d err=%s", code, errb)
	}
	var parsed struct {
		Styles map[string]*EffectiveStyle `json:"styles"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatal(err)
	}
	d := parsed.Styles["Default"]
	if d == nil {
		t.Fatal("缺少 Default")
	}
	if d.Fields["fontname"].Raw != "Calibri" {
		t.Fatalf("Default fontname 应保留 Calibri, got %+v", d.Fields["fontname"])
	}
	if d.Fields["fontsize"].Raw != "23" {
		t.Fatalf("Default fontsize 应为最后有效值 23, got %q", d.Fields["fontsize"].Raw)
	}
	if len(d.Chain) != 2 || d.Chain[0] != "Global" {
		t.Fatalf("Default chain = %v", d.Chain)
	}
}

func TestCLIConvertReportsUnmapped(t *testing.T) {
	code, out, errb := runCLI(t, "convert", "-in", "testdata/legacy.ssa")
	if code != ExitOK {
		t.Fatalf("convert 失败 code=%d err=%s", code, errb)
	}
	var cr ConversionReport
	if err := json.Unmarshal([]byte(out), &cr); err != nil {
		t.Fatalf("convert 输出不是 JSON: %v\n%s", err, out)
	}
	found := false
	for _, u := range cr.Unmapped {
		if u.Field == "xstudioid" {
			found = true
		}
	}
	if !found {
		t.Fatalf("convert 应报告 xstudioid: %+v", cr.Unmapped)
	}
}

func TestCLICompareMissingFile(t *testing.T) {
	code, _, errb := runCLI(t, "compare", "-legacy", "testdata/nope.ssa", "-converted", "testdata/case_ok.json")
	if code != ExitError || !strings.Contains(errb, "读取 legacy") {
		t.Fatalf("缺失文件应返回 %d 并说明, code=%d err=%s", ExitError, code, errb)
	}
}

func TestCLICompareRequiresInputs(t *testing.T) {
	if code, _, _ := runCLI(t, "compare"); code != ExitError {
		t.Fatalf("缺少参数应返回 %d", ExitError)
	}
}
