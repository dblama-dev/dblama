//go:build integration

package postgres

import (
	"context"
	"fmt"
	"io"
	"strings"
	"testing"
)

type sqlBarrierReader struct {
	first, rest string
	verify      func() error
	checked     bool
}

func (r *sqlBarrierReader) Read(p []byte) (int, error) {
	if r.first != "" {
		n := copy(p, r.first)
		r.first = r.first[n:]
		return n, nil
	}
	if !r.checked {
		r.checked = true
		if err := r.verify(); err != nil {
			return 0, err
		}
	}
	if r.rest == "" {
		return 0, io.EOF
	}
	n := copy(p, r.rest)
	r.rest = r.rest[n:]
	return n, nil
}
func TestImportSQLStreamingBeforeEOF(t *testing.T) {
	d := connectOK(t, startDB(t)).(*PostgresDriver)
	ctx := context.Background()
	r := &sqlBarrierReader{first: "CREATE TABLE stream_probe(id int);", rest: "CREATE FUNCTION stream_insert() RETURNS int AS $body$ BEGIN INSERT INTO stream_probe VALUES (1); RETURN 1; END $body$ LANGUAGE plpgsql; SELECT stream_insert();", verify: func() error {
		var count int
		if err := d.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM stream_probe").Scan(&count); err != nil {
			return fmt.Errorf("input read ahead before preceding statement executed: %w", err)
		}
		return nil
	}}
	if err := d.ImportSQL(ctx, d.loginDB, r, nil); err != nil {
		t.Fatal(err)
	}
	if err := d.ImportSQL(ctx, d.loginDB, strings.NewReader("INSERT INTO stream_probe VALUES (2); INSERT INTO stream_probe VALUES (3) /* unfinished"), nil); err == nil || !strings.Contains(err.Error(), "unterminated") {
		t.Fatalf("lexical error=%v", err)
	}
	var count int
	if err := d.db.QueryRowContext(ctx, "SELECT count(*) FROM stream_probe").Scan(&count); err != nil || count != 2 {
		t.Fatalf("partial statement executed: rows=%d err=%v", count, err)
	}
}
