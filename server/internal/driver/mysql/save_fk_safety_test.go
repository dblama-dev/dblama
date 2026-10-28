package mysql

import (
	"github.com/hitslab/db-manager/server/internal/driver"
	"strings"
	"testing"
)

func TestForeignKeyFragmentRejectsUnsafeActionsAndMismatchedColumns(t *testing.T) {
	fk := driver.ForeignKey{Name: "fk", Columns: []string{"id"}, RefTable: "parent", RefColumns: []string{"id"}, OnDelete: "CASCADE; DROP TABLE x"}
	if _, err := foreignKeyFragment(fk); err == nil {
		t.Fatal("unsafe action accepted")
	}
	fk.OnDelete = ""
	fk.RefColumns = []string{"a", "b"}
	if _, err := foreignKeyFragment(fk); err == nil {
		t.Fatal("mismatched columns accepted")
	}
}
func TestForeignKeyFragmentQuotesSchemaAndDottedTableSeparately(t *testing.T) {
	fk := driver.ForeignKey{Name: "fk", Columns: []string{"id"}, RefSchema: "other db", RefTable: "parent.table", RefColumns: []string{"id"}, OnDelete: " cascade "}
	got, err := foreignKeyFragment(fk)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "REFERENCES `other db`.`parent.table`") || !strings.HasSuffix(got, " ON DELETE CASCADE") {
		t.Fatal(got)
	}
}
