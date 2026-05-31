-- Rename the default public brand for new viz2ai deployments.
-- Preserve any administrator-customized site_name.
INSERT INTO settings (key, value, updated_at)
VALUES ('site_name', 'viz2ai', NOW())
ON CONFLICT (key) DO UPDATE
SET value = EXCLUDED.value,
    updated_at = NOW()
WHERE settings.value = '' OR settings.value = 'Sub2API';
