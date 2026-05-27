package middleware

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

type RequestAuditRecorder interface {
	Record(ctx context.Context, input service.RequestAuditRecordInput) error
}

func RequestAudit(recorder RequestAuditRecorder, maxBodyBytes int) gin.HandlerFunc {
	if maxBodyBytes <= 0 {
		maxBodyBytes = service.DefaultRequestAuditMaxBodyBytes
	}
	return func(c *gin.Context) {
		if recorder == nil || c.Request == nil {
			c.Next()
			return
		}
		start := time.Now()
		requestBody := readAndRestoreAuditRequestBody(c, maxBodyBytes)
		writer := &requestAuditResponseWriter{
			ResponseWriter: c.Writer,
			maxBytes:       maxBodyBytes,
		}
		c.Writer = writer

		c.Next()

		input := buildRequestAuditInput(c, requestBody, writer.body.Bytes(), writer.totalBytes, start)
		ctx, cancel := context.WithTimeout(context.WithoutCancel(c.Request.Context()), 3*time.Second)
		defer cancel()
		_ = recorder.Record(ctx, input)
	}
}

func readAndRestoreAuditRequestBody(c *gin.Context, maxBytes int) []byte {
	if c == nil || c.Request == nil || c.Request.Body == nil {
		return nil
	}
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.Request.Body = io.NopCloser(bytes.NewReader(nil))
		return nil
	}
	c.Request.Body = io.NopCloser(bytes.NewReader(body))
	return body
}

func buildRequestAuditInput(c *gin.Context, requestBody []byte, responseBody []byte, responseBytes int, start time.Time) service.RequestAuditRecordInput {
	status := http.StatusOK
	if c.Writer != nil && c.Writer.Status() > 0 {
		status = c.Writer.Status()
	}
	model := strings.TrimSpace(gjson.GetBytes(requestBody, "model").String())
	stream := gjson.GetBytes(requestBody, "stream").Bool()
	if model == "" {
		if v, ok := c.Request.Context().Value(ctxkey.Model).(string); ok {
			model = strings.TrimSpace(v)
		}
	}
	platform, _ := c.Request.Context().Value(ctxkey.Platform).(string)
	var accountID *int64
	if v, ok := c.Request.Context().Value(ctxkey.AccountID).(int64); ok && v > 0 {
		accountID = &v
	}
	requestID, _ := c.Request.Context().Value(ctxkey.ClientRequestID).(string)
	endpoint := c.FullPath()
	if endpoint == "" && c.Request != nil && c.Request.URL != nil {
		endpoint = c.Request.URL.Path
	}

	var userID, apiKeyID int64
	var groupID *int64
	if apiKey, ok := GetAPIKeyFromContext(c); ok && apiKey != nil {
		apiKeyID = apiKey.ID
		userID = apiKey.UserID
		if userID == 0 && apiKey.User != nil {
			userID = apiKey.User.ID
		}
		groupID = apiKey.GroupID
		if groupID == nil && apiKey.Group != nil {
			groupID = &apiKey.Group.ID
		}
		if platform == "" && apiKey.Group != nil {
			platform = apiKey.Group.Platform
		}
	}

	return service.RequestAuditRecordInput{
		RequestID:           requestID,
		UserID:              userID,
		APIKeyID:            apiKeyID,
		AccountID:           accountID,
		GroupID:             groupID,
		Platform:            platform,
		Model:               model,
		Endpoint:            endpoint,
		Stream:              stream,
		StatusCode:          status,
		Success:             status >= 200 && status < 400,
		Duration:            time.Since(start),
		RequestBody:         requestBody,
		ResponseBody:        responseBody,
		RequestContentType:  c.GetHeader("Content-Type"),
		ResponseContentType: c.Writer.Header().Get("Content-Type"),
		RequestBytes:        len(requestBody),
		ResponseBytes:       responseBytes,
		CreatedAt:           start,
	}
}

type requestAuditResponseWriter struct {
	gin.ResponseWriter
	body       bytes.Buffer
	maxBytes   int
	totalBytes int
}

func (w *requestAuditResponseWriter) Write(data []byte) (int, error) {
	w.capture(data)
	return w.ResponseWriter.Write(data)
}

func (w *requestAuditResponseWriter) WriteString(s string) (int, error) {
	w.capture([]byte(s))
	return w.ResponseWriter.WriteString(s)
}

func (w *requestAuditResponseWriter) capture(data []byte) {
	if w == nil || len(data) == 0 {
		return
	}
	w.totalBytes += len(data)
	if w.maxBytes <= 0 || w.body.Len() >= w.maxBytes {
		return
	}
	remaining := w.maxBytes - w.body.Len()
	if len(data) > remaining {
		data = data[:remaining]
	}
	_, _ = w.body.Write(data)
}
