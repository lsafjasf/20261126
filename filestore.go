package timelinesnap

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

const (
	blobsDir   = "blobs"
	commitsDir = "commits"
	headFile   = "head"
	tempSuffix = ".tmp"
)

// FileStore is a durable on-disk Store. Snapshot blobs are content-addressed
// and therefore deduplicated; commits are small pointer records.
type FileStore struct {
	dir string
	baseStore
}

// OpenFileStore opens (or creates) a store rooted at dir.
func OpenFileStore(dir string) (*FileStore, error) {
	for _, sub := range []string{dir, filepath.Join(dir, blobsDir), filepath.Join(dir, commitsDir)} {
		if err := os.MkdirAll(sub, 0o755); err != nil {
			return nil, err
		}
	}
	f := &FileStore{dir: dir, baseStore: newBaseStore()}
	if err := f.hydrate(); err != nil {
		return nil, err
	}
	return f, nil
}

func (f *FileStore) hydrate() error {
	entries, err := os.ReadDir(filepath.Join(f.dir, commitsDir))
	if err != nil {
		return err
	}
	var revs []int
	for _, e := range entries {
		var rev int
		if _, err := fmt.Sscanf(e.Name(), "%d.json", &rev); err == nil {
			revs = append(revs, rev)
		}
	}
	sort.Ints(revs)
	for _, rev := range revs {
		var c Commit
		if err := readJSON(f.commitPath(rev), &c); err != nil {
			return err
		}
		f.commits = append(f.commits, &c)
		snap, err := f.readBlob(c.SnapshotID)
		if err != nil {
			return err
		}
		f.blobs[c.SnapshotID] = snap
	}
	return nil
}

func (f *FileStore) blobPath(id string) string {
	return filepath.Join(f.dir, blobsDir, id+".json")
}

func (f *FileStore) commitPath(rev int) string {
	return filepath.Join(f.dir, commitsDir, fmt.Sprintf("%06d.json", rev))
}

func (f *FileStore) headPath() string { return filepath.Join(f.dir, headFile) }

func (f *FileStore) readBlob(id string) (*Snapshot, error) {
	var snap Snapshot
	if err := readJSON(f.blobPath(id), &snap); err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &snap, nil
}

func (f *FileStore) writeBlob(snap *Snapshot) error {
	return atomicWriteJSON(f.blobPath(snap.ID), snap)
}

func (f *FileStore) deleteBlob(id string) error {
	err := os.Remove(f.blobPath(id))
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (f *FileStore) writeCommit(c *Commit) error {
	if err := atomicWriteJSON(f.commitPath(c.Revision), c); err != nil {
		return err
	}
	return atomicWriteFile(f.headPath(), []byte(c.SnapshotID+"\n"))
}

func (f *FileStore) deleteCommit(rev int) error {
	err := os.Remove(f.commitPath(rev))
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (f *FileStore) Save(ctx context.Context, snap *Snapshot) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return f.save(f, snap)
}

func (f *FileStore) Load(ctx context.Context, id string) (*Snapshot, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return f.load(id)
}

func (f *FileStore) Commit(ctx context.Context, snap *Snapshot, message, author string) (*Commit, bool, error) {
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	return f.commit(f, snap, message, author, 0)
}

func (f *FileStore) Head(context.Context) (*Commit, error) {
	if c := f.head(); c != nil {
		return c, nil
	}
	return nil, ErrNotFound
}

func (f *FileStore) GetCommit(_ context.Context, revision int) (*Commit, error) {
	return f.getCommit(revision)
}

func (f *FileStore) List(context.Context) ([]*Commit, error) { return f.list(), nil }

func (f *FileStore) Revert(ctx context.Context, revision int, message, author string) (*Commit, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return f.revert(f, revision, message, author)
}

func (f *FileStore) GC(ctx context.Context, policy RetentionPolicy) (GCResult, error) {
	if err := ctx.Err(); err != nil {
		return GCResult{}, err
	}
	return f.gc(f, policy)
}

func (f *FileStore) Close() error { return nil }

func readJSON(path string, v any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}

func atomicWriteJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return atomicWriteFile(path, data)
}

func atomicWriteFile(path string, data []byte) error {
	tmp := path + tempSuffix
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
