package mysql

import (
	"strings"
	"testing"
)

func TestEnsureViewReplacementOnlyChangesCreateHeader(t *testing.T) {
	for _, tc := range []struct{ source, want string }{
		{"create view sample as select 2", "create OR REPLACE view sample as select 2"},
		{"CREATE VIEW sample AS SELECT 'OR REPLACE'", "CREATE OR REPLACE VIEW sample AS SELECT 'OR REPLACE'"},
		{"/* OR REPLACE */ CREATE VIEW sample AS SELECT 2", "/* OR REPLACE */ CREATE OR REPLACE VIEW sample AS SELECT 2"},
		{"CREATE OR REPLACE SQL SECURITY INVOKER VIEW sample AS SELECT 2", "CREATE OR REPLACE SQL SECURITY INVOKER VIEW sample AS SELECT 2"},
	} {
		t.Run(tc.source, func(t *testing.T) {
			got, err := ensureViewReplacement(tc.source)
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
	if _, err := ensureViewReplacement(strings.Repeat("x", 6)); err == nil {
		t.Fatal("missing CREATE accepted")
	}
}
