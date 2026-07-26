package strs

import "strconv"

// Atoi64 parses a base-10 string into int64. Mirror of strconv.Atoi for int.
func Atoi64(s string) (int64, error) {
	return strconv.ParseInt(s, 10, 64)
}

// AsIs returns the input string unchanged with a nil error.
// Useful when an API requires a "parse"-shaped function but no parsing is needed.
func AsIs(s string) (string, error) {
	return s, nil
}
