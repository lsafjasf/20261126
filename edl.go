package main

import (
	"bufio"
	"io"
	"regexp"
	"strconv"
	"strings"
)

// Decl 是一条显式帧率声明，可能来自 FCM 行或事件注释。
// FPS 为 0 表示只声明了丢帧属性（如 FCM 行）。
type Decl struct {
	FPS  float64
	Drop *bool
	From string
}

func (d Decl) String() string {
	s := ""
	if d.FPS > 0 {
		s = strconv.FormatFloat(d.FPS, 'f', -1, 64) + " fps"
	}
	if d.Drop != nil {
		mode := "DF"
		if !*d.Drop {
			mode = "NDF"
		}
		if s != "" {
			s += " "
		}
		s += mode
	}
	if s == "" {
		s = "(空声明)"
	}
	return s + "（来源: " + d.From + "）"
}

func (d Decl) matches(r Rate) bool {
	if d.FPS > 0 {
		diff := d.FPS - r.FPS
		if diff < -0.01 || diff > 0.01 {
			return false
		}
	}
	if d.Drop != nil && *d.Drop != r.Drop {
		return false
	}
	return true
}

// Event 是一条剪辑事件。解析容忍缺失字段：时码不足 4 个的事件
// 会被跳过并计入 Warnings。
type Event struct {
	Num    string
	Reel   string
	Clip   string // 来自 * FROM CLIP NAME / * SOURCE FILE，可能为空
	SrcIn  Timecode
	SrcOut Timecode
	RecIn  Timecode
	RecOut Timecode
	Decls  []Decl
}

type EDL struct {
	Title      string
	Events     []Event
	RecordDecl []Decl // FCM 行，作用于记录（时间线）侧
	Warnings   []string
}

var (
	eventRe   = regexp.MustCompile(`^\s*(\d{3,})\s+(\S+)\s+\S+\s+\S+\s+(.*)$`)
	titleRe   = regexp.MustCompile(`(?i)^\s*TITLE\s*:\s*(.*)$`)
	fcmRe     = regexp.MustCompile(`(?i)^\s*FCM\s*:\s*(.*)$`)
	clipRe    = regexp.MustCompile(`(?i)^\*\s*(?:FROM\s+CLIP\s+NAME|SOURCE\s+FILE)\s*:\s*(.+?)\s*$`)
	fpsRe     = regexp.MustCompile(`(?i)(?:frame\s*rate|fps)\s*[:=]?\s*(\d+(?:\.\d+)?)`)
	fpsRe2    = regexp.MustCompile(`(?i)(\d+(?:\.\d+)?)\s*fps`)
	nonDropRe = regexp.MustCompile(`(?i)non[- ]?drop`)
	dropRe    = regexp.MustCompile(`(?i)drop`)
	dfTokenRe = regexp.MustCompile(`(?i)\b(N?DF)\b`)
)

func parseRateDecl(text, from string) *Decl {
	var d Decl
	if m := fpsRe.FindStringSubmatch(text); m != nil {
		d.FPS, _ = strconv.ParseFloat(m[1], 64)
	} else if m := fpsRe2.FindStringSubmatch(text); m != nil {
		d.FPS, _ = strconv.ParseFloat(m[1], 64)
	}
	if m := dfTokenRe.FindStringSubmatch(text); m != nil {
		b := strings.EqualFold(m[1], "DF")
		d.Drop = &b
	} else if dropRe.MatchString(text) {
		b := !nonDropRe.MatchString(text)
		d.Drop = &b
	}
	if d.FPS == 0 && d.Drop == nil {
		return nil
	}
	d.From = from
	return &d
}

// ParseEDL 解析 CMX3600 风格剪辑表。无法识别的行一律跳过，
// 不会因缺少 FCM 或帧率注释而失败。
func ParseEDL(r io.Reader) (*EDL, error) {
	e := &EDL{}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 1<<20), 1<<20)
	var cur *Event
	lineNo := 0
	for sc.Scan() {
		lineNo++
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		if m := titleRe.FindStringSubmatch(line); m != nil {
			e.Title = m[1]
			continue
		}
		if m := fcmRe.FindStringSubmatch(line); m != nil {
			if d := parseRateDecl(m[1], "FCM 行"); d != nil {
				e.RecordDecl = append(e.RecordDecl, *d)
			} else {
				e.Warnings = append(e.Warnings, "FCM 行无法识别: "+line)
			}
			continue
		}
		if strings.HasPrefix(line, "*") {
			if cur == nil {
				continue
			}
			if m := clipRe.FindStringSubmatch(line); m != nil {
				cur.Clip = m[1]
			}
			if d := parseRateDecl(line, "事件 "+cur.Num+" 注释"); d != nil {
				cur.Decls = append(cur.Decls, *d)
			}
			continue
		}
		m := eventRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		tcs := tcRe.FindAllString(m[3], -1)
		if len(tcs) < 4 {
			e.Warnings = append(e.Warnings, "事件 "+m[1]+" 时码不足 4 个，已跳过")
			cur = nil
			continue
		}
		tcs = tcs[len(tcs)-4:]
		var parsed [4]Timecode
		ok := true
		for i, s := range tcs {
			tc, err := parseTimecode(s)
			if err != nil {
				e.Warnings = append(e.Warnings, "事件 "+m[1]+": "+err.Error())
				ok = false
				break
			}
			parsed[i] = tc
		}
		if !ok {
			cur = nil
			continue
		}
		e.Events = append(e.Events, Event{
			Num: m[1], Reel: m[2],
			SrcIn: parsed[0], SrcOut: parsed[1],
			RecIn: parsed[2], RecOut: parsed[3],
		})
		cur = &e.Events[len(e.Events)-1]
	}
	return e, sc.Err()
}
