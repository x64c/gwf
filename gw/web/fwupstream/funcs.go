package fwupstream

// AccessTokenField returns the hash-field name where the upstream access
// token for the given clientID is stored: "up:a:<clientID>".
func AccessTokenField(clientID string) string {
	return accessTknFieldPrefix + clientID
}

// RefreshTokenField returns the hash-field name where the upstream refresh
// token for the given clientID is stored: "up:r:<clientID>".
func RefreshTokenField(clientID string) string {
	return refreshTknFieldPrefix + clientID
}
