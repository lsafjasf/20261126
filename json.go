package cafmarker

import "encoding/json"

// MarshalJSON represents a FourCC as its printable four-character string.
func (c FourCC) MarshalJSON() ([]byte, error) {
	return json.Marshal(c.String())
}
