package security

import (
	"crypto/hkdf"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"sort"
	"strings"

	"golang.org/x/crypto/chacha20poly1305"
)

// Algorithms a keyring entry may name. The alg travels with the key so an
// algorithm migration rides the same machinery as a key rotation: add an
// entry with the new alg, flip active, drain, retire.
const AlgXChaCha20Poly1305 = "xchacha20poly1305"

// KeyringConf is the at-rest keyring config: named master keys and which one
// seals new values. Rotation is a conf change, not an outage: add a key, flip
// active — values written under the old key keep decrypting (the ciphertext
// names its key) until they age out, then the old entry is removed.
type KeyringConf struct {
	Active string                     `json:"active"` // key id new values are sealed under; must be present in Keys
	Keys   map[string]*KeyringKeyConf `json:"keys"`   // all keys that may still decrypt, by key id
}

// KeyringKeyConf is one keyring entry: a master key and its algorithm.
type KeyringKeyConf struct {
	Alg    string `json:"alg"`    // e.g. "xchacha20poly1305"
	EncKey string `json:"enckey"` // base64 (std, padded) of 32 random master bytes — openssl rand -base64 32
}

// KeyringCipher is the EncodedCipher over a keyring: seals under the active
// key, decrypts by the key id the ciphertext names, and binds every value to
// its CipherContext. Ciphertext form: "<kid>.<encoded>" — the key id rides in
// the clear (it is an identifier, not key material; cf. JWKS kid).
//
// Each instance serves ONE purpose (e.g. "upstream-token"): its working keys
// are HKDF-derived from the conf's master keys with the purpose as the info
// label, so two purposes built from the same conf are cryptographically
// distinct ciphers — a reused or shared master key cannot make their values
// interchangeable.
type KeyringCipher struct {
	purpose   string
	activeKid string
	ciphers   map[string]*XChaCha20Poly1305Cipher // by kid, derived for this purpose
}

// NewKeyringCipher validates the whole conf and derives this purpose's cipher
// per key — all misconfiguration is a construction (= boot) failure, never a
// per-request surprise: active missing or not among keys, empty or dotted key
// ids, unknown algs, malformed keys.
func NewKeyringCipher(conf *KeyringConf, purpose string) (*KeyringCipher, error) {
	if purpose == "" {
		return nil, errors.New("keyring cipher: purpose must not be empty")
	}
	if conf == nil {
		return nil, fmt.Errorf("keyring cipher (%s): no keyring conf", purpose)
	}
	if conf.Active == "" {
		return nil, fmt.Errorf("keyring cipher (%s): active key id must not be empty", purpose)
	}
	if len(conf.Keys) == 0 {
		return nil, fmt.Errorf("keyring cipher (%s): keys must not be empty", purpose)
	}
	if _, ok := conf.Keys[conf.Active]; !ok {
		return nil, fmt.Errorf("keyring cipher (%s): active key id %q is not among keys (have %s)",
			purpose, conf.Active, joinKids(conf.Keys))
	}

	ciphers := make(map[string]*XChaCha20Poly1305Cipher, len(conf.Keys))
	for kid, kc := range conf.Keys {
		if kid == "" {
			return nil, fmt.Errorf("keyring cipher (%s): empty key id", purpose)
		}
		if strings.Contains(kid, ".") {
			return nil, fmt.Errorf("keyring cipher (%s): key id %q must not contain '.' (the ciphertext separator)", purpose, kid)
		}
		if kc == nil {
			return nil, fmt.Errorf("keyring cipher (%s): key %q has no entry", purpose, kid)
		}
		if kc.Alg != AlgXChaCha20Poly1305 {
			return nil, fmt.Errorf("keyring cipher (%s): key %q names unknown alg %q", purpose, kid, kc.Alg)
		}
		master, err := base64.StdEncoding.DecodeString(kc.EncKey)
		if err != nil {
			return nil, fmt.Errorf("keyring cipher (%s): key %q enckey is not valid base64: %v", purpose, kid, err)
		}
		if len(master) != chacha20poly1305.KeySize {
			return nil, fmt.Errorf("keyring cipher (%s): key %q must be %d bytes, got %d",
				purpose, kid, chacha20poly1305.KeySize, len(master))
		}
		derived, err := hkdf.Key(sha256.New, master, nil, "gwf-at-rest:"+purpose, chacha20poly1305.KeySize)
		if err != nil {
			return nil, fmt.Errorf("keyring cipher (%s): key %q derivation: %v", purpose, kid, err)
		}
		cipher, err := NewXChaCha20Poly1305CipherBase64(derived)
		if err != nil {
			return nil, fmt.Errorf("keyring cipher (%s): key %q: %v", purpose, kid, err)
		}
		ciphers[kid] = cipher
	}

	return &KeyringCipher{
		purpose:   purpose,
		activeKid: conf.Active,
		ciphers:   ciphers,
	}, nil
}

// EncryptEncode seals plaintext under the active key, bound to cc, and
// prefixes the active key id.
func (k *KeyringCipher) EncryptEncode(plaintext []byte, cc CipherContext) (string, error) {
	if cc.Location == "" {
		return "", fmt.Errorf("keyring cipher (%s): cipher context Location must not be empty", k.purpose)
	}
	encoded, err := k.ciphers[k.activeKid].SealEncode(plaintext, cc.render())
	if err != nil {
		return "", err
	}
	return k.activeKid + "." + encoded, nil
}

// DecodeDecrypt opens a stored value with the key its ciphertext names, under
// cc. Failures name what the caller can act on: a value with no key id
// predates the keyring; an unknown key id means that key was retired (or the
// value is foreign); an open failure under a known key names the key — the
// value was moved, tampered with, or the key material changed under that id.
func (k *KeyringCipher) DecodeDecrypt(encoded string, cc CipherContext) ([]byte, error) {
	if cc.Location == "" {
		return nil, fmt.Errorf("keyring cipher (%s): cipher context Location must not be empty", k.purpose)
	}
	kid, rest, found := strings.Cut(encoded, ".")
	if !found {
		return nil, fmt.Errorf("keyring cipher (%s): ciphertext names no key id (pre-keyring value?)", k.purpose)
	}
	cipher, ok := k.ciphers[kid]
	if !ok {
		return nil, fmt.Errorf("keyring cipher (%s): ciphertext names key id %q, not in the keyring (have %s)",
			k.purpose, kid, joinKids(k.ciphers))
	}
	plaintext, err := cipher.DecodeOpen(rest, cc.render())
	if err != nil {
		return nil, fmt.Errorf("keyring cipher (%s): key %q: %w", k.purpose, kid, err)
	}
	return plaintext, nil
}

func joinKids[V any](byKid map[string]V) string {
	kids := make([]string, 0, len(byKid))
	for kid := range byKid {
		kids = append(kids, fmt.Sprintf("%q", kid))
	}
	sort.Strings(kids)
	return strings.Join(kids, ", ")
}
