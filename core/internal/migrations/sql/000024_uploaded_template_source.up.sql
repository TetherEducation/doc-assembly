-- An uploaded PDF is a template version whose bytes and field boxes do not
-- come from Typst. Absence of a row means the version is still authored.
--
-- page_sizes is one {width, height} object per page, in PDF points.
-- fields is the placed boxes, also in PDF points, origin bottom-left,
-- each bound to a content.template_version_signer_roles id.

CREATE TABLE content.template_version_sources (
    template_version_id uuid PRIMARY KEY
        REFERENCES content.template_versions (id) ON DELETE CASCADE,
    object_key          text NOT NULL,
    page_count          int NOT NULL CHECK (page_count > 0),
    page_sizes          jsonb NOT NULL,
    fields              jsonb NOT NULL DEFAULT '[]'::jsonb,
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now()
);
