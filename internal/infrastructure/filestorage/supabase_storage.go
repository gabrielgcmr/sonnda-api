// internal/infrastructure/filestorage/supabase_storage.go
package filestorage

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/gabrielgcmr/sonnda/internal/features/capture"
	"github.com/gabrielgcmr/sonnda/internal/kernel/apperr"
)

const (
	// DefaultMaxFileSize define o limite padrão de upload (5 MiB).
	DefaultMaxFileSize  int64 = 5 * 1024 * 1024
	maxResponseBodySize       = 64 * 1024

	// URIScheme define o prefixo de protocolo para arquivos no Supabase Storage.
	URIScheme = "supabase://"
)

var (
	tokenQueryRegex = regexp.MustCompile(`(?i)([?&]token=)[^&\s]+`)
	validBucketName = regexp.MustCompile(`^[a-z0-9][a-z0-9-_]{1,62}[a-z0-9]$`)
)

var errUploadSizeExceeded = errors.New("upload size exceeded")

// SupabaseClientConfig contém os parâmetros de configuração do cliente Supabase Storage.
type SupabaseClientConfig struct {
	ProjectURL string
	SecretKey  string
	HTTPClient *http.Client
}

// SupabaseStorageClient gerencia a comunicação HTTP com a API REST do Supabase Storage.
type SupabaseStorageClient struct {
	baseURL    string
	secretKey  string
	httpClient *http.Client
}

// NewSupabaseStorageClient inicializa e valida o cliente base do Supabase Storage.
func NewSupabaseStorageClient(cfg SupabaseClientConfig) (*SupabaseStorageClient, error) {
	trimmedURL := strings.TrimSpace(cfg.ProjectURL)
	if trimmedURL == "" {
		return nil, &apperr.AppError{
			Kind:    apperr.REQUIRED_FIELD_MISSING,
			Message: "URL do projeto Supabase é obrigatória",
		}
	}

	parsedURL, err := url.Parse(trimmedURL)
	if err != nil || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") || parsedURL.Host == "" {
		return nil, &apperr.AppError{
			Kind:    apperr.INVALID_FIELD_FORMAT,
			Message: "URL do projeto Supabase inválida",
			Cause:   err,
		}
	}

	secretKey := strings.TrimSpace(cfg.SecretKey)
	if secretKey == "" {
		return nil, &apperr.AppError{
			Kind:    apperr.REQUIRED_FIELD_MISSING,
			Message: "chave secreta do Supabase é obrigatória",
		}
	}

	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{
			Timeout: 30 * time.Second,
		}
	}

	// Normaliza baseURL removendo barras finais
	baseURL := strings.TrimRight(fmt.Sprintf("%s://%s%s", parsedURL.Scheme, parsedURL.Host, parsedURL.Path), "/")

	return &SupabaseStorageClient{
		baseURL:    baseURL,
		secretKey:  secretKey,
		httpClient: httpClient,
	}, nil
}

// BucketConfig define parâmetros para um adapter de bucket específico.
type BucketConfig struct {
	BucketName  string
	MaxFileSize int64
}

// SupabaseBucketStorage implementa operações de armazenamento vinculadas a um bucket.
type SupabaseBucketStorage struct {
	client      *SupabaseStorageClient
	bucketName  string
	maxFileSize int64
}

var _ capture.FileStorage = (*SupabaseBucketStorage)(nil)

// ForBucket cria um adapter SupabaseBucketStorage vinculado ao bucket configurado.
func (c *SupabaseStorageClient) ForBucket(cfg BucketConfig) (*SupabaseBucketStorage, error) {
	bucketName := strings.TrimSpace(cfg.BucketName)
	if bucketName == "" {
		return nil, &apperr.AppError{
			Kind:    apperr.REQUIRED_FIELD_MISSING,
			Message: "nome do bucket é obrigatório",
		}
	}

	if !validBucketName.MatchString(bucketName) {
		return nil, &apperr.AppError{
			Kind:    apperr.INVALID_FIELD_FORMAT,
			Message: "nome do bucket inválido",
		}
	}

	maxFileSize := cfg.MaxFileSize
	if maxFileSize <= 0 {
		maxFileSize = DefaultMaxFileSize
	}

	return &SupabaseBucketStorage{
		client:      c,
		bucketName:  bucketName,
		maxFileSize: maxFileSize,
	}, nil
}

// BucketName retorna o nome do bucket configurado neste adapter.
func (s *SupabaseBucketStorage) BucketName() string {
	return s.bucketName
}

// MaxFileSize retorna o tamanho máximo de arquivo permitido (em bytes).
func (s *SupabaseBucketStorage) MaxFileSize() int64 {
	return s.maxFileSize
}

// ObjectURI returns the canonical private URI for an object in this bucket.
func (s *SupabaseBucketStorage) ObjectURI(objectName string) (string, error) {
	cleanObjectName, err := validateObjectPath(objectName)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s%s/%s", URIScheme, s.bucketName, cleanObjectName), nil
}

// Upload envia um arquivo para o bucket via POST /storage/v1/object/<bucket>/<objectName> sem upsert.
func (s *SupabaseBucketStorage) Upload(
	ctx context.Context,
	file io.Reader,
	objectName string,
	contentType string,
) (string, error) {
	if file == nil {
		return "", &apperr.AppError{
			Kind:    apperr.REQUIRED_FIELD_MISSING,
			Message: "arquivo para envio é obrigatório",
		}
	}

	cleanObjectName, err := validateObjectPath(objectName)
	if err != nil {
		return "", err
	}

	if sizer, ok := file.(interface{ Size() int64 }); ok {
		if sizer.Size() > s.maxFileSize {
			return "", &apperr.AppError{
				Kind:    apperr.UPLOAD_SIZE_EXCEEDED,
				Message: fmt.Sprintf("tamanho do arquivo excede o limite permitido (%d bytes)", s.maxFileSize),
				Cause:   errUploadSizeExceeded,
			}
		}
	} else if lenner, ok := file.(interface{ Len() int }); ok {
		if int64(lenner.Len()) > s.maxFileSize {
			return "", &apperr.AppError{
				Kind:    apperr.UPLOAD_SIZE_EXCEEDED,
				Message: fmt.Sprintf("tamanho do arquivo excede o limite permitido (%d bytes)", s.maxFileSize),
				Cause:   errUploadSizeExceeded,
			}
		}
	}

	if strings.TrimSpace(contentType) == "" {
		contentType = "application/octet-stream"
	}

	uploadURL := fmt.Sprintf("%s/storage/v1/object/%s/%s", s.client.baseURL, s.bucketName, escapeObjectPath(cleanObjectName))

	countingR := &limitedCountingReader{
		reader: file,
		limit:  s.maxFileSize,
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, uploadURL, countingR)
	if err != nil {
		return "", wrapClientError("falha ao criar requisição de envio", "supabase.upload.new_request", err, s.client.secretKey)
	}

	s.setAuthHeaders(req)
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("x-upsert", "false")

	resp, err := s.client.httpClient.Do(req)
	if err != nil {
		if countingR.exceeded || errors.Is(err, errUploadSizeExceeded) {
			return "", &apperr.AppError{
				Kind:    apperr.UPLOAD_SIZE_EXCEEDED,
				Message: fmt.Sprintf("tamanho do arquivo excede o limite permitido (%d bytes)", s.maxFileSize),
				Cause:   errUploadSizeExceeded,
			}
		}
		return "", wrapClientError("falha ao enviar arquivo", "supabase.upload.do", err, s.client.secretKey)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, readErr := s.readResponseBody(resp.Body, "supabase.upload.read_response")
		if readErr != nil {
			return "", readErr
		}
		return "", s.mapHTTPResponseError(resp.StatusCode, respBody, "supabase.upload")
	}

	return s.ObjectURI(cleanObjectName)
}

// Open abre um arquivo para streaming autenticado via GET /storage/v1/object/authenticated/<bucket>/<objectPath>.
// O chamador é responsável por fechar o io.ReadCloser retornado.
func (s *SupabaseBucketStorage) Open(ctx context.Context, uri string) (io.ReadCloser, error) {
	objectPath, err := s.parseAndValidateURI(uri)
	if err != nil {
		return nil, err
	}

	openURL := fmt.Sprintf("%s/storage/v1/object/authenticated/%s/%s", s.client.baseURL, s.bucketName, escapeObjectPath(objectPath))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, openURL, nil)
	if err != nil {
		return nil, wrapClientError("falha ao criar requisição de leitura", "supabase.open.new_request", err, s.client.secretKey)
	}

	s.setAuthHeaders(req)

	resp, err := s.client.httpClient.Do(req)
	if err != nil {
		return nil, wrapClientError("falha ao abrir arquivo para leitura", "supabase.open.do", err, s.client.secretKey)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer resp.Body.Close()
		respBody, readErr := s.readResponseBody(resp.Body, "supabase.open.read_response")
		if readErr != nil {
			return nil, readErr
		}
		return nil, s.mapHTTPResponseError(resp.StatusCode, respBody, "supabase.open")
	}

	return resp.Body, nil
}

// Delete remove um arquivo do bucket via DELETE /storage/v1/object/<bucket> com prefixes.
// É estritamente idempotente quando o arquivo já não existe.
func (s *SupabaseBucketStorage) Delete(ctx context.Context, uri string) error {
	objectPath, err := s.parseAndValidateURI(uri)
	if err != nil {
		return err
	}

	deleteURL := fmt.Sprintf("%s/storage/v1/object/%s", s.client.baseURL, s.bucketName)

	payload := struct {
		Prefixes []string `json:"prefixes"`
	}{
		Prefixes: []string{objectPath},
	}

	jsonBytes, err := json.Marshal(payload)
	if err != nil {
		return wrapClientError("falha ao serializar payload de exclusão", "supabase.delete.marshal", err, s.client.secretKey)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, deleteURL, bytes.NewReader(jsonBytes))
	if err != nil {
		return wrapClientError("falha ao criar requisição de exclusão", "supabase.delete.new_request", err, s.client.secretKey)
	}

	s.setAuthHeaders(req)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.httpClient.Do(req)
	if err != nil {
		return wrapClientError("falha ao excluir arquivo", "supabase.delete.do", err, s.client.secretKey)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, readErr := s.readResponseBody(resp.Body, "supabase.delete.read_response")
		if readErr != nil {
			return readErr
		}
		return s.mapHTTPResponseError(resp.StatusCode, respBody, "supabase.delete")
	}

	return nil
}

// GetSignedURL gera uma URL assinada temporária para download via POST /storage/v1/object/sign/<bucket>/<objectPath>.
func (s *SupabaseBucketStorage) GetSignedURL(
	ctx context.Context,
	uri string,
	expiresIn time.Duration,
) (string, error) {
	if expiresIn <= 0 {
		return "", &apperr.AppError{
			Kind:    apperr.VALIDATION_FAILED,
			Message: "tempo de expiração deve ser positivo",
		}
	}

	objectPath, err := s.parseAndValidateURI(uri)
	if err != nil {
		return "", err
	}

	signURL := fmt.Sprintf("%s/storage/v1/object/sign/%s/%s", s.client.baseURL, s.bucketName, escapeObjectPath(objectPath))

	seconds := int(expiresIn.Seconds())
	if seconds <= 0 {
		seconds = 1
	}

	payload := struct {
		ExpiresIn int `json:"expiresIn"`
	}{
		ExpiresIn: seconds,
	}

	jsonBytes, err := json.Marshal(payload)
	if err != nil {
		return "", wrapClientError("falha ao serializar payload de assinatura", "supabase.signed_url.marshal", err, s.client.secretKey)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, signURL, bytes.NewReader(jsonBytes))
	if err != nil {
		return "", wrapClientError("falha ao criar requisição de URL assinada", "supabase.signed_url.new_request", err, s.client.secretKey)
	}

	s.setAuthHeaders(req)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.httpClient.Do(req)
	if err != nil {
		return "", wrapClientError("falha ao gerar URL assinada", "supabase.signed_url.do", err, s.client.secretKey)
	}
	defer resp.Body.Close()

	respBody, readErr := s.readResponseBody(resp.Body, "supabase.signed_url.read_response")
	if readErr != nil {
		return "", readErr
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", s.mapHTTPResponseError(resp.StatusCode, respBody, "supabase.signed_url")
	}

	var signResponse struct {
		SignedURL string `json:"signedURL"`
		SignedUrl string `json:"signedUrl"`
	}
	if err := json.Unmarshal(respBody, &signResponse); err != nil {
		return "", &apperr.AppError{
			Kind:    apperr.INFRA_STORAGE_ERROR,
			Message: "resposta inválida do serviço de armazenamento",
			Cause:   sanitizeError(fmt.Errorf("supabase.signed_url.unmarshal: %w", err), s.client.secretKey),
		}
	}

	rawSignedURL := signResponse.SignedURL
	if rawSignedURL == "" {
		rawSignedURL = signResponse.SignedUrl
	}
	if strings.TrimSpace(rawSignedURL) == "" {
		return "", &apperr.AppError{
			Kind:    apperr.INFRA_STORAGE_ERROR,
			Message: "URL assinada ausente na resposta do serviço de armazenamento",
		}
	}

	return s.buildAbsoluteSignedURL(rawSignedURL), nil
}

func (s *SupabaseBucketStorage) setAuthHeaders(req *http.Request) {
	req.Header.Set("apikey", s.client.secretKey)
}

func (s *SupabaseBucketStorage) readResponseBody(body io.Reader, op string) ([]byte, error) {
	responseBody, err := io.ReadAll(io.LimitReader(body, maxResponseBodySize+1))
	if err != nil {
		return nil, wrapClientError("falha ao ler resposta do serviço de armazenamento", op, err, s.client.secretKey)
	}
	if len(responseBody) > maxResponseBodySize {
		return nil, &apperr.AppError{
			Kind:    apperr.INFRA_STORAGE_ERROR,
			Message: "resposta inválida do serviço de armazenamento",
			Cause:   fmt.Errorf("%s: response body exceeds %d bytes", op, maxResponseBodySize),
		}
	}
	return responseBody, nil
}

func (s *SupabaseBucketStorage) parseAndValidateURI(rawURI string) (string, error) {
	trimmed := strings.TrimSpace(rawURI)
	if !strings.HasPrefix(trimmed, URIScheme) {
		return "", &apperr.AppError{
			Kind:    apperr.VALIDATION_FAILED,
			Message: "esquema de URI inválido: esperado prefixo " + URIScheme,
		}
	}

	remainder := trimmed[len(URIScheme):]
	slashIdx := strings.Index(remainder, "/")
	if slashIdx <= 0 {
		return "", &apperr.AppError{
			Kind:    apperr.VALIDATION_FAILED,
			Message: "URI inválida: caminho do objeto ausente",
		}
	}

	bucket := remainder[:slashIdx]
	if bucket != s.bucketName {
		return "", &apperr.AppError{
			Kind:    apperr.VALIDATION_FAILED,
			Message: fmt.Sprintf("bucket na URI (%s) diverge do bucket configurado (%s)", bucket, s.bucketName),
		}
	}

	objectPath := remainder[slashIdx+1:]
	cleanPath, err := validateObjectPath(objectPath)
	if err != nil {
		return "", err
	}

	return cleanPath, nil
}

func (s *SupabaseBucketStorage) buildAbsoluteSignedURL(signedURLPath string) string {
	if strings.HasPrefix(signedURLPath, "http://") || strings.HasPrefix(signedURLPath, "https://") {
		return signedURLPath
	}

	cleanPath := strings.TrimPrefix(signedURLPath, "/")
	if strings.HasPrefix(cleanPath, "storage/v1/") {
		return fmt.Sprintf("%s/%s", s.client.baseURL, cleanPath)
	}

	return fmt.Sprintf("%s/storage/v1/%s", s.client.baseURL, cleanPath)
}

func validateObjectPath(objectPath string) (string, error) {
	trimmed := strings.TrimSpace(objectPath)
	if trimmed == "" {
		return "", &apperr.AppError{
			Kind:    apperr.VALIDATION_FAILED,
			Message: "caminho do arquivo não pode ser vazio",
		}
	}

	if strings.HasPrefix(trimmed, "/") || strings.HasSuffix(trimmed, "/") {
		return "", &apperr.AppError{
			Kind:    apperr.VALIDATION_FAILED,
			Message: "caminho do arquivo não pode iniciar ou terminar com barra",
		}
	}

	segments := strings.Split(trimmed, "/")
	for _, seg := range segments {
		segTrimmed := strings.TrimSpace(seg)
		if segTrimmed == "" {
			return "", &apperr.AppError{
				Kind:    apperr.VALIDATION_FAILED,
				Message: "caminho do arquivo contém segmento vazio",
			}
		}
		if segTrimmed == "." || segTrimmed == ".." {
			return "", &apperr.AppError{
				Kind:    apperr.VALIDATION_FAILED,
				Message: "caminho do arquivo contém caracteres de navegação inválidos",
			}
		}
		if strings.ContainsAny(segTrimmed, "\x00\r\n") {
			return "", &apperr.AppError{
				Kind:    apperr.VALIDATION_FAILED,
				Message: "caminho do arquivo contém caracteres de controle inválidos",
			}
		}
	}

	return trimmed, nil
}

func escapeObjectPath(objectPath string) string {
	segments := strings.Split(objectPath, "/")
	escaped := make([]string, len(segments))
	for i, seg := range segments {
		escaped[i] = url.PathEscape(seg)
	}
	return strings.Join(escaped, "/")
}

type supabaseErrorPayload struct {
	StatusCode string `json:"statusCode"`
	Error      string `json:"error"`
	Message    string `json:"message"`
	Code       string `json:"code"`
}

func (s *SupabaseBucketStorage) mapHTTPResponseError(statusCode int, body []byte, op string) error {
	var payload supabaseErrorPayload
	_ = json.Unmarshal(body, &payload)

	errStatus := strings.TrimSpace(payload.StatusCode)
	errCode := strings.TrimSpace(payload.Code)
	errName := strings.TrimSpace(payload.Error)
	errMsg := strings.TrimSpace(payload.Message)

	sanitizedDetail := sanitizeString(fmt.Sprintf("%s: http_status=%d status_code=%s code=%s error=%s msg=%s",
		op, statusCode, errStatus, errCode, errName, errMsg), s.client.secretKey)

	cause := errors.New(sanitizedDetail)

	// Arquivo ausente (Supabase Storage pode retornar 404 direto ou 400 com NoSuchKey / not_found)
	if statusCode == http.StatusNotFound || errStatus == "404" ||
		strings.EqualFold(errCode, "NoSuchKey") || strings.EqualFold(errName, "not_found") ||
		strings.EqualFold(errMsg, "Object not found") {
		return &apperr.AppError{
			Kind:    apperr.NOT_FOUND,
			Message: "arquivo não encontrado",
			Cause:   cause,
		}
	}

	// Conflito (Supabase retorna 409 ou 400 com KeyAlreadyExists / Duplicate)
	if statusCode == http.StatusConflict || errStatus == "409" ||
		strings.EqualFold(errCode, "KeyAlreadyExists") || strings.EqualFold(errName, "Duplicate") ||
		strings.Contains(strings.ToLower(errMsg), "already exists") {
		return &apperr.AppError{
			Kind:    apperr.RESOURCE_ALREADY_EXISTS,
			Message: "arquivo já existe",
			Cause:   cause,
		}
	}

	// Autenticação
	if statusCode == http.StatusUnauthorized || errStatus == "401" {
		return &apperr.AppError{
			Kind:    apperr.INFRA_AUTHENTICATION_ERROR,
			Message: "falha de autenticação com o serviço de armazenamento",
			Cause:   cause,
		}
	}

	// Uma resposta 403 do Storage indica falha de credencial ou configuração do backend,
	// não uma decisão de autorização do usuário da API.
	if statusCode == http.StatusForbidden || errStatus == "403" ||
		strings.EqualFold(errCode, "AccessDenied") || strings.EqualFold(errName, "Unauthorized") {
		return &apperr.AppError{
			Kind:    apperr.INFRA_AUTHENTICATION_ERROR,
			Message: "falha de autenticação com o serviço de armazenamento",
			Cause:   cause,
		}
	}

	// Tamanho excedido
	if statusCode == http.StatusRequestEntityTooLarge || errStatus == "413" ||
		strings.Contains(strings.ToLower(errMsg), "exceeded the maximum allowed size") ||
		strings.Contains(strings.ToLower(errMsg), "payload too large") {
		return &apperr.AppError{
			Kind:    apperr.UPLOAD_SIZE_EXCEEDED,
			Message: fmt.Sprintf("tamanho do arquivo excede o limite permitido (%d bytes)", s.maxFileSize),
			Cause:   cause,
		}
	}

	// 5xx ou outros
	if statusCode >= 500 {
		return &apperr.AppError{
			Kind:    apperr.INFRA_STORAGE_ERROR,
			Message: "falha interna no serviço de armazenamento",
			Cause:   cause,
		}
	}

	return &apperr.AppError{
		Kind:    apperr.INFRA_STORAGE_ERROR,
		Message: "falha na operação com o serviço de armazenamento",
		Cause:   cause,
	}
}

func wrapClientError(message, op string, err error, secretKey string) error {
	if err == nil {
		return nil
	}

	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return &apperr.AppError{
			Kind:    apperr.INFRA_TIMEOUT,
			Message: "tempo limite excedido",
			Cause:   sanitizeError(fmt.Errorf("%s: %w", op, err), secretKey),
		}
	}

	return &apperr.AppError{
		Kind:    apperr.INFRA_STORAGE_ERROR,
		Message: message,
		Cause:   sanitizeError(fmt.Errorf("%s: %w", op, err), secretKey),
	}
}

func sanitizeError(err error, secretKey string) error {
	if err == nil {
		return nil
	}
	return errors.New(sanitizeString(err.Error(), secretKey))
}

func sanitizeString(raw, secretKey string) string {
	result := raw
	if secretKey != "" {
		result = strings.ReplaceAll(result, secretKey, "[REDACTED]")
	}
	result = tokenQueryRegex.ReplaceAllString(result, "${1}[REDACTED]")
	return result
}

type limitedCountingReader struct {
	reader   io.Reader
	limit    int64
	read     int64
	exceeded bool
}

func (l *limitedCountingReader) Read(p []byte) (int, error) {
	if l.read > l.limit {
		l.exceeded = true
		return 0, errUploadSizeExceeded
	}

	toRead := int64(len(p))
	if l.limit-l.read+1 < toRead {
		toRead = l.limit - l.read + 1
	}

	n, err := l.reader.Read(p[:toRead])
	l.read += int64(n)

	if l.read > l.limit {
		l.exceeded = true
		return n, errUploadSizeExceeded
	}

	return n, err
}
