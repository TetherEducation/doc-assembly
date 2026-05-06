-- Remove templates and template-created documents from system workspaces.
--
-- SYS_WRKSP remains a tenancy/administration workspace, but it is no longer
-- eligible for templates or documents generated from templates. This migration
-- removes existing data that would violate that contract before the application
-- starts serving the new behavior.

CREATE TEMP TABLE migration_020_system_templates AS
SELECT t.id
FROM content.templates t
JOIN tenancy.workspaces w ON w.id = t.workspace_id
WHERE w.type = 'SYSTEM'
   OR w.code = 'SYS_WRKSP';

CREATE TEMP TABLE migration_020_system_template_versions AS
SELECT tv.id
FROM content.template_versions tv
JOIN migration_020_system_templates st ON st.id = tv.template_id;

CREATE TEMP TABLE migration_020_system_documents AS
SELECT d.id
FROM execution.documents d
JOIN migration_020_system_template_versions stv ON stv.id = d.template_version_id;

CREATE TEMP TABLE migration_020_system_attempts AS
SELECT sa.id
FROM execution.signing_attempts sa
JOIN migration_020_system_documents sd ON sd.id = sa.document_id;

DO $$
BEGIN
    IF to_regclass('public.river_job') IS NOT NULL THEN
        DELETE FROM river_job
        WHERE kind IN (
            'render_attempt_pdf',
            'submit_attempt_to_provider',
            'advance_provider_submission',
            'reconcile_provider_submission',
            'refresh_attempt_provider_status',
            'cleanup_provider_attempt',
            'dispatch_attempt_completion'
        )
          AND args->>'attempt_id' IN (
              SELECT id::text FROM migration_020_system_attempts
          );
    END IF;
END $$;

UPDATE execution.documents d
SET active_attempt_id = NULL
FROM migration_020_system_documents sd
WHERE d.id = sd.id;

DELETE FROM execution.documents d
USING migration_020_system_documents sd
WHERE d.id = sd.id;

DELETE FROM content.templates t
USING migration_020_system_templates st
WHERE t.id = st.id;

DROP TABLE migration_020_system_attempts;
DROP TABLE migration_020_system_documents;
DROP TABLE migration_020_system_template_versions;
DROP TABLE migration_020_system_templates;
