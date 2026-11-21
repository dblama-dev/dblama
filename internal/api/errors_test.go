package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestWriteErr_Envelope(t *testing.T) {
	rr := httptest.NewRecorder()
	WriteErr(rr, http.StatusUnauthorized, ErrAuthFailed("bad password"))

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
	if got := rr.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Errorf("Content-Type = %q, want %q", got, "application/json; charset=utf-8")
	}

	var body map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal body %q: %v", rr.Body.String(), err)
	}
	want := map[string]any{
		"error": map[string]any{
			"code":    "auth_failed",
			"message": "bad password",
		},
	}
	if !reflect.DeepEqual(body, want) {
		t.Errorf("body = %v, want %v", body, want)
	}
}

func TestWriteErr_OmitsEmptyFields(t *testing.T) {
	rr := httptest.NewRecorder()
	WriteErr(rr, http.StatusBadRequest, ErrBadRequest("invalid input"))

	var body map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal body %q: %v", rr.Body.String(), err)
	}
	inner, ok := body["error"].(map[string]any)
	if !ok {
		t.Fatalf("body[\"error\"] = %T, want map[string]any", body["error"])
	}
	for _, key := range []string{"detail", "sqlState", "driverCode"} {
		if _, present := inner[key]; present {
			t.Errorf("error.%q present in JSON, want omitted when empty", key)
		}
	}
}
