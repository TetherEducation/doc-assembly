package documenteventrepo

const (
	queryCreate = `
		INSERT INTO execution.document_events (
			document_id, event_type, actor_type, actor_id,
			old_status, new_status, recipient_id, metadata, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`

	queryFindByDocumentID = `
		SELECT id, document_id, event_type, actor_type, actor_id,
			   old_status, new_status, recipient_id, metadata, created_at
		FROM execution.document_events
		WHERE document_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`
)

const (
	// queryLockDocumentEvents serializes CreateOnce calls for one document until
	// the transaction ends, so two writers cannot both see "no such event".
	queryLockDocumentEvents = `SELECT pg_advisory_xact_lock(hashtext('document_events:' || $1::text))`

	// queryCreateOnce inserts the event unless one with the same document,
	// type, recipient, attemptId and recipientEmail exists. The email keeps two
	// signers apart when an attempt recipient has no document recipient.
	queryCreateOnce = `
		INSERT INTO execution.document_events (
			document_id, event_type, actor_type, actor_id,
			old_status, new_status, recipient_id, metadata, created_at
		)
		SELECT $1::uuid, $2::varchar, $3::varchar, $4::varchar, $5::varchar, $6::varchar, $7::uuid, $8::jsonb, $9::timestamptz
		WHERE NOT EXISTS (
			SELECT 1 FROM execution.document_events
			WHERE document_id = $1::uuid
			  AND event_type = $2::varchar
			  AND recipient_id IS NOT DISTINCT FROM $7::uuid
			  AND metadata->>'attemptId' IS NOT DISTINCT FROM $8::jsonb->>'attemptId'
			  AND metadata->>'recipientEmail' IS NOT DISTINCT FROM $8::jsonb->>'recipientEmail'
		)
	`

	// queryUpgradeSignedTime replaces an engine-guessed signing time with the
	// provider's, on the same event.
	queryUpgradeSignedTime = `
		UPDATE execution.document_events
		SET created_at = $5::timestamptz, metadata = $4::jsonb, actor_type = $6::varchar
		WHERE document_id = $1::uuid
		  AND event_type = $2::varchar
		  AND recipient_id IS NOT DISTINCT FROM $3::uuid
		  AND metadata->>'attemptId' IS NOT DISTINCT FROM $4::jsonb->>'attemptId'
		  AND metadata->>'recipientEmail' IS NOT DISTINCT FROM $4::jsonb->>'recipientEmail'
		  AND COALESCE(metadata->>'timeSource', '') <> 'provider'
	`
)
