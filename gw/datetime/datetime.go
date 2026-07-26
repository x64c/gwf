package datetime

import "time"

// Date returns t truncated to midnight (00:00:00) in t's location.
// If t is nil, uses time.Now() to return today.
func Date(t *time.Time) time.Time {
	if t == nil {
		t = new(time.Now())
	}
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}
