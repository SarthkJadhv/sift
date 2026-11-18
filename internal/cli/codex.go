package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/SarthkJadhv/sift/internal/agents/codex"
	"github.com/SarthkJadhv/sift/internal/appdata"
	"github.com/SarthkJadhv/sift/internal/config"
	"github.com/SarthkJadhv/sift/internal/project"
	"github.com/SarthkJadhv/sift/internal/storage"
	"github.com/SarthkJadhv/sift/internal/storage/repositories"
	"github.com/spf13/cobra"
)

func newCodexCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "codex",
		Short: "Manage Codex activity ingestion",
	}

	cmd.AddCommand(newCodexImportCommand())
	cmd.AddCommand(newCodexStatsCommand())
	cmd.AddCommand(newCodexHistoryCommand())

	return cmd
}

func newCodexImportCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "import",
		Short: "Import Codex activity for the current SIFT project",
		Args:  cobra.NoArgs,
		RunE:  runCodexImport,
	}
}

func runCodexImport(cmd *cobra.Command, _ []string) error {
	ctx := cmd.Context()

	// Resolve the current SIFT project.
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}

	root, err := project.FindRoot(cwd)
	if err != nil {
		return fmt.Errorf("find project root: %w", err)
	}

	configPath := filepath.Join(root, ".sift", "config.json")

	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf(
			"load SIFT project configuration: %w; run sift init first",
			err,
		)
	}

	// Open the existing SIFT database.
	paths, err := appdata.Resolve()
	if err != nil {
		return fmt.Errorf("resolve SIFT application data: %w", err)
	}

	if _, err := os.Stat(paths.Database); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("SIFT database not found; run sift init first")
		}
		return fmt.Errorf("access SIFT database: %w", err)
	}

	db, err := storage.OpenSQLite(ctx, paths.Database)
	if err != nil {
		return fmt.Errorf("open SIFT database: %w", err)
	}
	defer db.Close()

	if err := db.Migrate(ctx); err != nil {
		return fmt.Errorf("apply database migrations: %w", err)
	}

	projectRepo, err := repositories.NewProjectRepository(db.DB)
	if err != nil {
		return fmt.Errorf("create project repository: %w", err)
	}

	registeredProject, err := projectRepo.GetByID(ctx, cfg.ProjectID)
	if err != nil {
		return fmt.Errorf("find registered SIFT project: %w", err)
	}

	if registeredProject.RootPath != cfg.RootPath {
		return fmt.Errorf("project configuration does not match registered project")
	}

	codexRepo := repositories.NewCodexRepository(db.DB)

	// Discover active and archived Codex rollout files.
	rollouts, err := codex.DiscoverRollouts("")
	if err != nil {
		return fmt.Errorf("discover Codex rollouts: %w", err)
	}

	fmt.Fprintf(
		cmd.OutOrStdout(),
		"Scanning %d Codex rollout files for project %s...\n",
		len(rollouts),
		registeredProject.Name,
	)

	var imported, unchanged, skipped, failed int

	for _, rollout := range rollouts {
		// Skip files that have already been imported without changes.
		isUnchanged, err := codexRepo.IsUnchanged(
			ctx,
			registeredProject.ID,
			rollout.Path,
			rollout.Size,
			rollout.ModTimeNS,
		)
		if err != nil {
			failed++
			fmt.Fprintf(
				cmd.ErrOrStderr(),
				"Warning: could not check rollout %s: %v\n",
				filepath.Base(rollout.Path),
				err,
			)
			continue
		}

		if isUnchanged {
			unchanged++
			continue
		}

		session, err := codex.ParseRolloutFile(rollout.Path)
		if err != nil {
			failed++
			fmt.Fprintf(
				cmd.ErrOrStderr(),
				"Warning: could not parse rollout %s: %v\n",
				filepath.Base(rollout.Path),
				err,
			)
			continue
		}

		workingDirectory := normalizeProjectPath(session.WorkingDirectory)

		if workingDirectory == "" ||
			!isWithinProject(root, workingDirectory) {
			skipped++
			continue
		}

		if err := codexRepo.Import(
			ctx,
			registeredProject.ID,
			rollout.Path,
			rollout.Size,
			rollout.ModTimeNS,
			session,
		); err != nil {
			failed++
			fmt.Fprintf(
				cmd.ErrOrStderr(),
				"Warning: could not import rollout %s: %v\n",
				filepath.Base(rollout.Path),
				err,
			)
			continue
		}

		imported++
	}

	fmt.Fprintln(cmd.OutOrStdout())
	fmt.Fprintln(cmd.OutOrStdout(), "Codex import complete")
	fmt.Fprintf(cmd.OutOrStdout(), "  Imported:  %d\n", imported)
	fmt.Fprintf(cmd.OutOrStdout(), "  Unchanged: %d\n", unchanged)
	fmt.Fprintf(cmd.OutOrStdout(), "  Skipped:   %d\n", skipped)
	fmt.Fprintf(cmd.OutOrStdout(), "  Failed:    %d\n", failed)

	if failed > 0 {
		return fmt.Errorf("%d Codex rollout files could not be imported", failed)
	}

	return nil
}

func newCodexStatsCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "stats",
		Short: "Display Codex activity statistics for the current project",
		Args:  cobra.NoArgs,
		RunE:  runCodexStats,
	}
}

func newCodexHistoryCommand() *cobra.Command {
	var limit int

	cmd := &cobra.Command{
		Use:   "history",
		Short: "Display recent Codex sessions for the current project",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runCodexHistory(cmd, limit)
		},
	}

	cmd.Flags().IntVarP(
		&limit,
		"limit",
		"n",
		20,
		"Maximum number of sessions to display",
	)

	return cmd
}

func runCodexHistory(cmd *cobra.Command, limit int) error {
	ctx := cmd.Context()

	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}

	root, err := project.FindRoot(cwd)
	if err != nil {
		return fmt.Errorf("find project root: %w", err)
	}

	cfg, err := config.Load(filepath.Join(root, ".sift", "config.json"))
	if err != nil {
		return fmt.Errorf("load SIFT configuration: %w; run sift init first", err)
	}

	paths, err := appdata.Resolve()
	if err != nil {
		return fmt.Errorf("resolve SIFT application data: %w", err)
	}

	db, err := storage.OpenSQLite(ctx, paths.Database)
	if err != nil {
		return fmt.Errorf("open SIFT database: %w", err)
	}
	defer db.Close()

	projectRepo, err := repositories.NewProjectRepository(db.DB)
	if err != nil {
		return fmt.Errorf("create project repository: %w", err)
	}

	registeredProject, err := projectRepo.GetByID(ctx, cfg.ProjectID)
	if err != nil {
		return fmt.Errorf("find registered project: %w", err)
	}

	if registeredProject.RootPath != cfg.RootPath {
		return fmt.Errorf("project configuration does not match registered project")
	}

	codexRepo := repositories.NewCodexRepository(db.DB)

	sessions, err := codexRepo.ListSessions(
		ctx,
		registeredProject.ID,
		limit,
	)
	if err != nil {
		return fmt.Errorf("get Codex session history: %w", err)
	}

	out := cmd.OutOrStdout()

	fmt.Fprintf(out, "\nCodex Session History — %s\n", registeredProject.Name)
	fmt.Fprintln(out, "----------------------------------------")

	if len(sessions) == 0 {
		fmt.Fprintln(out, "No imported Codex sessions found.")
		return nil
	}

	for i, session := range sessions {
		fmt.Fprintf(
			out,
			"\n%d. %s\n",
			i+1,
			session.SessionID,
		)

		startedAt := "Unknown"
		if !session.StartedAt.IsZero() {
			startedAt = session.StartedAt.Format("2006-01-02 15:04:05")
		}

		fmt.Fprintf(out, "   Started:     %s\n", startedAt)
		fmt.Fprintf(out, "   Models:      %s\n", displayOrUnknown(session.Models))
		fmt.Fprintf(out, "   Turns:       %d (%d completed)\n",
			session.Turns, session.CompletedTurns)
		fmt.Fprintf(out, "   Responses:   %d\n", session.Responses)
		fmt.Fprintf(out, "   Tokens:      %d\n", session.TotalTokens)
		fmt.Fprintf(out, "   Tool calls:  %d\n", session.ToolCalls)
		fmt.Fprintf(out, "   Turn time:   %s\n", session.TotalTurnDuration)
	}

	return nil
}

func displayOrUnknown(value string) string {
	if strings.TrimSpace(value) == "" {
		return "Unknown"
	}

	return value
}

func runCodexStats(cmd *cobra.Command, _ []string) error {
	ctx := cmd.Context()

	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}

	root, err := project.FindRoot(cwd)
	if err != nil {
		return fmt.Errorf("find project root: %w", err)
	}

	cfg, err := config.Load(filepath.Join(root, ".sift", "config.json"))
	if err != nil {
		return fmt.Errorf("load SIFT configuration: %w; run sift init first", err)
	}

	paths, err := appdata.Resolve()
	if err != nil {
		return fmt.Errorf("resolve SIFT application data: %w", err)
	}

	db, err := storage.OpenSQLite(ctx, paths.Database)
	if err != nil {
		return fmt.Errorf("open SIFT database: %w", err)
	}
	defer db.Close()

	projectRepo, err := repositories.NewProjectRepository(db.DB)
	if err != nil {
		return fmt.Errorf("create project repository: %w", err)
	}

	registeredProject, err := projectRepo.GetByID(ctx, cfg.ProjectID)
	if err != nil {
		return fmt.Errorf("find registered SIFT project: %w", err)
	}

	if registeredProject.RootPath != cfg.RootPath {
		return fmt.Errorf("project configuration does not match registered project")
	}

	codexRepo := repositories.NewCodexRepository(db.DB)

	stats, err := codexRepo.GetStats(ctx, registeredProject.ID)
	if err != nil {
		return fmt.Errorf("get Codex statistics: %w", err)
	}

	modelUsage, err := codexRepo.GetModelUsage(ctx, registeredProject.ID)
	if err != nil {
		return fmt.Errorf("get model usage: %w", err)
	}

	effortUsage, err := codexRepo.GetEffortUsage(ctx, registeredProject.ID)
	if err != nil {
		return fmt.Errorf("get effort usage: %w", err)
	}

	out := cmd.OutOrStdout()

	fmt.Fprintln(out)
	fmt.Fprintf(out, "SIFT Codex Statistics — %s\n", registeredProject.Name)
	fmt.Fprintln(out, "----------------------------------------")

	fmt.Fprintln(out, "\nACTIVITY")
	fmt.Fprintf(out, "  Sessions:          %d\n", stats.Sessions)
	fmt.Fprintf(out, "  Turns:             %d\n", stats.Turns)
	fmt.Fprintf(out, "  Completed turns:   %d\n", stats.CompletedTurns)
	fmt.Fprintf(out, "  Responses:         %d\n", stats.Responses)

	fmt.Fprintln(out, "\nTOKEN USAGE")
	fmt.Fprintf(out, "  Input tokens:      %d\n", stats.InputTokens)
	fmt.Fprintf(out, "  Cached input:      %d (included in input)\n", stats.CachedInputTokens)
	fmt.Fprintf(out, "  Output tokens:     %d\n", stats.OutputTokens)
	fmt.Fprintf(out, "  Reasoning output:  %d (included in output)\n", stats.ReasoningOutputTokens)
	fmt.Fprintf(out, "  Total tokens:      %d\n", stats.TotalTokens)

	fmt.Fprintln(out, "\nTOOL ACTIVITY")
	fmt.Fprintf(out, "  Tool calls:        %d\n", stats.ToolCalls)
	fmt.Fprintf(out, "  With output:       %d\n", stats.ToolsWithOutput)

	fmt.Fprintln(out, "\nPERFORMANCE")
	fmt.Fprintf(out, "  Avg turn duration: %s\n", stats.AverageTurnDuration)

	fmt.Fprintln(out, "\nMODEL BREAKDOWN")

	if len(modelUsage) == 0 {
		fmt.Fprintln(out, "  No model usage recorded")
	}

	for _, usage := range modelUsage {
		fmt.Fprintf(out, "  Model: %s\n", usage.Model)
		fmt.Fprintf(out, "    Responses:     %d\n", usage.Responses)
		fmt.Fprintf(out, "    Input tokens:  %d\n", usage.InputTokens)
		fmt.Fprintf(out, "    Output tokens: %d\n", usage.OutputTokens)
		fmt.Fprintf(out, "    Total tokens:  %d\n", usage.TotalTokens)
	}

	fmt.Fprintln(out, "\nREASONING EFFORT BREAKDOWN")

	if len(effortUsage) == 0 {
		fmt.Fprintln(out, "  No effort data recorded")
	}

	for _, usage := range effortUsage {
		fmt.Fprintf(out, "  Effort: %s\n", usage.Effort)
		fmt.Fprintf(out, "    Responses:    %d\n", usage.Responses)
		fmt.Fprintf(out, "    Total tokens: %d\n", usage.TotalTokens)
	}
	return nil
}

func normalizeProjectPath(path string) string {
	if path == "" {
		return ""
	}

	absolute, err := filepath.Abs(path)
	if err != nil {
		return ""
	}

	resolved, err := filepath.EvalSymlinks(absolute)
	if err == nil {
		absolute = resolved
	}

	return filepath.Clean(absolute)
}

func isWithinProject(root, workingDirectory string) bool {
	root = normalizeProjectPath(root)
	workingDirectory = normalizeProjectPath(workingDirectory)

	if root == "" || workingDirectory == "" {
		return false
	}

	relative, err := filepath.Rel(root, workingDirectory)
	if err != nil {
		return false
	}

	return relative == "." ||
		(relative != ".." &&
			!filepath.IsAbs(relative) &&
			!strings.HasPrefix(relative, ".."+string(filepath.Separator)))
}
