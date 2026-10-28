package dump

import (
	"strings"
	"testing"
)

func TestRewriteViewQueryPreservesLiteralsAliasesAndQuotedIdentifiers(t *testing.T) {
	for _, tc := range []struct {
		query, source, want string
		postgres            bool
	}{
		{"SELECT `src`.`table`.`col`, 'src.literal', `src`.`col` FROM `src`.`table` AS `src`", "src", "SELECT `table`.`col`, 'src.literal', `src`.`col` FROM `table` AS `src`", false},
		{`SELECT "Source schema".id, 'Source schema.literal', $$Source schema.literal$$ FROM "Source schema"."base.table" AS "Source schema" JOIN "Source schema"."v" ON true`, "Source schema", `SELECT "Source schema".id, 'Source schema.literal', $$Source schema.literal$$ FROM "base.table" AS "Source schema" JOIN "v" ON true`, true},
		{"SELECT 'src.literal', t.id FROM `src`.`t` t, `src`.`other` o WHERE t.id=o.id", "src", "SELECT 'src.literal', t.id FROM `t` t, `other` o WHERE t.id=o.id", false},
	} {
		got, _, err := RewriteViewQuery(tc.query, tc.source, tc.postgres)
		if err != nil || got != tc.want {
			t.Fatalf("rewrite %s => %s err %v want %s", tc.query, got, err, tc.want)
		}
	}
}
func TestViewDependencyOrderAndMissingExternalReferences(t *testing.T) {
	views := []View{{Name: "a", Query: "SELECT * FROM z", Dependencies: []string{"z"}}, {Name: "z", Query: "SELECT * FROM t", Dependencies: []string{"t"}}}
	ordered, err := OrderViews(views)
	if err != nil || len(ordered) != 2 || ordered[0].Name != "z" || ordered[1].Name != "a" {
		t.Fatalf("order: %v %v", ordered, err)
	}
	_, err = OrderViews([]View{{Name: "a", Dependencies: []string{"b"}}, {Name: "b", Dependencies: []string{"a"}}})
	if err == nil {
		t.Fatal("cyclic views accepted")
	}
	_, _, err = RewriteViewQuery("SELECT * FROM outside.tbl", "src", false)
	if err == nil || !strings.Contains(err.Error(), "external") {
		t.Fatalf("external ref: %v", err)
	}
}

func TestRewriteViewQueryRecognizesFromInsideExpressions(t *testing.T) {
	query := "SELECT EXTRACT(YEAR FROM t.created), TRIM('x' FROM t.note) AS `from` FROM `src`.`t` t"
	want := "SELECT EXTRACT(YEAR FROM t.created), TRIM('x' FROM t.note) AS `from` FROM `t` t"
	got, _, err := RewriteViewQuery(query, "src", false)
	if err != nil || got != want {
		t.Fatalf("expression FROM rewritten: %s %v", got, err)
	}
}
