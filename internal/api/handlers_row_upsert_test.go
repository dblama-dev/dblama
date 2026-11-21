package api

import (
	"context"
	"github.com/dblama-dev/dblama/internal/driver"
	"net/http"
	"strings"
	"testing"
)

type upsertDriver struct {
	*stubDriver
	rows []driver.RowValues
}

func (d *upsertDriver) UpsertRows(_ context.Context, _, _ string, rows []driver.RowValues) error {
	d.rows = rows
	return nil
}
func TestRowUpsert(t *testing.T) {
	d := &upsertDriver{stubDriver: &stubDriver{caps: driver.Caps{Upsert: true}}}
	_, c, base := loginStubServer(t, d)
	req, _ := http.NewRequest(http.MethodPost, base+"/api/databases/appdb/tables/t/upsert", strings.NewReader(`{"rows":[{"id":1,"name":null}]}`))
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != 204 || len(d.rows) != 1 || d.rows[0]["name"] != nil {
		t.Fatalf("status %d rows %#v", resp.StatusCode, d.rows)
	}
}
