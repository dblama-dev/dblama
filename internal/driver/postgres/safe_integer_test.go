package postgres

import "testing"

func TestConvertValuePreservesLargeInteger(t *testing.T) {
	const large int64 = 9007199254740993
	if got := convertValue(large, "bigint"); got != "9007199254740993" {
		t.Fatalf("large integer = %#v, want exact text", got)
	}
	if got := convertValue(int64(42), "bigint"); got != int64(42) {
		t.Fatalf("small integer changed: %#v", got)
	}
}
