package service

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"io"
	"mime"
	"mime/multipart"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

const (
	DefaultRequestAuditPreviewBytes = 64 * 1024
	DefaultRequestAuditMaxBodyBytes = 5 * 1024 * 1024
	DefaultRequestAuditMaxRows      = 50000
)

type RequestAuditBodyRole string

const (
	RequestAuditBodyRoleRequest  RequestAuditBodyRole = "request"
	RequestAuditBodyRoleResponse RequestAuditBodyRole = "response"
)

type RequestAuditBodyKind string

const (
	RequestAuditBodyKindJSON   RequestAuditBodyKind = "json"
	RequestAuditBodyKindSSE    RequestAuditBodyKind = "sse"
	RequestAuditBodyKindText   RequestAuditBodyKind = "text"
	RequestAuditBodyKindImage  RequestAuditBodyKind = "image"
	RequestAuditBodyKindAudio  RequestAuditBodyKind = "audio"
	RequestAuditBodyKindBinary RequestAuditBodyKind = "binary"
)

type RequestAuditBodyEncoding string

const (
	RequestAuditBodyEncodingText   RequestAuditBodyEncoding = "text"
	RequestAuditBodyEncodingBase64 RequestAuditBodyEncoding = "base64"
)

var requestAuditRedactKeys = map[string]struct{}{
	"authorization":  {},
	"api_key":        {},
	"access_token":   {},
	"refresh_token":  {},
	"cookie":         {},
	"password":       {},
	"client_secret":  {},
	"x-api-key":      {},
	"x-goog-api-key": {},
}

type RequestAuditLog struct {
	ID                  int64     `json:"id"`
	RequestID           string    `json:"request_id"`
	UserID              int64     `json:"user_id"`
	APIKeyID            int64     `json:"api_key_id"`
	AccountID           *int64    `json:"account_id,omitempty"`
	GroupID             *int64    `json:"group_id,omitempty"`
	Platform            string    `json:"platform"`
	Model               string    `json:"model"`
	Endpoint            string    `json:"endpoint"`
	Stream              bool      `json:"stream"`
	StatusCode          int       `json:"status_code"`
	Success             bool      `json:"success"`
	DurationMS          int       `json:"duration_ms"`
	RequestBody         string    `json:"request_body"`
	ResponseBody        string    `json:"response_body"`
	ErrorBody           string    `json:"error_body"`
	RequestContentType  string    `json:"request_content_type"`
	ResponseContentType string    `json:"response_content_type"`
	RequestBodyKind     string    `json:"request_body_kind"`
	ResponseBodyKind    string    `json:"response_body_kind"`
	RequestTruncated    bool      `json:"request_truncated"`
	ResponseTruncated   bool      `json:"response_truncated"`
	RequestBytes        int       `json:"request_bytes"`
	ResponseBytes       int       `json:"response_bytes"`
	CreatedAt           time.Time `json:"created_at"`
	UserEmail           *string   `json:"user_email,omitempty"`
	APIKeyName          *string   `json:"api_key_name,omitempty"`
	AccountName         *string   `json:"account_name,omitempty"`
	GroupName           *string   `json:"group_name,omitempty"`
}

type RequestAuditBody struct {
	ID              int64                    `json:"id"`
	AuditLogID      int64                    `json:"audit_log_id"`
	Role            RequestAuditBodyRole     `json:"role"`
	BodyKind        RequestAuditBodyKind     `json:"body_kind"`
	ContentType     string                   `json:"content_type"`
	ContentEncoding RequestAuditBodyEncoding `json:"content_encoding"`
	Body            string                   `json:"body"`
	Truncated       bool                     `json:"truncated"`
	SizeBytes       int                      `json:"size_bytes"`
	Previewable     bool                     `json:"previewable"`
	MediaMIME       string                   `json:"media_mime"`
	CreatedAt       time.Time                `json:"created_at"`
}

type RequestAuditRecordInput struct {
	RequestID           string
	UserID              int64
	APIKeyID            int64
	AccountID           *int64
	GroupID             *int64
	Platform            string
	Model               string
	Endpoint            string
	Stream              bool
	StatusCode          int
	Success             bool
	Duration            time.Duration
	RequestBody         []byte
	ResponseBody        []byte
	RequestContentType  string
	ResponseContentType string
	RequestBytes        int
	ResponseBytes       int
	CreatedAt           time.Time
}

type RequestAuditListParams struct {
	pagination.PaginationParams
	RequestID  string
	UserID     int64
	APIKeyID   int64
	Model      string
	StatusCode int
	Success    *bool
	StartTime  *time.Time
	EndTime    *time.Time
}

type RequestAuditRepository interface {
	Create(ctx context.Context, log *RequestAuditLog, bodies []RequestAuditBody) error
	List(ctx context.Context, params RequestAuditListParams) ([]RequestAuditLog, int64, error)
	GetByID(ctx context.Context, id int64) (*RequestAuditLog, error)
	GetByRequestID(ctx context.Context, requestID string) (*RequestAuditLog, error)
	GetBody(ctx context.Context, auditLogID int64, role RequestAuditBodyRole) (*RequestAuditBody, error)
	CleanupOverflow(ctx context.Context, maxRows int) (int64, error)
}

type RequestAuditOptions struct {
	MaxBodyBytes    int
	MaxPreviewBytes int
	MaxRows         int
}

type RequestAuditService struct {
	repo            RequestAuditRepository
	maxBodyBytes    int
	maxPreviewBytes int
	maxRows         int
}

func NewRequestAuditService(repo RequestAuditRepository, opts ...RequestAuditOptions) *RequestAuditService {
	maxBodyBytes := DefaultRequestAuditMaxBodyBytes
	maxPreviewBytes := DefaultRequestAuditPreviewBytes
	maxRows := DefaultRequestAuditMaxRows
	if len(opts) > 0 {
		if opts[0].MaxBodyBytes > 0 {
			maxBodyBytes = opts[0].MaxBodyBytes
		}
		if opts[0].MaxPreviewBytes > 0 {
			maxPreviewBytes = opts[0].MaxPreviewBytes
		}
		if opts[0].MaxRows > 0 {
			maxRows = opts[0].MaxRows
		}
	}
	return &RequestAuditService{repo: repo, maxBodyBytes: maxBodyBytes, maxPreviewBytes: maxPreviewBytes, maxRows: maxRows}
}

func (s *RequestAuditService) Record(ctx context.Context, input RequestAuditRecordInput) error {
	if s == nil || s.repo == nil {
		return nil
	}
	createdAt := input.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now()
	}
	requestAuditBody := BuildRequestAuditBody(RequestAuditBodyRoleRequest, input.RequestContentType, input.RequestBody, s.maxBodyBytes)
	responseAuditBody := BuildRequestAuditBody(RequestAuditBodyRoleResponse, input.ResponseContentType, input.ResponseBody, s.maxBodyBytes)
	requestBody, requestPreviewTruncated := RequestAuditBodyPreview(requestAuditBody, s.maxPreviewBytes)
	responseBody, responsePreviewTruncated := RequestAuditBodyPreview(responseAuditBody, s.maxPreviewBytes)
	requestTruncated := requestAuditBody.Truncated || requestPreviewTruncated
	responseTruncated := responseAuditBody.Truncated || responsePreviewTruncated
	requestBytes := requestAuditBody.SizeBytes
	responseBytes := responseAuditBody.SizeBytes
	if input.RequestBytes > 0 {
		requestBytes = input.RequestBytes
	}
	if input.ResponseBytes > 0 {
		responseBytes = input.ResponseBytes
	}
	errorBody := ""
	if !input.Success {
		errorBody = responseBody
	}
	log := &RequestAuditLog{
		RequestID:           strings.TrimSpace(input.RequestID),
		UserID:              input.UserID,
		APIKeyID:            input.APIKeyID,
		AccountID:           input.AccountID,
		GroupID:             input.GroupID,
		Platform:            strings.TrimSpace(input.Platform),
		Model:               strings.TrimSpace(input.Model),
		Endpoint:            strings.TrimSpace(input.Endpoint),
		Stream:              input.Stream,
		StatusCode:          input.StatusCode,
		Success:             input.Success,
		DurationMS:          int(input.Duration.Milliseconds()),
		RequestBody:         requestBody,
		ResponseBody:        responseBody,
		ErrorBody:           errorBody,
		RequestContentType:  requestAuditBody.ContentType,
		ResponseContentType: responseAuditBody.ContentType,
		RequestBodyKind:     string(requestAuditBody.BodyKind),
		ResponseBodyKind:    string(responseAuditBody.BodyKind),
		RequestTruncated:    requestTruncated,
		ResponseTruncated:   responseTruncated,
		RequestBytes:        requestBytes,
		ResponseBytes:       responseBytes,
		CreatedAt:           createdAt,
	}
	bodies := make([]RequestAuditBody, 0, 2)
	if requestAuditBody.SizeBytes > 0 {
		bodies = append(bodies, requestAuditBody)
	}
	if responseAuditBody.SizeBytes > 0 {
		bodies = append(bodies, responseAuditBody)
	}
	if err := s.repo.Create(ctx, log, bodies); err != nil {
		return err
	}
	if s.maxRows > 0 {
		_, err := s.repo.CleanupOverflow(ctx, s.maxRows)
		return err
	}
	return nil
}

func (s *RequestAuditService) List(ctx context.Context, params RequestAuditListParams) ([]RequestAuditLog, int64, error) {
	if params.Page <= 0 {
		params.Page = 1
	}
	if params.PageSize <= 0 {
		params.PageSize = 20
	}
	if params.PageSize > 100 {
		params.PageSize = 100
	}
	return s.repo.List(ctx, params)
}

func (s *RequestAuditService) GetByID(ctx context.Context, id int64) (*RequestAuditLog, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *RequestAuditService) GetByRequestID(ctx context.Context, requestID string) (*RequestAuditLog, error) {
	return s.repo.GetByRequestID(ctx, strings.TrimSpace(requestID))
}

func (s *RequestAuditService) GetByUsageLog(ctx context.Context, usage UsageLog) (*RequestAuditLog, error) {
	if s == nil || s.repo == nil {
		return nil, sql.ErrNoRows
	}
	for _, candidate := range auditRequestIDCandidates(usage.RequestID) {
		log, err := s.repo.GetByRequestID(ctx, candidate)
		if err == nil && log != nil {
			return log, nil
		}
		if err != nil && err != sql.ErrNoRows {
			return nil, err
		}
	}

	if !strings.HasPrefix(strings.TrimSpace(usage.RequestID), "generated:") {
		return nil, sql.ErrNoRows
	}
	start := usage.CreatedAt.Add(-2 * time.Minute)
	end := usage.CreatedAt.Add(2 * time.Minute)
	if usage.CreatedAt.IsZero() {
		now := time.Now()
		start = now.Add(-10 * time.Minute)
		end = now.Add(10 * time.Minute)
	}
	logs, _, err := s.repo.List(ctx, RequestAuditListParams{
		UserID:    usage.UserID,
		APIKeyID:  usage.APIKeyID,
		Model:     strings.TrimSpace(usage.Model),
		StartTime: &start,
		EndTime:   &end,
		PaginationParams: pagination.PaginationParams{
			Page:      1,
			PageSize:  20,
			SortBy:    "created_at",
			SortOrder: "desc",
		},
	})
	if err != nil {
		return nil, err
	}
	endpoint := strings.TrimSpace(derefString(usage.InboundEndpoint))
	model := strings.TrimSpace(usage.Model)
	for i := range logs {
		if model != "" && strings.TrimSpace(logs[i].Model) != model {
			continue
		}
		if endpoint == "" || strings.TrimSpace(logs[i].Endpoint) == endpoint {
			return &logs[i], nil
		}
	}
	return nil, sql.ErrNoRows
}

func (s *RequestAuditService) GetBody(ctx context.Context, auditLogID int64, role RequestAuditBodyRole) (*RequestAuditBody, error) {
	return s.repo.GetBody(ctx, auditLogID, role)
}

func BuildRequestAuditBody(role RequestAuditBodyRole, contentType string, body []byte, maxBytes int) RequestAuditBody {
	originalContentType := strings.TrimSpace(contentType)
	contentType = normalizeAuditContentType(originalContentType)
	if multipartBody, ok := buildMultipartAuditBody(role, originalContentType, contentType, body, maxBytes); ok {
		return multipartBody
	}
	mediaType := auditMediaType(contentType)
	kind := classifyRequestAuditBody(mediaType, body)
	encoding := RequestAuditBodyEncodingText
	previewable := true
	mediaMIME := ""
	truncated := false
	size := len(body)
	stored := body
	if maxBytes > 0 && len(stored) > maxBytes {
		stored = stored[:maxBytes]
		truncated = true
	}

	switch kind {
	case RequestAuditBodyKindJSON:
		stored = redactRequestAuditJSON(body)
		if maxBytes > 0 && len(stored) > maxBytes {
			stored = stored[:maxBytes]
			truncated = true
		}
	case RequestAuditBodyKindImage, RequestAuditBodyKindAudio, RequestAuditBodyKindBinary:
		encoding = RequestAuditBodyEncodingBase64
		mediaMIME = mediaType
		if kind == RequestAuditBodyKindBinary {
			previewable = false
		}
	default:
		if !utf8.Valid(stored) {
			kind = RequestAuditBodyKindBinary
			encoding = RequestAuditBodyEncodingBase64
			previewable = false
		}
	}

	bodyText := string(stored)
	if encoding == RequestAuditBodyEncodingBase64 {
		bodyText = base64.StdEncoding.EncodeToString(stored)
	}
	return RequestAuditBody{
		Role:            role,
		BodyKind:        kind,
		ContentType:     contentType,
		ContentEncoding: encoding,
		Body:            bodyText,
		Truncated:       truncated,
		SizeBytes:       size,
		Previewable:     previewable,
		MediaMIME:       mediaMIME,
	}
}

func auditRequestIDCandidates(requestID string) []string {
	requestID = strings.TrimSpace(requestID)
	if requestID == "" {
		return nil
	}
	candidates := []string{requestID}
	for _, prefix := range []string{"client:", "local:"} {
		if strings.HasPrefix(requestID, prefix) {
			stripped := strings.TrimSpace(strings.TrimPrefix(requestID, prefix))
			if stripped != "" && stripped != requestID {
				candidates = append(candidates, stripped)
			}
		}
	}
	return candidates
}

func derefString(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

type multipartAuditSummary struct {
	Fields map[string]any          `json:"fields"`
	Files  []multipartAuditFileRef `json:"files"`
}

type multipartAuditFileRef struct {
	Field          string `json:"field"`
	Filename       string `json:"filename,omitempty"`
	ContentType    string `json:"content_type,omitempty"`
	SizeBytes      int    `json:"size_bytes"`
	PreviewDataURL string `json:"preview_data_url,omitempty"`
}

func buildMultipartAuditBody(role RequestAuditBodyRole, originalContentType, normalizedContentType string, body []byte, maxBytes int) (RequestAuditBody, bool) {
	mediaType, params, err := mime.ParseMediaType(originalContentType)
	if err != nil || !strings.EqualFold(mediaType, "multipart/form-data") {
		return RequestAuditBody{}, false
	}
	boundary := params["boundary"]
	if boundary == "" {
		return RequestAuditBody{}, false
	}
	reader := multipart.NewReader(bytes.NewReader(body), boundary)
	summary := multipartAuditSummary{
		Fields: map[string]any{},
		Files:  []multipartAuditFileRef{},
	}
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return RequestAuditBody{}, false
		}
		name := strings.TrimSpace(part.FormName())
		if name == "" {
			continue
		}
		data, _ := io.ReadAll(part)
		if filename := strings.TrimSpace(part.FileName()); filename != "" {
			contentType := normalizeAuditContentType(part.Header.Get("Content-Type"))
			file := multipartAuditFileRef{
				Field:       name,
				Filename:    filepath.Base(filename),
				ContentType: contentType,
				SizeBytes:   len(data),
			}
			if isPreviewableAuditMedia(contentType) && (maxBytes <= 0 || len(data) <= maxBytes) {
				file.PreviewDataURL = "data:" + contentType + ";base64," + base64.StdEncoding.EncodeToString(data)
			}
			summary.Files = append(summary.Files, file)
			continue
		}
		value := string(data)
		if _, sensitive := requestAuditRedactKeys[strings.ToLower(name)]; sensitive {
			summary.Fields[name] = "[REDACTED]"
		} else {
			summary.Fields[name] = value
		}
	}
	out, err := json.Marshal(summary)
	if err != nil {
		return RequestAuditBody{}, false
	}
	truncated := false
	if maxBytes > 0 && len(out) > maxBytes {
		out = out[:maxBytes]
		truncated = true
	}
	return RequestAuditBody{
		Role:            role,
		BodyKind:        RequestAuditBodyKindJSON,
		ContentType:     normalizedContentType,
		ContentEncoding: RequestAuditBodyEncodingText,
		Body:            string(out),
		Truncated:       truncated,
		SizeBytes:       len(body),
		Previewable:     true,
	}, true
}

func isPreviewableAuditMedia(contentType string) bool {
	return strings.HasPrefix(contentType, "image/") || strings.HasPrefix(contentType, "audio/")
}

func RequestAuditBodyPreview(body RequestAuditBody, maxBytes int) (string, bool) {
	if body.Body == "" {
		return "", false
	}
	if maxBytes <= 0 || len(body.Body) <= maxBytes {
		return body.Body, false
	}
	return body.Body[:maxBytes], true
}

func NormalizeRequestAuditBody(body []byte, maxBytes int) (string, bool, int) {
	size := len(body)
	if size == 0 {
		return "", false, 0
	}
	normalized := redactRequestAuditJSON(body)
	truncated := false
	if maxBytes > 0 && len(normalized) > maxBytes {
		normalized = normalized[:maxBytes]
		truncated = true
	}
	return string(normalized), truncated, size
}

func redactRequestAuditJSON(body []byte) []byte {
	var value any
	if err := json.Unmarshal(body, &value); err != nil {
		return body
	}
	redactRequestAuditValue(value)
	out, err := json.Marshal(value)
	if err != nil {
		return body
	}
	return out
}

func redactRequestAuditValue(value any) {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if _, sensitive := requestAuditRedactKeys[strings.ToLower(strings.TrimSpace(key))]; sensitive {
				typed[key] = "[REDACTED]"
				continue
			}
			redactRequestAuditValue(child)
		}
	case []any:
		for _, child := range typed {
			redactRequestAuditValue(child)
		}
	}
}

func classifyRequestAuditBody(mediaType string, body []byte) RequestAuditBodyKind {
	if strings.HasPrefix(mediaType, "image/") {
		return RequestAuditBodyKindImage
	}
	if strings.HasPrefix(mediaType, "audio/") {
		return RequestAuditBodyKindAudio
	}
	if mediaType == "text/event-stream" {
		return RequestAuditBodyKindSSE
	}
	if mediaType == "application/json" || strings.HasSuffix(mediaType, "+json") || json.Valid(body) {
		return RequestAuditBodyKindJSON
	}
	if strings.HasPrefix(mediaType, "text/") || utf8.Valid(body) {
		if looksLikeSSE(body) {
			return RequestAuditBodyKindSSE
		}
		return RequestAuditBodyKindText
	}
	return RequestAuditBodyKindBinary
}

func normalizeAuditContentType(contentType string) string {
	contentType = strings.TrimSpace(contentType)
	if contentType == "" {
		return ""
	}
	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		return strings.ToLower(contentType)
	}
	if len(params) == 0 {
		return strings.ToLower(mediaType)
	}
	return strings.ToLower(mediaType)
}

func auditMediaType(contentType string) string {
	if contentType == "" {
		return ""
	}
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))
	}
	return strings.ToLower(mediaType)
}

func looksLikeSSE(body []byte) bool {
	text := strings.TrimSpace(string(body))
	return strings.HasPrefix(text, "data:") || strings.HasPrefix(text, "event:")
}
