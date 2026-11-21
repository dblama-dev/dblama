package driver

import "testing"

func TestValidateObjectSource(t *testing.T) {
	for _, tc := range []struct {
		source, kind, schema, name string
		valid                      bool
	}{
		{`CREATE OR REPLACE VIEW "Odd Schema"."my view" AS SELECT 1`, "VIEW", "Odd Schema", "my view", true},
		{"CREATE DEFINER=`root`@`localhost` PROCEDURE `work`() BEGIN SELECT 1; END", "PROCEDURE", "test", "work", true},
		{`/* SQL source */ CREATE FUNCTION public.f() RETURNS int LANGUAGE sql AS $$ SELECT 1 $$`, "FUNCTION", "public", "f", true},
		{`CREATE VIEW other.v AS SELECT 1`, "VIEW", "public", "v", false},
		{`CREATE TABLE v(id int)`, "VIEW", "public", "v", false},
		{`CREATE VIEW wrong AS SELECT 1`, "VIEW", "public", "v", false},
		{`DROP VIEW v; CREATE VIEW v AS SELECT 1`, "VIEW", "public", "v", false},
		{`CREATE VIEW v AS SELECT 1`, "VIEW", "public", "", false},
	} {
		t.Run(tc.source, func(t *testing.T) {
			err := ValidateObjectSource(tc.source, tc.kind, tc.schema, tc.name)
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v err=%v", tc.valid, err)
			}
		})
	}
}
