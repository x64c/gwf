package passwd

import (
	"errors"
	"fmt"
	"strings"
)

// ErrUnknownHasherID : an encoding led by an id no hasher of the group owns — a hasher left the
// group while encodings it made were still stored.
var ErrUnknownHasherID = errors.New("passwd: no hasher of the group owns the encoding's id")

// ErrNotAnEncoding : a string that does not start with "$" id "$".
var ErrNotAnEncoding = errors.New("passwd: not an encoding")

// HasherGroup is an app's password hashers: exactly one current, and any number accepted.
//
//	passwd.HasherGroup := (currentHasher, acceptedHashers)   currentHasher ∈ passwd.Hasher, acceptedHashers ⊆ passwd.Hasher, every id owned by one hasher at most
//	Hash    → by the current hasher only: nothing new is ever made by another
//	Verify  → by the hasher owning the encoding's id — any of them; the accepted ones are the grace period
//	rehash  := the owner is not the current hasher, or the current hasher's NeedsRehash
//
// Accepted hashers exist for a grace period, while encodings they made are still stored: every
// successful Verify of one reports rehash, the app stores the current hasher's encoding
// instead, so their count only falls; at zero the app drops them. A group with no accepted
// hasher is the normal state — its rehash still reports the current hasher's parameter
// changes. Errors never carry an encoding: a stored hash is not something to log.
type HasherGroup struct {
	currentHasher    Hasher
	currentHasherIDs map[string]bool
	hasherMapByID    map[string]Hasher
	dummyEncoding    string // an encoding by the current hasher, for VerifyUnknown
}

// NewHasherGroup builds the group from its current hasher and its accepted ones, checked at
// the door.
//
//   - Returns: the *passwd.HasherGroup; or an error for a nil hasher, a hasher with no id, an id
//     owned by two hashers (the same hasher twice included), or the current hasher failing to
//     hash the dummy.
//   - Uses: passwd.Hasher.IDs, passwd.Hasher.Hash.
func NewHasherGroup(currentHasher Hasher, acceptedHashers ...Hasher) (*HasherGroup, error) {
	if currentHasher == nil {
		return nil, errors.New("passwd.NewHasherGroup: current hasher required")
	}
	g := &HasherGroup{currentHasher: currentHasher, currentHasherIDs: map[string]bool{}, hasherMapByID: map[string]Hasher{}}
	for i, h := range append([]Hasher{currentHasher}, acceptedHashers...) {
		if h == nil {
			return nil, fmt.Errorf("passwd.NewHasherGroup: accepted hasher %d is nil", i)
		}
		ids := h.IDs()
		if len(ids) == 0 {
			return nil, fmt.Errorf("passwd.NewHasherGroup: hasher %d owns no id", i)
		}
		for _, id := range ids {
			if _, taken := g.hasherMapByID[id]; taken {
				return nil, fmt.Errorf("passwd.NewHasherGroup: id %q owned by two hashers", id)
			}
			g.hasherMapByID[id] = h
			if i == 0 {
				g.currentHasherIDs[id] = true
			}
		}
	}
	dummyEncoding, err := currentHasher.Hash([]byte("passwd.HasherGroup: the dummy password"))
	if err != nil {
		return nil, fmt.Errorf("passwd.NewHasherGroup: current hasher: %w", err)
	}
	g.dummyEncoding = dummyEncoding
	return g, nil
}

// Hash encodes password by the current hasher — the only one that makes encodings.
//
//   - Returns: the encoding; or the current hasher's error.
//   - Uses: passwd.Hasher.Hash.
func (g *HasherGroup) Hash(password []byte) (string, error) {
	return g.currentHasher.Hash(password)
}

// Verify checks password against encoded by the hasher owning its id, as defined on
// passwd.HasherGroup.
//
//   - Returns: ok true for a match, with rehash as defined on passwd.HasherGroup; ok false for a
//     mismatch; an error — ok false — for an encoding no hasher of the group owns (passwd.ErrUnknownHasherID),
//     one that is not an encoding (passwd.ErrNotAnEncoding), or one its owner cannot parse.
//   - Uses: passwd.HasherGroup.owner, passwd.Hasher.Verify, passwd.HasherGroup.stale.
func (g *HasherGroup) Verify(password []byte, encoded string) (ok, rehash bool, err error) {
	owner, id, err := g.owner(encoded)
	if err != nil {
		return false, false, err
	}
	if ok, err = owner.Verify(password, encoded); err != nil || !ok {
		return false, false, err
	}
	rehash, err = g.stale(id, encoded)
	return true, rehash, err
}

// VerifyUnknown does the work of a Verify by the current hasher, for a name the app does not
// know — so the time an answer takes does not tell which names exist. There is nothing to
// accept: the result is dropped.
//
//   - Returns: nothing.
//   - Uses: passwd.Hasher.Verify on the dummy encoding.
func (g *HasherGroup) VerifyUnknown(password []byte) {
	_, _ = g.currentHasher.Verify(password, g.dummyEncoding)
}

// IsCurrent reports, without a password, whether encoded is what the group makes today: led
// by an id of the current hasher and at its parameters — the judgment behind rehash, for counting
// how many stored encodings a grace period still has to drain.
//
//   - Returns: the answer; an error for an encoding no hasher of the group owns or that is not an encoding.
//   - Uses: passwd.HasherGroup.owner, passwd.HasherGroup.stale.
func (g *HasherGroup) IsCurrent(encoded string) (bool, error) {
	_, id, err := g.owner(encoded)
	if err != nil {
		return false, err
	}
	stale, err := g.stale(id, encoded)
	return !stale, err
}

// owner is the hasher owning encoded's id.
//
//   - Returns: the hasher and the id; or passwd.ErrNotAnEncoding, or passwd.ErrUnknownHasherID
//     (wrapped with the id — an algorithm name, never the encoding).
//   - Uses: passwd.encodingID.
func (g *HasherGroup) owner(encoded string) (Hasher, string, error) {
	id, ok := encodingID(encoded)
	if !ok {
		return nil, "", ErrNotAnEncoding
	}
	h, ok := g.hasherMapByID[id]
	if !ok {
		return nil, "", fmt.Errorf("%w: %q", ErrUnknownHasherID, id)
	}
	return h, id, nil
}

// stale reports whether an encoding with this id is not what the group makes today: another
// hasher's, or the current hasher's at other parameters.
//
//   - Returns: the answer; or the current hasher's NeedsRehash error.
//   - Uses: passwd.Hasher.NeedsRehash.
func (g *HasherGroup) stale(id, encoded string) (bool, error) {
	if !g.currentHasherIDs[id] {
		return true, nil
	}
	return g.currentHasher.NeedsRehash(encoded)
}

// encodingID is the id leading an encoding: "$argon2id$…" → "argon2id", "$2b$12$…" → "2b".
//
//   - Returns: the id and true; "" and false when encoded does not start with "$" id "$".
//   - Uses: strings.CutPrefix, strings.Cut.
func encodingID(encoded string) (string, bool) {
	rest, ok := strings.CutPrefix(encoded, "$")
	if !ok {
		return "", false
	}
	id, _, ok := strings.Cut(rest, "$")
	if !ok || id == "" {
		return "", false
	}
	return id, true
}
