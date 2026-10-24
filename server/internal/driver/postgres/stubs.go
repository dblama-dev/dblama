package postgres

import (
	"context"
	"fmt"
	"io"

	"github.com/hitslab/db-manager/server/internal/driver"
)

// This file holds the not-yet-implemented half of the Driver contract so
// the package compiles against the full interface from day one. Every
// method below fails with a "not implemented yet" error naming itself;
// tasks 2–5 of the phase 4 plan replace them one group at a time: tables
// and structure (task 2), SaveTable/SaveIndexes (task 3), row data
// (task 4), console and dumps (task 5) — only task 5 remains. When the
// last stub leaves this file, the file goes too.

// notImplemented builds the placeholder error every stub returns; the
// method name names the missing piece.
func notImplemented(method string) error {
	return fmt.Errorf("postgres: %s not implemented yet", method)
}

// ShowCreateTable renders a table's CREATE statement (task 5).
func (d *PostgresDriver) ShowCreateTable(context.Context, string, string) (string, error) {
	return "", notImplemented("ShowCreateTable")
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
