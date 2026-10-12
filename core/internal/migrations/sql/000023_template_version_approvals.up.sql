-- A school's approval of one exact contract text.
--
-- WHY
--
-- Contract templates are authored on schools' behalf, transcribed from documents
-- the schools themselves signed. There is no moment where a school confirms the
-- transcription is right, and no record that they did. When the Ayelen contract
-- changed in August 2026 the only review was a static HTML page emailed by hand,
-- and no trace of their agreement survives it.
--
-- WHY A SEPARATE TABLE RATHER THAN A VERSION STATUS
--
-- The instinct is to add PENDING_APPROVAL / APPROVED to version_status. That
-- conflates two different things: DRAFT -> PUBLISHED -> ARCHIVED is a publication
-- lifecycle, while approval is a governance fact about one exact content state.
-- Merged, they cannot express "approved, then edited" - which is precisely the
-- failure the feature exists to prevent.
--
-- WHY THE CHECKSUM IS THE LOAD-BEARING COLUMN
--
-- The approval records a hash of the content it approved, not just a pointer to the
-- version. If anyone edits the template afterwards the hash stops matching and the
-- approval is stale automatically: no flag to remember to clear, and no way to have
-- one text approved while a different one is published.
--
-- Rows are never updated in place. A re-proposal is a new row, so the table is its
-- own audit trail.

CREATE TYPE approval_status AS ENUM ('PENDING', 'APPROVED', 'CHANGES_REQUESTED');

CREATE TABLE content.template_version_approvals (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    template_version_id uuid NOT NULL
                        REFERENCES content.template_versions (id) ON DELETE CASCADE,

    -- SHA-256 of content_structure as it stood when the version was proposed.
    -- Publish compares this against the content as it stands now.
    content_checksum    text NOT NULL,

    status              approval_status NOT NULL DEFAULT 'PENDING',

    -- Who at Tether put it forward.
    proposed_by         text NOT NULL,
    proposed_at         timestamptz NOT NULL DEFAULT now(),

    -- The person at the school. Name and email are stored rather than a user id
    -- alone: the user record can change or be deleted, the legal fact cannot.
    decided_by_email    text,
    decided_by_name     text,

    -- Which campus context the approver was acting in. Capabilities in
    -- crm-identity-access are per-campus, but a network approves its shared
    -- template once - so the campus the decision was made from is part of the
    -- record, not derivable from the template.
    decided_at_campus   text,
    decided_at          timestamptz,

    -- Required when requesting changes. That text is the whole point of the round
    -- trip; without it a rejection is unactionable.
    comment             text,

    -- The PDF as rendered at the moment of approval, in GCS. An immutable artifact
    -- beats reconstructing what the text looked like months later.
    approved_pdf_path   text,

    created_at          timestamptz NOT NULL DEFAULT now(),

    -- A decision must carry who made it and when. PENDING must not.
    CONSTRAINT template_version_approvals_decision_complete CHECK (
        (status = 'PENDING'
            AND decided_at IS NULL
            AND decided_by_email IS NULL)
        OR (status IN ('APPROVED', 'CHANGES_REQUESTED')
            AND decided_at IS NOT NULL
            AND decided_by_email IS NOT NULL)
    ),

    -- A change request without a reason cannot be acted on.
    CONSTRAINT template_version_approvals_changes_need_comment CHECK (
        status <> 'CHANGES_REQUESTED'
        OR (comment IS NOT NULL AND btrim(comment) <> '')
    )
);

-- The publish gate asks one question on every publish: what is the newest approval
-- for this version? That lookup must not degrade as history accumulates.
CREATE INDEX idx_template_version_approvals_version_proposed
    ON content.template_version_approvals (template_version_id, proposed_at DESC);

-- At most one proposal may be outstanding per version. Two concurrent PENDING rows
-- would make "the newest approval" ambiguous exactly when it matters.
CREATE UNIQUE INDEX idx_template_version_approvals_one_pending
    ON content.template_version_approvals (template_version_id)
    WHERE status = 'PENDING';
