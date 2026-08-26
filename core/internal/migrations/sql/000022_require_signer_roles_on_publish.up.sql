-- A PUBLISHED version whose document declares signer roles must carry the derived
-- rows in content.template_version_signer_roles.
--
-- WHY
--
-- TemplateVersionService.PublishVersion derives those rows:
--
--     ValidateForPublish -> replaceSignerRoles -> replaceInjectables
--                        -> archiveCurrentPublished -> version.Publish
--
-- On 2026-08-14, 22 versions were promoted to PUBLISHED by direct SQL, reproducing
-- only the final status flip. The derived rows were never written, and because
-- DocumentGenerator builds its recipient lookup from them:
--
--     roleByAnchor[r.AnchorString] = r          -- from the DB rows: empty
--     anchor := GenerateAnchorString(sr.Label)  -- "Apoderado/a" -> __sig_apoderadoa__
--     dbRole, found := roleByAnchor[anchor]     -- never found
--
-- every document creation failed recipient validation with HTTP 422 for eleven days
-- (11,994 x 422 against 4 x 201). Nothing in the schema objected, because the
-- invariant lived only inside the service that was bypassed.
--
-- The bypass was direct SQL, so the guard has to be in the database. A service-level
-- check would have been skipped by exactly the same action.
--
-- SCOPE
--
-- Fires only on a transition INTO 'PUBLISHED', so the versions already in this broken
-- state can still be archived or edited while they are repaired. It does not fire for
-- documents that declare no signer roles, which keeps unsigned document types
-- publishable.
--
-- The legitimate path passes: replaceSignerRoles runs before the status flip, so the
-- rows exist by the time this trigger sees NEW.status = 'PUBLISHED'.
--
-- NOT ENFORCED HERE
--
-- The same bypass also skipped replaceInjectables, leaving those versions with zero
-- template_version_injectables. That is deliberately not constrained: no published
-- version currently has signer roles and zero injectables (0 of 78), but a template
-- that references no variables could legitimately have none, so there is no safe
-- invariant to assert yet.

CREATE OR REPLACE FUNCTION content.assert_published_version_has_signer_roles()
RETURNS TRIGGER AS $$
DECLARE
    declared_roles INT;
BEGIN
    IF NEW.status <> 'PUBLISHED' THEN
        RETURN NEW;
    END IF;

    -- Only guard the transition into PUBLISHED, not every write to a published row.
    IF TG_OP = 'UPDATE' AND OLD.status = 'PUBLISHED' THEN
        RETURN NEW;
    END IF;

    declared_roles := jsonb_array_length(
        COALESCE(NEW.content_structure -> 'signerRoles', '[]'::jsonb)
    );

    IF declared_roles > 0 AND NOT EXISTS (
        SELECT 1
        FROM content.template_version_signer_roles sr
        WHERE sr.template_version_id = NEW.id
    ) THEN
        RAISE EXCEPTION
            'template version % declares % signer role(s) but has no rows in '
            'content.template_version_signer_roles; publish through '
            'TemplateVersionService.PublishVersion so they are derived',
            NEW.id, declared_roles
            USING ERRCODE = 'check_violation';
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trigger_template_versions_require_signer_roles
    BEFORE INSERT OR UPDATE ON content.template_versions
    FOR EACH ROW
    EXECUTE FUNCTION content.assert_published_version_has_signer_roles();
