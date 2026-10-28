package sqlgen

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

// StatementStream reads one SQL statement at a time. Memory is bounded by the
// largest statement (plus a 64 KiB input buffer), rather than the whole dump.
// It understands mysql-client DELIMITER lines, SQL quotes/comments, nested
// PostgreSQL block comments and dollar-quoted bodies. Incomplete final quoted
// regions/comments return an error and are never emitted for execution.
type StatementStream struct {
	postgres  bool
	reader    *bufio.Reader
	delimiter string
	offset    int64
	lineStart bool
	done      bool
}

// NewStatementStream creates a scanner using MySQL comment/string rules.
func NewStatementStream(r io.Reader) *StatementStream {
	return &StatementStream{reader: bufio.NewReaderSize(r, 64*1024), delimiter: ";", lineStart: true}
}

// NewPostgresStatementStream applies PostgreSQL comment and string rules:
// -- comments need no trailing space, # is an operator, and only E strings
// use backslash escapes. Dollar-quoted bodies are supported in either mode.
func NewPostgresStatementStream(r io.Reader) *StatementStream {
	s := NewStatementStream(r)
	s.postgres = true
	return s
}

// Offset counts consumed source bytes, excluding buffered read-ahead.
func (s *StatementStream) Offset() int64 { return s.offset }
func (s *StatementStream) read() (byte, error) {
	b, err := s.reader.ReadByte()
	if err == nil {
		s.offset++
	}
	return b, err
}
func (s *StatementStream) prefix(prefix string) bool {
	p, _ := s.reader.Peek(len(prefix))
	return string(p) == prefix
}
func (s *StatementStream) consume(n int, b *strings.Builder) error {
	for range n {
		c, err := s.read()
		if err != nil {
			return err
		}
		if b != nil {
			b.WriteByte(c)
		}
	}
	return nil
}
func streamIdentifier(c byte) bool {
	return c == '_' || c > 127 || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}

// Next returns the next complete statement or io.EOF when input is exhausted.
func (s *StatementStream) Next() (string, error) {
	if s.done {
		return "", io.EOF
	}
	var text strings.Builder
	hasSQL := false
	var quote byte
	escapeQuote := false
	lineComment := false
	blockDepth := 0
	dollar := ""
	var previous byte
	for {
		next, err := s.reader.Peek(1)
		if len(next) == 0 {
			if err != io.EOF {
				return "", err
			}
			s.done = true
			if quote != 0 || blockDepth != 0 || dollar != "" {
				return "", fmt.Errorf("unterminated SQL quote or comment at byte %d", s.offset)
			}
			if hasSQL {
				return strings.TrimSpace(text.String()), nil
			}
			return "", io.EOF
		}
		c := next[0]
		if quote != 0 {
			if err := s.consume(1, &text); err != nil {
				return "", err
			}
			if c == '\\' && escapeQuote {
				if err := s.consume(1, &text); err != nil {
					return "", fmt.Errorf("unterminated SQL escape at byte %d: %w", s.offset, err)
				}
			} else if c == quote {
				if s.prefix(string(quote)) {
					_ = s.consume(1, &text)
				} else {
					quote = 0
				}
			}
			previous = c
			continue
		}
		if dollar != "" {
			if s.prefix(dollar) {
				if err := s.consume(len(dollar), &text); err != nil {
					return "", err
				}
				dollar = ""
				previous = '$'
			} else {
				_ = s.consume(1, &text)
				previous = c
			}
			continue
		}
		if blockDepth > 0 {
			if s.prefix("/*") {
				blockDepth++
				_ = s.consume(2, &text)
			} else if s.prefix("*/") {
				blockDepth--
				_ = s.consume(2, &text)
			} else {
				_ = s.consume(1, &text)
			}
			s.lineStart = false
			previous = c
			continue
		}
		if lineComment {
			_ = s.consume(1, &text)
			if c == '\n' {
				lineComment = false
				s.lineStart = true
			}
			previous = c
			continue
		}
		if c == '\n' {
			_ = s.consume(1, &text)
			s.lineStart = true
			previous = c
			continue
		}
		if s.lineStart && (c == ' ' || c == '\t' || c == '\r') {
			_ = s.consume(1, &text)
			previous = c
			continue
		}
		if s.lineStart {
			p, peekErr := s.reader.Peek(len("delimiter") + 1)
			isDirective := len(p) >= 9 && strings.EqualFold(string(p[:9]), "delimiter") && (len(p) == 9 && peekErr == io.EOF || len(p) > 9 && isSpaceByte(p[9]))
			if isDirective {
				var line strings.Builder
				for {
					b, readErr := s.read()
					if readErr == io.EOF {
						break
					}
					if readErr != nil {
						return "", readErr
					}
					line.WriteByte(b)
					if b == '\n' {
						break
					}
				}
				token, _, _ := delimiterDirective(line.String(), 0)
				if token != "" {
					if len(token) > 64*1024 {
						return "", fmt.Errorf("SQL delimiter exceeds 64 KiB")
					}
					s.delimiter = token
				}
				s.lineStart = true
				previous = '\n'
				if hasSQL {
					return strings.TrimSpace(text.String()), nil
				}
				text.Reset()
				continue
			}
			s.lineStart = false
		}
		// Match terminators before interpreting '$' as a dollar-quote opener or
		// '/' as a block comment, preserving DELIMITER $$ and DELIMITER //.
		if s.prefix(s.delimiter) {
			if err := s.consume(len(s.delimiter), nil); err != nil {
				return "", err
			}
			s.lineStart = false
			previous = 0
			if hasSQL {
				return strings.TrimSpace(text.String()), nil
			}
			text.Reset()
			continue
		}
		switch c {
		case '\'', '"', '`':
			quote = c
			escapeQuote = !s.postgres && c != '`' || s.postgres && c == '\'' && (previous == 'E' || previous == 'e')
			hasSQL = true
			_ = s.consume(1, &text)
		case '#':
			if s.postgres {
				hasSQL = true
				_ = s.consume(1, &text)
				break
			}
			lineComment = true
			_ = s.consume(1, &text)
		case '-':
			p, _ := s.reader.Peek(3)
			if len(p) >= 2 && p[1] == '-' && (s.postgres || len(p) == 2 || isSpaceByte(p[2])) {
				lineComment = true
				_ = s.consume(2, &text)
			} else {
				hasSQL = true
				_ = s.consume(1, &text)
			}
		case '/':
			if s.prefix("/*") {
				if s.prefix("/*!") {
					hasSQL = true
				}
				blockDepth = 1
				_ = s.consume(2, &text)
			} else {
				hasSQL = true
				_ = s.consume(1, &text)
			}
		case '$':
			hasSQL = true
			_ = s.consume(1, &text)
			if !streamIdentifier(previous) && previous != '$' {
				var tag strings.Builder
				tag.WriteByte('$')
				for {
					p, _ := s.reader.Peek(1)
					if len(p) == 0 {
						break
					}
					b := p[0]
					if b == '$' {
						_ = s.consume(1, &text)
						tag.WriteByte('$')
						dollar = tag.String()
						break
					}
					if !streamIdentifier(b) || (tag.Len() == 1 && b >= '0' && b <= '9') {
						break
					}
					_ = s.consume(1, &text)
					tag.WriteByte(b)
				}
			}
		default:
			if !isSpaceByte(c) {
				hasSQL = true
			}
			_ = s.consume(1, &text)
		}
		previous = c
	}
}
