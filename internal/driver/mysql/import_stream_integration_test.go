//go:build integration

package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/dblama-dev/dblama/internal/driver"
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
	for _, image := range testImages {
		t.Run(image.image, func(t *testing.T) {
			d := connectOK(t, startDB(t, image.image)).(*MysqlDriver)
			ctx := context.Background()
			r := &sqlBarrierReader{first: "CREATE TABLE stream_probe(id int);", rest: "INSERT INTO stream_probe VALUES (1);", verify: func() error {
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
		})
	}
}

func TestRawSQLSessionStateDoesNotLeak(t *testing.T) {
	for _, image := range testImages {
		t.Run(image.image, func(t *testing.T) {
			d := connectOK(t, startDB(t, image.image)).(*MysqlDriver)
			d.db.SetMaxOpenConns(1)
			ctx := context.Background()
			var originalMode string
			if err := d.db.QueryRowContext(ctx, "SELECT @@SESSION.sql_mode").Scan(&originalMode); err != nil {
				t.Fatal(err)
			}
			for _, mode := range []string{"import", "console"} {
				t.Run(mode, func(t *testing.T) {
					query := "SET @dblama_session_marker='leaked'; SET SESSION sql_mode='ANSI_QUOTES';"
					var err error
					if mode == "import" {
						err = d.ImportSQL(ctx, d.loginDB, strings.NewReader(query), nil)
					} else {
						_, err = d.ExecSQL(ctx, d.loginDB, query, "state-test", driver.ExecOpts{})
					}
					if err != nil {
						t.Fatal(err)
					}
					var marker sql.NullString
					var sqlMode string
					if err := d.db.QueryRowContext(ctx, "SELECT @dblama_session_marker, @@SESSION.sql_mode").Scan(&marker, &sqlMode); err != nil {
						t.Fatal(err)
					}
					if marker.Valid || sqlMode != originalMode {
						t.Fatalf("raw SQL session leaked: marker=%v sql_mode=%q want=%q", marker.Valid, sqlMode, originalMode)
					}
				})
			}
		})
	}
}
