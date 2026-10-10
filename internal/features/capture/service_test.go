// internal/features/capture/service_test.go
package capture

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
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
	transactionCalls        int
	revokedAccount          uuid.UUID
	revokedAt               time.Time
	created                 *capturedomain.Session
	current                 *capturedomain.Session
	currentAccount          uuid.UUID
	touchedSession          uuid.UUID
	touchedAccount          uuid.UUID
	touchedAt               time.Time
	revokedSession          uuid.UUID
	err                     error
	operations              []string
	claimed                 *capturedomain.Session
	pairingHash             []byte
	claim                   capturedomain.SessionClaim
	authenticated           *capturedomain.Session
	authenticatedHash       []byte
	touchedMobile           uuid.UUID
	createdCapture          *capturedomain.Capture
	availableCapture        *capturedomain.Capture
	markedDeleting          bool
	setAvailableErr         error
	markDeletingErr         error
	listedCaptures          []capturedomain.Capture
	listAccount             uuid.UUID
	listPage                Pagination
	listNow                 time.Time
	availableByID           *capturedomain.Capture
	ownedDeleting           *capturedomain.Capture
	ownedAccount            uuid.UUID
	ownedCapture            uuid.UUID
	deleteOwnedErr          error
	deletedOwned            bool
	cleanupCandidates       []capturedomain.Capture
	cleanupListCalls        int
	cleanupNow              time.Time
	cleanupUploadingCutoff  time.Time
	cleanupLimit            int
	cleanupCursor           *CleanupCursor
	cleanupRun              *CleanupRun
	cleanupRunCompletion    *CleanupRunCompletion
	createCleanupRunErr     error
	finishCleanupRunErr     error
	finishCleanupContextErr error
	deletedCaptureIDs       []uuid.UUID
	deleteCaptureErr        error
	deleteCaptureErrByID    map[uuid.UUID]error
	expiredSessionsToDelete int64
	deletedExpiredSessions  int64
	deleteExpiredErr        error
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

func (r *serviceRepository) ClaimSession(_ context.Context, pairingHash []byte, claim capturedomain.SessionClaim, _ time.Time) (*capturedomain.Session, error) {
	r.pairingHash = cloneBytes(pairingHash)
	r.claim = claim
	return r.claimed, r.err
}

func (r *serviceRepository) AuthenticateMobile(_ context.Context, uploadTokenHash []byte, _ time.Time) (*capturedomain.Session, error) {
	r.authenticatedHash = cloneBytes(uploadTokenHash)
	return r.authenticated, r.err
}

func (r *serviceRepository) AuthenticateUpload(_ context.Context, uploadTokenHash []byte, _, _ time.Time) (*capturedomain.Session, error) {
	r.authenticatedHash = cloneBytes(uploadTokenHash)
	return r.authenticated, r.err
}

func (r *serviceRepository) TouchMobile(_ context.Context, sessionID uuid.UUID, _ []byte, seenAt time.Time) error {
	r.touchedMobile = sessionID
	if r.current != nil {
		r.current.MobileLastSeenAt = &seenAt
	}
	return r.err
}

func (r *serviceRepository) CreateCapture(_ context.Context, item capturedomain.Capture) error {
	r.operations = append(r.operations, "reserve")
	r.createdCapture = &item
	return r.err
}

func (r *serviceRepository) SetCaptureAvailable(_ context.Context, _ uuid.UUID, storageURI string, updatedAt time.Time) (*capturedomain.Capture, error) {
	r.operations = append(r.operations, "available")
	if r.setAvailableErr != nil {
		return nil, r.setAvailableErr
	}
	item, err := r.createdCapture.MakeAvailable(storageURI, updatedAt)
	if err != nil {
		return nil, err
	}
	r.availableCapture = &item
	return &item, nil
}

func (r *serviceRepository) MarkCaptureDeleting(_ context.Context, captureID uuid.UUID, _ time.Time) (*capturedomain.Capture, error) {
	r.operations = append(r.operations, "deleting")
	r.markedDeleting = true
	if r.markDeletingErr != nil {
		return nil, r.markDeletingErr
	}
	for i := range r.cleanupCandidates {
		if r.cleanupCandidates[i].ID == captureID {
			r.cleanupCandidates[i].Status = capturedomain.StatusDeleting
			item := r.cleanupCandidates[i]
			return &item, nil
		}
	}
	if r.createdCapture != nil && r.createdCapture.ID == captureID {
		item := *r.createdCapture
		item.Status = capturedomain.StatusDeleting
		return &item, nil
	}
	return nil, ErrCaptureNotFound
}

func (r *serviceRepository) MarkOwnedCaptureDeleting(_ context.Context, accountID, captureID uuid.UUID, _ time.Time) (*capturedomain.Capture, error) {
	r.operations = append(r.operations, "mark-owned-deleting")
	r.ownedAccount, r.ownedCapture = accountID, captureID
	if r.markDeletingErr != nil {
		return nil, r.markDeletingErr
	}
	if r.ownedDeleting == nil {
		return nil, ErrCaptureNotFound
	}
	return r.ownedDeleting, nil
}

func (r *serviceRepository) FindAvailableCapture(_ context.Context, accountID, captureID uuid.UUID, now time.Time) (*capturedomain.Capture, error) {
	r.ownedAccount, r.ownedCapture, r.listNow = accountID, captureID, now
	if r.availableByID == nil {
		return nil, ErrCaptureNotFound
	}
	return r.availableByID, nil
}

func (r *serviceRepository) ListCaptures(_ context.Context, accountID uuid.UUID, now time.Time, page Pagination) ([]capturedomain.Capture, error) {
	r.listAccount, r.listNow, r.listPage = accountID, now, page
	if r.err != nil {
		return nil, r.err
	}
	return append([]capturedomain.Capture(nil), r.listedCaptures...), nil
}

func (r *serviceRepository) DeleteOwnedCapture(_ context.Context, accountID, captureID uuid.UUID) error {
	r.operations = append(r.operations, "delete-owned")
	r.ownedAccount, r.ownedCapture = accountID, captureID
	if r.deleteOwnedErr != nil {
		return r.deleteOwnedErr
	}
	r.deletedOwned = true
	return nil
}

func (r *serviceRepository) ListCleanupCandidates(
	_ context.Context,
	now, uploadingCutoff time.Time,
	cursor *CleanupCursor,
	limit int,
) ([]capturedomain.Capture, error) {
	r.operations = append(r.operations, "list-cleanup-candidates")
	r.cleanupListCalls++
	r.cleanupNow, r.cleanupUploadingCutoff, r.cleanupLimit = now, uploadingCutoff, limit
	if cursor == nil {
		r.cleanupCursor = nil
	} else {
		cloned := *cursor
		r.cleanupCursor = &cloned
	}
	if r.err != nil {
		return nil, r.err
	}
	if len(r.cleanupCandidates) == 0 {
		return nil, nil
	}
	candidates := append([]capturedomain.Capture(nil), r.cleanupCandidates...)
	sort.Slice(candidates, func(i, j int) bool {
		return cleanupPositionLess(candidates[i], candidates[j])
	})
	page := make([]capturedomain.Capture, 0, limit)
	for _, item := range candidates {
		if cursor != nil && !cleanupPositionAfter(item, *cursor) {
			continue
		}
		page = append(page, item)
		if len(page) == limit {
			break
		}
	}
	return page, nil
}

func cleanupPositionLess(left, right capturedomain.Capture) bool {
	if !left.ExpiresAt.Equal(right.ExpiresAt) {
		return left.ExpiresAt.Before(right.ExpiresAt)
	}
	if !left.CreatedAt.Equal(right.CreatedAt) {
		return left.CreatedAt.Before(right.CreatedAt)
	}
	return left.ID.String() < right.ID.String()
}

func cleanupPositionAfter(item capturedomain.Capture, cursor CleanupCursor) bool {
	if !item.ExpiresAt.Equal(cursor.ExpiresAt) {
		return item.ExpiresAt.After(cursor.ExpiresAt)
	}
	if !item.CreatedAt.Equal(cursor.CreatedAt) {
		return item.CreatedAt.After(cursor.CreatedAt)
	}
	return item.ID.String() > cursor.ID.String()
}

func (r *serviceRepository) DeleteCapture(_ context.Context, captureID uuid.UUID) error {
	r.operations = append(r.operations, "delete-capture")
	r.deletedCaptureIDs = append(r.deletedCaptureIDs, captureID)
	if r.deleteCaptureErrByID != nil {
		if err, ok := r.deleteCaptureErrByID[captureID]; ok {
			if errors.Is(err, ErrCaptureNotFound) {
				r.removeCleanupCandidate(captureID)
			}
			return err
		}
	}
	if r.deleteCaptureErr != nil {
		if errors.Is(r.deleteCaptureErr, ErrCaptureNotFound) {
			r.removeCleanupCandidate(captureID)
		}
		return r.deleteCaptureErr
	}
	r.removeCleanupCandidate(captureID)
	return nil
}

func (r *serviceRepository) removeCleanupCandidate(captureID uuid.UUID) {
	for i := range r.cleanupCandidates {
		if r.cleanupCandidates[i].ID != captureID {
			continue
		}
		r.cleanupCandidates = append(r.cleanupCandidates[:i], r.cleanupCandidates[i+1:]...)
		return
	}
}

func (r *serviceRepository) DeleteExpiredSessions(_ context.Context, _ time.Time, limit int) (int64, error) {
	r.operations = append(r.operations, "delete-expired-sessions")
	if r.deleteExpiredErr != nil {
		return 0, r.deleteExpiredErr
	}
	remaining := r.expiredSessionsToDelete - r.deletedExpiredSessions
	if remaining <= 0 {
		return 0, nil
	}
	toDelete := int64(limit)
	if toDelete > remaining {
		toDelete = remaining
	}
	r.deletedExpiredSessions += toDelete
	return toDelete, nil
}

func (r *serviceRepository) CreateCleanupRun(_ context.Context, run CleanupRun) error {
	r.operations = append(r.operations, "create-cleanup-run")
	if r.createCleanupRunErr != nil {
		return r.createCleanupRunErr
	}
	cloned := run
	r.cleanupRun = &cloned
	return nil
}

func (r *serviceRepository) FinishCleanupRun(ctx context.Context, completion CleanupRunCompletion) error {
	r.operations = append(r.operations, "finish-cleanup-run")
	r.finishCleanupContextErr = ctx.Err()
	if r.finishCleanupRunErr != nil {
		return r.finishCleanupRunErr
	}
	cloned := completion
	r.cleanupRunCompletion = &cloned
	return nil
}

type captureStorageStub struct {
	operations     *[]string
	uploaded       []byte
	objectName     string
	contentType    string
	deletedURI     string
	deletedURIs    []string
	deleteErrByURI map[string]error
	err            error
	signedURL      string
	signedURI      string
	signedTTL      time.Duration
	signedErr      error
	deleteErr      error
}

func (s *captureStorageStub) ObjectURI(objectName string) (string, error) {
	return "supabase://captures/" + objectName, nil
}

func (s *captureStorageStub) Upload(_ context.Context, file io.Reader, objectName, contentType string) (string, error) {
	if s.operations != nil {
		*s.operations = append(*s.operations, "upload")
	}
	s.objectName, s.contentType = objectName, contentType
	s.uploaded, _ = io.ReadAll(file)
	if s.err != nil {
		return "", s.err
	}
	return "supabase://captures/" + objectName, nil
}

func (*captureStorageStub) Open(context.Context, string) (io.ReadCloser, error) {
	return nil, errors.New("not implemented")
}

func (s *captureStorageStub) Delete(_ context.Context, uri string) error {
	if s.operations != nil {
		*s.operations = append(*s.operations, "delete-object")
	}
	s.deletedURI = uri
	s.deletedURIs = append(s.deletedURIs, uri)
	if s.deleteErrByURI != nil {
		if err, ok := s.deleteErrByURI[uri]; ok {
			return err
		}
	}
	return s.deleteErr
}

func (s *captureStorageStub) GetSignedURL(_ context.Context, uri string, expiresIn time.Duration) (string, error) {
	s.signedURI, s.signedTTL = uri, expiresIn
	if s.signedErr != nil {
		return "", s.signedErr
	}
	return s.signedURL, nil
}

func TestCreateSessionReplacesPreviousSessionAtomicallyAndStoresOnlyHash(t *testing.T) {
	now := time.Date(2026, 10, 9, 18, 0, 0, 0, time.UTC)
	accountID := uuid.New()
	hash := make([]byte, capturedomain.CredentialHashBytes)
	for i := range hash {
		hash[i] = byte(i + 1)
	}
	repository := &serviceRepository{}
	service := newService(repository, nil, fixedPairingCodeGenerator{plain: "qr-secret", hash: hash}, func() time.Time { return now })

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
	service := newService(repository, nil, fixedPairingCodeGenerator{}, func() time.Time { return now })

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
	service := newService(repository, nil, fixedPairingCodeGenerator{}, func() time.Time { return now })

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
	service := newService(repository, nil, fixedPairingCodeGenerator{}, time.Now)
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

func TestClaimSessionReturnsOpaqueTokenAndStoresOnlyHashes(t *testing.T) {
	now := time.Date(2026, 10, 9, 18, 0, 0, 0, time.UTC)
	pairingCode := "pairing-secret"
	uploadToken := "upload-secret"
	uploadHash := sha256.Sum256([]byte(uploadToken))
	session := &capturedomain.Session{ID: uuid.New(), AccountID: uuid.New()}
	repository := &serviceRepository{claimed: session}
	service := newService(repository, nil, fixedPairingCodeGenerator{plain: uploadToken, hash: uploadHash[:]}, func() time.Time { return now })

	claimed, err := service.ClaimSession(t.Context(), pairingCode)
	if err != nil {
		t.Fatal(err)
	}
	wantPairingHash := sha256.Sum256([]byte(pairingCode))
	if !bytes.Equal(repository.pairingHash, wantPairingHash[:]) || !bytes.Equal(repository.claim.UploadTokenHash, uploadHash[:]) {
		t.Fatal("claim did not persist credential hashes")
	}
	if claimed.SessionID != session.ID || claimed.UploadToken != uploadToken || !claimed.UploadTokenExpiresAt.Equal(now.Add(capturedomain.MaxUploadLifetime)) {
		t.Fatalf("unexpected claim output: %+v", claimed)
	}
}

func TestClaimSessionHidesInvalidExpiredAndReusedCodes(t *testing.T) {
	repository := &serviceRepository{err: ErrStateConflict}
	service := newService(repository, nil, fixedPairingCodeGenerator{plain: "upload", hash: make([]byte, capturedomain.CredentialHashBytes)}, time.Now)

	_, err := service.ClaimSession(t.Context(), "unknown-code")
	if apperr.ErrorCodeOf(err) != apperr.AUTH_TOKEN_INVALID || err.Error() != "código de pareamento inválido ou expirado" {
		t.Fatalf("unexpected generic claim failure: %v", err)
	}
}

func TestAuthenticateAndHeartbeatMobileUseOnlyTokenHash(t *testing.T) {
	now := time.Date(2026, 10, 9, 18, 0, 0, 0, time.UTC)
	token := "mobile-upload-token"
	session := &capturedomain.Session{ID: uuid.New(), AccountID: uuid.New(), DesktopLastSeenAt: now}
	repository := &serviceRepository{authenticated: session, current: session}
	service := newService(repository, nil, fixedPairingCodeGenerator{}, func() time.Time { return now })

	credential, err := service.AuthenticateMobile(t.Context(), token)
	if err != nil {
		t.Fatal(err)
	}
	wantHash := sha256.Sum256([]byte(token))
	if !bytes.Equal(repository.authenticatedHash, wantHash[:]) || credential.SessionID != session.ID || credential.AccountID != session.AccountID {
		t.Fatalf("unexpected mobile credential: %+v", credential)
	}
	state, err := service.MobileHeartbeat(t.Context(), *credential, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if repository.touchedMobile != session.ID || state.MobileLastSeenAt == nil || !state.MobileLastSeenAt.Equal(now) {
		t.Fatalf("mobile heartbeat was not persisted: %+v", state)
	}
}

func TestUploadCaptureDetectsContentAndTransitionsAfterStorage(t *testing.T) {
	now := time.Date(2026, 10, 9, 18, 0, 0, 0, time.UTC)
	tokenHash := sha256.Sum256([]byte("token"))
	session := &capturedomain.Session{ID: uuid.New(), AccountID: uuid.New()}
	repository := &serviceRepository{authenticated: session}
	storage := &captureStorageStub{operations: &repository.operations}
	service := newService(repository, storage, fixedPairingCodeGenerator{}, func() time.Time { return now })
	credential := MobileCredential{SessionID: session.ID, AccountID: session.AccountID, tokenHash: tokenHash[:]}
	content := []byte("%PDF-1.7\ncontent")

	item, err := service.UploadCapture(t.Context(), credential, UploadInput{
		File: bytes.NewReader(content), OriginalFilename: "fake.png", SizeBytes: int64(len(content)),
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(repository.operations, ",") != "reserve,upload,available" {
		t.Fatalf("unexpected upload order: %v", repository.operations)
	}
	if repository.createdCapture.StorageURI == nil || *repository.createdCapture.StorageURI != "supabase://captures/"+storage.objectName {
		t.Fatalf("deterministic storage URI was not persisted before upload: %+v", repository.createdCapture)
	}
	if item.Status != capturedomain.StatusAvailable || item.MIMEType != "application/pdf" || storage.contentType != "application/pdf" {
		t.Fatalf("content type was trusted instead of detected: item=%+v storage=%s", item, storage.contentType)
	}
	if !strings.HasSuffix(storage.objectName, ".pdf") || strings.Contains(storage.objectName, "fake.png") || !bytes.Equal(storage.uploaded, content) {
		t.Fatalf("unexpected stored object: name=%s content=%q", storage.objectName, storage.uploaded)
	}
}

func TestUploadCaptureRejectsInvalidContentAndStaleDesktop(t *testing.T) {
	now := time.Date(2026, 10, 9, 18, 0, 0, 0, time.UTC)
	tokenHash := sha256.Sum256([]byte("token"))
	session := &capturedomain.Session{ID: uuid.New(), AccountID: uuid.New()}
	credential := MobileCredential{SessionID: session.ID, AccountID: session.AccountID, tokenHash: tokenHash[:]}

	repository := &serviceRepository{authenticated: session}
	service := newService(repository, &captureStorageStub{}, fixedPairingCodeGenerator{}, func() time.Time { return now })
	_, err := service.UploadCapture(t.Context(), credential, UploadInput{
		File: bytes.NewReader([]byte("%PDF-1.7")), OriginalFilename: "exam.pdf", SizeBytes: capturedomain.MaxFileSizeBytes + 1,
	})
	if apperr.ErrorCodeOf(err) != apperr.UPLOAD_SIZE_EXCEEDED || repository.createdCapture != nil {
		t.Fatalf("oversized file reached persistence: %v", err)
	}

	_, err = service.UploadCapture(t.Context(), credential, UploadInput{
		File: bytes.NewReader([]byte("not an image")), OriginalFilename: "exam.pdf", SizeBytes: 12,
	})
	if apperr.ErrorCodeOf(err) != apperr.UNSUPPORTED_MEDIA_TYPE || repository.createdCapture != nil {
		t.Fatalf("invalid content reached persistence: %v", err)
	}

	repository.err = ErrSessionNotFound
	_, err = service.UploadCapture(t.Context(), credential, UploadInput{
		File: bytes.NewReader([]byte("%PDF-1.7")), OriginalFilename: "exam.pdf", SizeBytes: 8,
	})
	if apperr.ErrorCodeOf(err) != apperr.AUTH_TOKEN_INVALID || repository.createdCapture != nil {
		t.Fatalf("stale desktop allowed upload: %v", err)
	}
}

func TestDetectCaptureFileTypeUsesMagicBytes(t *testing.T) {
	tests := []struct {
		name     string
		content  []byte
		wantMIME string
		wantExt  string
	}{
		{name: "PDF", content: []byte("%PDF-1.7"), wantMIME: "application/pdf", wantExt: "pdf"},
		{name: "JPEG", content: []byte{0xff, 0xd8, 0xff, 0xe0}, wantMIME: "image/jpeg", wantExt: "jpg"},
		{name: "PNG", content: []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a}, wantMIME: "image/png", wantExt: "png"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			file := bytes.NewReader(test.content)
			mimeType, extension, err := detectCaptureFileType(file)
			if err != nil || mimeType != test.wantMIME || extension != test.wantExt {
				t.Fatalf("detected mime=%q ext=%q err=%v", mimeType, extension, err)
			}
			position, _ := file.Seek(0, io.SeekCurrent)
			if position != 0 {
				t.Fatalf("file was not rewound: position=%d", position)
			}
		})
	}
}

func TestUploadFailureMarksDeletingAndAttemptsObjectRemoval(t *testing.T) {
	now := time.Date(2026, 10, 9, 18, 0, 0, 0, time.UTC)
	tokenHash := sha256.Sum256([]byte("token"))
	session := &capturedomain.Session{ID: uuid.New(), AccountID: uuid.New()}
	repository := &serviceRepository{authenticated: session}
	storage := &captureStorageStub{operations: &repository.operations, err: &apperr.AppError{Kind: apperr.INFRA_STORAGE_ERROR, Message: "storage unavailable"}}
	service := newService(repository, storage, fixedPairingCodeGenerator{}, func() time.Time { return now })
	credential := MobileCredential{SessionID: session.ID, AccountID: session.AccountID, tokenHash: tokenHash[:]}

	_, err := service.UploadCapture(t.Context(), credential, UploadInput{
		File: bytes.NewReader([]byte("%PDF-1.7")), OriginalFilename: "exam.pdf", SizeBytes: 8,
	})
	if apperr.ErrorCodeOf(err) != apperr.INFRA_STORAGE_ERROR || !repository.markedDeleting || storage.deletedURI == "" {
		t.Fatalf("upload compensation was not attempted: err=%v deleting=%t uri=%q", err, repository.markedDeleting, storage.deletedURI)
	}
}

func TestFinalizationFailureCompensatesUploadedObject(t *testing.T) {
	now := time.Date(2026, 10, 9, 18, 0, 0, 0, time.UTC)
	tokenHash := sha256.Sum256([]byte("token"))
	session := &capturedomain.Session{ID: uuid.New(), AccountID: uuid.New()}
	repository := &serviceRepository{authenticated: session, setAvailableErr: ErrStateConflict}
	storage := &captureStorageStub{operations: &repository.operations}
	service := newService(repository, storage, fixedPairingCodeGenerator{}, func() time.Time { return now })
	credential := MobileCredential{SessionID: session.ID, AccountID: session.AccountID, tokenHash: tokenHash[:]}

	_, err := service.UploadCapture(t.Context(), credential, UploadInput{
		File: bytes.NewReader([]byte("%PDF-1.7")), OriginalFilename: "exam.pdf", SizeBytes: 8,
	})
	if apperr.ErrorCodeOf(err) != apperr.RESOURCE_CONFLICT || !repository.markedDeleting || storage.deletedURI == "" {
		t.Fatalf("finalization compensation was not attempted: err=%v deleting=%t uri=%q", err, repository.markedDeleting, storage.deletedURI)
	}
}

func TestListCapturesReturnsPageWithoutConnectionDependency(t *testing.T) {
	now := time.Date(2026, 10, 9, 18, 0, 0, 0, time.UTC)
	accountID := uuid.New()
	items := make([]capturedomain.Capture, 3)
	for i := range items {
		items[i] = capturedomain.Capture{ID: uuid.New(), AccountID: accountID}
	}
	repository := &serviceRepository{listedCaptures: items}
	service := newService(repository, nil, fixedPairingCodeGenerator{}, func() time.Time { return now })

	page, err := service.ListCaptures(t.Context(), accountID, Pagination{Limit: 2, Offset: 4})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 2 || !page.HasMore || page.Limit != 2 || page.Offset != 4 {
		t.Fatalf("unexpected page: %+v", page)
	}
	if repository.listAccount != accountID || repository.listPage.Limit != 3 || repository.listPage.Offset != 4 || !repository.listNow.Equal(now) {
		t.Fatalf("unexpected repository list scope: account=%s page=%+v now=%s", repository.listAccount, repository.listPage, repository.listNow)
	}
}

func TestGetCaptureFileLimitsSignedURLToCaptureExpiration(t *testing.T) {
	now := time.Date(2026, 10, 9, 18, 0, 0, 0, time.UTC)
	accountID, captureID := uuid.New(), uuid.New()
	uri := "supabase://captures/account/capture.pdf"
	item := &capturedomain.Capture{ID: captureID, AccountID: accountID, StorageURI: &uri, ExpiresAt: now.Add(2*time.Minute + 900*time.Millisecond)}
	repository := &serviceRepository{availableByID: item}
	storage := &captureStorageStub{signedURL: "https://storage.test/signed"}
	service := newService(repository, storage, fixedPairingCodeGenerator{}, func() time.Time { return now })

	file, err := service.GetCaptureFile(t.Context(), accountID, captureID)
	if err != nil {
		t.Fatal(err)
	}
	if storage.signedURI != uri || storage.signedTTL != 2*time.Minute {
		t.Fatalf("unexpected signature request: uri=%q ttl=%s", storage.signedURI, storage.signedTTL)
	}
	if file.URL != storage.signedURL || !file.ExpiresAt.Equal(now.Add(2*time.Minute)) || file.ExpiresAt.After(item.ExpiresAt) {
		t.Fatalf("unexpected signed file: %+v", file)
	}
}

func TestGetCaptureFileNeverSignsLongerThanFiveMinutes(t *testing.T) {
	now := time.Date(2026, 10, 9, 18, 0, 0, 0, time.UTC)
	accountID, captureID := uuid.New(), uuid.New()
	uri := "supabase://captures/account/capture.pdf"
	repository := &serviceRepository{availableByID: &capturedomain.Capture{
		ID: captureID, AccountID: accountID, StorageURI: &uri, ExpiresAt: now.Add(time.Hour),
	}}
	storage := &captureStorageStub{signedURL: "https://storage.test/signed"}
	service := newService(repository, storage, fixedPairingCodeGenerator{}, func() time.Time { return now })

	if _, err := service.GetCaptureFile(t.Context(), accountID, captureID); err != nil {
		t.Fatal(err)
	}
	if storage.signedTTL != MaxSignedURLLifetime {
		t.Fatalf("signed URL lifetime = %s", storage.signedTTL)
	}
}

func TestDeleteCaptureIsRecoverableAndIdempotent(t *testing.T) {
	now := time.Date(2026, 10, 9, 18, 0, 0, 0, time.UTC)
	accountID, captureID := uuid.New(), uuid.New()
	uri := "supabase://captures/account/capture.pdf"
	item := &capturedomain.Capture{ID: captureID, AccountID: accountID, StorageURI: &uri, Status: capturedomain.StatusDeleting}
	repository := &serviceRepository{ownedDeleting: item}
	storage := &captureStorageStub{operations: &repository.operations, deleteErr: errors.New("storage unavailable")}
	service := newService(repository, storage, fixedPairingCodeGenerator{}, func() time.Time { return now })

	err := service.DeleteCapture(t.Context(), accountID, captureID)
	if apperr.ErrorCodeOf(err) != apperr.INFRA_STORAGE_ERROR || repository.deletedOwned {
		t.Fatalf("failed object deletion removed database row: err=%v deleted=%t", err, repository.deletedOwned)
	}

	storage.deleteErr = nil
	if err := service.DeleteCapture(t.Context(), accountID, captureID); err != nil {
		t.Fatal(err)
	}
	if !repository.deletedOwned || strings.Join(repository.operations[len(repository.operations)-3:], ",") != "mark-owned-deleting,delete-object,delete-owned" {
		t.Fatalf("unexpected successful deletion: operations=%v deleted=%t", repository.operations, repository.deletedOwned)
	}

	repository.ownedDeleting = nil
	if err := service.DeleteCapture(t.Context(), accountID, captureID); err != nil {
		t.Fatalf("repeated deletion should be idempotent: %v", err)
	}
}

func TestDeleteCaptureTreatsMissingStorageObjectAsDeleted(t *testing.T) {
	now := time.Date(2026, 10, 9, 18, 0, 0, 0, time.UTC)
	accountID, captureID := uuid.New(), uuid.New()
	uri := "supabase://captures/account/missing.pdf"
	repository := &serviceRepository{ownedDeleting: &capturedomain.Capture{
		ID: captureID, AccountID: accountID, StorageURI: &uri, Status: capturedomain.StatusDeleting,
	}}
	storage := &captureStorageStub{deleteErr: &apperr.AppError{Kind: apperr.NOT_FOUND, Message: "arquivo não encontrado"}}
	service := newService(repository, storage, fixedPairingCodeGenerator{}, func() time.Time { return now })

	if err := service.DeleteCapture(t.Context(), accountID, captureID); err != nil {
		t.Fatalf("missing object should complete database deletion: %v", err)
	}
	if !repository.deletedOwned {
		t.Fatal("database row was not deleted after object was already absent")
	}
}

type fixedReader struct {
	value []byte
}

func (r *fixedReader) Read(target []byte) (int, error) {
	return copy(target, r.value), nil
}

func TestCleanup_RemovesExpiredDeletingAndStuckUploading(t *testing.T) {
	now := time.Date(2026, 10, 9, 18, 0, 0, 0, time.UTC)
	accountID := uuid.New()
	sessionID := uuid.New()

	uri1 := "supabase://captures/acc/expired.pdf"
	uri2 := "supabase://captures/acc/deleting.pdf"
	uri3 := "supabase://captures/acc/uploading.pdf"

	cap1 := capturedomain.Capture{
		ID:               uuid.New(),
		AccountID:        accountID,
		CaptureSessionID: sessionID,
		StorageURI:       &uri1,
		Status:           capturedomain.StatusAvailable,
		ExpiresAt:        now.Add(-10 * time.Minute), // expired
		CreatedAt:        now.Add(-25 * time.Hour),
		UpdatedAt:        now.Add(-25 * time.Hour),
	}

	cap2 := capturedomain.Capture{
		ID:               uuid.New(),
		AccountID:        accountID,
		CaptureSessionID: sessionID,
		StorageURI:       &uri2,
		Status:           capturedomain.StatusDeleting, // in deleting
		ExpiresAt:        now.Add(10 * time.Hour),
		CreatedAt:        now.Add(-2 * time.Hour),
		UpdatedAt:        now.Add(-30 * time.Minute),
	}

	cap3 := capturedomain.Capture{
		ID:               uuid.New(),
		AccountID:        accountID,
		CaptureSessionID: sessionID,
		StorageURI:       &uri3, // reserved before the upload starts
		Status:           capturedomain.StatusUploading,
		ExpiresAt:        now.Add(23 * time.Hour),
		CreatedAt:        now.Add(-2 * time.Hour),
		UpdatedAt:        now.Add(-75 * time.Minute), // stuck > 1 hour
	}

	operations := make([]string, 0)
	repository := &serviceRepository{
		operations:              operations,
		cleanupCandidates:       []capturedomain.Capture{cap1, cap2, cap3},
		expiredSessionsToDelete: 2,
	}
	storage := &captureStorageStub{operations: &repository.operations}
	service := newService(repository, storage, fixedPairingCodeGenerator{}, func() time.Time { return now })

	report, err := service.Cleanup(t.Context(), CleanupOptions{BatchSize: 10})
	if err != nil {
		t.Fatalf("unexpected cleanup error: %v", err)
	}

	if report.CapturesProcessed != 3 {
		t.Errorf("expected 3 processed captures, got %d", report.CapturesProcessed)
	}
	if report.CapturesDeleted != 3 {
		t.Errorf("expected 3 deleted captures, got %d", report.CapturesDeleted)
	}
	if report.StorageDeleted != 3 {
		t.Errorf("expected 3 deleted storage objects, got %d", report.StorageDeleted)
	}
	if report.SessionsDeleted != 2 {
		t.Errorf("expected 2 deleted sessions, got %d", report.SessionsDeleted)
	}
	if len(report.Errors) != 0 {
		t.Errorf("expected no errors, got: %v", report.Errors)
	}
	if report.RunID == uuid.Nil || repository.cleanupRun == nil || repository.cleanupRun.ID != report.RunID {
		t.Fatalf("cleanup run was not created consistently: report=%s run=%+v", report.RunID, repository.cleanupRun)
	}
	if repository.cleanupRunCompletion == nil || !repository.cleanupRunCompletion.Succeeded {
		t.Fatalf("cleanup run was not completed successfully: %+v", repository.cleanupRunCompletion)
	}

	// Verify storage deletions
	if len(storage.deletedURIs) != 3 || storage.deletedURIs[0] != uri1 || storage.deletedURIs[1] != uri2 || storage.deletedURIs[2] != uri3 {
		t.Errorf("expected storage deleted URIs [%s, %s, %s], got %v", uri1, uri2, uri3, storage.deletedURIs)
	}

	// Verify database deletions
	if len(repository.deletedCaptureIDs) != 3 {
		t.Errorf("expected 3 deleted capture IDs, got %v", repository.deletedCaptureIDs)
	}

	// Verify that sessions deletion happened AFTER captures deletion
	lastCaptureIdx := -1
	firstSessionIdx := -1
	for i, op := range repository.operations {
		if op == "delete-capture" {
			lastCaptureIdx = i
		}
		if op == "delete-expired-sessions" && firstSessionIdx == -1 {
			firstSessionIdx = i
		}
	}
	if lastCaptureIdx > firstSessionIdx {
		t.Errorf("captures must be deleted before sessions: ops=%v", repository.operations)
	}
}

func TestCleanup_BatchedProcessing(t *testing.T) {
	now := time.Date(2026, 10, 9, 18, 0, 0, 0, time.UTC)
	accountID := uuid.New()

	var candidates []capturedomain.Capture
	for i := 0; i < 5; i++ {
		uri := fmt.Sprintf("supabase://captures/acc/file%d.pdf", i)
		candidates = append(candidates, capturedomain.Capture{
			ID:         uuid.New(),
			AccountID:  accountID,
			StorageURI: &uri,
			Status:     capturedomain.StatusDeleting,
			ExpiresAt:  now.Add(time.Hour),
		})
	}

	repository := &serviceRepository{
		cleanupCandidates: candidates,
	}
	storage := &captureStorageStub{}
	service := newService(repository, storage, fixedPairingCodeGenerator{}, func() time.Time { return now })

	// Process in batches of 2
	report, err := service.Cleanup(t.Context(), CleanupOptions{BatchSize: 2})
	if err != nil {
		t.Fatalf("unexpected cleanup error: %v", err)
	}

	if report.CapturesProcessed != 5 {
		t.Errorf("expected 5 processed, got %d", report.CapturesProcessed)
	}
	if report.CapturesDeleted != 5 {
		t.Errorf("expected 5 deleted, got %d", report.CapturesDeleted)
	}
	if len(storage.deletedURIs) != 5 {
		t.Errorf("expected 5 storage objects deleted, got %d", len(storage.deletedURIs))
	}
}

func TestCleanup_StorageFailurePreservesDatabaseRecordAndMarksDeleting(t *testing.T) {
	now := time.Date(2026, 10, 9, 18, 0, 0, 0, time.UTC)
	accountID := uuid.New()
	failedURI := "supabase://captures/acc/failed.pdf"
	okURI := "supabase://captures/acc/ok.pdf"

	capFailed := capturedomain.Capture{
		ID:         uuid.New(),
		AccountID:  accountID,
		StorageURI: &failedURI,
		Status:     capturedomain.StatusAvailable,
		ExpiresAt:  now.Add(-time.Hour),
	}
	capOK := capturedomain.Capture{
		ID:         uuid.New(),
		AccountID:  accountID,
		StorageURI: &okURI,
		Status:     capturedomain.StatusAvailable,
		ExpiresAt:  now.Add(-time.Hour),
	}

	repository := &serviceRepository{
		cleanupCandidates: []capturedomain.Capture{capFailed, capOK},
	}
	storage := &captureStorageStub{
		deleteErrByURI: map[string]error{
			failedURI: errors.New("storage network timeout"),
		},
	}
	service := newService(repository, storage, fixedPairingCodeGenerator{}, func() time.Time { return now })

	report, err := service.Cleanup(t.Context(), CleanupOptions{BatchSize: 10})
	if err != nil {
		t.Fatalf("cleanup should handle partial failure gracefully: %v", err)
	}

	// capFailed storage failed -> not deleted from DB, marked as deleting
	// capOK storage succeeded -> deleted from DB
	if report.CapturesProcessed != 2 {
		t.Errorf("expected 2 processed, got %d", report.CapturesProcessed)
	}
	if report.CapturesDeleted != 1 {
		t.Errorf("expected 1 capture deleted from DB, got %d", report.CapturesDeleted)
	}
	if report.StorageDeleted != 1 {
		t.Errorf("expected 1 storage deleted, got %d", report.StorageDeleted)
	}
	if len(report.Errors) != 1 {
		t.Errorf("expected 1 recorded error, got %d: %v", len(report.Errors), report.Errors)
	}
	if repository.cleanupRunCompletion == nil || repository.cleanupRunCompletion.Succeeded || repository.cleanupRunCompletion.ErrorCount != 1 {
		t.Fatalf("partial cleanup run completion = %+v, want failed with one error", repository.cleanupRunCompletion)
	}
	if !repository.markedDeleting {
		t.Error("failed capture should have been marked deleting to prevent access and retry later")
	}
	if len(repository.deletedCaptureIDs) != 1 || repository.deletedCaptureIDs[0] != capOK.ID {
		t.Errorf("expected only capOK deleted from DB, got %v", repository.deletedCaptureIDs)
	}
}

func TestCleanup_AdvancesPastFullFailedBatchAndStillDeletesExpiredSessions(t *testing.T) {
	now := time.Date(2026, 10, 9, 18, 0, 0, 0, time.UTC)
	firstURI := "supabase://captures/acc/failed-1.pdf"
	secondURI := "supabase://captures/acc/failed-2.pdf"
	thirdURI := "supabase://captures/acc/ok.pdf"
	repository := &serviceRepository{
		cleanupCandidates: []capturedomain.Capture{
			{ID: uuid.New(), StorageURI: &firstURI, Status: capturedomain.StatusDeleting, ExpiresAt: now.Add(-3 * time.Hour), CreatedAt: now.Add(-27 * time.Hour)},
			{ID: uuid.New(), StorageURI: &secondURI, Status: capturedomain.StatusDeleting, ExpiresAt: now.Add(-2 * time.Hour), CreatedAt: now.Add(-26 * time.Hour)},
			{ID: uuid.New(), StorageURI: &thirdURI, Status: capturedomain.StatusDeleting, ExpiresAt: now.Add(-time.Hour), CreatedAt: now.Add(-25 * time.Hour)},
		},
		expiredSessionsToDelete: 1,
	}
	storage := &captureStorageStub{deleteErrByURI: map[string]error{
		firstURI:  errors.New("storage unavailable"),
		secondURI: errors.New("storage unavailable"),
	}}
	service := newService(repository, storage, fixedPairingCodeGenerator{}, func() time.Time { return now })

	report, err := service.Cleanup(t.Context(), CleanupOptions{BatchSize: 2})
	if err != nil {
		t.Fatalf("cleanup should report partial failures without a fatal error: %v", err)
	}
	if repository.cleanupListCalls != 2 {
		t.Fatalf("cleanup candidate queries = %d, want 2", repository.cleanupListCalls)
	}
	if report.CapturesProcessed != 3 || report.CapturesDeleted != 1 || len(report.Errors) != 2 {
		t.Fatalf("unexpected capture report: %+v", report)
	}
	if report.SessionsDeleted != 1 {
		t.Fatalf("expired sessions deleted = %d, want 1", report.SessionsDeleted)
	}
	if len(storage.deletedURIs) != 3 {
		t.Fatalf("storage delete attempts = %d, want 3", len(storage.deletedURIs))
	}
}

func TestCleanup_DoesNotRetryFailedCaptureWhileOtherCandidatesProgress(t *testing.T) {
	now := time.Date(2026, 10, 9, 18, 0, 0, 0, time.UTC)
	failedURI := "supabase://captures/acc/failed.pdf"
	firstOKURI := "supabase://captures/acc/ok-1.pdf"
	secondOKURI := "supabase://captures/acc/ok-2.pdf"
	failedID := uuid.New()
	repository := &serviceRepository{cleanupCandidates: []capturedomain.Capture{
		{ID: failedID, StorageURI: &failedURI, Status: capturedomain.StatusDeleting},
		{ID: uuid.New(), StorageURI: &firstOKURI, Status: capturedomain.StatusDeleting},
		{ID: uuid.New(), StorageURI: &secondOKURI, Status: capturedomain.StatusDeleting},
	}}
	storage := &captureStorageStub{deleteErrByURI: map[string]error{
		failedURI: errors.New("storage unavailable"),
	}}
	service := newService(repository, storage, fixedPairingCodeGenerator{}, func() time.Time { return now })

	report, err := service.Cleanup(t.Context(), CleanupOptions{BatchSize: 2})
	if err != nil {
		t.Fatalf("cleanup should report partial failures without a fatal error: %v", err)
	}
	if report.CapturesProcessed != 3 || report.CapturesDeleted != 2 || len(report.Errors) != 1 {
		t.Fatalf("unexpected cleanup report: %+v", report)
	}
	failedAttempts := 0
	for _, uri := range storage.deletedURIs {
		if uri == failedURI {
			failedAttempts++
		}
	}
	if failedAttempts != 1 {
		t.Fatalf("failed capture storage attempts = %d, want 1", failedAttempts)
	}
	for _, deletedID := range repository.deletedCaptureIDs {
		if deletedID == failedID {
			t.Fatal("failed capture should remain in the database for the next execution")
		}
	}
}

func TestCleanup_ReportsMarkDeletingFailure(t *testing.T) {
	now := time.Date(2026, 10, 9, 18, 0, 0, 0, time.UTC)
	uri := "supabase://captures/acc/failed.pdf"
	repository := &serviceRepository{
		cleanupCandidates: []capturedomain.Capture{{
			ID: uuid.New(), StorageURI: &uri, Status: capturedomain.StatusAvailable,
		}},
		markDeletingErr: errors.New("database unavailable"),
	}
	storage := &captureStorageStub{deleteErr: errors.New("storage unavailable")}
	service := newService(repository, storage, fixedPairingCodeGenerator{}, func() time.Time { return now })

	report, err := service.Cleanup(t.Context(), CleanupOptions{BatchSize: 10})
	if err != nil {
		t.Fatalf("cleanup should report partial failures without a fatal error: %v", err)
	}
	if len(report.Errors) != 2 {
		t.Fatalf("cleanup errors = %d, want storage and mark-deleting errors: %v", len(report.Errors), report.Errors)
	}
	if !strings.Contains(report.Errors[0].Error(), "mark capture") {
		t.Fatalf("first error does not describe mark-deleting failure: %v", report.Errors[0])
	}
}

func TestCleanup_StorageNotFoundStillDeletesDatabaseRecord(t *testing.T) {
	now := time.Date(2026, 10, 9, 18, 0, 0, 0, time.UTC)
	missingURI := "supabase://captures/acc/already-gone.pdf"
	capMissing := capturedomain.Capture{
		ID:         uuid.New(),
		StorageURI: &missingURI,
		Status:     capturedomain.StatusDeleting,
		ExpiresAt:  now.Add(-time.Hour),
	}

	repository := &serviceRepository{
		cleanupCandidates: []capturedomain.Capture{capMissing},
	}
	storage := &captureStorageStub{
		deleteErr: &apperr.AppError{Kind: apperr.NOT_FOUND, Message: "arquivo não encontrado"},
	}
	service := newService(repository, storage, fixedPairingCodeGenerator{}, func() time.Time { return now })

	report, err := service.Cleanup(t.Context(), CleanupOptions{BatchSize: 10})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if report.CapturesDeleted != 1 {
		t.Errorf("expected capture to be deleted from DB, got %d", report.CapturesDeleted)
	}
	if len(report.Errors) != 0 {
		t.Errorf("expected no errors when storage returns NOT_FOUND, got: %v", report.Errors)
	}
}

func TestCleanup_IdempotentOnRetry(t *testing.T) {
	now := time.Date(2026, 10, 9, 18, 0, 0, 0, time.UTC)
	repository := &serviceRepository{
		cleanupCandidates:       nil,
		expiredSessionsToDelete: 0,
	}
	storage := &captureStorageStub{}
	service := newService(repository, storage, fixedPairingCodeGenerator{}, func() time.Time { return now })

	report, err := service.Cleanup(t.Context(), CleanupOptions{BatchSize: 10})
	if err != nil {
		t.Fatalf("unexpected error on empty run: %v", err)
	}

	if report.CapturesProcessed != 0 || report.CapturesDeleted != 0 || report.SessionsDeleted != 0 {
		t.Errorf("expected 0 across all metrics on empty run, got %+v", report)
	}
}

func TestCleanup_AbortsWhenRunCannotBeRecorded(t *testing.T) {
	now := time.Date(2026, 10, 9, 18, 0, 0, 0, time.UTC)
	repository := &serviceRepository{createCleanupRunErr: errors.New("database unavailable")}
	service := newService(repository, &captureStorageStub{}, fixedPairingCodeGenerator{}, func() time.Time { return now })

	report, err := service.Cleanup(t.Context(), CleanupOptions{})
	if err == nil || report != nil {
		t.Fatalf("cleanup result = (%+v, %v), want nil report and error", report, err)
	}
	if repository.cleanupListCalls != 0 || repository.cleanupRunCompletion != nil {
		t.Fatalf("cleanup continued without observability: list calls=%d completion=%+v", repository.cleanupListCalls, repository.cleanupRunCompletion)
	}
}

func TestCleanup_ReturnsErrorWhenRunCannotBeFinalized(t *testing.T) {
	now := time.Date(2026, 10, 9, 18, 0, 0, 0, time.UTC)
	repository := &serviceRepository{finishCleanupRunErr: errors.New("database unavailable")}
	service := newService(repository, &captureStorageStub{}, fixedPairingCodeGenerator{}, func() time.Time { return now })

	report, err := service.Cleanup(t.Context(), CleanupOptions{})
	if err == nil || report == nil {
		t.Fatalf("cleanup result = (%+v, %v), want report and finalization error", report, err)
	}
	if len(report.Errors) != 1 {
		t.Fatalf("cleanup errors = %d, want finalization error: %v", len(report.Errors), report.Errors)
	}
	if repository.finishCleanupContextErr != nil {
		t.Fatalf("finalization context was unexpectedly canceled: %v", repository.finishCleanupContextErr)
	}
}
