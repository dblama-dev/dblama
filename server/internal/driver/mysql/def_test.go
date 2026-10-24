package mysql

import "testing"

// TestParseColumnType pins the COLUMN_TYPE split, including the
// enum('unsigned') false positive: the word "unsigned" inside an enum
// value must not set the Unsigned flag.
func TestParseColumnType(t *testing.T) {
	cases := []struct {
		colType  string
		base     string
		length   string
		unsigned bool
		enumVals []string
	}{
		{colType: "int unsigned", base: "int", unsigned: true},
		{colType: "varchar(64)", base: "varchar", length: "64"},
		{colType: "decimal(10,2)", base: "decimal", length: "10,2"},
		{colType: "int", base: "int"},
		// The false positive: "unsigned" as an enum value.
		{colType: "enum('unsigned','signed')", base: "enum", enumVals: []string{"unsigned", "signed"}},
		{colType: "set('a','b')", base: "set", enumVals: []string{"a", "b"}},
	}
	for _, tc := range cases {
		base, length, unsigned, enumVals := parseColumnType(tc.colType)
		if base != tc.base || length != tc.length || unsigned != tc.unsigned {
			t.Errorf("parseColumnType(%q) = (%q, %q, %v), want (%q, %q, %v)",
				tc.colType, base, length, unsigned, tc.base, tc.length, tc.unsigned)
		}
		if len(enumVals) != len(tc.enumVals) {
			t.Errorf("parseColumnType(%q) enum values = %v, want %v", tc.colType, enumVals, tc.enumVals)
			continue
		}
		for i := range enumVals {
			if enumVals[i] != tc.enumVals[i] {
				t.Errorf("parseColumnType(%q) enum values = %v, want %v", tc.colType, enumVals, tc.enumVals)
			}
		}
	}
}
