// internal/features/capture/service_test.go
package capture

import (
	"context"
	"crypto/sha256"
	"errors"
	"testing"
	"time"

	capturedomain "github.com/gabrielgcmr/sonnda/internal/features/capture/domain"
	"github.com/gabrielgcmr/sonnda/internal/kernel/apperr"
	"github.com/google/uuid"
)

type fixedPairingCodeGenerator struct {
	plain string
	hash  []byte
	err   error
}

func (g fixedPairingCodeGenerator) Generate() (string, []byte, error) {
	return g.plain, append([]byte(nil), g.hash...), g.err
}

type serviceRepository struct {
	Repository
	transactionCalls int
	revokedAccount   uuid.UUID
	revokedAt        time.Time
	created          *capturedomain.Session
	current          *capturedomain.Session
	currentAccount   uuid.UUID
	touchedSession   uuid.UUID
	touchedAccount   uuid.UUID
	touchedAt        time.Time
	revokedSession   uuid.UUID
	err              error
	operations       []string
}

func (r *serviceRepository) WithinTransaction(ctx context.Context, fn func(Repository) error) error {
	r.transactionCalls++
	return fn(r)
}

func (r *serviceRepository) RevokeSessionsByAccount(_ context.Context, accountID uuid.UUID, revokedAt time.Time) (int64, error) {
	r.operations = append(r.operations, "revoke")
	r.revokedAccount, r.revokedAt = accountID, revokedAt
	return 1, r.err
}

func (r *serviceRepository) CreateSession(_ context.Context, session capturedomain.Session) error {
	r.operations = append(r.operations, "create")
	r.created = &session
	return r.err
}

func (r *serviceRepository) FindCurrentSession(_ context.Context, accountID uuid.UUID) (*capturedomain.Session, error) {
	r.currentAccount = accountID
	return r.current, r.err
}

func (r *serviceRepository) FindSession(_ context.Context, _ uuid.UUID) (*capturedomain.Session, error) {
	return r.current, r.err
}

func (r *serviceRepository) TouchDesktop(_ context.Context, sessionID, accountID uuid.UUID, seenAt time.Time) error {
	r.touchedSession, r.touchedAccount, r.touchedAt = sessionID, accountID, seenAt
	if r.current != nil {
		r.current.DesktopLastSeenAt = seenAt
	}
	return r.err
}

func (r *serviceRepository) RevokeSession(_ context.Context, sessionID, accountID uuid.UUID, revokedAt time.Time) error {
	r.revokedSession, r.revokedAccount, r.revokedAt = sessionID, accountID, revokedAt
	return r.err
}

func TestCreateSessionReplacesPreviousSessionAtomicallyAndStoresOnlyHash(t *testing.T) {
	now := time.Date(2026, 10, 9, 18, 0, 0, 0, time.UTC)
	accountID := uuid.New()
	hash := make([]byte, capturedomain.CredentialHashBytes)
	for i := range hash {
		hash[i] = byte(i + 1)
	}
	repository := &serviceRepository{}
	service := newService(repository, fixedPairingCodeGenerator{plain: "qr-secret", hash: hash}, func() time.Time { return now })

	created, err := service.CreateSession(t.Context(), accountID)
	if err != nil {
		t.Fatal(err)
	}
	if repository.transactionCalls != 1 || len(repository.operations) != 2 || repository.operations[0] != "revoke" || repository.operations[1] != "create" {
		t.Fatalf("transaction operations = %v", repository.operations)
	}
	if repository.revokedAccount != accountID || repository.created == nil || repository.created.AccountID != accountID {
		t.Fatalf("wrong account scoped mutation: revoked=%s created=%+v", repository.revokedAccount, repository.created)
	}
	if string(repository.created.PairingCodeHash) != string(hash) || string(repository.created.PairingCodeHash) == created.PairingCode {
		t.Fatal("repository did not receive only the pairing-code hash")
	}
	if created.PairingCode != "qr-secret" || !created.PairingExpiresAt.Equal(now.Add(capturedomain.MaxPairingLifetime)) {
		t.Fatalf("unexpected creation output: %+v", created)
	}
}

func TestSessionStateRequiresRecentPresenceFromBothClients(t *testing.T) {
	now := time.Date(2026, 10, 9, 18, 0, 0, 0, time.UTC)
	claimedAt := now.Add(-time.Minute)
	uploadExpiresAt := now.Add(time.Hour)
	mobileSeenAt := now.Add(-30 * time.Second)
	session := capturedomain.Session{
		ID: uuid.New(), AccountID: uuid.New(), PairingCodeHash: make([]byte, capturedomain.CredentialHashBytes),
		PairingExpiresAt: now.Add(time.Minute), DesktopLastSeenAt: now.Add(-59 * time.Second),
		ClaimedAt: &claimedAt, UploadTokenExpiresAt: &uploadExpiresAt,
		MobileLastSeenAt: &mobileSeenAt, CreatedAt: now.Add(-time.Minute), UpdatedAt: now,
	}
	repository := &serviceRepository{current: &session}
	service := newService(repository, fixedPairingCodeGenerator{}, func() time.Time { return now })

	state, err := service.CurrentSession(t.Context(), session.AccountID)
	if err != nil {
		t.Fatal(err)
	}
	if !state.DesktopPresent || !state.MobilePresent || !state.Connected || repository.currentAccount != session.AccountID {
		t.Fatalf("unexpected connected state: %+v", state)
	}

	repository.current.DesktopLastSeenAt = now.Add(-61 * time.Second)
	state, err = service.CurrentSession(t.Context(), session.AccountID)
	if err != nil {
		t.Fatal(err)
	}
	if state.DesktopPresent || !state.MobilePresent || state.Connected {
		t.Fatalf("stale desktop remained connected: %+v", state)
	}

	repository.current.DesktopLastSeenAt = now
	expiredAt := now
	repository.current.UploadTokenExpiresAt = &expiredAt
	state, err = service.CurrentSession(t.Context(), session.AccountID)
	if err != nil {
		t.Fatal(err)
	}
	if state.MobilePresent || state.Connected {
		t.Fatalf("expired mobile credential remained connected: %+v", state)
	}
}

func TestHeartbeatAndRevokeAreScopedByAccountAndSession(t *testing.T) {
	now := time.Date(2026, 10, 9, 18, 0, 0, 0, time.UTC)
	accountID, sessionID := uuid.New(), uuid.New()
	session := capturedomain.Session{ID: sessionID, AccountID: accountID, DesktopLastSeenAt: now}
	repository := &serviceRepository{current: &session}
	service := newService(repository, fixedPairingCodeGenerator{}, func() time.Time { return now })

	if _, err := service.Heartbeat(t.Context(), accountID, sessionID); err != nil {
		t.Fatal(err)
	}
	if repository.touchedAccount != accountID || repository.touchedSession != sessionID || !repository.touchedAt.Equal(now) {
		t.Fatalf("wrong heartbeat scope: account=%s session=%s at=%s", repository.touchedAccount, repository.touchedSession, repository.touchedAt)
	}
	if err := service.RevokeSession(t.Context(), accountID, sessionID); err != nil {
		t.Fatal(err)
	}
	if repository.revokedAccount != accountID || repository.revokedSession != sessionID || !repository.revokedAt.Equal(now) {
		t.Fatalf("wrong revocation scope: account=%s session=%s at=%s", repository.revokedAccount, repository.revokedSession, repository.revokedAt)
	}
}

func TestRepositorySessionNotFoundBecomesApplicationNotFound(t *testing.T) {
	repository := &serviceRepository{err: ErrSessionNotFound}
	service := newService(repository, fixedPairingCodeGenerator{}, time.Now)
	_, err := service.CurrentSession(t.Context(), uuid.New())
	if apperr.ErrorCodeOf(err) != apperr.NOT_FOUND || !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSecurePairingCodeHasExpectedEntropyAndHash(t *testing.T) {
	random := make([]byte, pairingCodeEntropyBytes)
	for i := range random {
		random[i] = byte(i)
	}
	plain, hash, err := (securePairingCodeGenerator{random: &fixedReader{value: random}}).Generate()
	if err != nil {
		t.Fatal(err)
	}
	want := sha256.Sum256([]byte(plain))
	if len(plain) != 43 || len(hash) != capturedomain.CredentialHashBytes || string(hash) != string(want[:]) {
		t.Fatalf("unexpected generated credential: plain length=%d hash length=%d", len(plain), len(hash))
	}
}

type fixedReader struct {
	value []byte
}

func (r *fixedReader) Read(target []byte) (int, error) {
	return copy(target, r.value), nil
}
