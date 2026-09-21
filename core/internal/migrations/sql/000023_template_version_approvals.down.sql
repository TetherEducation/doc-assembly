DROP INDEX IF EXISTS content.idx_template_version_approvals_one_pending;
DROP INDEX IF EXISTS content.idx_template_version_approvals_version_proposed;
DROP TABLE IF EXISTS content.template_version_approvals;
DROP TYPE IF EXISTS approval_status;
