package sqlgen

import (
	"strings"
)

// SplitStatements splits a multi-statement SQL script on statement
// terminators, following mysql-client semantics:
//
//   - the default terminator is ';', but a line whose first word is
//     DELIMITER (case-insensitive) switches the terminator to the
//     whitespace-trimmed rest of that line (one pair of surrounding
//     single, double or backtick quotes is stripped); the DELIMITER
//     line itself is never a statement, and everything until the next
//     directive is split by the new terminator; a directive with no
//     usable token keeps the current terminator; any text after the
//     token on the directive line is ignored; statement text pending
//     before a directive is emitted first;
//   - a terminator only splits outside single-quoted and double-quoted
//     strings (backslash and doubled-quote escapes), backtick
//     identifiers, '--' line comments (which require trailing
//     whitespace or end of line, so "1--2" is subtraction), '#' line
//     comments and '/* */' block comments. A terminator starting with
//     '/' (the mysql client's customary "//") splits anyway: the '/'
//     byte checks the terminator before the block-comment rule, so
//     only a terminator that literally starts "/*" would be shadowed
//     by comment regions. Terminators whose first byte is a quote stay
//     shadowed by string regions and are not reliable;
//   - pinned: '/*! ... */' executable comments are treated exactly
//     like '/* ... */' block comments — unlike the mysql client, their
//     content never toggles splitting (the statement text, marker
//     included, is sent to the server verbatim);
//   - a DELIMITER directive is recognized only as the first word of a
//     line (up to leading whitespace) and only outside strings and
//     comments;
//   - statements are whitespace-trimmed; blank statements are dropped;
//     trailing text without a terminator is emitted as the final
//     statement; an input with no statements yields nil.
//
// Splitting is purely lexical: it does not validate SQL and never
// alters statement text.
func SplitStatements(sqlText string) []string {
	var statements []string
	delim := ";"
	start := 0
	i := 0
	// lineStart tracks whether only whitespace has been seen since the
	// last '\n'; a DELIMITER directive is only recognized there. Any
	// region that ends mid-line (string, identifier, block comment,
	// terminator) re-enters the line with non-whitespace behind it, so
	// it clears the flag.
	lineStart := true
	for i < len(sqlText) {
		c := sqlText[i]
		if c == '\n' {
			i++
			lineStart = true
			continue
		}
		if lineStart && (c == ' ' || c == '\t' || c == '\r') {
			i++
			continue
		}
		if lineStart {
			if token, ok, end := delimiterDirective(sqlText, i); ok {
				// Statement text pending before the directive is
				// emitted first; the directive line itself is skipped.
				if stmt := strings.TrimSpace(sqlText[start:i]); stmt != "" {
					statements = append(statements, stmt)
				}
				if token != "" {
					delim = token
				}
				i = end
				start = end
				continue
			}
			lineStart = false
		}
		switch c {
		case '\'':
			i = scanQuoted(sqlText, i, '\'', true)
			lineStart = false
		case '"':
			i = scanQuoted(sqlText, i, '"', true)
			lineStart = false
		case '`':
			i = scanQuoted(sqlText, i, '`', false)
			lineStart = false
		case '#':
			// Stops at '\n'; the loop head re-arms lineStart.
			i = scanLineComment(sqlText, i)
		case '-':
			// MySQL: "--" starts a comment only when followed by
			// whitespace or end of input ("1--2" is subtraction).
			if i+1 < len(sqlText) && sqlText[i+1] == '-' && (i+2 == len(sqlText) || isSpaceByte(sqlText[i+2])) {
				i = scanLineComment(sqlText, i)
			} else {
				i++
			}
		case '/':
			// A '/'-leading terminator (the mysql client's customary
			// "//") must win over the block-comment rule, so the
			// terminator check comes first; "/*" still opens a block
			// comment unless the terminator itself starts with "/*".
			if strings.HasPrefix(sqlText[i:], delim) {
				if stmt := strings.TrimSpace(sqlText[start:i]); stmt != "" {
					statements = append(statements, stmt)
				}
				i += len(delim)
				start = i
				lineStart = false
				continue
			}
			// '/*! ... */' executable comments are pinned to split
			// exactly like plain '/* ... */' block comments.
			if i+1 < len(sqlText) && sqlText[i+1] == '*' {
				i = scanBlockComment(sqlText, i)
				lineStart = false
			} else {
				i++
			}
		default:
			if strings.HasPrefix(sqlText[i:], delim) {
				if stmt := strings.TrimSpace(sqlText[start:i]); stmt != "" {
					statements = append(statements, stmt)
				}
				i += len(delim)
				start = i
				lineStart = false
				continue
			}
			i++
		}
	}
	if stmt := strings.TrimSpace(sqlText[start:]); stmt != "" {
		statements = append(statements, stmt)
	}
	return statements
}

// delimiterDirective reports whether a DELIMITER directive starts at i
// (the caller guarantees sqlText[i] is the first non-whitespace byte of
// its line). It returns the new terminator token — empty when the line
// has no usable token, in which case the current terminator stays —
// and end, the index of the directive line's terminating '\n' (or
// len(sqlText)). ok is false when the line's first word is not a
// case-insensitive "delimiter" followed by whitespace.
func delimiterDirective(sqlText string, i int) (token string, ok bool, end int) {
	const keyword = "delimiter"
	rest := sqlText[i:]
	if len(rest) < len(keyword) || !strings.EqualFold(rest[:len(keyword)], keyword) {
		return "", false, i
	}
	after := rest[len(keyword):]
	if after != "" && !isSpaceByte(after[0]) {
		return "", false, i
	}
	end = len(sqlText)
	if nl := strings.IndexByte(after, '\n'); nl >= 0 {
		end = i + len(keyword) + nl
		after = after[:nl]
	}
	fields := strings.Fields(after)
	if len(fields) == 0 {
		return "", true, end
	}
	token = fields[0]
	// The mysql client accepts the token quoted; strip one pair of
	// matching quotes.
	if len(token) >= 2 {
		if q := token[0]; (q == '\'' || q == '"' || q == '`') && token[len(token)-1] == q {
			token = token[1 : len(token)-1]
		}
	}
	return token, true, end
}

// scanQuoted returns the index just past the quoted region opening at
// i (which holds the opening quote). backslash enables the backslash
// escapes MySQL applies inside '...' and "..." strings; inside
// backtick identifiers a backslash is literal. A doubled quote is an
// escaped quote and stays inside the region. An unterminated region
// runs to end of input.
func scanQuoted(q string, i int, quote byte, backslash bool) int {
	i++
	for i < len(q) {
		switch q[i] {
		case '\\':
			if backslash {
				i += 2
				continue
			}
		case quote:
			if i+1 < len(q) && q[i+1] == quote {
				i += 2
				continue
			}
			return i + 1
		}
		i++
	}
	return i
}

// scanLineComment returns the index of the terminating '\n' (which the
// main loop then processes as ordinary text) or len(q).
func scanLineComment(q string, i int) int {
	for i < len(q) && q[i] != '\n' {
		i++
	}
	return i
}

// scanBlockComment returns the index just past the "*/" closing the
// block comment opening at i, or len(q) when unterminated.
func scanBlockComment(q string, i int) int {
	i += 2
	for i+1 < len(q) && (q[i] != '*' || q[i+1] != '/') {
		i++
	}
	if i+1 < len(q) {
		return i + 2
	}
	return len(q)
}

func isSpaceByte(c byte) bool {
	switch c {
	case ' ', '\t', '\n', '\r':
		return true
	}
	return false
}
