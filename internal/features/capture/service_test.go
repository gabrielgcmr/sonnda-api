// internal/features/capture/service_test.go
package capture

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"io"
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
	transactionCalls  int
	revokedAccount    uuid.UUID
	revokedAt         time.Time
	created           *capturedomain.Session
	current           *capturedomain.Session
	currentAccount    uuid.UUID
	touchedSession    uuid.UUID
	touchedAccount    uuid.UUID
	touchedAt         time.Time
	revokedSession    uuid.UUID
	err               error
	operations        []string
	claimed           *capturedomain.Session
	pairingHash       []byte
	claim             capturedomain.SessionClaim
	authenticated     *capturedomain.Session
	authenticatedHash []byte
	touchedMobile     uuid.UUID
	createdCapture    *capturedomain.Capture
	availableCapture  *capturedomain.Capture
	markedDeleting    bool
	setAvailableErr   error
	markDeletingErr   error
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

func (r *serviceRepository) MarkCaptureDeleting(_ context.Context, _ uuid.UUID, _ time.Time) (*capturedomain.Capture, error) {
	r.operations = append(r.operations, "deleting")
	r.markedDeleting = true
	if r.markDeletingErr != nil {
		return nil, r.markDeletingErr
	}
	if r.createdCapture == nil {
		return nil, ErrCaptureNotFound
	}
	item := *r.createdCapture
	item.Status = capturedomain.StatusDeleting
	return &item, nil
}

type captureStorageStub struct {
	operations  *[]string
	uploaded    []byte
	objectName  string
	contentType string
	deletedURI  string
	err         error
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
	s.deletedURI = uri
	return nil
}

func (*captureStorageStub) GetSignedURL(context.Context, string, time.Duration) (string, error) {
	return "", errors.New("not implemented")
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

type fixedReader struct {
	value []byte
}

func (r *fixedReader) Read(target []byte) (int, error) {
	return copy(target, r.value), nil
}
