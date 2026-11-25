package timelinesnap

import "context"

// MemStore keeps snapshots and history in memory. It is intended for tests and
// short-lived processes; FileStore is the durable backend.
type MemStore struct {
	baseStore
}

type noopSink struct{}

func (noopSink) writeBlob(*Snapshot) error { return nil }
func (noopSink) deleteBlob(string) error   { return nil }
func (noopSink) writeCommit(*Commit) error { return nil }
func (noopSink) deleteCommit(int) error    { return nil }

// NewMemStore creates an empty in-memory store.
func NewMemStore() *MemStore {
	return &MemStore{baseStore: newBaseStore()}
}

func (m *MemStore) Save(_ context.Context, snap *Snapshot) (string, error) {
	return m.save(noopSink{}, snap)
}

func (m *MemStore) Load(_ context.Context, id string) (*Snapshot, error) {
	return m.load(id)
}

func (m *MemStore) Commit(_ context.Context, snap *Snapshot, message, author string) (*Commit, bool, error) {
	return m.commit(noopSink{}, snap, message, author, 0)
}

func (m *MemStore) Head(_ context.Context) (*Commit, error) {
	if c := m.head(); c != nil {
		return c, nil
	}
	return nil, ErrNotFound
}

func (m *MemStore) GetCommit(_ context.Context, revision int) (*Commit, error) {
	return m.getCommit(revision)
}

func (m *MemStore) List(_ context.Context) ([]*Commit, error) {
	return m.list(), nil
}

func (m *MemStore) Revert(_ context.Context, revision int, message, author string) (*Commit, error) {
	return m.revert(noopSink{}, revision, message, author)
}

func (m *MemStore) GC(_ context.Context, policy RetentionPolicy) (GCResult, error) {
	return m.gc(noopSink{}, policy)
}

func (m *MemStore) Close() error { return nil }
