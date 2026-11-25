package framealign

// Rat 是一个最小化的有理数（int64 分子/分母，恒规范化），
// 用于在帧号与时间之间做无精度损失的换算。
// 帧率分母最大 1001，帧数在常规素材长度内不会溢出 int64。
type Rat struct {
	num int64
	den int64 // 恒 > 0
}

func gcd(a, b int64) int64 {
	if a < 0 {
		a = -a
	}
	if b < 0 {
		b = -b
	}
	for b != 0 {
		a, b = b, a%b
	}
	if a == 0 {
		return 1
	}
	return a
}

// NewRat 构造并规范化一个有理数。
func NewRat(num, den int64) Rat {
	if den == 0 {
		panic("framealign: zero denominator")
	}
	if den < 0 {
		num, den = -num, -den
	}
	g := gcd(num, den)
	return Rat{num: num / g, den: den / g}
}

func (r Rat) Num() int64 { return r.num }
func (r Rat) Den() int64 { return r.den }

// IsInt 报告该有理数是否为整数（即落点恰好合法）。
func (r Rat) IsInt() bool { return r.den == 1 }

// Float 返回浮点近似，仅用于展示与误差换算，不参与判定。
func (r Rat) Float() float64 { return float64(r.num) / float64(r.den) }

// MulInt 返回 r * k。
func (r Rat) MulInt(k int64) Rat { return NewRat(r.num*k, r.den) }

// Div 返回 r / s。
func (r Rat) Div(s Rat) Rat { return NewRat(r.num*s.den, r.den*s.num) }

// Sub 返回 r - s。
func (r Rat) Sub(s Rat) Rat { return NewRat(r.num*s.den-s.num*r.den, r.den*s.den) }

// Abs 返回 |r|。
func (r Rat) Abs() Rat {
	if r.num < 0 {
		return Rat{-r.num, r.den}
	}
	return r
}

// Cmp 比较两个有理数：-1 / 0 / +1。
func (r Rat) Cmp(s Rat) int {
	l, rr := r.num*s.den, s.num*r.den
	switch {
	case l < rr:
		return -1
	case l > rr:
		return 1
	default:
		return 0
	}
}

// Floor 返回不大于 r 的最大整数。
func (r Rat) Floor() int64 {
	q := r.num / r.den
	if r.num%r.den != 0 && r.num < 0 {
		q--
	}
	return q
}

// Ceil 返回不小于 r 的最小整数。
func (r Rat) Ceil() int64 {
	return -NewRat(-r.num, r.den).Floor()
}

// Frac 返回小数部分（0 <= frac < 1）。
func (r Rat) Frac() Rat {
	f := r.Floor()
	return NewRat(r.num-f*r.den, r.den)
}
