package driver

import "context"

// RowCopier inserts copies of matching source rows, excluding generated IDs.
type RowCopier interface {
	CopyRows(context.Context, string, string, []PK) error
}
