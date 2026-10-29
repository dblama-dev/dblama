package sqlgen

import (
	"reflect"
	"testing"
)

func TestContainsLiteralFilter(t *testing.T) {
	for _, tt := range []struct {
		name  string
		build func(string, SelectQuery) (string, []any, error)
		want  string
	}{
		{"mysql", BuildSelect, "SELECT * FROM `items` WHERE `body` LIKE ? ESCAPE '!' LIMIT ? OFFSET ?"},
		{"postgres", BuildSelectPg, `SELECT * FROM "items" WHERE "body" LIKE $1 ESCAPE '!' LIMIT $2 OFFSET $3`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			query, args, err := tt.build("items", SelectQuery{Filters: []Filter{{Column: "body", Operator: "CONTAINS", Value: `50%_!\' OR 1=1 --`}}, Limit: 20})
			if err != nil {
				t.Fatal(err)
			}
			if query != tt.want {
				t.Errorf("query=%q want=%q", query, tt.want)
			}
			wantArgs := []any{`%50!%!_!!\' OR 1=1 --%`, 20, 0}
			if !reflect.DeepEqual(args, wantArgs) {
				t.Errorf("args=%#v want=%#v", args, wantArgs)
			}
			_, _, err = tt.build("items", SelectQuery{Filters: []Filter{{Column: "body", Operator: "CONTAINS", Value: ""}}})
			if err == nil {
				t.Error("empty contains filter accepted")
			}
		})
	}
}
