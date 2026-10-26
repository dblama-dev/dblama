package postgres

import "fmt"

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

