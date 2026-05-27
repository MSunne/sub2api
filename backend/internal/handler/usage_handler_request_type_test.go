package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type userUsageRepoCapture struct {
	service.UsageLogRepository
	listParams  pagination.PaginationParams
	listFilters usagestats.UsageLogFilters
	usageByID   *service.UsageLog
}

type userUsageAuditRepoStub struct {
	logs []service.RequestAuditLog
	body *service.RequestAuditBody
}

func (s *userUsageAuditRepoStub) Create(ctx context.Context, log *service.RequestAuditLog, bodies []service.RequestAuditBody) error {
	return nil
}

func (s *userUsageAuditRepoStub) List(ctx context.Context, params service.RequestAuditListParams) ([]service.RequestAuditLog, int64, error) {
	return s.logs, int64(len(s.logs)), nil
}

func (s *userUsageAuditRepoStub) GetByID(ctx context.Context, id int64) (*service.RequestAuditLog, error) {
	for i := range s.logs {
		if s.logs[i].ID == id {
			return &s.logs[i], nil
		}
	}
	return nil, nil
}

func (s *userUsageAuditRepoStub) GetByRequestID(ctx context.Context, requestID string) (*service.RequestAuditLog, error) {
	for i := range s.logs {
		if s.logs[i].RequestID == requestID {
			return &s.logs[i], nil
		}
	}
	return nil, nil
}

func (s *userUsageAuditRepoStub) GetBody(ctx context.Context, auditLogID int64, role service.RequestAuditBodyRole) (*service.RequestAuditBody, error) {
	return s.body, nil
}

func (s *userUsageAuditRepoStub) CleanupOverflow(ctx context.Context, maxRows int) (int64, error) {
	return 0, nil
}

func (s *userUsageRepoCapture) ListWithFilters(ctx context.Context, params pagination.PaginationParams, filters usagestats.UsageLogFilters) ([]service.UsageLog, *pagination.PaginationResult, error) {
	s.listParams = params
	s.listFilters = filters
	return []service.UsageLog{}, &pagination.PaginationResult{
		Total:    0,
		Page:     params.Page,
		PageSize: params.PageSize,
		Pages:    0,
	}, nil
}

func (s *userUsageRepoCapture) GetByID(ctx context.Context, id int64) (*service.UsageLog, error) {
	if s.usageByID != nil {
		return s.usageByID, nil
	}
	return &service.UsageLog{ID: id, UserID: 42, APIKeyID: 7, RequestID: "client:req-1"}, nil
}

func newUserUsageRequestTypeTestRouter(repo *userUsageRepoCapture) *gin.Engine {
	gin.SetMode(gin.TestMode)
	usageSvc := service.NewUsageService(repo, nil, nil, nil)
	handler := NewUsageHandler(usageSvc, nil)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: 42})
		c.Next()
	})
	router.GET("/usage", handler.List)
	return router
}

func TestUserUsageListRequestTypePriority(t *testing.T) {
	repo := &userUsageRepoCapture{}
	router := newUserUsageRequestTypeTestRouter(repo)

	req := httptest.NewRequest(http.MethodGet, "/usage?request_type=ws_v2&stream=bad", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, int64(42), repo.listFilters.UserID)
	require.NotNil(t, repo.listFilters.RequestType)
	require.Equal(t, int16(service.RequestTypeWSV2), *repo.listFilters.RequestType)
	require.Nil(t, repo.listFilters.Stream)
}

func TestUserUsageListInvalidRequestType(t *testing.T) {
	repo := &userUsageRepoCapture{}
	router := newUserUsageRequestTypeTestRouter(repo)

	req := httptest.NewRequest(http.MethodGet, "/usage?request_type=invalid", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestUserUsageListInvalidStream(t *testing.T) {
	repo := &userUsageRepoCapture{}
	router := newUserUsageRequestTypeTestRouter(repo)

	req := httptest.NewRequest(http.MethodGet, "/usage?stream=invalid", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestUserUsageAuditRequiresUsageOwnership(t *testing.T) {
	repo := &userUsageRepoCapture{
		usageByID: &service.UsageLog{ID: 9, UserID: 99, APIKeyID: 7, RequestID: "client:req-1"},
	}
	usageSvc := service.NewUsageService(repo, nil, nil, nil)
	auditSvc := service.NewRequestAuditService(&userUsageAuditRepoStub{
		logs: []service.RequestAuditLog{{ID: 3, UserID: 99, APIKeyID: 7, RequestID: "req-1"}},
	})
	handler := NewUsageHandler(usageSvc, nil, auditSvc)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: 42})
		c.Next()
	})
	router.GET("/usage/:id/audit", handler.GetAuditLogByUsageID)

	req := httptest.NewRequest(http.MethodGet, "/usage/9/audit", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusForbidden, rec.Code)
}

func TestUserUsageAuditReturnsOwnedAudit(t *testing.T) {
	repo := &userUsageRepoCapture{
		usageByID: &service.UsageLog{ID: 9, UserID: 42, APIKeyID: 7, RequestID: "client:req-1"},
	}
	usageSvc := service.NewUsageService(repo, nil, nil, nil)
	auditSvc := service.NewRequestAuditService(&userUsageAuditRepoStub{
		logs: []service.RequestAuditLog{{ID: 3, UserID: 42, APIKeyID: 7, RequestID: "req-1"}},
	})
	handler := NewUsageHandler(usageSvc, nil, auditSvc)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: 42})
		c.Next()
	})
	router.GET("/usage/:id/audit", handler.GetAuditLogByUsageID)

	req := httptest.NewRequest(http.MethodGet, "/usage/9/audit", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
}
