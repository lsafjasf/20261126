package framealign

import (
	"fmt"
	"strconv"
	"strings"
)

// ParseTimecode 把 "HH:MM:SS:FF"（非丢帧）或 "HH:MM:SS;FF"（drop-frame）
// 解析为源帧号。分号仅允许用于声明了 DropFrame 的帧率（29.97）。
func ParseTimecode(rate FrameRate, tc string) (int64, error) {
	drop := strings.Contains(tc, ";")
	sep := ":"
	if drop {
		if !rate.DropFrame {
			return 0, fmt.Errorf("framealign: %s fps 不支持 drop-frame 时间码 %q", rate.Name, tc)
		}
		sep = ";"
	}
	parts := strings.Split(strings.Replace(tc, ";", ":", 1), ":")
	if len(parts) != 4 {
		return 0, fmt.Errorf("framealign: 时间码 %q 应为 HH:MM:SS%sFF", tc, sep)
	}
	v := make([]int64, 4)
	for i, p := range parts {
		n, err := strconv.ParseInt(p, 10, 64)
		if err != nil || n < 0 {
			return 0, fmt.Errorf("framealign: 时间码 %q 含非法字段 %q", tc, p)
		}
		v[i] = n
	}
	fpsInt := nominalFPS(rate)
	if v[1] > 59 || v[2] > 59 || v[3] >= fpsInt {
		return 0, fmt.Errorf("framealign: 时间码 %q 超出 %d fps 量程", tc, fpsInt)
	}
	totalMinutes := v[0]*60 + v[1]
	frames := (v[0]*3600+v[1]*60+v[2])*fpsInt + v[3]
	if drop {
		// 被丢弃的标签（非 10 分钟整点的 :00/:01 帧）非法。
		if v[2] == 0 && v[3] < 2 && totalMinutes%10 != 0 && totalMinutes > 0 {
			return 0, fmt.Errorf("framealign: %q 是 drop-frame 中被丢弃的标签", tc)
		}
		// NTSC DF：每整分钟丢 2 帧，每 10 分钟例外。
		frames -= 2 * (totalMinutes - totalMinutes/10)
	}
	return frames, nil
}

// FormatTimecode 把帧号格式化为时间码；DropFrame 帧率使用分号。
func FormatTimecode(rate FrameRate, frame int64) string {
	if frame < 0 {
		return "-" + FormatTimecode(rate, -frame)
	}
	fpsInt := nominalFPS(rate)
	if rate.DropFrame {
		// 标准 NTSC DF 反变换。
		framesPer10Min := fpsInt*600 - 18 // 17982 @ 30
		framesPerMin := fpsInt*60 - 2     // 1798 @ 30
		d := frame / framesPer10Min
		m := frame % framesPer10Min
		extra := int64(0)
		if m >= 2 {
			extra = (m - 2) / framesPerMin
		}
		frame += 18*d + 2*extra
	}
	ff := frame % fpsInt
	s := (frame / fpsInt) % 60
	m := (frame / (fpsInt * 60)) % 60
	h := frame / (fpsInt * 3600)
	sep := ":"
	if rate.DropFrame {
		sep = ";"
	}
	return fmt.Sprintf("%02d:%02d:%02d%s%02d", h, m, s, sep, ff)
}

// nominalFPS 返回时间码使用的标称整数帧率（29.97 → 30）。
func nominalFPS(rate FrameRate) int64 {
	r := rate.rate
	// 四舍五入到最近整数（30000/1001 → 30）。
	return (r.num + r.den/2) / r.den
}
