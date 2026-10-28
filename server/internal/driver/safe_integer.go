package driver

import "strconv"

// JSONSafeInteger keeps exact database integers outside JavaScript's safe
// integer range as decimal strings. Values inside the range retain their
// numeric JSON representation for existing clients.
func JSONSafeInteger(value any) any {
	const maxSafe = 9007199254740991
	switch n := value.(type) {
	case int64:
		if n > maxSafe || n < -maxSafe {
			return strconv.FormatInt(n, 10)
		}
	case uint64:
		if n > maxSafe {
			return strconv.FormatUint(n, 10)
		}
	}
	return value
}
