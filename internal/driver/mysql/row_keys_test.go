package mysql

import (
	"github.com/dblama-dev/dblama/internal/sqlgen"
	"reflect"
	"testing"
)

func TestOriginalKeyValuePreservesReadTypes(t *testing.T) {
	for _, tc := range []struct {
		name, typ   string
		input, want any
	}{
		{"binary", "varbinary", "0x00ff", []byte{0, 255}},
		{"empty binary", "blob", "0x", []byte{}},
		{"text looks binary", "varchar", "0x00ff", "0x00ff"},
		{"datetime microseconds", "datetime", "2026-10-28T12:34:56.123456Z", "2026-10-28 12:34:56.123456"},
		{"timestamp microseconds", "timestamp", "2026-10-28T12:34:56.123456Z", "2026-10-28 12:34:56.123456"},
		{"date", "date", "2026-10-28T00:00:00Z", "2026-10-28"},
		{"null", "blob", nil, nil},
		{"json", "json", `{"a": 1}`, `{"a": 1}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := originalKeyValue(tc.typ, tc.input)
			if err != nil || !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("original key=%#v err=%v want=%#v", got, err, tc.want)
			}
		})
	}
	if _, err := originalKeyValue("varbinary", "0xzz"); err == nil {
		t.Fatal("invalid read-path binary hex accepted")
	}
}

func TestOriginalJSONKeyPredicateDoesNotTreatJSONAsText(t *testing.T) {
	query, args, err := bindOriginalJSONKeys("UPDATE `a WHERE ?` SET `document` = ? WHERE `doc``?` = ? AND `label` = ?", []any{"new JSON", originalJSONKey(`{"a":1}`), "plain"})
	if err != nil || query != "UPDATE `a WHERE ?` SET `document` = ? WHERE JSON_EXTRACT(`doc``?`, '$') = JSON_EXTRACT(?, '$') AND `label` = ?" {
		t.Fatalf("JSON predicate=%q err=%v", query, err)
	}
	if !reflect.DeepEqual(args, []any{"new JSON", `{"a":1}`, "plain"}) {
		t.Fatalf("JSON args=%#v", args)
	}
}

func TestOriginalJSONKeyUsesActualBuilderPredicates(t *testing.T) {
	query, whereArgs, err := sqlgen.BuildUpdate("t", []string{"label"}, map[string]any{"doc": originalJSONKey(`{"a":1}`)})
	if err != nil {
		t.Fatal(err)
	}
	query, _, err = bindOriginalJSONKeys(query, append([]any{"new"}, whereArgs...))
	if err != nil || query != "UPDATE `t` SET `label`=? WHERE JSON_EXTRACT(`doc`, '$') = JSON_EXTRACT(?, '$')" {
		t.Fatalf("JSON builder predicate=%q err=%v", query, err)
	}
}
