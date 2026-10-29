package driver

import (
	"reflect"
	"testing"
)

func TestCSVImportColumns(t *testing.T) {
	columns := []ColumnMeta{{Name: "id", Type: "bigint"}, {Name: "имя", Type: "text"}}
	for _, tc := range []struct {
		name    string
		mapping []string
		want    []ColumnMeta
		indexes []int
		fail    bool
	}{
		{name: "positional", want: columns, indexes: []int{0, 1}},
		{name: "reordered and ignored", mapping: []string{"имя", "", "id"}, want: []ColumnMeta{{Name: "имя", Type: "text"}, {Name: "id", Type: "bigint"}}, indexes: []int{0, 2}},
		{name: "unknown", mapping: []string{"missing"}, fail: true},
		{name: "duplicate", mapping: []string{"id", "id"}, fail: true},
		{name: "empty mapping", mapping: []string{}, fail: true},
		{name: "all ignored", mapping: []string{"", ""}, fail: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, indexes, err := CSVImportColumns(columns, tc.mapping)
			if tc.fail {
				if err == nil {
					t.Fatal("expected validation error")
				}
				return
			}
			if err != nil || !reflect.DeepEqual(got, tc.want) || !reflect.DeepEqual(indexes, tc.indexes) {
				t.Fatalf("got %+v indexes %v error %v", got, indexes, err)
			}
		})
	}
}
