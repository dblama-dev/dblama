package dump

import (
	"bytes"
	"context"
	"encoding/csv"
	"reflect"
	"testing"

	"github.com/hitslab/db-manager/server/internal/driver"
	"github.com/hitslab/db-manager/server/internal/sqlgen"
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
func TestWriteSelectionSingleColumnRoundTrip(t *testing.T) {
	read := func(context.Context, driver.SelectQuery) (driver.Page, error) {
		return driver.Page{Columns: []driver.ColumnMeta{{Name: ""}}, Rows: [][]any{{nil}, {""}, {`\N`}}}, nil
	}
	for _, tc := range []struct {
		format string
		sep    rune
	}{
		{FormatCSV, ','}, {FormatCSVSemicolon, ';'}, {FormatTSV, '\t'},
	} {
		t.Run(tc.format, func(t *testing.T) {
			var out bytes.Buffer
			if err := WriteSelection(context.Background(), "t", tc.format, driver.SelectQuery{}, &out, read, sqlgen.QuoteIdentPg, nil, true); err != nil {
				t.Fatal(err)
			}
			r := csv.NewReader(&out)
			r.Comma = tc.sep
			records, err := r.ReadAll()
			if err != nil {
				t.Fatal(err)
			}
			want := [][]string{{""}, {`\N`}, {""}, {`\\N`}}
			if !reflect.DeepEqual(records, want) {
				t.Errorf("records = %q, want %q", records, want)
			}
		})
	}
}
