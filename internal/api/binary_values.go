package api

import (
	"encoding/hex"
	"net/http"
	"strings"

	"github.com/dblama-dev/dblama/internal/driver"
	"github.com/dblama-dev/dblama/internal/session"
)

// decodeBinaryValues accepts an explicit transport wrapper only for binary
// columns. Ordinary strings, including strings beginning with 0x, stay text.
func decodeBinaryValues(w http.ResponseWriter, r *http.Request, rows ...driver.RowValues) bool {
	var wrapped bool
	for _, row := range rows {
		for _, value := range row {
			if object, ok := value.(map[string]any); ok {
				if _, present := object["$binaryHex"]; present {
					wrapped = true
				}
			}
		}
	}
	if !wrapped {
		return true
	}
	sess, _ := session.FromContext(r.Context())
	def, err := sess.Driver.TableDef(r.Context(), pathParam(r, "db"), pathParam(r, "table"))
	if err != nil {
		writeDriverErr(w, err)
		return false
	}
	binaryColumns := make(map[string]bool, len(def.Columns))
	for _, col := range def.Columns {
		typ := strings.ToLower(col.Type)
		binaryColumns[col.Name] = strings.Contains(typ, "binary") || strings.Contains(typ, "blob") || typ == "bytea"
	}
	for _, row := range rows {
		for name, value := range row {
			object, ok := value.(map[string]any)
			if !ok {
				continue
			}
			raw, present := object["$binaryHex"]
			if !present {
				continue
			}
			encoded, valid := raw.(string)
			if !binaryColumns[name] || !valid || len(object) != 1 {
				WriteErr(w, http.StatusBadRequest, ErrBadRequest("invalid binary value for "+name))
				return false
			}
			decoded, err := hex.DecodeString(encoded)
			if err != nil {
				WriteErr(w, http.StatusBadRequest, ErrBadRequest("invalid binary value for "+name))
				return false
			}
			row[name] = decoded
		}
	}
	return true
}
