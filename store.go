package audiotracks

import (
	"context"
	"errors"
	"sync"
)

var ErrProjectNotFound = errors.New("audio project not found")

// Repository stores complete projects. Implementations must replace a
// project atomically and must not retain the caller's slice.
type Repository interface {
	Load(ctx context.Context, projectID string) (Project, error)
	Save(ctx context.Context, project Project) error
}

// SaveService rejects an invalid project before the repository is touched.
// No valid subset of tracks is ever committed.
type SaveService struct {
	repo Repository
}

func NewSaveService(repo Repository) *SaveService {
	return &SaveService{repo: repo}
}

func (s *SaveService) Save(ctx context.Context, project Project) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	normalized, err := NormalizeProject(project)
	if err != nil {
		return err
	}
	return s.repo.Save(ctx, normalized)
}

// MigrateAndSave validates the migrated project before performing one save.
func (s *SaveService) MigrateAndSave(ctx context.Context, table EditTable) (Project, error) {
	project, err := Migrate(table)
	if err != nil {
		return Project{}, err
	}
	if err := s.Save(ctx, project); err != nil {
		return Project{}, err
	}
	return project, nil
}

// MemoryRepository is an in-process repository useful for previews and tests.
// Save validates and canonicalizes the full project before replacing the
// previous value under one lock.
type MemoryRepository struct {
	mu       sync.RWMutex
	projects map[string]Project
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{projects: make(map[string]Project)}
}

func (r *MemoryRepository) Load(ctx context.Context, projectID string) (Project, error) {
	if err := ctx.Err(); err != nil {
		return Project{}, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	project, ok := r.projects[projectID]
	if !ok {
		return Project{}, ErrProjectNotFound
	}
	return project.Clone(), nil
}

func (r *MemoryRepository) Save(ctx context.Context, project Project) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	normalized, err := NormalizeProject(project)
	if err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.projects[normalized.ID] = normalized.Clone()
	return nil
}
