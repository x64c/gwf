package fwupstream

// AccessTknRefreshResBody is the framework's access-token refresh response body
// returned by the upstream on success. Extra carries app-specific fields the
// client app may inspect.
type AccessTknRefreshResBody struct {
	AccessToken  string         `json:"access_token"`
	RefreshToken string         `json:"refresh_token"`
	Extra        map[string]any `json:"extra,omitempty"`
}
