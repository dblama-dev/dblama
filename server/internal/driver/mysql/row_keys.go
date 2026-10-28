package mysql

import (
	"context"
	"encoding/hex"
	"fmt"
	"github.com/hitslab/db-manager/server/internal/driver"
	"sort"
	"strings"
	"time"
)

// normalizeOriginalKeys reverses only read-path encodings used for WHERE keys.
// Assignment values keep their existing API semantics (text remains text).
func (d *MysqlDriver) normalizeOriginalKeys(ctx context.Context, db, table string, keys []driver.PK) ([]driver.PK, error) {
	requested := map[string]bool{}
	for _, key := range keys {
		if len(key) == 0 {
			return nil, fmt.Errorf("original row key must not be empty")
		}
		for name := range key {
			requested[name] = true
		}
	}
	names := make([]string, 0, len(requested))
	for name := range requested {
		names = append(names, name)
	}
	sort.Strings(names)
	columns, err := d.selectColumns(ctx, db, table, names)
	if err != nil {
		return nil, err
	}
	types := map[string]string{}
	for _, col := range columns {
		types[col.Name] = col.Type
	}
	out := make([]driver.PK, len(keys))
	for i, key := range keys {
		out[i] = driver.PK{}
		for name, value := range key {
			normalized, err := originalKeyValue(types[name], value)
			if err != nil {
				return nil, fmt.Errorf("original key column %s: %w", name, err)
			}
			if text, ok := normalized.(string); ok && strings.EqualFold(types[name], "json") {
				normalized = originalJSONKey(text)
			}
			out[i][name] = normalized
		}
	}
	return out, nil
}
func originalKeyValue(typ string, value any) (any, error) {
	text, ok := value.(string)
	if !ok {
		return value, nil
	}
	if isBinaryTypeName(typ) && strings.HasPrefix(text, "0x") {
		data, err := hex.DecodeString(text[2:])
		if err != nil {
			return nil, fmt.Errorf("invalid binary hex key")
		}
		return data, nil
	}
	switch strings.ToLower(typ) {
	case "date", "datetime", "timestamp":
		parsed, err := time.Parse(time.RFC3339Nano, text)
		if err == nil {
			if strings.EqualFold(typ, "date") {
				return parsed.Format("2006-01-02"), nil
			}
			return parsed.Format("2006-01-02 15:04:05.999999"), nil
		}
	}
	return value, nil
}

type originalJSONKey string

// bindOriginalJSONKeys converts typed original JSON key predicates produced by
// sqlgen. It walks placeholders outside quoted identifiers, so a '?' or WHERE
// inside an identifier cannot shift the argument positions or rewrite a SET.
// SQL text comes only from our structured builders; values remain bound.
func bindOriginalJSONKeys(query string, args []any) (string, []any, error) {
	var out strings.Builder
	copied, argIndex, lastIdentStart, lastIdentEnd := 0, 0, -1, -1
	for i := 0; i < len(query); i++ {
		if query[i] == '`' {
			start := i
			i++
			for i < len(query) {
				if query[i] == '`' {
					if i+1 < len(query) && query[i+1] == '`' {
						i += 2
						continue
					}
					break
				}
				i++
			}
			if i >= len(query) {
				return "", nil, fmt.Errorf("unterminated generated identifier")
			}
			lastIdentStart = start
			lastIdentEnd = i + 1
			continue
		}
		if query[i] != '?' {
			continue
		}
		if argIndex >= len(args) {
			return "", nil, fmt.Errorf("generated placeholder mismatch")
		}
		if value, ok := args[argIndex].(originalJSONKey); ok {
			if lastIdentStart < copied || lastIdentEnd < 0 || strings.TrimSpace(query[lastIdentEnd:i]) != "=" {
				return "", nil, fmt.Errorf("invalid generated JSON key predicate")
			}
			out.WriteString(query[copied:lastIdentStart])
			out.WriteString("JSON_EXTRACT(")
			out.WriteString(query[lastIdentStart:lastIdentEnd])
			out.WriteString(", '$') = JSON_EXTRACT(?, '$')")
			copied = i + 1
			args[argIndex] = string(value)
		}
		argIndex++
	}
	if argIndex != len(args) {
		return "", nil, fmt.Errorf("generated argument mismatch")
	}
	out.WriteString(query[copied:])
	return out.String(), args, nil
}
