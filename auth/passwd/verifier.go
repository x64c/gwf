package passwd

import (
	"context"
	"errors"
	"log"

	"github.com/x64c/gwf/gw/authn"
	"github.com/x64c/gwf/gw/errs"
)

// Method is the authentication method the password verifier establishes identities by.
const Method authn.Method = "password"

// Verifier is the password method: a name and a password in, an authn.VerifiedIdentity out, or
// one answer for every failure the player could cause.
//
//	passwd.Verifier.Verify(name, password) :=
//	   lookup(name) not found              → the hasher group's VerifyUnknown, then errs.InvalidCredentials
//	   found, the hasher group's Verify    → mismatch: errs.InvalidCredentials
//	                                          match:    (Method, subject), and on rehash the current hasher's
//	                                                    encoding stored by storeRehash — a failure to store is
//	                                                    logged, never a failed login
//
// An unknown name and a wrong password take the same work and get the same answer, so neither the
// answer nor its time tells which names exist. All hashing of one Verify runs in one of at most
// maxConcurrentHashes slots — the hashes' memory is the bound a flood of logins meets; a Verify
// that cannot get a slot before ctx ends answers errs.PasswordCheckBusy.
type Verifier struct {
	hashers     *HasherGroup
	lookup      LookupFunc
	storeRehash StoreRehashFunc
	slots       chan struct{}
}

// NewVerifier is the password method over the app's hasher group and storage.
//
//   - Returns: the *passwd.Verifier; or an error for a missing hasher group, lookup or storeRehash,
//     or maxConcurrentHashes below 1.
//   - Uses: nothing beyond its arguments.
func NewVerifier(hashers *HasherGroup, lookup LookupFunc, storeRehash StoreRehashFunc, maxConcurrentHashes int) (*Verifier, error) {
	switch {
	case hashers == nil:
		return nil, errors.New("passwd.NewVerifier: hasher group required")
	case lookup == nil:
		return nil, errors.New("passwd.NewVerifier: lookup required")
	case storeRehash == nil:
		return nil, errors.New("passwd.NewVerifier: storeRehash required")
	case maxConcurrentHashes < 1:
		return nil, errors.New("passwd.NewVerifier: maxConcurrentHashes must be at least 1")
	}
	return &Verifier{hashers: hashers, lookup: lookup, storeRehash: storeRehash, slots: make(chan struct{}, maxConcurrentHashes)}, nil
}

// Verify checks a name and a password, as defined on passwd.Verifier. The caller keeps password
// and clears it after.
//
//   - Returns: the authn.VerifiedIdentity (passwd.Method, the lookup's subject); or
//     errs.InvalidCredentials — an unknown name or a wrong password; errs.PasswordCheckBusy — no
//     hashing slot before ctx ended; errs.InternalError with the cause — the lookup failed, or a
//     stored encoding its group cannot verify (a hasher dropped too early, a malformed row).
//   - Uses: passwd.LookupFunc, passwd.HasherGroup.Verify / VerifyUnknown / Hash,
//     passwd.StoreRehashFunc, log.Printf.
//
// Flow: lookup (no slot — a store call, not a hash) → a slot → not found: VerifyUnknown → the
// group's Verify → on rehash: Hash and storeRehash → the slot back.
func (v *Verifier) Verify(ctx context.Context, name string, password []byte) (authn.VerifiedIdentity, *errs.Error) {
	subject, encoding, found, err := v.lookup(ctx, name)
	if err != nil {
		return authn.VerifiedIdentity{}, errs.InternalError.WithCause(err)
	}
	select {
	case v.slots <- struct{}{}:
	case <-ctx.Done():
		return authn.VerifiedIdentity{}, errs.PasswordCheckBusy
	}
	defer func() { <-v.slots }()

	if !found {
		v.hashers.VerifyUnknown(password)
		return authn.VerifiedIdentity{}, errs.InvalidCredentials
	}
	ok, rehash, err := v.hashers.Verify(password, encoding)
	if err != nil {
		return authn.VerifiedIdentity{}, errs.InternalError.WithCause(err)
	}
	if !ok {
		return authn.VerifiedIdentity{}, errs.InvalidCredentials
	}
	if rehash {
		if fresh, err := v.hashers.Hash(password); err != nil {
			log.Printf("[WARN][passwd] rehash not made: %v", err)
		} else if err := v.storeRehash(ctx, subject, encoding, fresh); err != nil {
			log.Printf("[WARN][passwd] rehash not stored: %v", err)
		}
	}
	return authn.VerifiedIdentity{Method: Method, Subject: subject}, nil
}
