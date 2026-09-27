package passwd

// Hasher is one password-hashing algorithm — a material of the stored-password concept:
//
//	passwd.Hasher := an algorithm with .IDs() the identifiers leading its encodings, .Hash, .Verify, .NeedsRehash
//	an encoding   := the self-describing string a hasher writes — "$" id "$", then its parameters, the salt and the hash
//	                 (PHC string format: "$argon2id$v=19$m=19456,t=2,p=1$<salt>$<hash>"; modular crypt format: "$2b$12$…")
//
// The leading id names the algorithm that made an encoding, so encodings of several hashers
// live side by side in one column and each is verified by its own (passwd.HasherGroup).
// Implementations are named by their material.
type Hasher interface {
	// IDs are the identifiers that lead this hasher's encodings — "argon2id"; a family with
	// variants owns several (bcrypt: "2a", "2b", "2y").
	IDs() []string

	// Hash encodes password with a fresh random salt at this hasher's parameters.
	//
	//   - Returns: the encoding; or the error of the salt's randomness or of the algorithm.
	Hash(password []byte) (string, error)

	// Verify reports whether password matches encoded, an encoding led by one of IDs: the
	// algorithm runs again on password with encoded's parameters and salt, and the result is
	// compared with encoded's hash in constant time.
	//
	//   - Returns: true for a match, false for a mismatch; an error for an encoding it cannot
	//     parse — never a mismatch.
	Verify(password []byte, encoded string) (bool, error)

	// NeedsRehash reports whether encoded was made with parameters other than this hasher's
	// own — true means: hash the password just verified again, and store that instead.
	//
	//   - Returns: the answer; an error for an encoding it cannot parse.
	NeedsRehash(encoded string) (bool, error)
}
