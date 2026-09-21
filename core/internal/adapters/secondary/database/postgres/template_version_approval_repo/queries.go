package templateversionapprovalrepo

// SQL for template version approvals.
const (
	columns = `id, template_version_id, content_checksum, status, proposed_by, proposed_at,
		decided_by_email, decided_by_name, decided_at_campus, decided_at,
		comment, approved_pdf_path, created_at`

	queryCreate = `
		INSERT INTO content.template_version_approvals (
			template_version_id, content_checksum, status, proposed_by, proposed_at, created_at
		)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id`

	queryFindByID = `
		SELECT ` + columns + `
		FROM content.template_version_approvals
		WHERE id = $1`

	// Newest by proposal time. The publish gate consults only this row: an older
	// APPROVED row must never authorise a version that has since been re-proposed.
	queryFindLatestForVersion = `
		SELECT ` + columns + `
		FROM content.template_version_approvals
		WHERE template_version_id = $1
		ORDER BY proposed_at DESC
		LIMIT 1`

	queryFindPendingForVersion = `
		SELECT ` + columns + `
		FROM content.template_version_approvals
		WHERE template_version_id = $1 AND status = 'PENDING'
		LIMIT 1`

	queryListForVersion = `
		SELECT ` + columns + `
		FROM content.template_version_approvals
		WHERE template_version_id = $1
		ORDER BY proposed_at DESC`

	// The WHERE status = 'PENDING' is the concurrency guard: two simultaneous
	// decisions cannot both write, because the second matches no row.
	queryRecordDecision = `
		UPDATE content.template_version_approvals
		SET status = $2,
			decided_by_email = $3,
			decided_by_name = $4,
			decided_at_campus = $5,
			decided_at = $6,
			comment = $7
		WHERE id = $1 AND status = 'PENDING'`
)
