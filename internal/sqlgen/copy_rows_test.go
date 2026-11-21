package sqlgen

import (
	"reflect"
	"testing"
)

func TestBuildCopyRowsUsesQuotedNamesAndBoundOriginalKeys(t *testing.T) {
	q, args, err := BuildCopyRows(PgDialect, "Odd Schema", "t", []string{"value"}, []map[string]any{{"id": 1, "nullable": nil}, {"id": 2}})
	if err != nil {
		t.Fatal(err)
	}
	want := `INSERT INTO "Odd Schema"."t" ("value") SELECT "value" FROM "Odd Schema"."t" WHERE ("id"=$1 AND "nullable" IS NULL) OR ("id"=$2)`
	if q != want || !reflect.DeepEqual(args, []any{1, 2}) {
		t.Fatalf("query %s args %#v", q, args)
	}
	if _, _, err := BuildCopyRows(MySQLDialect, "d", "t", []string{"v"}, []map[string]any{{}}); err == nil {
		t.Fatal("unqualified copy accepted")
	}
}
