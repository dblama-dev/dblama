//go:build integration

package mysql

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/hitslab/db-manager/server/internal/driver"
	"github.com/hitslab/db-manager/server/internal/sqlgen"
)

func TestShowCreateTable(t *testing.T) {
	for _, tc := range testImages {
		t.Run(tc.image, func(t *testing.T) {
			cfg := startDB(t, tc.image)
			drv := connectOK(t, cfg)

			execDDL(t, cfg, "CREATE TABLE t_sc (id INT PRIMARY KEY, name VARCHAR(50)) COMMENT='show create test'")
			t.Cleanup(func() { execDDL(t, cfg, "DROP TABLE IF EXISTS t_sc") })

			create, err := drv.ShowCreateTable(context.Background(), cfg.Database, "t_sc")
			if err != nil {
				t.Fatalf("ShowCreateTable(%q, %q) error: %v", cfg.Database, "t_sc", err)
			}
			if !strings.Contains(create, "CREATE TABLE") {
				t.Errorf("ShowCreateTable() = %q, want it to contain %q", create, "CREATE TABLE")
			}
			if !strings.Contains(create, "t_sc") {
				t.Errorf("ShowCreateTable() = %q, want it to contain the table name %q", create, "t_sc")
			}
			if !strings.Contains(create, "show create test") {
				t.Errorf("ShowCreateTable() = %q, want it to contain the table comment", create)
			}
		})
	}
}

func TestShowCreateTable_NotFound(t *testing.T) {
	for _, tc := range testImages {
		t.Run(tc.image, func(t *testing.T) {
			cfg := startDB(t, tc.image)
			drv := connectOK(t, cfg)

			_, err := drv.ShowCreateTable(context.Background(), cfg.Database, "t_missing_sc")
			if err == nil {
				t.Fatal("ShowCreateTable(missing table) error: expected error, got nil")
			}
			if !strings.Contains(err.Error(), "not found") {
				t.Errorf("ShowCreateTable(missing table) error = %q, want it to contain %q", err, "not found")
			}
		})
	}
}

func TestShowCreateTable_EmptyName(t *testing.T) {
	cfg := startDB(t, "mysql:8.4")
	drv := connectOK(t, cfg)

	if _, err := drv.ShowCreateTable(context.Background(), "", "t_sc"); err == nil {
		t.Fatal("ShowCreateTable(empty db) error: expected error, got nil")
	}
	if _, err := drv.ShowCreateTable(context.Background(), cfg.Database, ""); err == nil {
		t.Fatal("ShowCreateTable(empty table) error: expected error, got nil")
	}
}

// TestExportSQL_RoundTrip exports two tables (one with a primary key —
// keyset paging — and one without — offset paging) holding the nasty
// value kinds, imports the dump into a NEW database through ExecSQL and
// compares structure and data against the source.
func TestExportSQL_RoundTrip(t *testing.T) {
	for _, tc := range testImages {
		t.Run(tc.image, func(t *testing.T) {
			cfg := startDB(t, tc.image)
			drv := connectOK(t, cfg)

			execDDL(t, cfg, "CREATE TABLE d_rt_pk (id INT PRIMARY KEY, name VARCHAR(100), note TEXT, bin VARBINARY(16), created DATETIME)")
			execDDL(t, cfg, "INSERT INTO d_rt_pk VALUES (1, NULL, NULL, NULL, NULL)")
			execDDL(t, cfg, "INSERT INTO d_rt_pk VALUES (2, '', '', X'', '2026-01-02 10:20:30')")
			execDDL(t, cfg, "INSERT INTO d_rt_pk VALUES (3, 'comma,quote''back\\slash', 'line1\nline2', 0x00FF10, '2026-12-31 23:59:59')")
			execDDL(t, cfg, "INSERT INTO d_rt_pk VALUES (4, 'emoji 🦄 текст', 'tab\there', 0xDEADBEEF, NULL)")
			execDDL(t, cfg, "INSERT INTO d_rt_pk VALUES (5, NULL, 'x', NULL, '2024-02-29 00:00:00')")
			execDDL(t, cfg, "CREATE TABLE d_rt_nopk (a VARCHAR(20), b INT)")
			execDDL(t, cfg, "INSERT INTO d_rt_nopk VALUES ('x', 1), (NULL, 2), ('', NULL)")
			t.Cleanup(func() {
				execDDL(t, cfg, "DROP TABLE IF EXISTS d_rt_nopk")
				execDDL(t, cfg, "DROP TABLE IF EXISTS d_rt_pk")
				execDDL(t, cfg, "DROP DATABASE IF EXISTS dump_rt_target")
			})

			var dump bytes.Buffer
			err := drv.Export(context.Background(), cfg.Database, driver.ExportRequest{
				Format: "sql",
				Tables: []string{"d_rt_pk", "d_rt_nopk"},
			}, &dump)
			if err != nil {
				t.Fatalf("Export(sql) error: %v", err)
			}

			// Import into a NEW database via the console batch runner:
			// it USEs the target db on its dedicated connection, so the
			// unqualified dump statements land there.
			execDDL(t, cfg, "CREATE DATABASE dump_rt_target")
			ctx := context.Background()
			if _, err := drv.ExecSQL(ctx, "dump_rt_target", dump.String(), "dump-import", driver.ExecOpts{}); err != nil {
				t.Fatalf("import dump error: %v", err)
			}

			for _, table := range []string{"d_rt_pk", "d_rt_nopk"} {
				srcDef, err := drv.TableDef(ctx, cfg.Database, table)
				if err != nil {
					t.Fatalf("TableDef(%s) source error: %v", table, err)
				}
				tgtDef, err := drv.TableDef(ctx, "dump_rt_target", table)
				if err != nil {
					t.Fatalf("TableDef(%s) target error: %v", table, err)
				}
				// The AUTO_INCREMENT counter may legitimately differ; it
				// is not part of the structural contract.
				srcDef.Options.AutoIncrement, tgtDef.Options.AutoIncrement = 0, 0
				if !reflect.DeepEqual(srcDef, tgtDef) {
					t.Errorf("TableDef(%s) mismatch:\nsource: %+v\ntarget: %+v", table, srcDef, tgtDef)
				}

				srcPage, err := drv.SelectRows(ctx, cfg.Database, table, sqlgen.SelectQuery{Limit: 1000})
				if err != nil {
					t.Fatalf("SelectRows(%s) source error: %v", table, err)
				}
				tgtPage, err := drv.SelectRows(ctx, "dump_rt_target", table, sqlgen.SelectQuery{Limit: 1000})
				if err != nil {
					t.Fatalf("SelectRows(%s) target error: %v", table, err)
				}
				if len(srcPage.Rows) == 0 {
					t.Fatalf("SelectRows(%s) source returned no rows", table)
				}
				sortDumpRows(srcPage.Rows)
				sortDumpRows(tgtPage.Rows)
				if !reflect.DeepEqual(srcPage.Rows, tgtPage.Rows) {
					t.Errorf("data mismatch of %s:\nsource: %v\ntarget: %v", table, srcPage.Rows, tgtPage.Rows)
				}
			}
		})
	}
}

// sortDumpRows orders rows by their serialized form so the natural-order
// comparison is order-independent.
func sortDumpRows(rows [][]any) {
	keys := make([]string, len(rows))
	for i, row := range rows {
		parts := make([]string, len(row))
		for j, v := range row {
			parts[j] = fmt.Sprintf("%v", v)
		}
		keys[i] = strings.Join(parts, "\x00")
	}
	// insertion sort keeps it dependency-free and is fine at test size
	for i := 1; i < len(rows); i++ {
		for j := i; j > 0 && keys[j] < keys[j-1]; j-- {
			keys[j], keys[j-1] = keys[j-1], keys[j]
			rows[j], rows[j-1] = rows[j-1], rows[j]
		}
	}
}

// TestExportCSV_SpecialValues exports a table holding NULL, commas,
// quotes, newlines and binary values as CSV and parses it back with
// encoding/csv: every value must survive the round trip, NULL as an
// empty field and binary as its "0x…" hex string.
func TestExportCSV_SpecialValues(t *testing.T) {
	for _, tc := range testImages {
		t.Run(tc.image, func(t *testing.T) {
			cfg := startDB(t, tc.image)
			drv := connectOK(t, cfg)

			execDDL(t, cfg, "CREATE TABLE d_csv_t (id INT PRIMARY KEY, s VARCHAR(100), bin VARBINARY(8))")
			execDDL(t, cfg, "INSERT INTO d_csv_t VALUES (1, NULL, NULL)")
			execDDL(t, cfg, "INSERT INTO d_csv_t VALUES (2, 'has,comma', 0x01FF)")
			execDDL(t, cfg, "INSERT INTO d_csv_t VALUES (3, 'has\"quote', X'')")
			execDDL(t, cfg, "INSERT INTO d_csv_t VALUES (4, 'line1\nline2', NULL)")
			execDDL(t, cfg, "INSERT INTO d_csv_t VALUES (5, '', 0x00)")
			t.Cleanup(func() { execDDL(t, cfg, "DROP TABLE IF EXISTS d_csv_t") })

			var out bytes.Buffer
			err := drv.Export(context.Background(), cfg.Database, driver.ExportRequest{
				Format: "csv",
				Tables: []string{"d_csv_t"},
			}, &out)
			if err != nil {
				t.Fatalf("Export(csv) error: %v", err)
			}

			records, err := csv.NewReader(bytes.NewReader(out.Bytes())).ReadAll()
			if err != nil {
				t.Fatalf("parse exported csv error: %v", err)
			}
			want := [][]string{
				{"id", "s", "bin"},
				{"1", "", ""},
				{"2", "has,comma", "0x01ff"}, // hex renders lowercase
				{`3`, `has"quote`, "0x"},     // empty binary → bare "0x" prefix
				{"4", "line1\nline2", ""},
				{"5", "", "0x00"},
			}
			if !reflect.DeepEqual(records, want) {
				t.Errorf("csv records =\n%q\nwant\n%q", records, want)
			}
		})
	}
}

// TestExport_Gzip pins the gzip envelope: the output starts with the
// 1f 8b magic bytes and decompresses to the same CSV as the plain run.
func TestExport_Gzip(t *testing.T) {
	cfg := startDB(t, "mysql:8.4")
	drv := connectOK(t, cfg)

	execDDL(t, cfg, "CREATE TABLE d_gz_t (id INT PRIMARY KEY, s VARCHAR(20))")
	execDDL(t, cfg, "INSERT INTO d_gz_t VALUES (1, 'plain'), (2, 'has,comma')")
	t.Cleanup(func() { execDDL(t, cfg, "DROP TABLE IF EXISTS d_gz_t") })

	var plain, zipped bytes.Buffer
	ctx := context.Background()
	req := driver.ExportRequest{Format: "csv", Tables: []string{"d_gz_t"}}
	if err := drv.Export(ctx, cfg.Database, req, &plain); err != nil {
		t.Fatalf("Export(csv) error: %v", err)
	}
	req.Gzip = true
	if err := drv.Export(ctx, cfg.Database, req, &zipped); err != nil {
		t.Fatalf("Export(csv, gzip) error: %v", err)
	}

	b := zipped.Bytes()
	if len(b) < 2 || b[0] != 0x1f || b[1] != 0x8b {
		t.Fatalf("gzip output starts with % x, want 1f 8b", b[:min(2, len(b))])
	}

	zr, err := gzip.NewReader(bytes.NewReader(b))
	if err != nil {
		t.Fatalf("gzip.NewReader error: %v", err)
	}
	defer func() { _ = zr.Close() }()
	decompressed, err := io.ReadAll(zr)
	if err != nil {
		t.Fatalf("gzip decompress error: %v", err)
	}
	if !bytes.Equal(decompressed, plain.Bytes()) {
		t.Errorf("decompressed output = %q, want the plain export %q", decompressed, plain.Bytes())
	}
}

// TestExportSQL_KeysetPaging exports a 250-row table (bigger than the
// 100-row batch) with Tables empty — the whole-database default — and
// verifies every row survives an import into a new database: no dup and
// no skip from the paging.
func TestExportSQL_KeysetPaging(t *testing.T) {
	for _, tc := range testImages {
		t.Run(tc.image, func(t *testing.T) {
			cfg := startDB(t, tc.image)
			drv := connectOK(t, cfg)

			execDDL(t, cfg, "CREATE TABLE d_kp (id INT PRIMARY KEY, v VARCHAR(10))")
			var tuples strings.Builder
			for i := range 250 {
				if i > 0 {
					tuples.WriteString(",")
				}
				fmt.Fprintf(&tuples, "(%d,'r%d')", i, i)
			}
			execDDL(t, cfg, "INSERT INTO d_kp VALUES "+tuples.String())
			t.Cleanup(func() {
				execDDL(t, cfg, "DROP TABLE IF EXISTS d_kp")
				execDDL(t, cfg, "DROP DATABASE IF EXISTS dump_kp_target")
			})

			var dump bytes.Buffer
			err := drv.Export(context.Background(), cfg.Database, driver.ExportRequest{Format: "sql"}, &dump)
			if err != nil {
				t.Fatalf("Export(sql, whole db) error: %v", err)
			}

			execDDL(t, cfg, "CREATE DATABASE dump_kp_target")
			ctx := context.Background()
			if _, err := drv.ExecSQL(ctx, "dump_kp_target", dump.String(), "dump-import", driver.ExecOpts{}); err != nil {
				t.Fatalf("import dump error: %v", err)
			}
			results, err := drv.ExecSQL(ctx, "dump_kp_target", "SELECT COUNT(*) FROM d_kp", "dump-count", driver.ExecOpts{})
			if err != nil {
				t.Fatalf("count query error: %v", err)
			}
			if len(results) != 1 || len(results[0].Rows) != 1 || len(results[0].Rows[0]) != 1 {
				t.Fatalf("count query returned %+v, want one single-cell row", results)
			}
			count, ok := results[0].Rows[0][0].(int64)
			if !ok {
				t.Fatalf("count value is %T (%v), want int64", results[0].Rows[0][0], results[0].Rows[0][0])
			}
			if count != 250 {
				t.Errorf("imported row count = %d, want 250", count)
			}
		})
	}
}

// TestExport_Options pins the SQL dump options in the output text:
// IfNotExists rewrites the CREATE statement, DropIfExists prepends the
// DROP, and both are absent by default.
func TestExport_Options(t *testing.T) {
	cfg := startDB(t, "mysql:8.4")
	drv := connectOK(t, cfg)

	execDDL(t, cfg, "CREATE TABLE d_opt_t (id INT PRIMARY KEY)")
	t.Cleanup(func() { execDDL(t, cfg, "DROP TABLE IF EXISTS d_opt_t") })

	ctx := context.Background()
	var plain, both bytes.Buffer
	if err := drv.Export(ctx, cfg.Database, driver.ExportRequest{Format: "sql", Tables: []string{"d_opt_t"}}, &plain); err != nil {
		t.Fatalf("Export(sql) error: %v", err)
	}
	if err := drv.Export(ctx, cfg.Database, driver.ExportRequest{
		Format:       "sql",
		Tables:       []string{"d_opt_t"},
		DropIfExists: true,
		IfNotExists:  true,
	}, &both); err != nil {
		t.Fatalf("Export(sql, options) error: %v", err)
	}

	if strings.Contains(plain.String(), "IF NOT EXISTS") || strings.Contains(plain.String(), "DROP TABLE") {
		t.Errorf("plain dump must not contain IF NOT EXISTS or DROP TABLE:\n%s", plain.String())
	}
	if !strings.Contains(both.String(), "CREATE TABLE IF NOT EXISTS") {
		t.Errorf("IfNotExists dump missing CREATE TABLE IF NOT EXISTS:\n%s", both.String())
	}
	if !strings.Contains(both.String(), "DROP TABLE IF EXISTS") {
		t.Errorf("DropIfExists dump missing DROP TABLE IF EXISTS:\n%s", both.String())
	}
	if !strings.Contains(both.String(), "SET NAMES utf8mb4;") {
		t.Errorf("dump header missing SET NAMES utf8mb4;:\n%s", both.String())
	}
}

// TestExport_Rejections pins the argument validation: an unknown
// format, csv without exactly one table, and both fail before anything
// is written.
func TestExport_Rejections(t *testing.T) {
	cfg := startDB(t, "mysql:8.4")
	drv := connectOK(t, cfg)

	execDDL(t, cfg, "CREATE TABLE d_rej_t (id INT PRIMARY KEY)")
	t.Cleanup(func() { execDDL(t, cfg, "DROP TABLE IF EXISTS d_rej_t") })

	ctx := context.Background()
	var buf bytes.Buffer
	if err := drv.Export(ctx, cfg.Database, driver.ExportRequest{Format: "xml", Tables: []string{"d_rej_t"}}, &buf); err == nil {
		t.Error("Export(unknown format) error: expected error, got nil")
	}
	if err := drv.Export(ctx, cfg.Database, driver.ExportRequest{Format: "csv"}, &buf); err == nil {
		t.Error("Export(csv, no tables) error: expected error, got nil")
	}
	if err := drv.Export(ctx, cfg.Database, driver.ExportRequest{Format: "tsv", Tables: []string{"d_rej_t", "other"}}, &buf); err == nil {
		t.Error("Export(tsv, two tables) error: expected error, got nil")
	}
	if err := drv.Export(ctx, cfg.Database, driver.ExportRequest{Format: "csv", Tables: []string{"d_missing_t"}}, &buf); err == nil {
		t.Error("Export(csv, missing table) error: expected error, got nil")
	}
	if buf.Len() != 0 {
		t.Errorf("rejected exports wrote %d bytes, want nothing", buf.Len())
	}
}

// errWriter fails every write.
type errWriter struct{}

func (errWriter) Write([]byte) (int, error) { return 0, errors.New("write failed") }

// TestExport_WriteError pins that a failing underlying writer aborts the
// export with an error — including the gzip case, whose tiny payloads
// only reach the writer when the stream footer is flushed at the end.
func TestExport_WriteError(t *testing.T) {
	cfg := startDB(t, "mysql:8.4")
	drv := connectOK(t, cfg)

	execDDL(t, cfg, "CREATE TABLE d_werr_t (id INT PRIMARY KEY)")
	t.Cleanup(func() { execDDL(t, cfg, "DROP TABLE IF EXISTS d_werr_t") })

	ctx := context.Background()
	for _, zipped := range []bool{false, true} {
		err := drv.Export(ctx, cfg.Database, driver.ExportRequest{
			Format: "csv",
			Tables: []string{"d_werr_t"},
			Gzip:   zipped,
		}, errWriter{})
		if err == nil {
			t.Errorf("Export(csv, gzip=%v) to failing writer: expected error, got nil", zipped)
		}
	}
}
