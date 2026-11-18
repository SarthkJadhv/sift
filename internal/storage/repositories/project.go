package repositories

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

var (
	ErrProjectNotFound = errors.New("project not found")

	ErrProjectIdentityConflict = errors.New(
		"project identity conflicts with existing registration",
	)
)

type Project struct {
	ID        string
	Name      string
	RootPath  string
	CreatedAt time.Time
}

type ProjectRepository struct {
	db *sql.DB
}

// NewProjectRepository creates a project repository.
func NewProjectRepository(db *sql.DB) (*ProjectRepository, error) {
	if db == nil {
		return nil, errors.New("database cannot be nil")
	}

	return &ProjectRepository{
		db: db,
	}, nil
}

// validateProject validates and normalizes project metadata.
func validateProject(p Project) (Project, error) {
	p.ID = strings.TrimSpace(p.ID)
	p.Name = strings.TrimSpace(p.Name)
	p.RootPath = filepath.Clean(p.RootPath)

	if p.ID == "" {
		return p, errors.New("project ID is required")
	}

	if p.Name == "" {
		return p, errors.New("project name is required")
	}

	if !filepath.IsAbs(p.RootPath) {
		return p, errors.New("project root path must be absolute")
	}

	if p.CreatedAt.IsZero() {
		return p, errors.New("project creation timestamp is required")
	}

	return p, nil
}

// Create inserts a new project into the database.
func (r *ProjectRepository) Create(
	ctx context.Context,
	p Project,
) error {
	if r == nil || r.db == nil {
		return errors.New("project repository is not initialized")
	}

	p, err := validateProject(p)
	if err != nil {
		return err
	}

	_, err = r.db.ExecContext(ctx, `
		INSERT INTO projects (
			id,
			name,
			root_path,
			created_at
		)
		VALUES (?, ?, ?, ?)
	`,
		p.ID,
		p.Name,
		p.RootPath,
		p.CreatedAt.UTC().Format(time.RFC3339Nano),
	)

	if err != nil {
		return fmt.Errorf("create project: %w", err)
	}

	return nil
}

// Register inserts a project if its root path is not registered.
// Existing registrations retain their original project identity.
func (r *ProjectRepository) Register(
	ctx context.Context,
	p Project,
) (Project, bool, error) {
	var result Project

	if r == nil || r.db == nil {
		return result, false, errors.New(
			"project repository is not initialized",
		)
	}

	p, err := validateProject(p)
	if err != nil {
		return result, false, err
	}

	res, err := r.db.ExecContext(ctx, `
		INSERT INTO projects (
			id,
			name,
			root_path,
			created_at
		)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(root_path) DO NOTHING
	`,
		p.ID,
		p.Name,
		p.RootPath,
		p.CreatedAt.UTC().Format(time.RFC3339Nano),
	)

	if err != nil {
		return result, false, fmt.Errorf(
			"register project: %w", err,
		)
	}

	affected, err := res.RowsAffected()
	if err != nil {
		return result, false, fmt.Errorf(
			"check project registration: %w", err,
		)
	}

	existing, err := r.GetByRootPath(ctx, p.RootPath)
	if err != nil {
		return result, false, fmt.Errorf(
			"retrieve registered project: %w", err,
		)
	}

	if existing.ID != p.ID {
		return result, false, fmt.Errorf(
			"%w: root path %s belongs to project %s",
			ErrProjectIdentityConflict,
			p.RootPath,
			existing.ID,
		)
	}

	return existing, affected == 1, nil
}

// GetByID retrieves a project using its unique ID.
func (r *ProjectRepository) GetByID(
	ctx context.Context,
	id string,
) (Project, error) {
	if r == nil || r.db == nil {
		return Project{}, errors.New(
			"project repository is not initialized",
		)
	}

	id = strings.TrimSpace(id)
	if id == "" {
		return Project{}, errors.New("project ID is required")
	}

	return r.findOne(ctx, `
		SELECT id, name, root_path, created_at
		FROM projects
		WHERE id = ?
	`, id)
}

// GetByRootPath retrieves a project using its repository path.
func (r *ProjectRepository) GetByRootPath(
	ctx context.Context,
	rootPath string,
) (Project, error) {
	if r == nil || r.db == nil {
		return Project{}, errors.New(
			"project repository is not initialized",
		)
	}

	if !filepath.IsAbs(rootPath) {
		return Project{}, errors.New(
			"project root path must be absolute",
		)
	}

	rootPath = filepath.Clean(rootPath)

	return r.findOne(ctx, `
		SELECT id, name, root_path, created_at
		FROM projects
		WHERE root_path = ?
	`, rootPath)
}

// List retrieves all registered projects.
func (r *ProjectRepository) List(
	ctx context.Context,
) ([]Project, error) {
	if r == nil || r.db == nil {
		return nil, errors.New(
			"project repository is not initialized",
		)
	}

	rows, err := r.db.QueryContext(ctx, `
		SELECT id, name, root_path, created_at
		FROM projects
		ORDER BY created_at ASC, id ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}
	defer rows.Close()

	projects := make([]Project, 0)

	for rows.Next() {
		p, err := scanProject(rows)
		if err != nil {
			return nil, err
		}

		projects = append(projects, p)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate projects: %w", err)
	}

	return projects, nil
}

// Update modifies the name and root path of an existing project.
// Project ID and creation timestamp remain unchanged.
func (r *ProjectRepository) Update(
	ctx context.Context,
	p Project,
) error {
	if r == nil || r.db == nil {
		return errors.New("project repository is not initialized")
	}

	p, err := validateProject(p)
	if err != nil {
		return err
	}

	res, err := r.db.ExecContext(ctx, `
		UPDATE projects
		SET name = ?, root_path = ?
		WHERE id = ?
	`,
		p.Name,
		p.RootPath,
		p.ID,
	)

	if err != nil {
		return fmt.Errorf("update project: %w", err)
	}

	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("check project update: %w", err)
	}

	if affected == 0 {
		return fmt.Errorf("%w: %s", ErrProjectNotFound, p.ID)
	}

	return nil
}

// DeleteByID removes a registered project.
func (r *ProjectRepository) DeleteByID(
	ctx context.Context,
	id string,
) error {
	if r == nil || r.db == nil {
		return errors.New("project repository is not initialized")
	}

	id = strings.TrimSpace(id)
	if id == "" {
		return errors.New("project ID is required")
	}

	res, err := r.db.ExecContext(
		ctx,
		"DELETE FROM projects WHERE id = ?",
		id,
	)
	if err != nil {
		return fmt.Errorf("delete project: %w", err)
	}

	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("check project deletion: %w", err)
	}

	if affected == 0 {
		return fmt.Errorf("%w: %s", ErrProjectNotFound, id)
	}

	return nil
}

// findOne executes a project lookup query.
func (r *ProjectRepository) findOne(
	ctx context.Context,
	query string,
	args ...any,
) (Project, error) {
	row := r.db.QueryRowContext(ctx, query, args...)

	p, err := scanProject(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Project{}, fmt.Errorf(
				"%w", ErrProjectNotFound,
			)
		}

		return Project{}, fmt.Errorf("read project: %w", err)
	}

	return p, nil
}

// scanProject converts a database row into a Project.
func scanProject(row interface {
	Scan(dest ...any) error
}) (Project, error) {
	var p Project
	var createdAt string

	err := row.Scan(
		&p.ID,
		&p.Name,
		&p.RootPath,
		&createdAt,
	)
	if err != nil {
		return Project{}, err
	}

	p.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return Project{}, fmt.Errorf(
			"parse project creation timestamp: %w", err,
		)
	}

	return p, nil
}
