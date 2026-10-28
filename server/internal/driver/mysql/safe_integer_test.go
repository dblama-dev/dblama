package mysql

import "testing"

func TestConvertValuePreservesLargeInteger(t *testing.T) {
	const large int64 = 9007199254740993
	if got := convertValue(large, false); got != "9007199254740993" {
		t.Fatalf("large integer = %#v, want exact text", got)
	}
	if got := convertValue(int64(42), false); got != int64(42) {
		t.Fatalf("small integer changed: %#v", got)
	}
}
