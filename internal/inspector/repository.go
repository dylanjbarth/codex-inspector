package inspector

import "github.com/dylanjbarth/codex-inspector/internal/storage"

type Repository struct{ Store *storage.Store }

func (r Repository) Sessions(revision int64, query string, limit int) (string, int64, []storage.SessionSummary, error) {
	return r.Store.Sessions(revision, query, limit)
}
