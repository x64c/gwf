package fwupstream

import (
	"context"
	"encoding/json/v2"
	"io"
	"net/http"

	"github.com/x64c/gwf/gw/errs"
)

// RetriableFn is the shape of each session-data type's
// UpstreamRequestWithBearerRetriable method, exposed as a method value (i.e.
// the bound receiver is the session-data instance). The JSON/PDF helpers
// below take a RetriableFn so they can be implemented once and shared across
// every session-data type.
type RetriableFn = func(
	ctx context.Context,
	fwClient *Client,
	method, endpoint string,
	payload *RequestPayload,
) (*http.Response, int, *errs.Error)

// RequestJSON forces Accept: application/json on the request and delegates to
// retriable. Returns the raw *http.Response (caller closes body) on success.
// res.Body is io.ReadCloser — the JSON byte stream; consume or pass through.
func RequestJSON(
	ctx context.Context,
	retriable RetriableFn,
	fwClient *Client,
	method, endpoint string,
	payload *RequestPayload,
) (*http.Response, int, *errs.Error) {
	actual := &RequestPayload{Headers: http.Header{}}
	if payload != nil {
		for k, vs := range payload.Headers {
			actual.Headers[k] = vs
		}
		actual.BodyProvider = payload.BodyProvider
	}
	actual.Headers.Set("Accept", "application/json")
	return retriable(ctx, fwClient, method, endpoint, actual)
}

// FetchJSON calls RequestJSON and JSON-unmarshals the response body into
// target. target must be a pointer to the caller's destination struct.
func FetchJSON(
	ctx context.Context,
	retriable RetriableFn,
	fwClient *Client,
	method, endpoint string,
	payload *RequestPayload,
	target any,
) (http.Header, int, *errs.Error) {
	res, status, resErr := RequestJSON(ctx, retriable, fwClient, method, endpoint, payload)
	if resErr != nil {
		return nil, status, resErr
	}
	defer func() { _ = res.Body.Close() }()

	if err := json.UnmarshalRead(res.Body, target); err != nil {
		return res.Header, status, errs.JSONUnmarshalFailed.Wrap(err)
	}
	return res.Header, status, nil
}

// RequestPDF forces Accept: application/pdf on the request and delegates to
// retriable. Returns the raw *http.Response (caller closes body) on success.
// res.Body is io.ReadCloser — the PDF byte stream; consume or pass through.
func RequestPDF(
	ctx context.Context,
	retriable RetriableFn,
	fwClient *Client,
	method, endpoint string,
	payload *RequestPayload,
) (*http.Response, int, *errs.Error) {
	actual := &RequestPayload{Headers: http.Header{}}
	if payload != nil {
		for k, vs := range payload.Headers {
			actual.Headers[k] = vs
		}
		actual.BodyProvider = payload.BodyProvider
	}
	actual.Headers.Set("Accept", "application/pdf")
	return retriable(ctx, fwClient, method, endpoint, actual)
}

// FetchPDFBytes calls RequestPDF and reads the entire response body into []byte.
func FetchPDFBytes(
	ctx context.Context,
	retriable RetriableFn,
	fwClient *Client,
	method, endpoint string,
	payload *RequestPayload,
) ([]byte, http.Header, int, *errs.Error) {
	res, status, resErr := RequestPDF(ctx, retriable, fwClient, method, endpoint, payload)
	if resErr != nil {
		return nil, nil, status, resErr
	}
	defer func() { _ = res.Body.Close() }()

	bs, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, res.Header, status, errs.Upstream.Wrap(err)
	}
	return bs, res.Header, status, nil
}
