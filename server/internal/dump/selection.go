package dump

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/hitslab/db-manager/server/internal/driver"
)

// WriteSelection streams the structured query in bounded batches. Limit zero
// exports all matching rows; a positive limit preserves a selected page/range.
func WriteSelection(ctx context.Context, table, format string, query driver.SelectQuery, out io.Writer,
	read func(context.Context, driver.SelectQuery) (driver.Page, error), quote func(string) string,
	literal func(any, string) string, postgres bool) error {
	if table == "" || query.Limit < 0 || query.Offset < 0 {
		return fmt.Errorf("invalid export selection")
	}
	var csv *CSVWriter
	if format != FormatSQL {
		sep, err := Separator(format)
		if err != nil {
			return err
		}
		csv = NewCSVWriter(out, sep)
	} else if literal == nil {
		return fmt.Errorf("SQL literal writer required")
	}
	remaining := query.Limit
	first := true
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		batch := 100
		if remaining > 0 && remaining < batch {
			batch = remaining
		}
		q := query
		q.Limit = batch
		page, err := read(ctx, q)
		if err != nil {
			return err
		}
		names := make([]string, len(page.Columns))
		for i, c := range page.Columns {
			names[i] = c.Name
		}
		if csv != nil && first {
			if err := csv.WriteHeader(names); err != nil {
				return err
			}
		}
		first = false
		for _, row := range page.Rows {
			if len(row) != len(page.Columns) {
				return fmt.Errorf("export row length does not match columns")
			}
			if csv != nil {
				values := row
				if postgres {
					values = append([]any(nil), row...)
					for i, v := range values {
						if v == nil {
							values[i] = `\N`
						} else if text, ok := v.(string); ok && strings.HasPrefix(text, `\`) {
							values[i] = `\` + text
						}
					}
				}
				if err := csv.WriteRow(values); err != nil {
					return err
				}
			} else {
				columns := make([]string, len(names))
				values := make([]string, len(row))
				for i, name := range names {
					columns[i] = quote(name)
					values[i] = literal(row[i], page.Columns[i].Type)
				}
				statement := "INSERT INTO " + quote(table) + " (" + strings.Join(columns, ",") + ") VALUES (" + strings.Join(values, ",") + ");\n"
				if _, err := io.WriteString(out, statement); err != nil {
					return err
				}
			}
		}
		if remaining > 0 {
			remaining -= len(page.Rows)
			if remaining <= 0 {
				return nil
			}
		}
		if len(page.Rows) < batch {
			return nil
		}
		query.Offset += len(page.Rows)
	}
}
