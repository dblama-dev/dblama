package postgres

import (
	"context"
	"fmt"
	"io"

	"github.com/hitslab/db-manager/server/internal/driver"
	"github.com/hitslab/db-manager/server/internal/sqlgen"
)

// This file holds the not-yet-implemented half of the Driver contract so
// the package compiles against the full interface from day one. Every
// method below fails with a "not implemented yet" error naming itself;
// tasks 2–5 of the phase 4 plan replace them one group at a time: tables
// and structure (task 2), SaveTable/SaveIndexes (task 3), row data
// (task 4), console and dumps (task 5). When the last stub leaves this
// file, the file goes too.

// notImplemented builds the placeholder error every stub returns; the
// method name names the missing piece.
func notImplemented(method string) error {
	return fmt.Errorf("postgres: %s not implemented yet", method)
}

// ListTables lists the tables and views of the current schema (task 2).
func (d *PostgresDriver) ListTables(context.Context, string) ([]driver.TableInfo, error) {
	return nil, notImplemented("ListTables")
}

// TableDef reads one table's full structure (task 2).
func (d *PostgresDriver) TableDef(context.Context, string, string) (driver.TableDef, error) {
	return driver.TableDef{}, notImplemented("TableDef")
}

// ShowCreateTable renders a table's CREATE statement (task 5).
func (d *PostgresDriver) ShowCreateTable(context.Context, string, string) (string, error) {
	return "", notImplemented("ShowCreateTable")
}

// SaveTable creates or alters a table (task 3).
func (d *PostgresDriver) SaveTable(context.Context, string, string, driver.TableDef) error {
	return notImplemented("SaveTable")
}

// SaveIndexes applies a table's desired index state (task 3).
func (d *PostgresDriver) SaveIndexes(context.Context, string, string, []driver.Index) error {
	return notImplemented("SaveIndexes")
}

// TruncateTables empties the named tables in order (task 3).
func (d *PostgresDriver) TruncateTables(context.Context, string, []string) error {
	return notImplemented("TruncateTables")
}

// DropTables drops the named tables (task 3).
func (d *PostgresDriver) DropTables(context.Context, string, []string) error {
	return notImplemented("DropTables")
}

// DropViews drops the named views (task 3).
func (d *PostgresDriver) DropViews(context.Context, string, []string) error {
	return notImplemented("DropViews")
}

// SelectRows returns one browsed page of a table (task 4).
func (d *PostgresDriver) SelectRows(context.Context, string, string, sqlgen.SelectQuery) (driver.Page, error) {
	return driver.Page{}, notImplemented("SelectRows")
}

// PrimaryKey returns a table's primary key column names (task 2).
func (d *PostgresDriver) PrimaryKey(context.Context, string, string) ([]string, error) {
	return nil, notImplemented("PrimaryKey")
}

// InsertRow inserts one row (task 4).
func (d *PostgresDriver) InsertRow(context.Context, string, string, driver.RowValues) error {
	return notImplemented("InsertRow")
}

// UpdateRow updates the rows matching where (task 4).
func (d *PostgresDriver) UpdateRow(context.Context, string, string, driver.PK, driver.RowValues) error {
	return notImplemented("UpdateRow")
}

// DeleteRows deletes one row per key in one transaction (task 4).
func (d *PostgresDriver) DeleteRows(context.Context, string, string, []driver.PK) error {
	return notImplemented("DeleteRows")
}

// UpsertRows inserts or updates on conflict (task 4).
func (d *PostgresDriver) UpsertRows(context.Context, string, string, []driver.RowValues) error {
	return notImplemented("UpsertRows")
}

// LastInsertID returns the id of the most recent insert (task 4).
func (d *PostgresDriver) LastInsertID(context.Context) (int64, error) {
	return 0, notImplemented("LastInsertID")
}

// ExecSQL runs a console batch statement by statement (task 5).
func (d *PostgresDriver) ExecSQL(context.Context, string, string, string, driver.ExecOpts) ([]driver.Result, error) {
	return nil, notImplemented("ExecSQL")
}

// CancelQuery cancels the query registered under queryID (task 5).
func (d *PostgresDriver) CancelQuery(context.Context, string) error {
	return notImplemented("CancelQuery")
}

// Export streams a dump of the login database (task 5).
func (d *PostgresDriver) Export(context.Context, driver.ExportRequest, io.Writer) error {
	return notImplemented("Export")
}

// ImportSQL executes an SQL script against db (task 5).
func (d *PostgresDriver) ImportSQL(context.Context, string, io.Reader, func(done, total int64)) error {
	return notImplemented("ImportSQL")
}

// ImportCSV imports delimited rows into a table (task 5).
func (d *PostgresDriver) ImportCSV(context.Context, string, string, io.Reader, driver.CSVImportOpts, func(done, total int64)) error {
	return notImplemented("ImportCSV")
}
