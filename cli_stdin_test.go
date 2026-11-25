package ssastyle

import (
	"bytes"
	"strings"
	"testing"
)

func TestCLIResolveFromStdin(t *testing.T) {
	var out, errb bytes.Buffer
	ssa := strings.NewReader(sampleASS)
	code := RunCLI([]string{"resolve", "-in", "-", "-style", "Default"}, ssa, &out, &errb)
	if code != ExitOK {
		t.Fatalf("stdin resolve code=%d err=%s", code, errb.String())
	}
	if !strings.Contains(out.String(), "Calibri") {
		t.Fatalf("应解析 stdin ASS 中的 Calibri:\n%s", out.String())
	}
}

func TestCLINullResetFlagChangesSemantics(t *testing.T) {
	// 默认：子层 null 不覆盖父值，比对一致。
	doc := `{"styles":[{"name":"P","fields":{"fontname":"Arial"}},
	        {"name":"C","parent":"P","fields":[["fontname",null]]}]}`
	legacy := strings.NewReader(doc)
	var out1, err1 bytes.Buffer
	code := RunCLI([]string{"resolve", "-in", "-", "-style", "C", "-json"}, legacy, &out1, &err1)
	if code != ExitOK || !strings.Contains(out1.String(), "Arial") {
		t.Fatalf("默认 null 应继续继承 Arial, code=%d out=%s err=%s", code, out1.String(), err1.String())
	}

	// -null-reset：显式 null 切断继承，输出 reset 标记而非 Arial。
	var out2, err2 bytes.Buffer
	code = RunCLI([]string{"resolve", "-in", "-", "-style", "C", "-null-reset"}, strings.NewReader(doc), &out2, &err2)
	if code != ExitOK {
		t.Fatalf("null-reset code=%d err=%s", code, err2.String())
	}
	if strings.Contains(out2.String(), "Arial") || !strings.Contains(out2.String(), "reset") {
		t.Fatalf("null-reset 应显示 reset 且不再取 Arial:\n%s", out2.String())
	}
}
