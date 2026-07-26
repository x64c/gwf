package responses

import (
	"bytes"
	"encoding/json/v2"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/x64c/gwf/gw/web/requests"
)

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
		EncodeWriteJSON(w, http.StatusOK, resPayload)
		return
	}

	defer func() {
		if closeErr := r.Body.Close(); closeErr != nil {
			log.Printf("[ERROR] %v", closeErr)
		}
	}()

	rBodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		WriteSimpleErrorJSON(w, http.StatusInternalServerError, fmt.Sprintf("Failed to Read OriginalData: %v", err))
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
	EncodeWriteJSON(w, http.StatusOK, resPayload)
}
