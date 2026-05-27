package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type requestAuditRepoStub struct {
	created          *RequestAuditLog
	bodies           []RequestAuditBody
	listLogs         []RequestAuditLog
	listParams       RequestAuditListParams
	cleanupMaxRows   int
	cleanupCallCount int
}

func (r *requestAuditRepoStub) Create(ctx context.Context, log *RequestAuditLog, bodies []RequestAuditBody) error {
	copy := *log
	r.created = &copy
	r.bodies = append([]RequestAuditBody(nil), bodies...)
	return nil
}

func (r *requestAuditRepoStub) List(ctx context.Context, params RequestAuditListParams) ([]RequestAuditLog, int64, error) {
	r.listParams = params
	return r.listLogs, int64(len(r.listLogs)), nil
}

func (r *requestAuditRepoStub) GetByID(ctx context.Context, id int64) (*RequestAuditLog, error) {
	return nil, nil
}

func (r *requestAuditRepoStub) GetByRequestID(ctx context.Context, requestID string) (*RequestAuditLog, error) {
	for i := range r.listLogs {
		if r.listLogs[i].RequestID == requestID {
			return &r.listLogs[i], nil
		}
	}
	return nil, nil
}

func (r *requestAuditRepoStub) GetBody(ctx context.Context, auditLogID int64, role RequestAuditBodyRole) (*RequestAuditBody, error) {
	return nil, nil
}

func (r *requestAuditRepoStub) CleanupOverflow(ctx context.Context, maxRows int) (int64, error) {
	r.cleanupMaxRows = maxRows
	r.cleanupCallCount++
	return 3, nil
}

func TestRequestAuditServiceRecordSanitizesAndTruncatesBodies(t *testing.T) {
	repo := &requestAuditRepoStub{}
	svc := NewRequestAuditService(repo, RequestAuditOptions{
		MaxBodyBytes: 48,
		MaxRows:      50000,
	})

	err := svc.Record(context.Background(), RequestAuditRecordInput{
		RequestID:  "req_123",
		UserID:     10,
		APIKeyID:   20,
		GroupID:    ptrInt64(30),
		Platform:   "openai",
		Model:      "gpt-5.4",
		Endpoint:   "/v1/responses",
		Stream:     true,
		StatusCode: 200,
		Success:    true,
		Duration:   1200 * time.Millisecond,
		RequestBody: []byte(`{"model":"gpt-5.4","authorization":"Bearer secret","nested":{"password":"pw"},"input":"` +
			strings.Repeat("x", 120) + `"}`),
		ResponseBody: []byte(`{"id":"resp_1","access_token":"token","output":"` + strings.Repeat("y", 120) + `"}`),
	})

	require.NoError(t, err)
	require.NotNil(t, repo.created)
	require.Equal(t, "req_123", repo.created.RequestID)
	require.Equal(t, int64(10), repo.created.UserID)
	require.Equal(t, int64(20), repo.created.APIKeyID)
	require.Equal(t, int64(30), derefAuditTestInt64(repo.created.GroupID))
	require.True(t, repo.created.Stream)
	require.True(t, repo.created.Success)
	require.Equal(t, 1200, repo.created.DurationMS)
	require.True(t, repo.created.RequestTruncated)
	require.True(t, repo.created.ResponseTruncated)
	require.Equal(t, len([]byte(`{"model":"gpt-5.4","authorization":"Bearer secret","nested":{"password":"pw"},"input":"`+
		strings.Repeat("x", 120)+`"}`)), repo.created.RequestBytes)
	require.NotContains(t, repo.created.RequestBody, "secret")
	require.NotContains(t, repo.created.RequestBody, "pw")
	require.Contains(t, repo.created.RequestBody, "[REDACTED]")
	require.NotContains(t, repo.created.ResponseBody, `"token"`)
	require.Equal(t, 1, repo.cleanupCallCount)
	require.Equal(t, 50000, repo.cleanupMaxRows)
}

func TestRequestAuditNormalizeBodyRedactsCaseInsensitiveKeys(t *testing.T) {
	body, truncated, size := NormalizeRequestAuditBody([]byte(`{"Cookie":"sid=abc","items":[{"client_secret":"s"}],"safe":"ok"}`), 4096)

	require.False(t, truncated)
	require.Equal(t, len([]byte(`{"Cookie":"sid=abc","items":[{"client_secret":"s"}],"safe":"ok"}`)), size)
	require.Contains(t, body, `"Cookie":"[REDACTED]"`)
	require.Contains(t, body, `"client_secret":"[REDACTED]"`)
	require.Contains(t, body, `"safe":"ok"`)
	require.NotContains(t, body, "sid=abc")
}

func TestRequestAuditServiceRecordStoresPreviewAndTypedBodies(t *testing.T) {
	repo := &requestAuditRepoStub{}
	svc := NewRequestAuditService(repo, RequestAuditOptions{
		MaxBodyBytes: 96,
		MaxRows:      50000,
	})

	err := svc.Record(context.Background(), RequestAuditRecordInput{
		RequestID:           "req_media",
		UserID:              10,
		APIKeyID:            20,
		StatusCode:          200,
		Success:             true,
		RequestContentType:  "application/json",
		ResponseContentType: "image/png",
		RequestBody:         []byte(`{"model":"gpt-5.4","input":"` + strings.Repeat("x", 160) + `"}`),
		ResponseBody:        []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a},
		ResponseBytes:       8,
	})

	require.NoError(t, err)
	require.NotNil(t, repo.created)
	require.True(t, repo.created.RequestTruncated)
	require.False(t, repo.created.ResponseTruncated)
	require.Len(t, repo.bodies, 2)

	requestBody := findAuditBodyForRole(t, repo.bodies, RequestAuditBodyRoleRequest)
	require.Equal(t, RequestAuditBodyKindJSON, requestBody.BodyKind)
	require.Equal(t, RequestAuditBodyEncodingText, requestBody.ContentEncoding)
	require.Equal(t, "application/json", requestBody.ContentType)
	require.True(t, requestBody.Truncated)
	require.True(t, requestBody.Previewable)
	require.NotEmpty(t, requestBody.Body)
	require.LessOrEqual(t, len(requestBody.Body), 96)

	responseBody := findAuditBodyForRole(t, repo.bodies, RequestAuditBodyRoleResponse)
	require.Equal(t, RequestAuditBodyKindImage, responseBody.BodyKind)
	require.Equal(t, RequestAuditBodyEncodingBase64, responseBody.ContentEncoding)
	require.Equal(t, "image/png", responseBody.ContentType)
	require.Equal(t, "image/png", responseBody.MediaMIME)
	require.True(t, responseBody.Previewable)
	require.Equal(t, "iVBORw0KGgo=", responseBody.Body)
}

func TestRequestAuditServiceGetByUsageLogMatchesPrefixedClientRequestID(t *testing.T) {
	repo := &requestAuditRepoStub{
		listLogs: []RequestAuditLog{{ID: 7, RequestID: "req-123"}},
	}
	svc := NewRequestAuditService(repo)

	audit, err := svc.GetByUsageLog(context.Background(), UsageLog{
		RequestID: "client:req-123",
		UserID:    10,
		APIKeyID:  20,
	})

	require.NoError(t, err)
	require.Equal(t, int64(7), audit.ID)
}

func TestRequestAuditServiceGetByUsageLogFallsBackForGeneratedRequestID(t *testing.T) {
	createdAt := time.Date(2026, 5, 26, 12, 0, 0, 0, time.UTC)
	endpoint := "/v1/images/generations"
	repo := &requestAuditRepoStub{
		listLogs: []RequestAuditLog{
			{ID: 1, UserID: 10, APIKeyID: 20, Model: "other", Endpoint: endpoint, CreatedAt: createdAt},
			{ID: 2, UserID: 10, APIKeyID: 20, Model: "gpt-image-1", Endpoint: endpoint, CreatedAt: createdAt.Add(500 * time.Millisecond)},
		},
	}
	svc := NewRequestAuditService(repo)

	audit, err := svc.GetByUsageLog(context.Background(), UsageLog{
		RequestID:       "generated:abc",
		UserID:          10,
		APIKeyID:        20,
		Model:           "gpt-image-1",
		InboundEndpoint: &endpoint,
		CreatedAt:       createdAt,
	})

	require.NoError(t, err)
	require.Equal(t, int64(2), audit.ID)
	require.Equal(t, int64(10), repo.listParams.UserID)
	require.Equal(t, int64(20), repo.listParams.APIKeyID)
	require.Equal(t, "gpt-image-1", repo.listParams.Model)
	require.NotNil(t, repo.listParams.StartTime)
	require.NotNil(t, repo.listParams.EndTime)
}

func TestBuildRequestAuditBodyMultipartSummarizesFieldsAndImagePreview(t *testing.T) {
	body := []byte("--abc\r\n" +
		"Content-Disposition: form-data; name=\"model\"\r\n\r\n" +
		"gpt-image-1\r\n" +
		"--abc\r\n" +
		"Content-Disposition: form-data; name=\"password\"\r\n\r\n" +
		"secret\r\n" +
		"--abc\r\n" +
		"Content-Disposition: form-data; name=\"image\"; filename=\"input.png\"\r\n" +
		"Content-Type: image/png\r\n\r\n" +
		"\x89PNG\r\n" +
		"--abc--\r\n")

	auditBody := BuildRequestAuditBody(RequestAuditBodyRoleRequest, "multipart/form-data; boundary=abc", body, 4096)

	require.Equal(t, RequestAuditBodyKindJSON, auditBody.BodyKind)
	require.Equal(t, RequestAuditBodyEncodingText, auditBody.ContentEncoding)
	require.True(t, auditBody.Previewable)
	require.Contains(t, auditBody.Body, `"model":"gpt-image-1"`)
	require.Contains(t, auditBody.Body, `"password":"[REDACTED]"`)
	require.NotContains(t, auditBody.Body, "secret")
	require.Contains(t, auditBody.Body, `"filename":"input.png"`)
	require.Contains(t, auditBody.Body, `"preview_data_url":"data:image/png;base64,`)
}

func findAuditBodyForRole(t *testing.T, bodies []RequestAuditBody, role RequestAuditBodyRole) RequestAuditBody {
	t.Helper()
	for _, body := range bodies {
		if body.Role == role {
			return body
		}
	}
	t.Fatalf("missing audit body role %s", role)
	return RequestAuditBody{}
}

func ptrInt64(v int64) *int64 {
	return &v
}

func derefAuditTestInt64(v *int64) int64 {
	if v == nil {
		return 0
	}
	return *v
}
