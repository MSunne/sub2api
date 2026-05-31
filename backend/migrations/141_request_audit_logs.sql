CREATE TABLE IF NOT EXISTS request_audit_logs (
    id BIGSERIAL PRIMARY KEY,
    request_id TEXT NOT NULL,
    user_id BIGINT NOT NULL,
    api_key_id BIGINT NOT NULL,
    account_id BIGINT NULL,
    group_id BIGINT NULL,
    platform TEXT NOT NULL DEFAULT '',
    model TEXT NOT NULL DEFAULT '',
    endpoint TEXT NOT NULL DEFAULT '',
    stream BOOLEAN NOT NULL DEFAULT FALSE,
    status_code INTEGER NOT NULL DEFAULT 0,
    success BOOLEAN NOT NULL DEFAULT FALSE,
    duration_ms INTEGER NOT NULL DEFAULT 0,
    request_body TEXT NOT NULL DEFAULT '',
    response_body TEXT NOT NULL DEFAULT '',
    error_body TEXT NOT NULL DEFAULT '',
    request_truncated BOOLEAN NOT NULL DEFAULT FALSE,
    response_truncated BOOLEAN NOT NULL DEFAULT FALSE,
    request_bytes INTEGER NOT NULL DEFAULT 0,
    response_bytes INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_request_audit_logs_created_id ON request_audit_logs (created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_request_audit_logs_request_id ON request_audit_logs (request_id);
CREATE INDEX IF NOT EXISTS idx_request_audit_logs_user_id_created ON request_audit_logs (user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_request_audit_logs_api_key_id_created ON request_audit_logs (api_key_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_request_audit_logs_model_created ON request_audit_logs (model, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_request_audit_logs_status_created ON request_audit_logs (status_code, created_at DESC);
