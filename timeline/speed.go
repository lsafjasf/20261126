package timeline

import (
	"errors"
	"fmt"
	"math/big"
)

// Speed 是一个正有理数速率（分子/分母），全程不使用浮点数，
// 避免多层变速叠加时的取整漂移。1.0 倍速写作 Speed{1,1}。
type Speed struct {
	Num int64 `json:"num"`
	Den int64 `json:"den"`
}

// SpeedOne 是默认 1.0 倍速。零值 Speed{} 也会被规范化为 1.0。
var SpeedOne = Speed{Num: 1, Den: 1}

// Normalize 返回约分后的速率；零值视为 1.0。
func (s Speed) Normalize() Speed {
	num, den := s.Num, s.Den
	if num == 0 && den == 0 {
		return SpeedOne
	}
	if num <= 0 || den <= 0 {
		return s
	}
	g := new(big.Int).GCD(nil, nil, big.NewInt(num), big.NewInt(den)).Int64()
	return Speed{Num: num / g, Den: den / g}
}

// Valid 报告速率是否为正有理数。
func (s Speed) Valid() bool {
	return s.Num > 0 && s.Den > 0
}

// Mul 返回两个速率的乘积（已约分）。
func (s Speed) Mul(o Speed) Speed {
	return Speed{Num: s.Num * o.Num, Den: s.Den * o.Den}.Normalize()
}

// Invert 返回倒数速率。
func (s Speed) Invert() Speed {
	if !s.Valid() {
		return s
	}
	return Speed{Num: s.Den, Den: s.Num}
}

// Cmp 比较两个速率：-1 小于、0 相等、1 大于。
func (s Speed) Cmp(o Speed) int {
	l := s.Num * o.Den
	r := o.Num * s.Den
	switch {
	case l < r:
		return -1
	case l > r:
		return 1
	default:
		return 0
	}
}

// Float 仅用于展示与导出，不参与映射计算。
func (s Speed) Float() float64 {
	if !s.Valid() {
		return 0
	}
	return float64(s.Num) / float64(s.Den)
}

func (s Speed) String() string {
	n := s.Normalize()
	if n.Den == 1 {
		return fmt.Sprintf("%d", n.Num)
	}
	return fmt.Sprintf("%d/%d", n.Num, n.Den)
}

// Map 是核心帧映射：子时间 t 经过速率 s 后对应的父时间偏移。
// 采用向下取整，保证整数帧、零漂移、可复现。
func (s Speed) Map(t int64) (int64, error) {
	if !s.Valid() {
		return 0, errors.New("timeline: invalid speed in frame mapping")
	}
	if t < 0 {
		return 0, fmt.Errorf("timeline: negative frame offset %d", t)
	}
	return floorDiv(t*s.Num, s.Den), nil
}

// floorDiv 返回整数向下取整除法（输入均为非负数）。
func floorDiv(a, b int64) int64 {
	return a / b
}
