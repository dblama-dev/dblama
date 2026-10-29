package api

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/hitslab/db-manager/server/internal/driver"
	"github.com/hitslab/db-manager/server/internal/session"
	"github.com/hitslab/db-manager/server/internal/sqlgen"
)

// importBodyLimit caps the whole multipart import request at 512 MiB
// (file plus form fields); it also limits the input read by streaming imports.
const importBodyLimit = 512 << 20

// exportFormats maps an ExportRequest.Format to its response content type
// and dump filename. The keys match the formats the driver accepts.
var exportFormats = map[string]struct {
	contentType string
	filename    string
}{
	"sql":           {"application/sql", "dump.sql"},
	"csv":           {"text/csv", "dump.csv"},
	"csv_semicolon": {"text/csv", "dump.csv"},
	"tsv":           {"text/tab-separated-values", "dump.tsv"},
}

// exportStreamWriter delays the success status until the driver writes its
// first byte. Preflight errors can then still become JSON errors. Every
// actual chunk is flushed to the client.
type exportStreamWriter struct {
	w       http.ResponseWriter
	started bool
}

func (ew *exportStreamWriter) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if !ew.started {
		ew.w.WriteHeader(http.StatusOK)
		ew.started = true
	}
	n, err := ew.w.Write(p)
	if flusher, ok := ew.w.(http.Flusher); ok {
		flusher.Flush()
	}
	return n, err
}

// handleExport honors the route database. PostgreSQL connections remain bound
// to their login database; MySQL catalog/data queries qualify the selected one.
func (s *apiServer) handleExport(w http.ResponseWriter, r *http.Request) {
	sess, _ := session.FromContext(r.Context())
	if sess.Driver.Info().Flavor == "postgres" && sess.Database == "" {
		WriteErr(w, http.StatusBadRequest, ErrBadRequest("export requires logging in with a database"))
		return
	}
	if urlDB := chi.URLParam(r, "db"); sess.Driver.Info().Flavor == "postgres" && urlDB != sess.Database {
		WriteErr(w, http.StatusBadRequest, ErrBadRequest(
			"export dumps the login database "+sess.Database+", not "+urlDB))
		return
	}

	var req driver.ExportRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.Mode != "" && req.Mode != "all" && req.Mode != "structure" && req.Mode != "data" {
		WriteErr(w, http.StatusBadRequest, ErrBadRequest("mode must be all, structure or data"))
		return
	}
	meta, ok := exportFormats[req.Format]
	if !ok {
		WriteErr(w, http.StatusBadRequest, ErrBadRequest("unsupported export format "+req.Format))
		return
	}
	// csv/tsv dumps are a single delimited file: exactly one table.
	if req.Format != "sql" && len(req.Tables) != 1 {
		WriteErr(w, http.StatusBadRequest, ErrBadRequest(
			"csv/tsv export requires exactly one table"))
		return
	}

	if req.Query != nil {
		if len(req.Tables) != 1 || req.Query.Offset < 0 || req.Query.Limit < 0 {
			WriteErr(w, 400, ErrBadRequest("selection export requires one table and nonnegative paging"))
			return
		}
		if _, _, err := sqlgen.BuildSelect(req.Tables[0], *req.Query); err != nil {
			WriteErr(w, 400, ErrBadRequest(err.Error()))
			return
		}
	}
	filename := meta.filename
	if req.Gzip {
		filename += ".gz"
	}
	w.Header().Set("Content-Type", meta.contentType)
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	out := &exportStreamWriter{w: w}
	if err := sess.Driver.Export(r.Context(), chi.URLParam(r, "db"), req, out); err != nil {
		if !out.started {
			w.Header().Del("Content-Disposition")
			writeDriverErr(w, err)
			return
		}
		panic(http.ErrAbortHandler)
	}
	if !out.started {
		w.WriteHeader(http.StatusOK)
	}
}

// importProgressLine is one JSON-lines record of the import stream: the
// running progress, the error record (with the error field) or the final
// success record (ok: true).
type importProgressLine struct {
	Done  int64  `json:"done"`
	Total int64  `json:"total"`
	OK    bool   `json:"ok,omitempty"`
	Error string `json:"error,omitempty"`
}

// handleImport runs an uploaded SQL script or CSV file through the
// driver's importer and streams its progress as application/x-ndjson:
// one {"done":N,"total":M} line per driver progress callback, then
// either {"done":..,"total":..,"error":"..."} (stream stops) or a final
// {"done":..,"total":..,"ok":true}. All validation happens before the
// headers are sent; gzip detection is the driver's job (magic bytes) —
// the body is handed over untouched.
func (s *apiServer) handleImport(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, importBodyLimit)
	mr, err := r.MultipartReader()
	if err != nil {
		WriteErr(w, http.StatusBadRequest, ErrBadRequest("multipart form data required"))
		return
	}

	// Fields must precede the file part (the frontend appends them in
	// that order): parts are streamed in order and the file part is
	// kept open for the driver, so anything after it is ignored.
	var (
		fileType, table, separator, enclosure string
		columns                               []string
		skipHeader                            bool
		file                                  io.Reader
	)
	for {
		p, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			WriteErr(w, http.StatusBadRequest, ErrBadRequest("malformed multipart body"))
			return
		}
		if p.FileName() != "" || p.FormName() == "file" {
			file = p
			break
		}
		val, err := io.ReadAll(io.LimitReader(p, 1<<20))
		if err != nil {
			WriteErr(w, http.StatusBadRequest, ErrBadRequest("malformed multipart body"))
			return
		}
		switch p.FormName() {
		case "type":
			fileType = string(val)
		case "table":
			table = string(val)
		case "separator":
			separator = string(val)
		case "enclosure":
			enclosure = string(val)
		case "columns":
			if err := json.Unmarshal(val, &columns); err != nil || columns == nil {
				WriteErr(w, http.StatusBadRequest, ErrBadRequest("columns must be a JSON array of target names"))
				return
			}
		case "skipHeader":
			skipHeader, err = strconv.ParseBool(string(val))
			if err != nil {
				WriteErr(w, http.StatusBadRequest, ErrBadRequest("invalid skipHeader value"))
				return
			}
		}
		_ = p.Close()
	}

	switch fileType {
	case "sql", "csv":
	default:
		WriteErr(w, http.StatusBadRequest, ErrBadRequest("type must be sql or csv"))
		return
	}
	if fileType == "csv" && table == "" {
		WriteErr(w, http.StatusBadRequest, ErrBadRequest("table is required for csv import"))
		return
	}
	if file == nil {
		WriteErr(w, http.StatusBadRequest, ErrBadRequest("file is required"))
		return
	}

	// HTTP/1 normally drains and closes the unread request body when the
	// response is flushed. SQL imports read and acknowledge one statement at
	// a time, so keep both directions open until the file part reaches EOF.
	_ = http.NewResponseController(w).EnableFullDuplex()

	sess, _ := session.FromContext(r.Context())
	w.Header().Set("Content-Type", "application/x-ndjson")
	w.WriteHeader(http.StatusOK)
	flusher, _ := w.(http.Flusher)

	var lastDone, lastTotal int64
	progress := func(done, total int64) {
		lastDone, lastTotal = done, total
		writeImportLine(w, importProgressLine{Done: done, Total: total})
		if flusher != nil {
			flusher.Flush()
		}
	}

	var impErr error
	if fileType == "sql" {
		impErr = sess.Driver.ImportSQL(r.Context(), chi.URLParam(r, "db"), file, progress)
	} else {
		opts := driver.CSVImportOpts{Separator: separator, Enclosure: enclosure, SkipHeader: skipHeader, Columns: columns}
		impErr = sess.Driver.ImportCSV(r.Context(), chi.URLParam(r, "db"), table, file, opts, progress)
	}
	if impErr != nil {
		writeImportLine(w, importProgressLine{Done: lastDone, Total: lastTotal, Error: impErr.Error()})
		return
	}
	writeImportLine(w, importProgressLine{Done: lastDone, Total: lastTotal, OK: true})
}

// writeImportLine encodes one NDJSON record; Encode appends the newline.
func writeImportLine(w io.Writer, line importProgressLine) {
	_ = json.NewEncoder(w).Encode(line)
}
