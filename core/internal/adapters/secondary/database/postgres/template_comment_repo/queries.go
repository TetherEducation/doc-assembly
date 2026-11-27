package templatecommentrepo

// SQL for template comments.
const (
	// The insert and the version-number lookup share one statement, so the returned
	// comment is complete without a second round trip.
	queryCreate = `
		WITH inserted AS (
			INSERT INTO content.template_comments (
				template_id, template_version_id, audience, body,
				author_email, author_name, author_campus
			)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
			RETURNING id, template_version_id, created_at
		)
		SELECT i.id, i.created_at, v.version_number
		FROM inserted i
		JOIN content.template_versions v ON v.id = i.template_version_id`

	// Oldest first. id breaks ties so the order is stable between reads.
	queryListForTemplate = `
		SELECT c.id, c.template_id, c.template_version_id, v.version_number,
			c.audience, c.body, c.author_email, c.author_name, c.author_campus,
			c.created_at
		FROM content.template_comments c
		JOIN content.template_versions v ON v.id = c.template_version_id
		WHERE c.template_id = $1 AND c.audience = $2
		ORDER BY c.created_at ASC, c.id ASC`
)
