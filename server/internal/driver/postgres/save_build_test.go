package postgres

import (
	"strings"
	"testing"

	"github.com/hitslab/db-manager/server/internal/driver"
)

// buildSchema is the current schema the golden builders qualify with — a
// name with a space proves that every part of every identifier is
// double-quoted.
const buildSchema = "odd schema"

// pgCreateFixture is the golden CREATE table: serial, varchar with a
// literal default and a column comment, numeric(10,2), jsonb, a
// multi-word type, PRIMARY + UNIQUE inline, a plain INDEX (which
// PostgreSQL cannot inline), a schema-qualified foreign key and a table
// comment.
func pgCreateFixture() driver.TableDef {
	return driver.TableDef{
		NewName: "child",
		Columns: []driver.ColumnDef{
			{Name: "id", Type: "integer", AutoIncrement: true},
			{Name: "name", Type: "varchar", Length: "64", Default: &driver.DefaultValue{Value: "x"}, Comment: "display name"},
			{Name: "price", Type: "numeric", Length: "10,2", Nullable: true},
			{Name: "data", Type: "jsonb", Nullable: true},
			{Name: "flags", Type: "double precision", Nullable: true},
			{Name: "parent_id", Type: "bigint", Nullable: true},
		},
		Indexes: []driver.Index{
			{Name: "PRIMARY", Type: "PRIMARY", Columns: []driver.IndexColumn{{Name: "id"}}},
			{Name: "uq_name", Type: "UNIQUE", Columns: []driver.IndexColumn{{Name: "name"}}},
			{Name: "idx_price", Type: "INDEX", Columns: []driver.IndexColumn{{Name: "price"}}},
		},
		ForeignKeys: []driver.ForeignKey{{
			Name:       "fk_parent",
			Columns:    []string{"parent_id"},
			RefTable:   "other schema.parent",
			RefColumns: []string{"id"},
			OnDelete:   "CASCADE",
			OnUpdate:   "RESTRICT",
		}},
		Options: driver.TableOptions{Comment: "child table"},
	}
}

func assertStatements(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("produced %d statements, want %d:\n%s", len(got), len(want), strings.Join(got, "\n"))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("step %d:\ngot:  %s\nwant: %s", i+1, got[i], want[i])
		}
	}
}

func TestBuildCreateTable_PG(t *testing.T) {
	got, err := buildCreateTable(buildSchema, pgCreateFixture())
	if err != nil {
		t.Fatalf("buildCreateTable() error: %v", err)
	}
	want := []string{
		"CREATE TABLE \"odd schema\".\"child\" (\n" +
			"  \"id\" serial NOT NULL,\n" +
			"  \"name\" varchar(64) NOT NULL DEFAULT 'x',\n" +
			"  \"price\" numeric(10,2) NULL,\n" +
			"  \"data\" jsonb NULL,\n" +
			"  \"flags\" double precision NULL,\n" +
			"  \"parent_id\" bigint NULL,\n" +
			"  PRIMARY KEY (\"id\"),\n" +
			"  CONSTRAINT \"uq_name\" UNIQUE (\"name\"),\n" +
			"  CONSTRAINT \"fk_parent\" FOREIGN KEY (\"parent_id\") REFERENCES \"other schema\".\"parent\" (\"id\") ON DELETE CASCADE ON UPDATE RESTRICT\n" +
			")",
		"CREATE INDEX \"idx_price\" ON \"odd schema\".\"child\" (\"price\")",
		"COMMENT ON TABLE \"odd schema\".\"child\" IS 'child table'",
		"COMMENT ON COLUMN \"odd schema\".\"child\".\"name\" IS 'display name'",
	}
	assertStatements(t, got, want)
}

// TestBuildCreateTable_PG_SerialNoDoubleDefault pins the carry-in
// contract from the Task 2 review: the read path reports a serial as
// BOTH AutoIncrement=true and a nextval(...) Default, and the save path
// must render the serial pseudo-type alone — a second DEFAULT clause
// would duplicate (or collide with) the sequence default.
func TestBuildCreateTable_PG_SerialNoDoubleDefault(t *testing.T) {
	desired := driver.TableDef{
		NewName: "t",
		Columns: []driver.ColumnDef{
			{
				Name:          "id",
				Type:          "integer",
				AutoIncrement: true,
				Default:       &driver.DefaultValue{Value: "nextval('t_id_seq'::regclass)", IsSQLExpression: true},
			},
		},
	}
	got, err := buildCreateTable(buildSchema, desired)
	if err != nil {
		t.Fatalf("buildCreateTable() error: %v", err)
	}
	want := []string{"CREATE TABLE \"odd schema\".\"t\" (\n  \"id\" serial NOT NULL\n)"}
	assertStatements(t, got, want)
	if strings.Contains(got[0], "nextval") {
		t.Errorf("serial column rendered a DEFAULT clause: %s", got[0])
	}
}

// TestBuildCreateTable_PG_FragmentVariants pins the column-fragment
// features the full golden does not cover: bigserial, expression and
// NULL-literal defaults, and a stored generated column.
func TestBuildCreateTable_PG_FragmentVariants(t *testing.T) {
	desired := driver.TableDef{
		NewName: "t",
		Columns: []driver.ColumnDef{
			{Name: "big_id", Type: "bigint", AutoIncrement: true},
			{Name: "ref", Type: "uuid", Nullable: true, Default: &driver.DefaultValue{Value: "gen_random_uuid()", IsSQLExpression: true}},
			{Name: "n", Type: "integer", Nullable: true, Default: &driver.DefaultValue{Value: "NULL", IsSQLExpression: true}},
			{Name: "full", Type: "varchar", Length: "128", Generated: "STORED", Default: &driver.DefaultValue{Value: "UPPER(name)", IsSQLExpression: true}},
		},
	}
	got, err := buildCreateTable(buildSchema, desired)
	if err != nil {
		t.Fatalf("buildCreateTable() error: %v", err)
	}
	want := []string{
		"CREATE TABLE \"odd schema\".\"t\" (\n" +
			"  \"big_id\" bigserial NOT NULL,\n" +
			"  \"ref\" uuid NULL DEFAULT gen_random_uuid(),\n" +
			"  \"n\" integer NULL DEFAULT NULL,\n" +
			"  \"full\" varchar(128) NOT NULL GENERATED ALWAYS AS (UPPER(name)) STORED\n" +
			")",
	}
	assertStatements(t, got, want)
}

func TestBuildAlterTable_PG(t *testing.T) {
	desired := driver.TableDef{
		NewName: "child_new",
		Columns: []driver.ColumnDef{
			// CHANGE: rename + redefine (nullable, no default).
			{Name: "title", OldName: "name", Type: "varchar", Length: "128", Nullable: true, Comment: "renamed"},
			// DROP: existing column marked with an empty type.
			{Name: "extra", OldName: "extra", Nullable: true},
			// ADD.
			{Name: "score", Type: "numeric", Length: "8,2", Nullable: true, Default: &driver.DefaultValue{Value: "0"}},
			// MODIFY: not-null toggle on.
			{Name: "flag", OldName: "flag", Type: "boolean"},
			// MODIFY: nullable with an expression default.
			{Name: "stamp", OldName: "stamp", Type: "timestamptz", Nullable: true, Default: &driver.DefaultValue{Value: "now()", IsSQLExpression: true}},
		},
		Options: driver.TableOptions{Comment: "new comment"},
	}

	got, err := buildAlterTable(buildSchema, "child", desired)
	if err != nil {
		t.Fatalf("buildAlterTable() error: %v", err)
	}
	want := []string{
		// The rename target is a bare name: PostgreSQL renames within
		// the table's own schema.
		"ALTER TABLE \"odd schema\".\"child\" RENAME TO \"child_new\"",
		// Steps after the rename address the new table name.
		"ALTER TABLE \"odd schema\".\"child_new\" RENAME COLUMN \"name\" TO \"title\"",
		"ALTER TABLE \"odd schema\".\"child_new\" ALTER COLUMN \"title\" TYPE varchar(128)",
		"ALTER TABLE \"odd schema\".\"child_new\" ALTER COLUMN \"title\" DROP NOT NULL",
		"ALTER TABLE \"odd schema\".\"child_new\" ALTER COLUMN \"title\" DROP DEFAULT",
		"COMMENT ON COLUMN \"odd schema\".\"child_new\".\"title\" IS 'renamed'",
		"ALTER TABLE \"odd schema\".\"child_new\" DROP COLUMN \"extra\"",
		"ALTER TABLE \"odd schema\".\"child_new\" ADD COLUMN \"score\" numeric(8,2) NULL DEFAULT '0'",
		"ALTER TABLE \"odd schema\".\"child_new\" ALTER COLUMN \"flag\" TYPE boolean",
		"ALTER TABLE \"odd schema\".\"child_new\" ALTER COLUMN \"flag\" SET NOT NULL",
		"ALTER TABLE \"odd schema\".\"child_new\" ALTER COLUMN \"flag\" DROP DEFAULT",
		"ALTER TABLE \"odd schema\".\"child_new\" ALTER COLUMN \"stamp\" TYPE timestamptz",
		"ALTER TABLE \"odd schema\".\"child_new\" ALTER COLUMN \"stamp\" DROP NOT NULL",
		"ALTER TABLE \"odd schema\".\"child_new\" ALTER COLUMN \"stamp\" SET DEFAULT now()",
		"COMMENT ON TABLE \"odd schema\".\"child_new\" IS 'new comment'",
	}
	assertStatements(t, got, want)
}

// TestBuildAlterTable_PG_SerialModifyKeepsDefault pins the modify side
// of the serial contract: an AutoIncrement column read back with its
// nextval Default renders only the type and nullability — no SET/DROP
// DEFAULT statement may touch the sequence default.
func TestBuildAlterTable_PG_SerialModifyKeepsDefault(t *testing.T) {
	desired := driver.TableDef{
		NewName: "same",
		Columns: []driver.ColumnDef{
			{
				Name:          "id",
				OldName:       "id",
				Type:          "integer",
				AutoIncrement: true,
				Default:       &driver.DefaultValue{Value: "nextval('same_id_seq'::regclass)", IsSQLExpression: true},
			},
		},
	}
	got, err := buildAlterTable(buildSchema, "same", desired)
	if err != nil {
		t.Fatalf("buildAlterTable() error: %v", err)
	}
	want := []string{
		"ALTER TABLE \"odd schema\".\"same\" ALTER COLUMN \"id\" TYPE integer",
		"ALTER TABLE \"odd schema\".\"same\" ALTER COLUMN \"id\" SET NOT NULL",
	}
	assertStatements(t, got, want)
}

// TestBuildAlterTable_PG_GeneratedModifySkipsAttributes pins the
// documented v0.1 limitation: a generated column can be renamed but its
// type, nullability and default cannot be re-rendered (PostgreSQL
// cannot re-type a generated column).
func TestBuildAlterTable_PG_GeneratedModifySkipsAttributes(t *testing.T) {
	desired := driver.TableDef{
		NewName: "t",
		Columns: []driver.ColumnDef{
			{Name: "g2", OldName: "g", Type: "integer", Nullable: true, Generated: "STORED",
				Default: &driver.DefaultValue{Value: "id * 2", IsSQLExpression: true}},
		},
	}
	got, err := buildAlterTable(buildSchema, "t", desired)
	if err != nil {
		t.Fatalf("buildAlterTable() error: %v", err)
	}
	want := []string{"ALTER TABLE \"odd schema\".\"t\" RENAME COLUMN \"g\" TO \"g2\""}
	assertStatements(t, got, want)
}

func TestBuildCreateTable_PG_Errors(t *testing.T) {
	t.Run("empty table name", func(t *testing.T) {
		_, err := buildCreateTable(buildSchema, driver.TableDef{Columns: []driver.ColumnDef{{Name: "id", Type: "integer"}}})
		if err == nil {
			t.Fatal("expected error for empty table name, got nil")
		}
	})
	t.Run("no columns", func(t *testing.T) {
		_, err := buildCreateTable(buildSchema, driver.TableDef{NewName: "t"})
		if err == nil {
			t.Fatal("expected error for empty column list, got nil")
		}
	})
	t.Run("malicious type", func(t *testing.T) {
		_, err := buildCreateTable(buildSchema, driver.TableDef{
			NewName: "t",
			Columns: []driver.ColumnDef{{Name: "c", Type: "int; DROP TABLE x"}},
		})
		if err == nil {
			t.Fatal("expected error for malicious type, got nil")
		}
	})
	t.Run("malicious length", func(t *testing.T) {
		_, err := buildCreateTable(buildSchema, driver.TableDef{
			NewName: "t",
			Columns: []driver.ColumnDef{{Name: "c", Type: "varchar", Length: "64) ; DROP TABLE x"}},
		})
		if err == nil {
			t.Fatal("expected error for malicious length, got nil")
		}
	})
	t.Run("fulltext index", func(t *testing.T) {
		_, err := buildCreateTable(buildSchema, driver.TableDef{
			NewName: "t",
			Columns: []driver.ColumnDef{{Name: "c", Type: "text"}},
			Indexes: []driver.Index{{Name: "ft", Type: "FULLTEXT", Columns: []driver.IndexColumn{{Name: "c"}}}},
		})
		if err == nil || !strings.Contains(err.Error(), "not supported on postgres") {
			t.Errorf("error = %v, want not supported on postgres", err)
		}
	})
}

func TestBuildAlterTable_PG_Errors(t *testing.T) {
	t.Run("empty table", func(t *testing.T) {
		_, err := buildAlterTable(buildSchema, "", driver.TableDef{Columns: []driver.ColumnDef{{Name: "id", OldName: "id", Type: "integer"}}})
		if err == nil {
			t.Fatal("expected error for empty table name, got nil")
		}
	})
	t.Run("column with neither type nor old name", func(t *testing.T) {
		_, err := buildAlterTable(buildSchema, "t", driver.TableDef{Columns: []driver.ColumnDef{{Name: "ghost"}}})
		if err == nil {
			t.Fatal("expected error for column without type and old name, got nil")
		}
	})
	t.Run("drop column without name", func(t *testing.T) {
		_, err := buildAlterTable(buildSchema, "t", driver.TableDef{Columns: []driver.ColumnDef{{OldName: "x", Nullable: true}}})
		if err == nil {
			t.Fatal("expected error for unnamed drop, got nil")
		}
	})
}

// TestBuildAlterIndexes pins the diff: drops before adds, PRIMARY
// matched by type (PostgreSQL names it <table>_pkey, not PRIMARY),
// everything else by name; changed means same name with different type
// or columns.
func TestBuildAlterIndexes(t *testing.T) {
	t.Run("diff with rename-in-place", func(t *testing.T) {
		current := []driver.Index{
			{Name: "t_pkey", Type: "PRIMARY", Columns: []driver.IndexColumn{{Name: "id"}}},
			{Name: "uq_old", Type: "UNIQUE", Columns: []driver.IndexColumn{{Name: "name"}}},
			{Name: "idx_gone", Type: "INDEX", Columns: []driver.IndexColumn{{Name: "price"}}},
		}
		desired := []driver.Index{
			{Name: "PRIMARY", Type: "PRIMARY", Columns: []driver.IndexColumn{{Name: "id"}}},
			{Name: "uq_old", Type: "UNIQUE", Columns: []driver.IndexColumn{{Name: "kind"}}},
			{Name: "idx_new", Type: "INDEX", Columns: []driver.IndexColumn{{Name: "price"}, {Name: "name"}}},
		}
		got, err := buildAlterIndexes(buildSchema, "t", current, desired)
		if err != nil {
			t.Fatalf("buildAlterIndexes() error: %v", err)
		}
		want := []string{
			"DROP INDEX \"odd schema\".\"uq_old\"",
			"DROP INDEX \"odd schema\".\"idx_gone\"",
			"CREATE UNIQUE INDEX \"uq_old\" ON \"odd schema\".\"t\" (\"kind\")",
			"CREATE INDEX \"idx_new\" ON \"odd schema\".\"t\" (\"price\", \"name\")",
		}
		assertStatements(t, got, want)
	})
	t.Run("primary drop uses constraint name", func(t *testing.T) {
		current := []driver.Index{
			{Name: "t_pkey", Type: "PRIMARY", Columns: []driver.IndexColumn{{Name: "id"}}},
		}
		got, err := buildAlterIndexes(buildSchema, "t", current, nil)
		if err != nil {
			t.Fatalf("buildAlterIndexes() error: %v", err)
		}
		want := []string{"ALTER TABLE \"odd schema\".\"t\" DROP CONSTRAINT \"t_pkey\""}
		assertStatements(t, got, want)
	})
	t.Run("primary add", func(t *testing.T) {
		desired := []driver.Index{
			{Name: "PRIMARY", Type: "PRIMARY", Columns: []driver.IndexColumn{{Name: "id"}, {Name: "name"}}},
		}
		got, err := buildAlterIndexes(buildSchema, "t", nil, desired)
		if err != nil {
			t.Fatalf("buildAlterIndexes() error: %v", err)
		}
		want := []string{"ALTER TABLE \"odd schema\".\"t\" ADD PRIMARY KEY (\"id\", \"name\")"}
		assertStatements(t, got, want)
	})
	t.Run("changed primary re-created", func(t *testing.T) {
		current := []driver.Index{
			{Name: "t_pkey", Type: "PRIMARY", Columns: []driver.IndexColumn{{Name: "id"}}},
		}
		desired := []driver.Index{
			{Name: "PRIMARY", Type: "PRIMARY", Columns: []driver.IndexColumn{{Name: "id"}, {Name: "name"}}},
		}
		got, err := buildAlterIndexes(buildSchema, "t", current, desired)
		if err != nil {
			t.Fatalf("buildAlterIndexes() error: %v", err)
		}
		want := []string{
			"ALTER TABLE \"odd schema\".\"t\" DROP CONSTRAINT \"t_pkey\"",
			"ALTER TABLE \"odd schema\".\"t\" ADD PRIMARY KEY (\"id\", \"name\")",
		}
		assertStatements(t, got, want)
	})
	t.Run("fulltext not supported", func(t *testing.T) {
		desired := []driver.Index{{Name: "ft", Type: "FULLTEXT", Columns: []driver.IndexColumn{{Name: "c"}}}}
		if _, err := buildAlterIndexes(buildSchema, "t", nil, desired); err == nil || !strings.Contains(err.Error(), "not supported on postgres") {
			t.Errorf("error = %v, want not supported on postgres", err)
		}
	})
	t.Run("spatial not supported", func(t *testing.T) {
		desired := []driver.Index{{Name: "sp", Type: "SPATIAL", Columns: []driver.IndexColumn{{Name: "c"}}}}
		if _, err := buildAlterIndexes(buildSchema, "t", nil, desired); err == nil || !strings.Contains(err.Error(), "not supported on postgres") {
			t.Errorf("error = %v, want not supported on postgres", err)
		}
	})
	t.Run("unnamed non-primary", func(t *testing.T) {
		desired := []driver.Index{{Type: "UNIQUE", Columns: []driver.IndexColumn{{Name: "c"}}}}
		if _, err := buildAlterIndexes(buildSchema, "t", nil, desired); err == nil {
			t.Fatal("expected error for unnamed unique index, got nil")
		}
	})
	t.Run("duplicate names", func(t *testing.T) {
		desired := []driver.Index{
			{Name: "uq", Type: "INDEX", Columns: []driver.IndexColumn{{Name: "c"}}},
			{Name: "UQ", Type: "INDEX", Columns: []driver.IndexColumn{{Name: "d"}}},
		}
		if _, err := buildAlterIndexes(buildSchema, "t", nil, desired); err == nil {
			t.Fatal("expected error for duplicate index names, got nil")
		}
	})
	t.Run("no columns", func(t *testing.T) {
		desired := []driver.Index{{Name: "uq", Type: "UNIQUE"}}
		if _, err := buildAlterIndexes(buildSchema, "t", nil, desired); err == nil {
			t.Fatal("expected error for index without columns, got nil")
		}
	})
	t.Run("prefix sub-part not supported", func(t *testing.T) {
		desired := []driver.Index{{Name: "uq", Type: "UNIQUE", Columns: []driver.IndexColumn{{Name: "c", SubPart: 10}}}}
		if _, err := buildAlterIndexes(buildSchema, "t", nil, desired); err == nil || !strings.Contains(err.Error(), "not supported on postgres") {
			t.Errorf("error = %v, want not supported on postgres", err)
		}
	})
	t.Run("unknown type", func(t *testing.T) {
		desired := []driver.Index{{Name: "weird", Type: "HASH", Columns: []driver.IndexColumn{{Name: "c"}}}}
		if _, err := buildAlterIndexes(buildSchema, "t", nil, desired); err == nil {
			t.Fatal("expected error for unknown index type, got nil")
		}
	})
}

// TestPgQuoteString pins PostgreSQL's literal quoting: only the single
// quote is special (doubled to ''); with standard_conforming_strings a
// backslash is an ordinary character and must NOT be escaped (the MySQL
// helper doubles it — PostgreSQL's must not).
func TestPgQuoteString(t *testing.T) {
	cases := map[string]string{
		"x":       "'x'",
		"":        "''",
		"it's":    "'it''s'",
		"a''b'c":  "'a''''b''c'",
		`back\x`:  `'back\x'`, // backslash stays literal
		`path\`:   `'path\'`,  // trailing backslash cannot swallow the quote
		`a\'b`:    `'a\''b'`,  // backslash untouched, quote doubled independently
		"multi\n": "'multi\n'",
	}
	for in, want := range cases {
		if got := sqlQuoteStringPg(in); got != want {
			t.Errorf("sqlQuoteStringPg(%q) = %s, want %s", in, got, want)
		}
	}
	if !strings.Contains(sqlQuoteStringPg("'"), "''") {
		t.Error("sqlQuoteStringPg must double inner single quotes")
	}
}
