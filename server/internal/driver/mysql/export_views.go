package mysql

import (
	"context"
	"fmt"
	"io"

	"github.com/hitslab/db-manager/server/internal/driver"
	"github.com/hitslab/db-manager/server/internal/dump"
	"github.com/hitslab/db-manager/server/internal/sqlgen"
)

func (d *MysqlDriver) prepareViewExport(ctx context.Context, db string, selected []string) ([]string, []dump.View, error) {
	infos, err := d.ListTables(ctx, db)
	if err != nil {
		return nil, nil, err
	}
	catalog := map[string]bool{}
	for _, info := range infos {
		catalog[info.Name] = info.IsView
	}
	return dump.PrepareViews(catalog, selected, func(name string) (dump.View, error) {
		var query, check, security string
		if err := d.db.QueryRowContext(ctx, "SELECT VIEW_DEFINITION, CHECK_OPTION, SECURITY_TYPE FROM information_schema.VIEWS WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ?", db, name).Scan(&query, &check, &security); err != nil {
			return dump.View{}, fmt.Errorf("read view %s: %w", name, err)
		}
		portable, deps, err := dump.RewriteViewQuery(query, db, false)
		if err != nil {
			return dump.View{}, fmt.Errorf("export view %s: %w", name, err)
		}
		// CTE names are not catalog relations. Their actual source relations are
		// discovered in the nested FROM clauses and included independently.
		local := []string{}
		for _, dep := range deps {
			if _, ok := catalog[dep]; ok {
				local = append(local, dep)
			}
		}
		return dump.View{Name: name, Query: portable, Dependencies: local, SecurityInvoker: security == "INVOKER", CheckOption: check}, nil
	})
}
func writeViewsSQL(out io.Writer, views []dump.View, req driver.ExportRequest) error {
	// Remove dependent views before their dependencies when restoring in place.
	for _, view := range views {
		verb := "CREATE VIEW "
		if view.SecurityInvoker {
			verb = "CREATE SQL SECURITY INVOKER VIEW "
		}
		ddl := verb + sqlgen.QuoteIdent(view.Name) + " AS " + view.Query
		if view.CheckOption == "LOCAL" || view.CheckOption == "CASCADED" {
			ddl += " WITH " + view.CheckOption + " CHECK OPTION"
		}
		statement := ddl + ";\n"
		if req.IfNotExists {
			statement = "SET @DBM_VIEW_SQL=IF(EXISTS(SELECT 1 FROM information_schema.VIEWS WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME=" + sqlQuoteString(view.Name) + "), 'DO 0', " + sqlQuoteString(ddl) + ");\nPREPARE dbm_view FROM @DBM_VIEW_SQL;\nEXECUTE dbm_view;\nDEALLOCATE PREPARE dbm_view;\n"
		}
		if err := writeExport(out, statement); err != nil {
			return err
		}
	}

	return nil
}
func (d *MysqlDriver) generatedExportColumns(ctx context.Context, db, table string) (map[string]bool, error) {
	def, err := d.TableDef(ctx, db, table)
	if err != nil {
		return nil, err
	}
	generated := map[string]bool{}
	for _, col := range def.Columns {
		if col.Generated != "" {
			generated[col.Name] = true
		}
	}
	return generated, nil
}

func dropViewsSQL(out io.Writer, views []dump.View) error {
	{
		for i := len(views) - 1; i >= 0; i-- {
			if err := writeExport(out, "DROP VIEW IF EXISTS "+sqlgen.QuoteIdent(views[i].Name)+";\n"); err != nil {
				return err
			}
		}
	}
	return nil
}
