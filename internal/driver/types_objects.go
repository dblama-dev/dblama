package driver

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode"
)

// ObjectInfo includes the server's executable CREATE source. Identity is the
// PostgreSQL overload identity (never concatenate a client-supplied signature).
type ObjectInfo struct {
	Name     string `json:"name"`
	Kind     string `json:"kind"`
	Table    string `json:"table,omitempty"`
	Identity string `json:"identity,omitempty"`
	Source   string `json:"source"`
	Status   string `json:"status,omitempty"`
}

// ObjectDef is explicitly edited SQL source and its object identity.
type ObjectDef struct {
	Source   string `json:"source"`
	Kind     string `json:"kind,omitempty"`
	Table    string `json:"table,omitempty"`
	Identity string `json:"identity,omitempty"`
}

// PartitionInfo describes one table partition and its data boundary.
type PartitionInfo struct {
	Name        string `json:"name"`
	Method      string `json:"method"`
	Expression  string `json:"expression"`
	Description string `json:"description"`
	Rows        int64  `json:"rows"`
}

// ViewManager is the optional view source CRUD capability.
type ViewManager interface {
	ListViews(context.Context, string) ([]ObjectInfo, error)
	AlterView(context.Context, string, string, ObjectDef) error
	DropView(context.Context, string, string) error
}

// TriggerManager is the optional trigger source CRUD capability.
type TriggerManager interface {
	ListTriggers(context.Context, string) ([]ObjectInfo, error)
	CreateTrigger(context.Context, string, string, ObjectDef) error
	DropTrigger(context.Context, string, string, string) error
}

// RoutineManager is the optional function/procedure source and CALL capability.
type RoutineManager interface {
	ListRoutines(context.Context, string) ([]ObjectInfo, error)
	CreateRoutine(context.Context, string, string, ObjectDef) error
	DropRoutine(context.Context, string, string, string, string) error
	CallProcedure(context.Context, string, string, string, []any) ([]Result, error)
}

// EventManager is the optional MySQL event source CRUD capability.
type EventManager interface {
	ListEvents(context.Context, string) ([]ObjectInfo, error)
	CreateEvent(context.Context, string, string, ObjectDef) error
	DropEvent(context.Context, string, string) error
}

// PartitionManager is the optional MySQL table partition capability.
type PartitionManager interface {
	ListPartitions(context.Context, string, string) ([]PartitionInfo, error)
	AddPartition(context.Context, string, string, string) error
	DropPartition(context.Context, string, string, string) error
	TruncatePartition(context.Context, string, string, string) error
}

// ErrObjectValidation identifies invalid structured object requests.
var ErrObjectValidation = errors.New("invalid object definition")

// ValidateObjectSource checks the CREATE header, leaving the SQL body to the
// server parser. Source editing intentionally has SQL console privileges.
// It is not a sandbox for arbitrary SQL or stored routine bodies.
func ValidateObjectSource(source, kind, schema, name string) error {
	fail := func() error { return fmt.Errorf("%w: expected CREATE %s for %s", ErrObjectValidation, kind, name) }
	if name == "" || strings.ContainsRune(name, 0) || strings.TrimSpace(source) == "" {
		return fail()
	}
	tokens, err := objectHeaderTokens(source)
	if err != nil {
		return fail()
	}
	i := 0
	next := func(w string) bool {
		if i < len(tokens) && strings.EqualFold(tokens[i], w) {
			i++
			return true
		}
		return false
	}
	if !next("CREATE") {
		return fail()
	}
	if next("OR") && !next("REPLACE") {
		return fail()
	}
	// MySQL SHOW CREATE can include ALGORITHM, DEFINER and SQL SECURITY.
	for i < len(tokens) && !strings.EqualFold(tokens[i], kind) {
		switch strings.ToUpper(tokens[i]) {
		case "ALGORITHM":
			i++
			if !next("=") || i >= len(tokens) {
				return fail()
			}
			i++
		case "DEFINER":
			i++
			if !next("=") || i >= len(tokens) {
				return fail()
			}
			i++
			if next("@") {
				if i >= len(tokens) {
					return fail()
				}
				i++
			}
		case "SQL":
			i++
			if !next("SECURITY") || i >= len(tokens) {
				return fail()
			}
			i++
		default:
			return fail()
		}
	}
	if !next(kind) || i >= len(tokens) {
		return fail()
	}
	target := tokens[i]
	i++
	if next(".") {
		if target != schema || i >= len(tokens) {
			return fail()
		}
		target = tokens[i]
	}
	if target != name {
		return fail()
	}
	return nil
}

func objectHeaderTokens(source string) ([]string, error) {
	tokens := make([]string, 0, 24)
	for i := 0; i < len(source) && len(tokens) < 40; {
		c := source[i]
		if unicode.IsSpace(rune(c)) {
			i++
			continue
		}
		if strings.HasPrefix(source[i:], "--") || c == '#' {
			j := strings.IndexByte(source[i:], '\n')
			if j < 0 {
				break
			}
			i += j + 1
			continue
		}
		if strings.HasPrefix(source[i:], "/*") {
			j := strings.Index(source[i+2:], "*/")
			if j < 0 {
				return nil, ErrObjectValidation
			}
			i += j + 4
			continue
		}
		if c == '`' || c == '"' || c == '\'' {
			quote := c
			i++
			var b strings.Builder
			closed := false
			for i < len(source) {
				if source[i] == quote {
					if i+1 < len(source) && source[i+1] == quote {
						b.WriteByte(quote)
						i += 2
						continue
					}
					i++
					closed = true
					break
				}
				b.WriteByte(source[i])
				i++
			}
			if !closed {
				return nil, ErrObjectValidation
			}
			tokens = append(tokens, b.String())
			continue
		}
		if strings.ContainsRune(".=@();", rune(c)) {
			tokens = append(tokens, string(c))
			i++
			continue
		}
		start := i
		for i < len(source) && !unicode.IsSpace(rune(source[i])) && !strings.ContainsRune(".=@();`\"'", rune(source[i])) {
			i++
		}
		tokens = append(tokens, source[start:i])
	}
	return tokens, nil
}
