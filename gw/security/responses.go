package security

// AccessTokenResponseBody is the answer that issues an access token (RFC 6749 §5.1). TokenType
// names how the access token is presented on a call, e.g. "bearer" (RFC 6750).
type AccessTokenResponseBody struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
	TokenType    string `json:"token_type"`
}

// AccessTokenAndIDTokenResponseBody is AccessTokenResponseBody plus the issuer's ID token — its
// signed statement of who signed in, for a client that did not check the credentials itself. On
// the wire the fields are flat, one object.
type AccessTokenAndIDTokenResponseBody struct {
	AccessTokenResponseBody
	IDToken string `json:"id_token"`
}
