package passwd

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Argon2id is the passwd.Hasher made of Argon2id (RFC 9106), the memory-hard winner of the
// Password Hashing Competition, writing the PHC string format:
//
//	passwd.Argon2id encoding := "$argon2id$v=19$m=" m ",t=" t ",p=" p "$" B64(salt) "$" B64(hash)
//	B64 := standard base64 without padding       hash := Argon2id(password, salt, t, m, p, KeyLen)
//
// The floor a conf must meet, from OWASP's Password Storage Cheat Sheet and RFC 9106:
//
//	(m, t) ≥ one of OWASP's minimum configurations (46 MiB, 1) (19 MiB, 2) (12 MiB, 3) (9 MiB, 4) (7 MiB, 5), both at once
//	p ≥ 1      SaltLen ≥ 16 bytes      KeyLen ≥ 32 bytes      (RFC 9106's recommended 128-bit salt and 256-bit tag)
//
// Verify accepts encodings at any parameters this type can run — they report NeedsRehash —
// but refuses one whose memory exceeds passwd.argon2idMaxMemoryKiB: a stored hash is the app's
// own data, yet a tampered one must not be able to make a login allocate unbounded memory.
type Argon2id struct {
	conf Argon2idConf
}

// argon2idFloor is OWASP's list of minimum Argon2id configurations — memory KiB and passes, any
// one of which suffices.
var argon2idFloor = [...]struct{ memoryKiB, iterations uint32 }{
	{47104, 1}, {19456, 2}, {12288, 3}, {9216, 4}, {7168, 5},
}

// argon2idMaxMemoryKiB is the most memory Verify runs for an encoding: 4 GiB.
const argon2idMaxMemoryKiB = 4 << 20

const argon2idID = "argon2id"

// NewArgon2id is an Argon2id hasher at the conf's parameters, checked against the floor
// defined on passwd.Argon2id.
//
//   - Returns: the *passwd.Argon2id; or an error naming the parameter below the floor or out of
//     range.
//   - Uses: passwd.argon2idFloor, passwd.argon2idMaxMemoryKiB.
func NewArgon2id(conf Argon2idConf) (*Argon2id, error) {
	meets := false
	for _, f := range argon2idFloor {
		if conf.MemoryKiB >= f.memoryKiB && conf.Iterations >= f.iterations {
			meets = true
			break
		}
	}
	switch {
	case !meets:
		return nil, fmt.Errorf("passwd.NewArgon2id: memory %d KiB with %d passes is below every OWASP minimum configuration", conf.MemoryKiB, conf.Iterations)
	case conf.MemoryKiB > argon2idMaxMemoryKiB:
		return nil, fmt.Errorf("passwd.NewArgon2id: memory %d KiB above %d KiB", conf.MemoryKiB, argon2idMaxMemoryKiB)
	case conf.Parallelism < 1:
		return nil, errors.New("passwd.NewArgon2id: parallelism must be at least 1")
	case conf.SaltLen < 16:
		return nil, fmt.Errorf("passwd.NewArgon2id: salt of %d bytes, at least 16 required", conf.SaltLen)
	case conf.KeyLen < 32:
		return nil, fmt.Errorf("passwd.NewArgon2id: hash of %d bytes, at least 32 required", conf.KeyLen)
	}
	return &Argon2id{conf: conf}, nil
}

// IDs is the one id Argon2id encodings lead with.
//
//   - Returns: ["argon2id"].
func (a *Argon2id) IDs() []string { return []string{argon2idID} }

// Hash encodes password as defined on passwd.Argon2id, with a fresh random salt.
//
//   - Returns: the encoding; or the salt's randomness error.
//   - Uses: crypto/rand.Read, argon2.IDKey, passwd.argon2idEncoding.
func (a *Argon2id) Hash(password []byte) (string, error) {
	c := a.conf
	salt := make([]byte, c.SaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("passwd.Argon2id.Hash: salt: %w", err)
	}
	e := argon2idEncoding{memoryKiB: c.MemoryKiB, iterations: c.Iterations, parallelism: c.Parallelism, salt: salt}
	e.hash = argon2.IDKey(password, salt, c.Iterations, c.MemoryKiB, c.Parallelism, c.KeyLen)
	return e.String(), nil
}

// Verify runs Argon2id again on password with the encoding's parameters and salt, and compares
// the result with its hash in constant time.
//
//   - Returns: true for a match, false for a mismatch; an error for an encoding it cannot parse
//     or run (another version, memory above passwd.argon2idMaxMemoryKiB).
//   - Uses: passwd.parseArgon2id, argon2.IDKey, subtle.ConstantTimeCompare.
func (a *Argon2id) Verify(password []byte, encoded string) (bool, error) {
	e, err := parseArgon2id(encoded)
	if err != nil {
		return false, err
	}
	got := argon2.IDKey(password, e.salt, e.iterations, e.memoryKiB, e.parallelism, uint32(len(e.hash)))
	return subtle.ConstantTimeCompare(got, e.hash) == 1, nil
}

// NeedsRehash reports whether the encoding was made with parameters other than this hasher's:
// memory, passes, lanes, salt length or hash length.
//
//   - Returns: the answer; an error for an encoding it cannot parse.
//   - Uses: passwd.parseArgon2id.
func (a *Argon2id) NeedsRehash(encoded string) (bool, error) {
	e, err := parseArgon2id(encoded)
	if err != nil {
		return false, err
	}
	c := a.conf
	return e.memoryKiB != c.MemoryKiB || e.iterations != c.Iterations || e.parallelism != c.Parallelism ||
		uint32(len(e.salt)) != c.SaltLen || uint32(len(e.hash)) != c.KeyLen, nil
}

// argon2idEncoding is one Argon2id encoding's parts, as defined on passwd.Argon2id.
type argon2idEncoding struct {
	memoryKiB   uint32
	iterations  uint32
	parallelism uint8
	salt, hash  []byte
}

// String writes the PHC string, as defined on passwd.Argon2id.
func (e argon2idEncoding) String() string {
	b64 := base64.RawStdEncoding
	return fmt.Sprintf("$%s$v=%d$m=%d,t=%d,p=%d$%s$%s", argon2idID, argon2.Version, e.memoryKiB, e.iterations, e.parallelism,
		b64.EncodeToString(e.salt), b64.EncodeToString(e.hash))
}

// parseArgon2id reads an Argon2id PHC string, strictly: the version this package runs (19),
// the three parameters in order, a salt of at least 8 bytes and a hash of at least 16.
//
//   - Returns: the parts; or an error — never one that carries the encoding.
//   - Uses: strings.Split, strconv.ParseUint, base64.RawStdEncoding.
func parseArgon2id(encoded string) (argon2idEncoding, error) {
	var e argon2idEncoding
	fail := func(what string) (argon2idEncoding, error) {
		return argon2idEncoding{}, fmt.Errorf("%w: argon2id %s", ErrNotAnEncoding, what)
	}
	parts := strings.Split(encoded, "$") // "", "argon2id", "v=19", "m=…,t=…,p=…", salt, hash
	if len(parts) != 6 || parts[0] != "" || parts[1] != argon2idID {
		return fail("layout")
	}
	if parts[2] != "v="+strconv.Itoa(argon2.Version) {
		return fail("version")
	}
	params := strings.Split(parts[3], ",")
	if len(params) != 3 {
		return fail("parameters")
	}
	var vals [3]uint64
	for i, key := range [3]string{"m=", "t=", "p="} {
		num, ok := strings.CutPrefix(params[i], key)
		if !ok {
			return fail("parameters")
		}
		v, err := strconv.ParseUint(num, 10, 32)
		if err != nil {
			return fail("parameters")
		}
		vals[i] = v
	}
	e.memoryKiB, e.iterations = uint32(vals[0]), uint32(vals[1])
	switch {
	case vals[2] < 1 || vals[2] > 255 || e.iterations < 1:
		return fail("parameters")
	case e.memoryKiB < 8*uint32(vals[2]) || e.memoryKiB > argon2idMaxMemoryKiB:
		return fail("memory")
	}
	e.parallelism = uint8(vals[2])
	var err error
	if e.salt, err = base64.RawStdEncoding.DecodeString(parts[4]); err != nil || len(e.salt) < 8 {
		return fail("salt")
	}
	if e.hash, err = base64.RawStdEncoding.DecodeString(parts[5]); err != nil || len(e.hash) < 16 {
		return fail("hash")
	}
	return e, nil
}
