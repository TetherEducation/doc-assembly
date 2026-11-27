-- Schools' comments on a contract template, shown only in the CRM.
--
-- WHY ON THE TEMPLATE, TAGGED WITH A VERSION
--
-- A review thread must survive a new version: hanging comments off the version
-- would lose the conversation the moment the fix it asked for is published. Each
-- comment still records the version it was written against, so a reader can tell
-- whether a remark is about the current text or an older one.
--
-- WHY OUTSIDE content_structure
--
-- Comments never reach families or the PDF. Keeping them out of the content means
-- they do not change the approval checksum, so commenting can never make an
-- approval stale.
--
-- WHY AN AUDIENCE COLUMN
--
-- A template can be shared by many campuses (the Tether default serves over a
-- hundred). The audience is the boundary a thread is read within, computed by
-- crm-admission: the workspace code for a campus or network template, the campus
-- code for the default one. Without it every school would read every other
-- school's remarks.
--
-- Rows are append-only: comments are never edited or deleted, like approvals.

CREATE TABLE content.template_comments (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    template_id         uuid NOT NULL
                        REFERENCES content.templates (id) ON DELETE CASCADE,
    template_version_id uuid NOT NULL
                        REFERENCES content.template_versions (id) ON DELETE CASCADE,
    audience            text NOT NULL,
    body                text NOT NULL,

    -- Name and email rather than a user id, for the same reason as approvals: the
    -- user record can change or be deleted, what was said and by whom cannot.
    author_email        text NOT NULL,
    author_name         text,
    author_campus       text NOT NULL,

    created_at          timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT template_comments_body_valid CHECK (
        btrim(body) <> '' AND char_length(body) <= 4000
    ),

    -- An empty audience would be a thread nobody is scoped to.
    CONSTRAINT template_comments_audience_present CHECK (btrim(audience) <> '')
);

-- The only read: one audience's thread on one template, oldest first.
CREATE INDEX idx_template_comments_template_audience_created
    ON content.template_comments (template_id, audience, created_at);
