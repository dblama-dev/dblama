package driver

import "testing"

func TestValidateForeignKeys(t *testing.T) {
	good := ForeignKey{Name: "parent fk", Columns: []string{"tenant", "parent"}, RefSchema: "Odd Schema", RefTable: "Parent Table", RefColumns: []string{"tenant", "id"}, OnDelete: "CASCADE", OnUpdate: "NO ACTION"}
	if err := ValidateForeignKeys([]ForeignKey{good}, true); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*ForeignKey){
		func(f *ForeignKey) { f.Name = "" }, func(f *ForeignKey) { f.RefTable = "" }, func(f *ForeignKey) { f.Columns = []string{} }, func(f *ForeignKey) { f.RefColumns = []string{"id"} }, func(f *ForeignKey) { f.OnDelete = "CASCADE; DROP TABLE t" }, func(f *ForeignKey) { f.Columns = []string{"id", "id"} }, func(f *ForeignKey) { f.RefColumns = []string{"a", ""} },
	} {
		bad := good
		change(&bad)
		if err := ValidateForeignKeys([]ForeignKey{bad}, true); err == nil {
			t.Fatalf("accepted %#v", bad)
		}
	}
	if err := ValidateForeignKeys([]ForeignKey{good, good}, true); err == nil {
		t.Fatal("accepted duplicate constraint names")
	}
	good.Name = ""
	if err := ValidateForeignKeys([]ForeignKey{good}, false); err != nil {
		t.Fatal("CREATE must allow generated constraint name", err)
	}
}
