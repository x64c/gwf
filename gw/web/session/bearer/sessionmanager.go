package bearer

import (
	"context"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/x64c/gwf/gw/errs"
	"github.com/x64c/gwf/gw/kvdbs"
	"github.com/x64c/gwf/gw/security"
	"github.com/x64c/gwf/gw/svc"
	"github.com/x64c/gwf/gw/web/fwupstream"
	"github.com/x64c/gwf/gw/web/session/lockstore"
)

// SessionManager owns bearer-session operations across all configured session groups.
//
// The boot loader builds the typed config tree (SessionGroupConf →
// SessionClientConf), then constructs the by-ID lookup map (ClientConfs).
// Per-name lookup is intermediate-only and not retained at runtime; group
// data is reachable via SessionClientConf.Group back-refs.
type SessionManager struct {
	ClientConfs   map[string]*SessionClientConf // keyed by SessionClientConf.ID ("" key valid for clientless)
	GroupConfs    map[string]*SessionGroupConf  // keyed by group name
	FWUpstream    *fwupstream.Hub               // upstream subsystem; token I/O delegates here. nil iff this app has no upstream
	AppName       string
	KVDB          kvdbs.DB          // holds session rows
	SessionLocks  *lockstore.Store  // shared with session.Service for per-bucket cap-enforcement locks
	ParentService svc.StateReporter // the owning session.Service (read-only on State()), for Serving()

	enabled atomic.Bool // the bearer protocol's on/off switch (svc.Switchable)
}

// Enable / Disable / Enabled implement svc.Switchable — the bearer protocol's
// own on/off switch. Enabled() reports only this switch; whether requests
// actually serve also depends on the service lifecycle (session.Service.Serving).
func (m *SessionManager) Enable()       { m.enabled.Store(true) }
func (m *SessionManager) Disable()      { m.enabled.Store(false) }
func (m *SessionManager) Enabled() bool { return m.enabled.Load() }

// Serving reports whether the bearer protocol should serve right now: its parent
// service is RUNNING and its own switch is Enabled. Fail-closed — a nil
// ParentService (manager not attached to a service) reports false.
func (m *SessionManager) Serving() bool {
	if m.ParentService == nil {
		return false
	}
	return m.ParentService.State() == svc.StateRUNNING && m.Enabled()
}

func (m *SessionManager) SessionRowKey(sessionID string) string {
	return m.AppName + ":b:" + sessionID
}

func (m *SessionManager) AccessTokenRowKey(tokenHash string) string {
	return m.AppName + ":bat:" + tokenHash
}

func (m *SessionManager) RefreshTokenRowKey(tokenHash string) string {
	return m.AppName + ":brt:" + tokenHash
}

// CapBucketRowKey returns the KVDB key for the session-ID list used to enforce
// per-bucket session count caps within a group. bindValues are the resolved
// values of the group's cap.by binding tuple, in declaration order. Client
// binds contribute the client ID (not name).
//
// Each variable segment is length-prefixed (":"len(s)":"s) so distinct
// (group, bindValues…) tuples can't collide regardless of segment content —
// e.g. ("a:b","c") and ("a","b:c") encode to different keys. The fixed
// "<AppName>:bcl" namespace prefix needs no prefix (constant, can't collide).
// The leading ':' on each segment is cosmetic — the length pins the boundary
// either way — but keeps the key uniformly delimited for human inspection.
// Examples:
//
//	("web", "client-a")          → <AppName>:bcl:3:web:8:client-a
//	("web", "client-a", "alice") → <AppName>:bcl:3:web:8:client-a:5:alice
func (m *SessionManager) CapBucketRowKey(groupName string, bindValues ...string) string {
	const tag = ":bcl"
	n := len(m.AppName) + len(tag) + lenPrefixedSize(groupName)
	for _, v := range bindValues {
		n += lenPrefixedSize(v)
	}
	var b strings.Builder
	b.Grow(n)
	b.WriteString(m.AppName)
	b.WriteString(tag)
	writeLenPrefixed(&b, groupName)
	for _, v := range bindValues {
		writeLenPrefixed(&b, v)
	}
	return b.String()
}

// lenPrefixedSize returns the byte length of writeLenPrefixed(s)'s output.
func lenPrefixedSize(s string) int {
	return 1 + len(strconv.Itoa(len(s))) + 1 + len(s) // ':' + len digits + ':' + s
}

// writeLenPrefixed writes ":"len(s)":"s so segments concatenated this way can't
// collide regardless of their content (the length pins the boundary; a ':'
// inside s is harmless). The leading ':' uniformly delimits adjacent segments.
func writeLenPrefixed(b *strings.Builder, s string) {
	b.WriteByte(':')
	b.WriteString(strconv.Itoa(len(s)))
	b.WriteByte(':')
	b.WriteString(s)
}

func (m *SessionManager) SessionRowExists(ctx context.Context, sessionID string) (bool, error) {
	return m.KVDB.Exists(ctx, m.SessionRowKey(sessionID))
}

// CreateSession persists a new bearer session for the given group + bindValues
// tuple, generating sid + access/refresh tokens. bindValues must provide a value
// for every label listed in group.Binds (values may be "" for clientless/userless).
//
// Returns the session ID and the raw access/refresh tokens to send to the caller.
func (m *SessionManager) CreateSession(
	ctx context.Context,
	group *SessionGroupConf,
	bindValues map[string]string,
) (string, string, string, error) {
	if !m.Serving() {
		return "", "", "", errs.SessionServiceUnavailable.WithDetail("bearer session")
	}

	sessionID := security.GenerateHex(16)
	accessToken := security.GenerateBase64RawURL(32)
	refreshToken := security.GenerateBase64RawURL(32)
	accessHash := security.HashHexSHA256(accessToken)
	refreshHash := security.HashHexSHA256(refreshToken)
	now := time.Now().Unix()

	accessTTL := time.Duration(group.AccessTTL) * time.Second
	refreshTTL := time.Duration(group.RefreshTTL) * time.Second

	// Job 1: write session state — umbrella row + access token row + refresh token row.
	umbrellaFields := map[string]any{
		"uid": bindValues["user"],
		"cid": bindValues["client"],
		"grp": group.Name,
		"ath": accessHash,
		"rth": refreshHash,
		"rcs": strconv.FormatInt(now, 10),
	}
	if err := m.KVDB.HashSetFieldsWithKeyTTL(ctx, m.SessionRowKey(sessionID), umbrellaFields, refreshTTL); err != nil {
		return "", "", "", err
	}
	if err := m.KVDB.ValueSet(ctx, m.AccessTokenRowKey(accessHash), sessionID, accessTTL); err != nil {
		return "", "", "", err
	}
	if err := m.KVDB.ValueSet(ctx, m.RefreshTokenRowKey(refreshHash), sessionID, refreshTTL); err != nil {
		return "", "", "", err
	}

	// Job 2: cap enforcement (only if group has a cap policy).
	if group.Cap != nil {
		bucketBindValues := make([]string, 0, len(group.Cap.By))
		for _, label := range group.Cap.By {
			bucketBindValues = append(bucketBindValues, bindValues[label])
		}
		bucketKey := m.CapBucketRowKey(group.Name, bucketBindValues...)
		entry := m.SessionLocks.Acquire(bucketKey)
		entry.Lock()
		defer entry.Unlock()

		if err := m.KVDB.ListPush(ctx, bucketKey, sessionID); err != nil {
			return "", "", "", err
		}
		defer func() {
			_, _ = m.KVDB.Expire(ctx, bucketKey, refreshTTL)
		}()

		if err := m.enforceSessionCap(ctx, bucketKey, group.Cap.Max); err != nil {
			return "", "", "", err
		}
	}

	return sessionID, accessToken, refreshToken, nil
}

// DestroySession removes a bearer session and all its associated KVDB rows:
// umbrella + access token row + refresh token row, plus the sid entry in the
// cap bucket list if the group has a cap policy. Idempotent — no-op if the
// session is already gone.
//
// If the session's client_id has been removed from config since the session was
// created, the cap-bucket list cleanup is skipped (the orphan list entry will
// TTL out); the row deletes still proceed.
func (m *SessionManager) DestroySession(ctx context.Context, sid string) error {
	row, err := m.FetchSession(ctx, sid)
	if err != nil {
		return err
	}
	if row == nil {
		return nil
	}

	keysToDelete := []string{
		m.SessionRowKey(sid),
		m.AccessTokenRowKey(row.AccessTokenHash),
		m.RefreshTokenRowKey(row.RefreshTokenHash),
	}

	clientConf, ok := m.ClientConfs[row.ClientID]
	if ok && clientConf.Group.Cap != nil {
		group := clientConf.Group
		bucketBindValues := make([]string, 0, len(group.Cap.By))
		for _, label := range group.Cap.By {
			switch label {
			case "user":
				bucketBindValues = append(bucketBindValues, row.UID)
			case "client":
				bucketBindValues = append(bucketBindValues, row.ClientID)
			}
		}
		bucketKey := m.CapBucketRowKey(group.Name, bucketBindValues...)
		entry := m.SessionLocks.Acquire(bucketKey)
		entry.Lock()
		defer entry.Unlock()

		_, _ = m.KVDB.ListRemove(ctx, bucketKey, 0, sid)
	}

	_, _ = m.KVDB.Delete(ctx, keysToDelete...)
	return nil
}

// FetchSession reads the bearer session umbrella row from KVDB.
// Returns (nil, nil) if the row doesn't exist (session expired or never existed).
// UID may be empty (userless flow); ClientID may be empty (clientless flow).
func (m *SessionManager) FetchSession(ctx context.Context, sessionID string) (*SessionRow, error) {
	fields, err := m.KVDB.HashGetFields(ctx, m.SessionRowKey(sessionID), "uid", "cid", "grp", "ath", "rth", "rcs")
	if err != nil {
		return nil, err
	}
	if len(fields) == 0 {
		return nil, nil
	}
	rcs, _ := strconv.ParseInt(fields["rcs"], 10, 64)
	return &SessionRow{
		UID:                 fields["uid"],
		ClientID:            fields["cid"],
		GroupName:           fields["grp"],
		AccessTokenHash:     fields["ath"],
		RefreshTokenHash:    fields["rth"],
		RefreshChainStartAt: rcs,
	}, nil
}

// FetchSessionByAccessToken resolves a raw access token to its umbrella session row
// via the two-hop path: hash → access token row → sid → umbrella row.
// Returns (nil, nil) if the token row is missing (expired/invalid) or the umbrella
// row is missing (evicted/expired).
func (m *SessionManager) FetchSessionByAccessToken(ctx context.Context, rawToken string) (*SessionRow, error) {
	hash := security.HashHexSHA256(rawToken)
	sid, found, err := m.KVDB.ValueGet(ctx, m.AccessTokenRowKey(hash))
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, nil
	}
	return m.FetchSession(ctx, sid)
}

// FetchSessionByRefreshToken resolves a raw refresh token to its umbrella session row
// via the two-hop path: hash → refresh token row → sid → umbrella row.
// Returns (nil, nil) if the token row is missing (expired/invalid) or the umbrella
// row is missing (evicted/expired).
func (m *SessionManager) FetchSessionByRefreshToken(ctx context.Context, rawToken string) (*SessionRow, error) {
	hash := security.HashHexSHA256(rawToken)
	sid, found, err := m.KVDB.ValueGet(ctx, m.RefreshTokenRowKey(hash))
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, nil
	}
	return m.FetchSession(ctx, sid)
}

// ExtendSession rotates the bearer session's access/refresh token pair given a
// valid refresh token. Returns the new (access, refresh) tokens.
func (m *SessionManager) ExtendSession(ctx context.Context, refreshToken string) (string, string, *errs.Error) {
	if !m.Serving() {
		return "", "", errs.SessionServiceUnavailable.WithDetail("bearer session")
	}

	// STEP 1: resolve refresh token → sid + umbrella row + group conf
	refreshHash := security.HashHexSHA256(refreshToken)
	sid, found, err := m.KVDB.ValueGet(ctx, m.RefreshTokenRowKey(refreshHash))
	if err != nil {
		return "", "", errs.KVDB.WithDetail("fetching refresh token row").WithCause(err)
	}
	if !found {
		return "", "", errs.RefreshTokenNotFound
	}

	// One rotation at a time per session. STEP 2's anti-replay check and the
	// STEP 5 write that retires the token it checked are three KVDB round
	// trips apart; unguarded, concurrent presentations of ONE refresh token
	// all read the pre-rotation row, all pass, and all rotate — so the reuse
	// detection below would catch only SEQUENTIAL replay. The umbrella row is
	// fetched inside the guard, because a row read before it can be stale by
	// the time the check runs, which is the same defect wearing a lock.
	//
	// Keyed by the session row, as the cap-enforcement locks are keyed by
	// their bucket row. DestroySession takes only a bucket lock, so the
	// replay branch below cannot deadlock against this one.
	locker := m.SessionLocks.Acquire(m.SessionRowKey(sid))
	locker.Lock()
	defer locker.Unlock()

	row, err := m.FetchSession(ctx, sid)
	if err != nil {
		return "", "", errs.KVDB.WithDetail("fetching session row").WithCause(err)
	}
	if row == nil {
		// The token resolved, so the credential is not the problem — the
		// container it points at has ended.
		return "", "", errs.BearerSessionNotFound
	}

	clientConf, ok := m.ClientConfs[row.ClientID]
	if !ok {
		return "", "", errs.InvalidRefreshToken.WithDetail("unknown client_id")
	}
	group := clientConf.Group

	// STEP 2: hardcap + anti-replay checks
	now := time.Now().Unix()
	if group.RefreshChainTTL > 0 && now-row.RefreshChainStartAt > int64(group.RefreshChainTTL) {
		_ = m.DestroySession(ctx, sid) // hardcap exceeded — force re-login
		return "", "", errs.InvalidRefreshToken.WithDetail("refresh chain ttl exceeded")
	}
	if refreshHash != row.RefreshTokenHash {
		// A replayed (already-rotated) token is rejected, and nothing more.
		// Deliberately NOT RFC 6819's destroy-the-chain response: this signal
		// cannot distinguish a thief from the victim from an honest retry
		// after a lost response, so automatic destruction becomes a
		// thief-plays-victim DoS — anyone holding a retired token could log
		// the legitimate user out at will — and a session kill for a network
		// glitch. Rotation already makes a refresh token one-shot; the chain
		// hardcap above bounds any stolen chain's lifetime. Recovery from
		// actual theft belongs to the account owner — re-login and session
		// revocation — where thief and victim CAN be told apart.
		return "", "", errs.InvalidRefreshToken.WithDetail("refresh token mismatch")
	}

	// STEP 3: generate new access + refresh tokens (no KVDB writes yet)
	newAccess := security.GenerateBase64RawURL(32)
	newRefresh := security.GenerateBase64RawURL(32)
	newAccessHash := security.HashHexSHA256(newAccess)
	newRefreshHash := security.HashHexSHA256(newRefresh)

	// STEP 4: write new access + refresh token rows (pointers to sid)
	accessTTL := time.Duration(group.AccessTTL) * time.Second
	refreshTTL := time.Duration(group.RefreshTTL) * time.Second
	if err := m.KVDB.ValueSet(ctx, m.AccessTokenRowKey(newAccessHash), sid, accessTTL); err != nil {
		return "", "", errs.KVDB.WithDetail("writing new access token row").WithCause(err)
	}
	if err := m.KVDB.ValueSet(ctx, m.RefreshTokenRowKey(newRefreshHash), sid, refreshTTL); err != nil {
		return "", "", errs.KVDB.WithDetail("writing new refresh token row").WithCause(err)
	}

	// STEP 5: update umbrella row's ath/rth fields + extend its TTL to refreshTTL.
	//
	// Conditional, because STEP 1 established the row exists and this write is
	// two round trips later. An unconditional write would RECREATE a row that
	// expired in between — carrying only ath/rth, no uid/cid/grp/rcs, on a full
	// fresh lifetime — and FetchSession would then hand that back as a session
	// with an empty principal.
	umbrellaFields := map[string]any{
		"ath": newAccessHash,
		"rth": newRefreshHash,
	}
	existed, err := m.KVDB.HashSetFieldsWithKeyTTLIfExists(ctx, m.SessionRowKey(sid), umbrellaFields, refreshTTL)
	if err != nil {
		return "", "", errs.KVDB.WithDetail("updating umbrella row").WithCause(err)
	}
	if !existed {
		// The session ended mid-rotation. STEP 4's token rows now point at
		// nothing, so drop them rather than leave them to time out.
		_, _ = m.KVDB.Delete(ctx,
			m.AccessTokenRowKey(newAccessHash),
			m.RefreshTokenRowKey(newRefreshHash),
		)
		return "", "", errs.BearerSessionNotFound
	}

	// STEP 6: delete old access + refresh token rows (cleanup); slide cap list TTL
	_, _ = m.KVDB.Delete(ctx,
		m.AccessTokenRowKey(row.AccessTokenHash),
		m.RefreshTokenRowKey(row.RefreshTokenHash),
	)

	if group.Cap != nil {
		bucketBindValues := make([]string, 0, len(group.Cap.By))
		for _, label := range group.Cap.By {
			switch label {
			case "user":
				bucketBindValues = append(bucketBindValues, row.UID)
			case "client":
				bucketBindValues = append(bucketBindValues, row.ClientID)
			}
		}
		bucketKey := m.CapBucketRowKey(group.Name, bucketBindValues...)
		_, _ = m.KVDB.Expire(ctx, bucketKey, refreshTTL)
	}

	return newAccess, newRefresh, nil
}

// enforceSessionCap enforces capMax on the cap bucket's session list. If the
// list size exceeds the cap, the oldest sessions are evicted — both their
// umbrella rows in KVDB AND their entries in the list — until the list size
// is back at the cap. No-op if the list is already at or below the cap.
//
// Caller MUST hold the SessionLocks mutex for bucketKey before calling.
// Without the lock, a concurrent session-create can push a new sid between the
// Len read and the Trim, evicting the wrong entries.
//
// Access/refresh token rows of evicted sessions are intentionally not deleted
// here — they become harmless pointer rows whose lookup target (the umbrella)
// is now missing, so requests using them fail at the umbrella-fetch step.
// Redis TTL reclaims their memory naturally; doing it explicitly would cost an
// extra HMGET + 2 deletes per evicted session for no functional benefit.
func (m *SessionManager) enforceSessionCap(ctx context.Context, bucketKey string, capMax int) error {
	sessionCnt, err := m.KVDB.ListLen(ctx, bucketKey)
	if err != nil {
		return err
	}
	if sessionCnt <= int64(capMax) {
		return nil
	}

	diff := sessionCnt - int64(capMax)
	sessionsToDel, err := m.KVDB.ListRange(ctx, bucketKey, 0, diff-1)
	if err != nil {
		return err
	}
	keysToDel := make([]string, 0, len(sessionsToDel))
	for _, sid := range sessionsToDel {
		keysToDel = append(keysToDel, m.SessionRowKey(sid))
	}
	_, _ = m.KVDB.Delete(ctx, keysToDel...)
	if err = m.KVDB.ListTrim(ctx, bucketKey, diff, -1); err != nil {
		return err
	}
	return nil
}
