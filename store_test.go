package audiotracks

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

type countingRepository struct {
	saves int
}

func (r *countingRepository) Load(context.Context, string) (Project, error) {
	return Project{}, ErrProjectNotFound
}

func (r *countingRepository) Save(context.Context, Project) error {
	r.saves++
	return nil
}

func TestSaveServiceDoesNotPersistInvalidProject(t *testing.T) {
	repo := &countingRepository{}
	service := NewSaveService(repo)
	project := validProject()
	project.Tracks = append(project.Tracks,
		Track{ID: "en-2", Language: "en_US", Type: TrackDub, Default: true, Audible: true})

	err := service.Save(context.Background(), project)
	var validation *ValidationError
	if !errors.As(err, &validation) {
		t.Fatalf("error = %v, want ValidationError", err)
	}
	if repo.saves != 0 {
		t.Fatalf("repository was called %d times for an invalid project", repo.saves)
	}
}

func TestFailedMigrationLeavesPreviousProjectUntouched(t *testing.T) {
	ctx := context.Background()
	repo := NewMemoryRepository()
	service := NewSaveService(repo)
	initial, err := NormalizeProject(validProject())
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Save(ctx, initial); err != nil {
		t.Fatal(err)
	}

	def := true
	_, err = service.MigrateAndSave(ctx, EditTable{
		ProjectID: "film-001",
		Tracks: []EditTableTrack{
			{ID: "en-1", Name: "Dialogue", Role: "audio", Language: "en_US", Default: &def},
			{ID: "en-2", Name: "Dub", Role: "audio", Language: "en_US", Default: &def},
		},
	})
	var validation *ValidationError
	if !errors.As(err, &validation) || !validation.HasCode(CodeDuplicateDefault) {
		t.Fatalf("error = %#v, want duplicate-default ValidationError", err)
	}

	got, err := repo.Load(ctx, "film-001")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, initial) {
		t.Fatalf("failed save changed repository:\ngot  %#v\nwant %#v", got, initial)
	}
}

func TestMemoryRepositoryRejectsInvalidReplacement(t *testing.T) {
	ctx := context.Background()
	repo := NewMemoryRepository()
	if err := repo.Save(ctx, validProject()); err != nil {
		t.Fatal(err)
	}
	before, err := repo.Load(ctx, "film-001")
	if err != nil {
		t.Fatal(err)
	}

	invalid := validProject()
	invalid.Tracks[0].Type = TrackType("audio")
	if err := repo.Save(ctx, invalid); err == nil {
		t.Fatal("Save() succeeded for an invalid project")
	}
	after, err := repo.Load(ctx, "film-001")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(after, before) {
		t.Fatalf("invalid save replaced project:\ngot  %#v\nwant %#v", after, before)
	}
}

func TestSaveProjectJSONAtomicRejectsWholeDocument(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "project.json")
	if err := os.WriteFile(path, []byte("previous\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	invalid := validProject()
	invalid.Tracks = append(invalid.Tracks,
		Track{ID: "en-2", Language: "EN-us", Type: TrackDub, Default: true, Audible: true})
	if err := SaveProjectJSONAtomic(path, invalid); err == nil {
		t.Fatal("SaveProjectJSONAtomic() succeeded for invalid project")
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "previous\n" {
		t.Fatalf("invalid save changed output file: %q", content)
	}

	if err := SaveProjectJSONAtomic(path, validProject()); err != nil {
		t.Fatal(err)
	}
	content, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) == "previous\n" {
		t.Fatal("valid save did not replace output file")
	}
}
