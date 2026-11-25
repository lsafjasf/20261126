package timeline

import (
	"fmt"
	"math/big"
)

type Rat struct {
	big.Rat
}

func NewRat(num, den int64) Rat {
	if den == 0 {
		panic("zero denominator")
	}
	return Rat{Rat: *new(big.Rat).SetFrac(big.NewInt(num), big.NewInt(den))}
}

func RatInt(value int64) Rat {
	return Rat{Rat: *big.NewRat(value, 1)}
}

func ParseRat(text string) (Rat, error) {
	value, ok := new(big.Rat).SetString(text)
	if !ok {
		return Rat{}, fmt.Errorf("invalid rational number %q", text)
	}
	return Rat{Rat: *value}, nil
}

func MustParseRat(text string) Rat {
	value, err := ParseRat(text)
	if err != nil {
		panic(err)
	}
	return value
}

func (r Rat) Clone() Rat {
	return Rat{Rat: *new(big.Rat).Set(&r.Rat)}
}

func (r Rat) Add(other Rat) Rat {
	return Rat{Rat: *new(big.Rat).Add(&r.Rat, &other.Rat)}
}

func (r Rat) Sub(other Rat) Rat {
	return Rat{Rat: *new(big.Rat).Sub(&r.Rat, &other.Rat)}
}

func (r Rat) Mul(other Rat) Rat {
	return Rat{Rat: *new(big.Rat).Mul(&r.Rat, &other.Rat)}
}

func (r Rat) Quo(other Rat) Rat {
	return Rat{Rat: *new(big.Rat).Quo(&r.Rat, &other.Rat)}
}

func (r Rat) Abs() Rat {
	return Rat{Rat: *new(big.Rat).Abs(&r.Rat)}
}

func (r Rat) Neg() Rat {
	return Rat{Rat: *new(big.Rat).Neg(&r.Rat)}
}

func (r Rat) Cmp(other Rat) int {
	return r.Rat.Cmp(&other.Rat)
}

func (r Rat) IsInt() bool {
	return r.Denom().Cmp(big.NewInt(1)) == 0
}

func (r Rat) Floor() int64 {
	num, den := r.Num(), r.Denom()
	quotient, remainder := new(big.Int).QuoRem(num, den, new(big.Int))
	if remainder.Sign() < 0 {
		quotient.Sub(quotient, big.NewInt(1))
	}
	if !quotient.IsInt64() {
		panic("rational floor does not fit int64")
	}
	return quotient.Int64()
}

func (r Rat) Ceil() int64 {
	num, den := r.Num(), r.Denom()
	quotient, remainder := new(big.Int).QuoRem(num, den, new(big.Int))
	if remainder.Sign() > 0 {
		quotient.Add(quotient, big.NewInt(1))
	}
	if !quotient.IsInt64() {
		panic("rational ceil does not fit int64")
	}
	return quotient.Int64()
}

func (r Rat) NearestEven() int64 {
	floor := r.Floor()
	fraction := r.Sub(RatInt(floor))
	half := NewRat(1, 2)
	switch fraction.Cmp(half) {
	case -1:
		return floor
	case 1:
		return floor + 1
	default:
		if floor%2 == 0 {
			return floor
		}
		return floor + 1
	}
}

func (r Rat) NearestUp() int64 {
	floor := r.Floor()
	fraction := r.Sub(RatInt(floor))
	if fraction.Cmp(NewRat(1, 2)) >= 0 {
		return floor + 1
	}
	return floor
}

func (r Rat) Float64Value() float64 {
	value, _ := r.Float64()
	return value
}

func (r Rat) MarshalJSON() ([]byte, error) {
	return []byte(`"` + r.RatString() + `"`), nil
}

func (r *Rat) UnmarshalJSON(data []byte) error {
	if len(data) == 0 || string(data) == "null" {
		return nil
	}
	if data[0] == '"' {
		if len(data) < 2 || data[len(data)-1] != '"' {
			return fmt.Errorf("invalid rational JSON %s", string(data))
		}
		data = data[1 : len(data)-1]
	}
	parsed, err := ParseRat(string(data))
	if err != nil {
		return err
	}
	*r = parsed
	return nil
}
