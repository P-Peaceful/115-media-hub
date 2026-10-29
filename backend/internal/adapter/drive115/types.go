package drive115

import (
	"encoding/json"
	"errors"
	"math/big"
	"regexp"
	"strconv"
	"strings"
)

// wireInteger preserves arbitrary decimal IDs without a float64 conversion.
type wireInteger string

func (v *wireInteger) UnmarshalJSON(data []byte) error {
	text := string(data)
	if len(text) > 0 && text[0] == '"' {
		if json.Unmarshal(data, &text) != nil {
			return errors.New("invalid integer")
		}
	}
	if text == "" {
		return errors.New("empty integer")
	}
	digits := text
	if strings.HasPrefix(digits, "-") {
		digits = digits[1:]
	}
	if digits == "" {
		return errors.New("invalid integer")
	}
	for _, c := range digits {
		if c < '0' || c > '9' {
			return errors.New("invalid integer")
		}
	}
	*v = wireInteger(text)
	return nil
}
func (v wireInteger) int64() (int64, error) { return strconv.ParseInt(string(v), 10, 64) }

type wireBool bool

func (v *wireBool) UnmarshalJSON(data []byte) error {
	switch string(data) {
	case "true", "1", "\"1\"":
		*v = true
	case "false", "0", "\"0\"":
		*v = false
	default:
		return errors.New("invalid boolean")
	}
	return nil
}

// byteSize accepts the fractional/scientific sizes emitted by index_info;
// truncation uses rational arithmetic and rejects overflow and negatives.
var sizePattern = regexp.MustCompile(`^[0-9]+(?:\.[0-9]+)?(?:[eE][+-]?[0-9]{1,2})?$`)

type byteSize int64

func (v *byteSize) UnmarshalJSON(data []byte) error {
	text := string(data)
	if strings.HasPrefix(text, "\"") {
		if json.Unmarshal(data, &text) != nil {
			return errors.New("invalid size")
		}
	}
	if len(text) > 100 || !sizePattern.MatchString(text) {
		return errors.New("invalid size")
	}
	r, ok := new(big.Rat).SetString(text)
	if !ok || r.Sign() < 0 {
		return errors.New("invalid size")
	}
	n := new(big.Int).Quo(r.Num(), r.Denom())
	if !n.IsInt64() {
		return errors.New("size overflow")
	}
	*v = byteSize(n.Int64())
	return nil
}
