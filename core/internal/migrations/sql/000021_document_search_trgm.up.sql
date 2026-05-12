-- GIN (pg_trgm) indexes for substring search on document list filters (title / recipient email).
-- Extension pg_trgm is created in 000001_extensions_types.

CREATE INDEX idx_documents_title_trgm
    ON execution.documents
    USING gin (title gin_trgm_ops);

CREATE INDEX idx_document_recipients_email_trgm
    ON execution.document_recipients
    USING gin (email gin_trgm_ops);
