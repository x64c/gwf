package fwupstream

// AccessTknRefreshReqBody is the framework's access-token refresh request body
// sent from a client app to its upstream FW app. RefreshToken is required;
// Extra carries app-specific fields the upstream's handler interprets.
type AccessTknRefreshReqBody struct {
	RefreshToken string         `json:"refresh_token"`
	Extra        map[string]any `json:"extra,omitempty"`
}
