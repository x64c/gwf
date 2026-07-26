package handlerwrappers

import (
	"fmt"
	"net/http"

	"github.com/x64c/gwf/gw/errs"
	"github.com/x64c/gwf/gw/web/responses"
)

// BodyLimit caps the request body to Max bytes via two-layer enforcement:
//
//  1. Fast-path: if the incoming Content-Length header declares a size larger
//     than Max, reject upfront with 413 + ContentLengthTooLarge. No body read.
//
//  2. Source-of-truth: wrap r.Body with http.MaxBytesReader so any actual read
//     past Max returns an error during streaming. Catches chunked-encoding
//     bodies (which carry no Content-Length) and clients lying about the
//     declared length. Handlers that observe the read error should respond
//     with 413 + RequestBodyTooLarge.
//
// Use on POST/PUT/PATCH endpoints that read a request body. Not needed on
// GET routes or other endpoints that don't read r.Body.
//
// Pick Max based on the expected legitimate body shape for the endpoint
// (e.g. a few KB for token-JSON endpoints, larger for upload endpoints).
type BodyLimit struct {
	Max int64
}

func (m *BodyLimit) Wrap(inner http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.ContentLength > m.Max {
			responses.WriteErrorJSON(w, http.StatusRequestEntityTooLarge,
				errs.ContentLengthTooLarge.WithDetail(fmt.Sprintf("max %d < got %d bytes", m.Max, r.ContentLength)))
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, m.Max)
		inner.ServeHTTP(w, r)
	})
}
