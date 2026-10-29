package postgres

import (
	"github.com/hitslab/db-manager/server/internal/driver"
	"reflect"
	"testing"
)

func TestOrderDataTablesDependenciesAndCycles(t *testing.T) {
	def := func(parents ...string) driver.TableDef {
		d := driver.TableDef{}
		for _, parent := range parents {
			d.ForeignKeys = append(d.ForeignKeys, driver.ForeignKey{RefTable: parent})
		}
		return d
	}
	for _, tc := range []struct {
		name   string
		tables []string
		defs   map[string]driver.TableDef
		want   []string
		fail   bool
	}{
		{name: "parents before child and shared ancestor once", tables: []string{"a_child"}, defs: map[string]driver.TableDef{"a_child": def("z_parent", "m_parent"), "m_parent": def("z_parent"), "z_parent": def()}, want: []string{"z_parent", "m_parent", "a_child"}},
		{name: "cycle", tables: []string{"a"}, defs: map[string]driver.TableDef{"a": def("b"), "b": def("a")}, fail: true},
		{name: "self reference", tables: []string{"a"}, defs: map[string]driver.TableDef{"a": def("a")}, fail: true},
		{name: "missing dependency", tables: []string{"a"}, defs: map[string]driver.TableDef{"a": def("missing")}, fail: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := orderDataTables(tc.tables, tc.defs)
			if tc.fail {
				if err == nil {
					t.Fatal("expected rejection")
				}
				return
			}
			if err != nil || !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("order=%v err=%v", got, err)
			}
		})
	}
}
