package strs

import "fmt"

// PadLeft right-aligns s within width w (pads on the left with spaces).
// If len(s) >= w, s is returned unchanged.
func PadLeft(s string, w int) string {
	return fmt.Sprintf("%*s", w, s)
}
