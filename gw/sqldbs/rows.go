package sqldbs

// Rows is a result set being iterated. The caller owns it: Close frees the
// connection it holds, and Err must be checked once the loop ends, because a
// loop cut short by a failure ends exactly like one that finished.
type Rows interface {
	Next() bool
	Scan(dest ...any) error
	Close() error
	Err() error
	NextResultSet() bool
}

// Row is a single-row result.
type Row interface {
	// Scan is the only method, and reports the whole outcome of the read: the
	// query failing, no row matching (ErrNoRows), or a value the destination
	// cannot hold. It must be called even when the values are unwanted — that is
	// what finishes the read and frees the connection.
	//
	// Rows carries Err because a multi-row read can fail twice, at the start and
	// then during iteration. A Row has no second phase, so one report is enough.
	//
	// The stdlib database/sql Row does carry Err, for wrapping packages wanting
	// the error without scanning, and this interface deliberately does not follow
	// it there. An implementation whose underlying driver also defers the error
	// has nothing to return from Err, and could answer it only by reimplementing
	// the single-row read on top of a multi-row one. Scan is the one place every
	// implementation can report from.
	Scan(dest ...any) error
}
