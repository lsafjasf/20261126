package clipmerge

import (
	"encoding/json"
	"fmt"
	"math"
)

// Value is a typed property value: either a millisecond integer
// (inPoint/outPoint) or a rational speed.
type Value struct {
	Num     int64
	Spd     Speed
	IsSpeed bool
}

// IntValue builds an inPoint/outPoint value.
func IntValue(v int64) Value { return Value{Num: v} }

// SpeedValue builds a speed value.
func SpeedValue(s Speed) Value { return Value{Spd: s.Normalized(), IsSpeed: true} }

// Equal reports whether two values are identical after normalization.
func (v Value) Equal(o Value) bool {
	if v.IsSpeed != o.IsSpeed {
		return false
	}
	if v.IsSpeed {
		return v.Spd.Normalized() == o.Spd.Normalized()
	}
	return v.Num == o.Num
}

// Less gives values a deterministic total order, used to sort candidates.
// Numbers compare numerically; speeds compare as exact rationals.
func (v Value) Less(o Value) bool {
	if v.IsSpeed != o.IsSpeed {
		return !v.IsSpeed
	}
	if v.IsSpeed {
		a, b := v.Spd.Normalized(), o.Spd.Normalized()
		l, r := a.Numerator*b.Denominator, b.Numerator*a.Denominator
		if l != r {
			return l < r
		}
		return a.Denominator < b.Denominator
	}
	return v.Num < o.Num
}

// String renders the value deterministically for report "observed" fields.
func (v Value) String() string {
	if v.IsSpeed {
		return v.Spd.String()
	}
	return fmt.Sprintf("%d", v.Num)
}

// MarshalJSON emits numbers for points and {numerator, denominator} for speed.
func (v Value) MarshalJSON() ([]byte, error) {
	if v.IsSpeed {
		s := v.Spd.Normalized()
		return json.Marshal(s)
	}
	return json.Marshal(v.Num)
}

// ParseValue converts a JSON-decoded edit value (float64 or map) into a
// typed Value for the given property. Non-integral numbers are rejected so
// that merge semantics stay exact.
func ParseValue(property string, raw interface{}) (Value, error) {
	switch property {
	case PropertyInPoint, PropertyOutPoint:
		f, ok := raw.(float64)
		if !ok {
			return Value{}, fmt.Errorf("property %s expects a number, got %T", property, raw)
		}
		if f != math.Trunc(f) || math.Abs(f) > 9.0e15 {
			return Value{}, fmt.Errorf("property %s expects an integral millisecond value, got %v", property, f)
		}
		return IntValue(int64(f)), nil
	case PropertySpeed:
		m, ok := raw.(map[string]interface{})
		if !ok {
			return Value{}, fmt.Errorf("property speed expects {numerator, denominator}, got %T", raw)
		}
		num, okN := m["numerator"].(float64)
		den, okD := m["denominator"].(float64)
		if !okN || !okD || num != math.Trunc(num) || den != math.Trunc(den) {
			return Value{}, fmt.Errorf("speed numerator/denominator must be integers")
		}
		if den == 0 {
			return Value{}, fmt.Errorf("speed denominator must not be zero")
		}
		return SpeedValue(Speed{Numerator: int64(num), Denominator: int64(den)}), nil
	}
	return Value{}, fmt.Errorf("unknown property %q", property)
}

// ValueOf reads a typed property value from a clip.
func ValueOf(c Clip, property string) Value {
	switch property {
	case PropertyInPoint:
		return IntValue(c.InPoint)
	case PropertyOutPoint:
		return IntValue(c.OutPoint)
	case PropertySpeed:
		return SpeedValue(c.Speed)
	}
	return Value{}
}

// WithValue returns a copy of the clip with the property set.
func WithValue(c Clip, property string, v Value) Clip {
	switch property {
	case PropertyInPoint:
		c.InPoint = v.Num
	case PropertyOutPoint:
		c.OutPoint = v.Num
	case PropertySpeed:
		c.Speed = v.Spd.Normalized()
	}
	return c
}

// UnmarshalJSON accepts either a number (point) or {numerator, denominator}.
func (v *Value) UnmarshalJSON(data []byte) error {
	var num int64
	if err := json.Unmarshal(data, &num); err == nil {
		*v = IntValue(num)
		return nil
	}
	var spd Speed
	if err := json.Unmarshal(data, &spd); err != nil {
		return err
	}
	*v = SpeedValue(spd)
	return nil
}
