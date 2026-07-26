package security

// EncodedCipher encrypts plaintext bytes into an encoded (string) ciphertext
// and reverses it — an authenticated cipher whose output is a storable string
// rather than raw bytes.
type EncodedCipher interface {
	EncryptEncode(plaintext []byte) (string, error)
	DecodeDecrypt(encoded string) ([]byte, error)
}
