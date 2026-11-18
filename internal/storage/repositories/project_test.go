package repositories

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/SarthkJadhv/sift/internal/storage"
)

func newTestProjectRepository(
	t *testing.T,
) (*ProjectRepository, context.Context) {
	t.Helper()

	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "test.db")

	db, err := storage.OpenSQLite(ctx, dbPath)
	if err != nil {
		t.Fatalf("OpenSQLite() failed: %v", err)
	}

	if err := db.Migrate(ctx); err != nil {
		db.Close()
		t.Fatalf("Migrate() failed: %v", err)
	}

	repo, err := NewProjectRepository(db.DB)
	if err != nil {
		db.Close()
		t.Fatalf("NewProjectRepository() failed: %v", err)
	}

	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("failed to close database: %v", err)
		}
	})

	return repo, ctx
}

func newTestProject(id, name, root string) Project {
	return Project{
		ID:        id,
		Name:      name,
		RootPath:  root,
		CreatedAt: time.Now().UTC().Truncate(time.Second),
	}
}

func TestProjectRepositoryCreateAndGetByID(t *testing.T) {
	repo, ctx := newTestProjectRepository(t)

	root := filepath.Join(t.TempDir(), "repository")
	project := newTestProject("project-1", "sift", root)

	if err := repo.Create(ctx, project); err != nil {
		t.Fatalf("Create() failed: %v", err)
	}

	got, err := repo.GetByID(ctx, project.ID)
	if err != nil {
		t.Fatalf("GetByID() failed: %v", err)
	}

	if got.ID != project.ID {
		t.Errorf("expected ID %s, got %s", project.ID, got.ID)
	}

	if got.Name != project.Name {
		t.Errorf("expected name %s, got %s", project.Name, got.Name)
	}

	if got.RootPath != project.RootPath {
		t.Errorf("expected root %s, got %s", project.RootPath, got.RootPath)
	}

	if !got.CreatedAt.Equal(project.CreatedAt) {
		t.Error("creation timestamp does not match")
	}
}

func TestProjectRepositoryRegisterIsIdempotent(t *testing.T) {
	repo, ctx := newTestProjectRepository(t)

	project := newTestProject(
		"project-1",
		"sift",
		filepath.Join(t.TempDir(), "sift"),
	)

	first, created, err := repo.Register(ctx, project)
	if err != nil {
		t.Fatalf("first Register() failed: %v", err)
	}

	if !created {
		t.Fatal("expected first registration to create project")
	}

	second, created, err := repo.Register(ctx, project)
	if err != nil {
		t.Fatalf("second Register() failed: %v", err)
	}

	if created {
		t.Fatal("expected second registration to reuse project")
	}

	if first.ID != second.ID {
		t.Fatal("project identity changed")
	}

	projects, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List() failed: %v", err)
	}

	if len(projects) != 1 {
		t.Fatalf("expected 1 project, got %d", len(projects))
	}
}

func TestProjectRepositoryRejectsIdentityConflict(t *testing.T) {
	repo, ctx := newTestProjectRepository(t)

	root := filepath.Join(t.TempDir(), "sift")

	first := newTestProject("project-1", "sift", root)

	if _, _, err := repo.Register(ctx, first); err != nil {
		t.Fatalf("first Register() failed: %v", err)
	}

	second := newTestProject("project-2", "sift", root)

	_, _, err := repo.Register(ctx, second)

	if !errors.Is(err, ErrProjectIdentityConflict) {
		t.Fatalf(
			"expected identity conflict, got %v",
			err,
		)
	}
}

func TestProjectRepositoryGetByRootPath(t *testing.T) {
	repo, ctx := newTestProjectRepository(t)

	project := newTestProject(
		"project-1",
		"sift",
		filepath.Join(t.TempDir(), "sift"),
	)

	if err := repo.Create(ctx, project); err != nil {
		t.Fatalf("Create() failed: %v", err)
	}

	got, err := repo.GetByRootPath(ctx, project.RootPath)
	if err != nil {
		t.Fatalf("GetByRootPath() failed: %v", err)
	}

	if got.ID != project.ID {
		t.Fatalf("expected project %s, got %s", project.ID, got.ID)
	}
}

func TestProjectRepositoryList(t *testing.T) {
	repo, ctx := newTestProjectRepository(t)

	first := newTestProject(
		"project-1",
		"first",
		filepath.Join(t.TempDir(), "first"),
	)

	second := newTestProject(
		"project-2",
		"second",
		filepath.Join(t.TempDir(), "second"),
	)

	second.CreatedAt = first.CreatedAt.Add(time.Minute)

	if err := repo.Create(ctx, first); err != nil {
		t.Fatalf("Create(first) failed: %v", err)
	}

	if err := repo.Create(ctx, second); err != nil {
		t.Fatalf("Create(second) failed: %v", err)
	}

	projects, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List() failed: %v", err)
	}

	if len(projects) != 2 {
		t.Fatalf("expected 2 projects, got %d", len(projects))
	}

	if projects[0].ID != first.ID {
		t.Error("projects are not returned in creation order")
	}

	if projects[1].ID != second.ID {
		t.Error("unexpected second project")
	}
}

func TestProjectRepositoryUpdate(t *testing.T) {
	repo, ctx := newTestProjectRepository(t)

	project := newTestProject(
		"project-1",
		"sift",
		filepath.Join(t.TempDir(), "sift"),
	)

	if err := repo.Create(ctx, project); err != nil {
		t.Fatalf("Create() failed: %v", err)
	}

	project.Name = "sift-updated"
	project.RootPath = filepath.Join(t.TempDir(), "sift-updated")

	if err := repo.Update(ctx, project); err != nil {
		t.Fatalf("Update() failed: %v", err)
	}

	got, err := repo.GetByID(ctx, project.ID)
	if err != nil {
		t.Fatalf("GetByID() failed: %v", err)
	}

	if got.Name != project.Name {
		t.Error("project name was not updated")
	}

	if got.RootPath != project.RootPath {
		t.Error("project root was not updated")
	}
}

func TestProjectRepositoryDelete(t *testing.T) {
	repo, ctx := newTestProjectRepository(t)

	project := newTestProject(
		"project-1",
		"sift",
		filepath.Join(t.TempDir(), "sift"),
	)

	if err := repo.Create(ctx, project); err != nil {
		t.Fatalf("Create() failed: %v", err)
	}

	if err := repo.DeleteByID(ctx, project.ID); err != nil {
		t.Fatalf("DeleteByID() failed: %v", err)
	}

	_, err := repo.GetByID(ctx, project.ID)

	if !errors.Is(err, ErrProjectNotFound) {
		t.Fatalf("expected ErrProjectNotFound, got %v", err)
	}
}
