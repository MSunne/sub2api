package middleware

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type requestAuditRecorderStub struct {
	input service.RequestAuditRecordInput
}

func (r *requestAuditRecorderStub) Record(ctx context.Context, input service.RequestAuditRecordInput) error {
	r.input = input
	return nil
}

func TestRequestAuditMiddlewareCapturesGatewayRequestAndResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := &requestAuditRecorderStub{}
	router := gin.New()
	router.Use(RequestAudit(recorder, 64))
	router.POST("/v1/responses", func(c *gin.Context) {
		apiKey := &service.APIKey{
			ID:      22,
			UserID:  11,
			GroupID: ptrInt64(33),
			Group:   &service.Group{ID: 33, Platform: service.PlatformOpenAI},
		}
		c.Set(string(ContextKeyAPIKey), apiKey)
		ctx := context.WithValue(c.Request.Context(), ctxkey.ClientRequestID, "req_audit_1")
		ctx = context.WithValue(ctx, ctxkey.AccountID, int64(44))
		c.Request = c.Request.WithContext(ctx)

		body, err := io.ReadAll(c.Request.Body)
		require.NoError(t, err)
		require.JSONEq(t, `{"model":"gpt-5.4","stream":true}`, string(body))
		c.JSON(http.StatusCreated, gin.H{"ok": true, "api_key": "secret"})
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewBufferString(`{"model":"gpt-5.4","stream":true}`))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusCreated, w.Code)
	require.Equal(t, "req_audit_1", recorder.input.RequestID)
	require.Equal(t, int64(11), recorder.input.UserID)
	require.Equal(t, int64(22), recorder.input.APIKeyID)
	require.Equal(t, int64(33), derefAuditTestInt64(recorder.input.GroupID))
	require.Equal(t, int64(44), derefAuditTestInt64(recorder.input.AccountID))
	require.Equal(t, service.PlatformOpenAI, recorder.input.Platform)
	require.Equal(t, "gpt-5.4", recorder.input.Model)
	require.True(t, recorder.input.Stream)
	require.Equal(t, "/v1/responses", recorder.input.Endpoint)
	require.Equal(t, http.StatusCreated, recorder.input.StatusCode)
	require.True(t, recorder.input.Success)
	require.GreaterOrEqual(t, recorder.input.Duration, time.Duration(0))
	require.JSONEq(t, `{"model":"gpt-5.4","stream":true}`, string(recorder.input.RequestBody))
	require.Contains(t, string(recorder.input.ResponseBody), `"ok":true`)
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
