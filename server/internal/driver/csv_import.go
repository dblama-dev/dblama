package driver

import "fmt"

// CSVImportColumns resolves target columns and their source-field indexes.
// Omitted mapping retains ordinal behavior; an explicit mapping must select
// at least one unique, existing target. Empty entries consume ignored fields.
func CSVImportColumns(columns []ColumnMeta, mapping []string) ([]ColumnMeta, []int, error) {
	if mapping == nil {
		indexes := make([]int, len(columns))
		for i := range indexes {
			indexes[i] = i
		}
		return columns, indexes, nil
	}
	byName := make(map[string]ColumnMeta, len(columns))
	for _, c := range columns {
		byName[c.Name] = c
	}
	seen := map[string]bool{}
	var selected []ColumnMeta
	var indexes []int
	for i, name := range mapping {
		if name == "" {
			continue
		}
		c, ok := byName[name]
		if !ok {
			return nil, nil, fmt.Errorf("import csv: unknown target column %q", name)
		}
		if seen[name] {
			return nil, nil, fmt.Errorf("import csv: duplicate target column %q", name)
		}
		seen[name] = true
		selected = append(selected, c)
		indexes = append(indexes, i)
	}
	if len(selected) == 0 {
		return nil, nil, fmt.Errorf("import csv: mapping must select at least one target column")
	}
	return selected, indexes, nil
}
