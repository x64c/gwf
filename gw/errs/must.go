package errs

// Must returns the 1st value consuming the 2nd error into panic(err)
func Must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}
