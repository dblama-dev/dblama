package postgres

import (
	"context"
	"encoding/json"
	"github.com/dblama-dev/dblama/internal/driver"
	"reflect"
	"strings"
	"testing"
)

func TestRequestSchemaIsolation(t *testing.T) {
	d := &PostgresDriver{schema: "public"}
	a := driver.WithSchema(context.Background(), "Odd Schema")
	b := driver.WithSchema(context.Background(), "second")
	if d.schemaFor(a) != "Odd Schema" || d.schemaFor(b) != "second" {
		t.Fatal("request schema lost")
	}
	if d.CurrentSchema() != "public" {
		t.Fatal("request mutated session schema")
	}
	if d.schemaFor(driver.WithSchema(context.Background(), "")) != "public" {
		t.Fatal("default schema must be public")
	}
}

func TestArrayRepresentationRoundTrip(t *testing.T) {
	for _, value := range []string{`["NULL",null,"", "a\\b"]`, `[9007199254740993,-9007199254740993]`, `[["a","b"],["c","d"]]`} {
		literal, ok := jsonToPgArray(value)
		if !ok {
			t.Fatal(value)
		}
		typ := "text"
		if strings.Contains(value, "9007199") {
			typ = "bigint"
		}
		actual := pgArrayToJSON(literal, typ)
		var want, got any
		a := json.NewDecoder(strings.NewReader(value))
		a.UseNumber()
		_ = a.Decode(&want)
		b := json.NewDecoder(strings.NewReader(actual))
		b.UseNumber()
		_ = b.Decode(&got)
		if !reflect.DeepEqual(want, got) {
			t.Errorf("%s -> %s -> %s", value, literal, actual)
		}
	}
}
