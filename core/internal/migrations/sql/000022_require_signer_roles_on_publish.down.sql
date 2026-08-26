DROP TRIGGER IF EXISTS trigger_template_versions_require_signer_roles ON content.template_versions;
DROP FUNCTION IF EXISTS content.assert_published_version_has_signer_roles();
