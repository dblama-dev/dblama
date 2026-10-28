package api

import (
	"bytes"
	"github.com/hitslab/db-manager/server/internal/driver"
	"net/http"
	"testing"
)

func TestRowBinaryHexTransport(t *testing.T) {
	d := &stubDriver{tableDef: driver.TableDef{Columns: []driver.ColumnDef{{Name: "blob", Type: "varbinary"}, {Name: "text", Type: "text"}}}}
	_, client, base := loginStubServer(t, d)
	response := apiRequest(t, client, http.MethodPost, base+"/api/databases/appdb/tables/t/rows", `{"blob":{"$binaryHex":"00ff"},"text":"0x00ff"}`, "")
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != 200 {
		t.Fatalf("status %d", response.StatusCode)
	}
	got, ok := d.insertCalls[0].vals["blob"].([]byte)
	if !ok || !bytes.Equal(got, []byte{0, 255}) {
		t.Fatalf("binary %#v", d.insertCalls[0].vals)
	}
	if d.insertCalls[0].vals["text"] != "0x00ff" {
		t.Fatal("text reinterpreted")
	}
}

func TestRowBinaryHexTransportRejectsInvalidWrappers(t *testing.T) {
	for _, tc := range []struct{ name, body string }{
		{"odd hex", `{"blob":{"$binaryHex":"abc"}}`},
		{"wrong column", `{"text":{"$binaryHex":"00ff"}}`},
		{"extra member", `{"blob":{"$binaryHex":"00ff","other":true}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := &stubDriver{tableDef: driver.TableDef{Columns: []driver.ColumnDef{{Name: "blob", Type: "varbinary"}, {Name: "text", Type: "text"}}}}
			_, client, base := loginStubServer(t, d)
			response := apiRequest(t, client, http.MethodPost, base+"/api/databases/appdb/tables/t/rows", tc.body, "")
			defer func() { _ = response.Body.Close() }()
			if response.StatusCode != http.StatusBadRequest {
				t.Fatalf("status %d", response.StatusCode)
			}
			if len(d.insertCalls) != 0 {
				t.Fatalf("unexpected insert %#v", d.insertCalls)
			}
		})
	}
}
