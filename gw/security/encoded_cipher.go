package security

// EncodedCipher encrypts plaintext bytes into an encoded (string) ciphertext
// and reverses it — an authenticated cipher whose output is a storable string
// rather than raw bytes.
//
// Requirements on implementations:
//   - The CipherContext is bound into the ciphertext: decryption with a
//     context other than the one sealed under MUST fail.
//   - The ciphertext states which key wrote it, so keys can rotate without
//     invalidating stored values, and a decrypt failure can name the key it
//     failed under.
type EncodedCipher interface {
	EncryptEncode(plaintext []byte, cc CipherContext) (string, error)
	DecodeDecrypt(encoded string, cc CipherContext) ([]byte, error)
}
