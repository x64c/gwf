package strs

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
)

// Atoi64 parses a base-10 string into int64. Mirror of strconv.Atoi for int.
func Atoi64(s string) (int64, error) {
	return strconv.ParseInt(s, 10, 64)
}

// NonEmpty returns s unchanged, or an error when s is empty. The most
// frequent case of Not.
func NonEmpty(s string) (string, error) {
	if s == "" {
		return "", errors.New("empty string")
	}
	return s, nil
}

// Not returns a parse-shaped function that rejects the listed strings and
// passes everything else through unchanged. At least one entry is required
// by the signature — a rejector with an empty denylist has no reason to exist.
func Not(first string, more ...string) func(string) (string, error) {
	notIn := append([]string{first}, more...)
	return func(s string) (string, error) {
		if slices.Contains(notIn, s) {
			return "", fmt.Errorf("string in denylist (%d entries)", len(notIn))
		}
		return s, nil
	}
}
