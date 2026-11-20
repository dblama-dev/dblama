package mysql

import (
	"database/sql"
	"reflect"
	"testing"

	"github.com/hitslab/db-manager/server/internal/driver"
)

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
		{colType: `enum('','a,b','it\'s,valid','x')`, base: "enum", enumVals: []string{"", "a,b", "it's,valid", "x"}},
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

func TestDefaultCatalogFormats(t *testing.T) {
	for _, tc := range []struct {
		flavor string
		raw    string
		extra  string
		want   *driver.DefaultValue
	}{
		{FlavorMySQL, "", "", &driver.DefaultValue{Value: ""}},
		{FlavorMySQL, "NULL", "", &driver.DefaultValue{Value: "NULL"}},
		{FlavorMySQL, "'quoted'", "", &driver.DefaultValue{Value: "'quoted'"}},
		{FlavorMySQL, "CURRENT_TIMESTAMP", "", &driver.DefaultValue{Value: "CURRENT_TIMESTAMP"}},
		{FlavorMySQL, "CURRENT_TIMESTAMP", "DEFAULT_GENERATED", &driver.DefaultValue{Value: "CURRENT_TIMESTAMP", IsSQLExpression: true}},
		{FlavorMariaDB, "''", "", &driver.DefaultValue{Value: ""}},
		{FlavorMariaDB, "'NULL'", "", &driver.DefaultValue{Value: "NULL"}},
		{FlavorMariaDB, "NULL", "", nil},
		{FlavorMariaDB, "current_timestamp()", "", &driver.DefaultValue{Value: "current_timestamp()", IsSQLExpression: true}},
	} {
		got := parseColumnDefault(sql.NullString{Valid: true, String: tc.raw}, tc.extra, tc.flavor)
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s default %q, extra %q: got %#v, want %#v", tc.flavor, tc.raw, tc.extra, got, tc.want)
		}
	}
}
