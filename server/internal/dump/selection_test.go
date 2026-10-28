package dump

import (
	"bytes"
	"context"
	"github.com/hitslab/db-manager/server/internal/driver"
	"github.com/hitslab/db-manager/server/internal/sqlgen"
	"testing"
)

func TestWriteSelectionFiltersSortAndPaging(t *testing.T) {
	var out bytes.Buffer
	calls := 0
	q := driver.SelectQuery{Filters: []sqlgen.Filter{{Column: "name", Operator: "=", Value: "O'Reilly"}}, Sorts: []sqlgen.Sort{{Column: "id", Desc: true}}, Limit: 3, Offset: 7}
	read := func(_ context.Context, got driver.SelectQuery) (driver.Page, error) {
		calls++
		if got.Offset != 7 || got.Limit != 3 || got.Filters[0].Value != "O'Reilly" || !got.Sorts[0].Desc {
			t.Fatalf("query = %+v", got)
		}
		return driver.Page{Columns: []driver.ColumnMeta{{Name: "id"}, {Name: "name"}}, Rows: [][]any{{1, "x,y"}, {2, nil}}}, nil
	}
	err := WriteSelection(context.Background(), "items", "csv", q, &out, read, sqlgen.QuoteIdent, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 || out.String() != "id,name\n1,\"x,y\"\n2,\n" {
		t.Fatalf("calls %d output %q", calls, out.String())
	}
}
func TestWriteSelectionPostgresNullConvention(t *testing.T) {
	var out bytes.Buffer
	read := func(context.Context, driver.SelectQuery) (driver.Page, error) {
		return driver.Page{Columns: []driver.ColumnMeta{{Name: "value"}}, Rows: [][]any{{nil}, {""}, {`\N`}}}, nil
	}
	if err := WriteSelection(context.Background(), "t", "tsv", driver.SelectQuery{}, &out, read, sqlgen.QuoteIdentPg, nil, true); err != nil {
		t.Fatal(err)
	}
	if out.String() != "value\n\\N\n\n\\\\N\n" {
		t.Fatalf("output %q", out.String())
	}
}
