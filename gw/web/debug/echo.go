// Package debug holds request-introspection tools for development and
// operator use. Everything here reflects request internals back to the caller
// by design, so nothing in this package is safe on an open route: mount these
// handlers only behind an operator/dev gate. The framework mounts nothing from
// this package on its own.
package debug

import (
	"bytes"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/x64c/gwf/gw/errs"
	"github.com/x64c/gwf/gw/web/requests"
	"github.com/x64c/gwf/gw/web/responses"
)

// EchoHandler reflects the request back at HTTP 200: full URL, method, ALL
// headers — Cookie and Authorization included — the raw body, and parsed
// form/multipart content (file metadata, not file bytes). That is its job: a
// debugging mirror for "what did the server actually receive". It therefore
// hands the caller everything the caller's request carried, and must sit
// behind a dev/operator gate.
//
// MaxMemoryMB bounds BOTH the raw-body copy and the multipart in-memory
// parse; a larger body is refused with 413 RequestBodyTooLarge. It must be
// > 0 — a zero value refuses every body as a misconfiguration, loudly.
type EchoHandler struct {
	MaxMemoryMB int64
}

func (h *EchoHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	resPayload := map[string]any{
		"url":    requests.FullURL(r),
		"method": r.Method,
		"header": r.Header,
	}

	if !requests.HasBody(r) {
		responses.EncodeWriteJSON(w, http.StatusOK, resPayload)
		return
	}

	if h.MaxMemoryMB <= 0 {
		responses.WriteSimpleErrorJSON(w, http.StatusInternalServerError, "EchoHandler.MaxMemoryMB must be > 0")
		return
	}

	defer func() {
		if closeErr := r.Body.Close(); closeErr != nil {
			log.Printf("[ERROR] %v", closeErr)
		}
	}()

	// The cap governs the raw copy, not just the multipart parse: ReadAll on
	// an unbounded body would buffer and reflect whatever the caller sends.
	limited := http.MaxBytesReader(w, r.Body, h.MaxMemoryMB<<20)
	rBodyBytes, err := io.ReadAll(limited)
	if err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			responses.WriteErrorJSON(w, http.StatusRequestEntityTooLarge, errs.RequestBodyTooLarge)
			return
		}
		responses.WriteSimpleErrorJSON(w, http.StatusInternalServerError, fmt.Sprintf("Failed to Read OriginalData: %v", err))
		return
	}

	rBodyPayload := map[string]any{
		"raw": string(rBodyBytes),
	}

	// reset body (rewind)
	// Since we already consumed r.OriginalData with io.ReadAll(r.OriginalData),
	// Reassign r.OriginalData to a No-op closer Reader on a copied buffer like rewinding r.OriginalData
	r.Body = io.NopCloser(bytes.NewReader(rBodyBytes))

	rContentType := r.Header.Get("Content-Type")

	switch {
	case strings.HasPrefix(rContentType, "application/json"):
		var tmp any
		if err = json.Unmarshal(rBodyBytes, &tmp); err == nil {
			// valid JSON
			rBodyPayload["json"] = string(rBodyBytes)
		} else {
			// invalid JSON
			rBodyPayload["json_error"] = err.Error()
		}
	case strings.HasPrefix(rContentType, "application/x-www-form-urlencoded"):
		if err = r.ParseForm(); err == nil {
			rBodyPayload["form"] = r.PostForm
		} else {
			rBodyPayload["form_error"] = err.Error()
		}
	case strings.HasPrefix(rContentType, "multipart/form-data"):
		if err = r.ParseMultipartForm(h.MaxMemoryMB << 20); err == nil {
			rBodyPayload["form"] = r.PostForm
			rBodyPayload["files"] = r.MultipartForm.File
		} else {
			rBodyPayload["form_error"] = err.Error()
		}
	}

	resPayload["body"] = rBodyPayload
	responses.EncodeWriteJSON(w, http.StatusOK, resPayload)
}
