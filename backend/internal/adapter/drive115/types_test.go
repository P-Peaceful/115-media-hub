package drive115

import (
	"encoding/json"
	"testing"
)

func TestIDParsingPreservesDecimalAndRejectsOtherTypes(t *testing.T) {
	for _, raw := range []string{`3549698981099472286`, `"3549698981099472286"`} {
		var id wireInteger
		if err := json.Unmarshal([]byte(raw), &id); err != nil || id != "3549698981099472286" {
			t.Fatalf("%s: %s %v", raw, id, err)
		}
	}
	for _, raw := range []string{`1.5`, `1e6`, `true`, `null`, `{}`, `[]`, `"123.0"`, `""`} {
		var id wireInteger
		if json.Unmarshal([]byte(raw), &id) == nil {
			t.Fatalf("accepted invalid id %s", raw)
		}
	}
}
func TestCapacityParsingBoundsAndGrammar(t *testing.T) {
	for _, raw := range []string{`-1`, `null`, `"1/2"`, `"0x10"`, `"1e999999"`, `"9223372036854775808"`, `true`} {
		var size byteSize
		if json.Unmarshal([]byte(raw), &size) == nil {
			t.Fatalf("accepted invalid capacity %s", raw)
		}
	}
	for _, raw := range []string{`1500000`, `"1500000.25"`, `1.5e6`} {
		var size byteSize
		if err := json.Unmarshal([]byte(raw), &size); err != nil || size != 1500000 {
			t.Fatalf("size %s: %d %v", raw, size, err)
		}
	}
}
