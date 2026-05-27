package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

type requestAuditRepository struct {
	db *sql.DB
}

func NewRequestAuditRepository(db *sql.DB) service.RequestAuditRepository {
	return &requestAuditRepository{db: db}
}

func (r *requestAuditRepository) Create(ctx context.Context, log *service.RequestAuditLog, bodies []service.RequestAuditBody) error {
	if r == nil || r.db == nil || log == nil {
		return nil
	}
	createdAt := log.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now()
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	err = tx.QueryRowContext(ctx, `
INSERT INTO request_audit_logs (
    request_id, user_id, api_key_id, account_id, group_id, platform, model, endpoint, stream,
    status_code, success, duration_ms, request_body, response_body, error_body,
    request_content_type, response_content_type, request_body_kind, response_body_kind,
    request_truncated, response_truncated, request_bytes, response_bytes, created_at
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24)
RETURNING id`,
		log.RequestID, log.UserID, log.APIKeyID, log.AccountID, log.GroupID, log.Platform, log.Model, log.Endpoint, log.Stream,
		log.StatusCode, log.Success, log.DurationMS, log.RequestBody, log.ResponseBody, log.ErrorBody,
		log.RequestContentType, log.ResponseContentType, log.RequestBodyKind, log.ResponseBodyKind,
		log.RequestTruncated, log.ResponseTruncated, log.RequestBytes, log.ResponseBytes, createdAt,
	).Scan(&log.ID)
	if err != nil {
		return err
	}

	for _, body := range bodies {
		body.AuditLogID = log.ID
		bodyCreatedAt := body.CreatedAt
		if bodyCreatedAt.IsZero() {
			bodyCreatedAt = createdAt
		}
		_, err = tx.ExecContext(ctx, `
INSERT INTO request_audit_bodies (
    audit_log_id, role, body_kind, content_type, content_encoding, body,
    truncated, size_bytes, previewable, media_mime, created_at
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
ON CONFLICT (audit_log_id, role) DO UPDATE SET
    body_kind = EXCLUDED.body_kind,
    content_type = EXCLUDED.content_type,
    content_encoding = EXCLUDED.content_encoding,
    body = EXCLUDED.body,
    truncated = EXCLUDED.truncated,
    size_bytes = EXCLUDED.size_bytes,
    previewable = EXCLUDED.previewable,
    media_mime = EXCLUDED.media_mime`,
			body.AuditLogID, body.Role, body.BodyKind, body.ContentType, body.ContentEncoding, body.Body,
			body.Truncated, body.SizeBytes, body.Previewable, body.MediaMIME, bodyCreatedAt,
		)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *requestAuditRepository) List(ctx context.Context, params service.RequestAuditListParams) ([]service.RequestAuditLog, int64, error) {
	conditions, args := requestAuditConditions(params)
	where := ""
	if len(conditions) > 0 {
		where = " WHERE " + strings.Join(conditions, " AND ")
	}

	var total int64
	countQuery := "SELECT COUNT(*) FROM request_audit_logs ral" + where
	if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	page := params.Page
	if page <= 0 {
		page = 1
	}
	pageSize := params.PageSize
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	offset := (page - 1) * pageSize
	listArgs := append(append([]any{}, args...), pageSize, offset)
	query := requestAuditSelectSQL + where + " ORDER BY ral.created_at DESC, ral.id DESC LIMIT $" + fmt.Sprint(len(args)+1) + " OFFSET $" + fmt.Sprint(len(args)+2)
	rows, err := r.db.QueryContext(ctx, query, listArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var logs []service.RequestAuditLog
	for rows.Next() {
		log, err := scanRequestAuditLog(rows)
		if err != nil {
			return nil, 0, err
		}
		logs = append(logs, *log)
	}
	return logs, total, rows.Err()
}

func (r *requestAuditRepository) GetByID(ctx context.Context, id int64) (*service.RequestAuditLog, error) {
	row := r.db.QueryRowContext(ctx, requestAuditSelectSQL+" WHERE ral.id = $1", id)
	return scanRequestAuditLog(row)
}

func (r *requestAuditRepository) GetByRequestID(ctx context.Context, requestID string) (*service.RequestAuditLog, error) {
	row := r.db.QueryRowContext(ctx, requestAuditSelectSQL+" WHERE ral.request_id = $1 ORDER BY ral.created_at DESC, ral.id DESC LIMIT 1", strings.TrimSpace(requestID))
	return scanRequestAuditLog(row)
}

func (r *requestAuditRepository) GetBody(ctx context.Context, auditLogID int64, role service.RequestAuditBodyRole) (*service.RequestAuditBody, error) {
	row := r.db.QueryRowContext(ctx, `
SELECT id, audit_log_id, role, body_kind, content_type, content_encoding, body,
       truncated, size_bytes, previewable, media_mime, created_at
FROM request_audit_bodies
WHERE audit_log_id = $1 AND role = $2`, auditLogID, role)
	var body service.RequestAuditBody
	if err := row.Scan(
		&body.ID,
		&body.AuditLogID,
		&body.Role,
		&body.BodyKind,
		&body.ContentType,
		&body.ContentEncoding,
		&body.Body,
		&body.Truncated,
		&body.SizeBytes,
		&body.Previewable,
		&body.MediaMIME,
		&body.CreatedAt,
	); err != nil {
		return nil, err
	}
	return &body, nil
}

func (r *requestAuditRepository) CleanupOverflow(ctx context.Context, maxRows int) (int64, error) {
	if maxRows <= 0 {
		return 0, nil
	}
	res, err := r.db.ExecContext(ctx, `
DELETE FROM request_audit_logs
WHERE id IN (
    SELECT id FROM request_audit_logs
    ORDER BY created_at DESC, id DESC
    OFFSET $1
)`, maxRows)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

const requestAuditSelectSQL = `
SELECT
    ral.id, ral.request_id, ral.user_id, ral.api_key_id, ral.account_id, ral.group_id,
    ral.platform, ral.model, ral.endpoint, ral.stream, ral.status_code, ral.success,
    ral.duration_ms, ral.request_body, ral.response_body, ral.error_body,
    ral.request_content_type, ral.response_content_type, ral.request_body_kind, ral.response_body_kind,
    ral.request_truncated, ral.response_truncated, ral.request_bytes, ral.response_bytes,
    ral.created_at,
    u.email, ak.name, a.name, g.name
FROM request_audit_logs ral
LEFT JOIN users u ON u.id = ral.user_id
LEFT JOIN api_keys ak ON ak.id = ral.api_key_id
LEFT JOIN accounts a ON a.id = ral.account_id
LEFT JOIN groups g ON g.id = ral.group_id`

type requestAuditScanner interface {
	Scan(dest ...any) error
}

func scanRequestAuditLog(scanner requestAuditScanner) (*service.RequestAuditLog, error) {
	var log service.RequestAuditLog
	var accountID, groupID sql.NullInt64
	var userEmail, apiKeyName, accountName, groupName sql.NullString
	if err := scanner.Scan(
		&log.ID, &log.RequestID, &log.UserID, &log.APIKeyID, &accountID, &groupID,
		&log.Platform, &log.Model, &log.Endpoint, &log.Stream, &log.StatusCode, &log.Success,
		&log.DurationMS, &log.RequestBody, &log.ResponseBody, &log.ErrorBody,
		&log.RequestContentType, &log.ResponseContentType, &log.RequestBodyKind, &log.ResponseBodyKind,
		&log.RequestTruncated, &log.ResponseTruncated, &log.RequestBytes, &log.ResponseBytes,
		&log.CreatedAt,
		&userEmail, &apiKeyName, &accountName, &groupName,
	); err != nil {
		return nil, err
	}
	if accountID.Valid {
		log.AccountID = &accountID.Int64
	}
	if groupID.Valid {
		log.GroupID = &groupID.Int64
	}
	if userEmail.Valid {
		log.UserEmail = &userEmail.String
	}
	if apiKeyName.Valid {
		log.APIKeyName = &apiKeyName.String
	}
	if accountName.Valid {
		log.AccountName = &accountName.String
	}
	if groupName.Valid {
		log.GroupName = &groupName.String
	}
	return &log, nil
}

func requestAuditConditions(params service.RequestAuditListParams) ([]string, []any) {
	var conditions []string
	var args []any
	add := func(condition string, value any) {
		args = append(args, value)
		conditions = append(conditions, fmt.Sprintf(condition, len(args)))
	}
	if v := strings.TrimSpace(params.RequestID); v != "" {
		add("ral.request_id = $%d", v)
	}
	if params.UserID > 0 {
		add("ral.user_id = $%d", params.UserID)
	}
	if params.APIKeyID > 0 {
		add("ral.api_key_id = $%d", params.APIKeyID)
	}
	if v := strings.TrimSpace(params.Model); v != "" {
		add("ral.model = $%d", v)
	}
	if params.StatusCode > 0 {
		add("ral.status_code = $%d", params.StatusCode)
	}
	if params.Success != nil {
		add("ral.success = $%d", *params.Success)
	}
	if params.StartTime != nil {
		add("ral.created_at >= $%d", *params.StartTime)
	}
	if params.EndTime != nil {
		add("ral.created_at < $%d", *params.EndTime)
	}
	return conditions, args
}
