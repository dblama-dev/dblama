package postgres

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/dblama-dev/dblama/internal/driver"
	"github.com/dblama-dev/dblama/internal/dump"
	"github.com/dblama-dev/dblama/internal/sqlgen"
)

func (d *PostgresDriver) prepareViewExport(ctx context.Context, selected []string) ([]string, []dump.View, error) {
	infos, err := d.ListTables(ctx, d.database)
	if err != nil {
		return nil, nil, err
	}
	catalog := map[string]bool{}
	for _, info := range infos {
		catalog[info.Name] = info.IsView
	}
	schema := d.schemaFor(ctx)
	return dump.PrepareViews(catalog, selected, func(name string) (dump.View, error) {
		var oid int64
		var query, check string
		var invoker, barrier bool
		if err := d.db.QueryRowContext(ctx, `SELECT c.oid,pg_get_viewdef(c.oid,true),
 COALESCE(c.reloptions @> ARRAY['security_invoker=true'],false),
 COALESCE(c.reloptions @> ARRAY['security_barrier=true'],false),
 CASE WHEN c.reloptions @> ARRAY['check_option=local'] THEN 'LOCAL' WHEN c.reloptions @> ARRAY['check_option=cascaded'] THEN 'CASCADED' ELSE '' END FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname=$1 AND c.relname=$2 AND c.relkind='v'`, schema, name).Scan(&oid, &query, &invoker, &barrier, &check); err != nil {
			return dump.View{}, err
		}
		rows, err := d.db.QueryContext(ctx, `SELECT DISTINCT n.nspname,c.relname,c.relkind::text FROM pg_rewrite r JOIN pg_depend dep ON dep.classid='pg_rewrite'::regclass AND dep.objid=r.oid JOIN pg_class c ON dep.refclassid='pg_class'::regclass AND c.oid=dep.refobjid JOIN pg_namespace n ON n.oid=c.relnamespace WHERE r.ev_class=$1 AND c.oid<>$1`, oid)
		if err != nil {
			return dump.View{}, err
		}
		deps := []string{}
		for rows.Next() {
			var ns, relation, kind string
			if err := rows.Scan(&ns, &relation, &kind); err != nil {
				_ = rows.Close()
				return dump.View{}, err
			}
			if ns != schema || kind != "r" && kind != "p" && kind != "v" {
				_ = rows.Close()
				return dump.View{}, fmt.Errorf("view %s has unsupported external schema or sequence dependency %s.%s", name, ns, relation)
			}
			deps = append(deps, relation)
		}
		err = rows.Err()
		_ = rows.Close()
		if err != nil {
			return dump.View{}, err
		}
		var routines bool
		if err := d.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM pg_rewrite r JOIN pg_depend dep ON dep.classid='pg_rewrite'::regclass AND dep.objid=r.oid JOIN pg_proc p ON dep.refclassid='pg_proc'::regclass AND p.oid=dep.refobjid JOIN pg_namespace n ON n.oid=p.pronamespace WHERE r.ev_class=$1 AND n.nspname<>'pg_catalog')`, oid).Scan(&routines); err != nil {
			return dump.View{}, err
		}
		if routines {
			return dump.View{}, fmt.Errorf("view %s depends on a routine not included in the export", name)
		}
		portable, _, err := dump.RewriteViewQuery(query, schema, true)
		if err != nil {
			return dump.View{}, err
		}
		return dump.View{Name: name, Query: portable, Dependencies: deps, SecurityInvoker: invoker, SecurityBarrier: barrier, CheckOption: check}, nil
	})
}
func writeViewsSQL(out io.Writer, views []dump.View, req driver.ExportRequest) error {
	for _, view := range views {
		options := []string{}
		if view.SecurityInvoker {
			options = append(options, "security_invoker=true")
		}
		if view.SecurityBarrier {
			options = append(options, "security_barrier=true")
		}
		clause := ""
		if len(options) > 0 {
			clause = " WITH (" + strings.Join(options, ",") + ")"
		}
		ddl := "CREATE VIEW " + sqlgen.QuoteIdentPg(view.Name) + clause + " AS " + view.Query
		if view.CheckOption == "LOCAL" || view.CheckOption == "CASCADED" {
			ddl += " WITH " + view.CheckOption + " CHECK OPTION"
		}
		statement := ddl + ";\n"
		if req.IfNotExists {
			tag := "$dblama_view$"
			for i := 0; strings.Contains(ddl, tag); i++ {
				tag = fmt.Sprintf("$dblama_view_%d$", i)
			}
			statement = "DO " + tag + " BEGIN IF NOT EXISTS(SELECT 1 FROM pg_catalog.pg_class c JOIN pg_catalog.pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname=current_schema() AND c.relkind='v' AND c.relname=" + sqlQuoteStringPg(view.Name) + ") THEN EXECUTE " + sqlQuoteStringPg(ddl) + "; END IF; END " + tag + ";\n"
		}
		if err := writeExport(out, statement); err != nil {
			return err
		}
	}

	return nil
}
func (d *PostgresDriver) generatedExportColumns(ctx context.Context, table string) (map[string]bool, error) {
	cols, err := d.tableColumns(ctx, d.schemaFor(ctx), table)
	if err != nil {
		return nil, err
	}
	generated := map[string]bool{}
	for _, col := range cols {
		if col.Generated != "" {
			generated[col.Name] = true
		}
	}
	return generated, nil
}

func dropViewsSQL(out io.Writer, views []dump.View) error {
	{
		for i := len(views) - 1; i >= 0; i-- {
			if err := writeExport(out, "DROP VIEW IF EXISTS "+sqlgen.QuoteIdentPg(views[i].Name)+";\n"); err != nil {
				return err
			}
		}
	}
	return nil
}
