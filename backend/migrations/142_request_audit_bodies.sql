ALTER TABLE request_audit_logs ADD COLUMN IF NOT EXISTS request_content_type TEXT NOT NULL DEFAULT '';
ALTER TABLE request_audit_logs ADD COLUMN IF NOT EXISTS response_content_type TEXT NOT NULL DEFAULT '';
ALTER TABLE request_audit_logs ADD COLUMN IF NOT EXISTS request_body_kind TEXT NOT NULL DEFAULT '';
ALTER TABLE request_audit_logs ADD COLUMN IF NOT EXISTS response_body_kind TEXT NOT NULL DEFAULT '';

CREATE TABLE IF NOT EXISTS request_audit_bodies (
    id BIGSERIAL PRIMARY KEY,
    audit_log_id BIGINT NOT NULL REFERENCES request_audit_logs(id) ON DELETE CASCADE,
    role TEXT NOT NULL,
    body_kind TEXT NOT NULL DEFAULT '',
    content_type TEXT NOT NULL DEFAULT '',
    content_encoding TEXT NOT NULL DEFAULT 'text',
    body TEXT NOT NULL DEFAULT '',
    truncated BOOLEAN NOT NULL DEFAULT FALSE,
    size_bytes INTEGER NOT NULL DEFAULT 0,
    previewable BOOLEAN NOT NULL DEFAULT TRUE,
    media_mime TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT request_audit_bodies_role_check CHECK (role IN ('request', 'response')),
    CONSTRAINT request_audit_bodies_unique_role UNIQUE (audit_log_id, role)
);

CREATE INDEX IF NOT EXISTS idx_request_audit_bodies_audit_role ON request_audit_bodies (audit_log_id, role);
