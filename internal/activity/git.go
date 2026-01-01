package activity

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"

	"go.temporal.io/sdk/activity"
)

// GitActivities contains activities for Git operations.
type GitActivities struct {
	workingDir string
}

// NewGitActivities creates a new GitActivities instance.
func NewGitActivities(workingDir string) *GitActivities {
	return &GitActivities{workingDir: workingDir}
}

// GitStatusResult represents the result of git status.
type GitStatusResult struct {
	Branch         string   `json:"branch"`
	Staged         []string `json:"staged"`
	Unstaged       []string `json:"unstaged"`
	Untracked      []string `json:"untracked"`
	IsClean        bool     `json:"is_clean"`
	AheadBy        int      `json:"ahead_by"`
	BehindBy       int      `json:"behind_by"`
}

// Status returns the current git status.
func (a *GitActivities) Status(ctx context.Context, dir string) (*GitStatusResult, error) {
	logger := activity.GetLogger(ctx)
	logger.Info("Getting git status", "dir", dir)

	workDir := a.resolveDir(dir)

	// Get branch name
	branch, err := a.runGit(ctx, workDir, "branch", "--show-current")
	if err != nil {
		return nil, fmt.Errorf("failed to get current branch: %w", err)
	}

	// Get status in porcelain format
	status, err := a.runGit(ctx, workDir, "status", "--porcelain", "-b")
	if err != nil {
		return nil, fmt.Errorf("failed to get status: %w", err)
	}

	result := &GitStatusResult{
		Branch:    strings.TrimSpace(branch),
		Staged:    []string{},
		Unstaged:  []string{},
		Untracked: []string{},
	}

	lines := strings.Split(status, "\n")
	for _, line := range lines {
		if len(line) < 2 {
			continue
		}

		// Parse branch line for ahead/behind
		if strings.HasPrefix(line, "##") {
			if strings.Contains(line, "ahead") {
				fmt.Sscanf(line, "%*s [ahead %d", &result.AheadBy)
			}
			if strings.Contains(line, "behind") {
				fmt.Sscanf(line, "%*s [%*s %d", &result.BehindBy)
			}
			continue
		}

		indexStatus := line[0]
		workTreeStatus := line[1]
		file := strings.TrimSpace(line[3:])

		// Staged changes
		if indexStatus != ' ' && indexStatus != '?' {
			result.Staged = append(result.Staged, file)
		}

		// Unstaged changes
		if workTreeStatus != ' ' && workTreeStatus != '?' {
			result.Unstaged = append(result.Unstaged, file)
		}

		// Untracked files
		if indexStatus == '?' {
			result.Untracked = append(result.Untracked, file)
		}
	}

	result.IsClean = len(result.Staged) == 0 && len(result.Unstaged) == 0

	return result, nil
}

// GitCommitRequest represents a request to create a commit.
type GitCommitRequest struct {
	Dir     string   `json:"dir"`
	Message string   `json:"message"`
	Files   []string `json:"files,omitempty"` // specific files, empty = all staged
	All     bool     `json:"all"`             // -a flag
}

// GitCommitResult represents the result of a commit.
type GitCommitResult struct {
	Hash    string `json:"hash"`
	Message string `json:"message"`
	Author  string `json:"author"`
}

// Commit creates a new commit.
func (a *GitActivities) Commit(ctx context.Context, req GitCommitRequest) (*GitCommitResult, error) {
	logger := activity.GetLogger(ctx)
	logger.Info("Creating commit", "message", req.Message)

	workDir := a.resolveDir(req.Dir)

	// Add specific files if provided
	if len(req.Files) > 0 {
		args := append([]string{"add"}, req.Files...)
		if _, err := a.runGit(ctx, workDir, args...); err != nil {
			return nil, fmt.Errorf("failed to stage files: %w", err)
		}
	}

	// Build commit command
	args := []string{"commit"}
	if req.All {
		args = append(args, "-a")
	}
	args = append(args, "-m", req.Message)

	if _, err := a.runGit(ctx, workDir, args...); err != nil {
		return nil, fmt.Errorf("failed to commit: %w", err)
	}

	// Get commit info
	hash, err := a.runGit(ctx, workDir, "rev-parse", "HEAD")
	if err != nil {
		return nil, fmt.Errorf("failed to get commit hash: %w", err)
	}

	author, err := a.runGit(ctx, workDir, "log", "-1", "--format=%an <%ae>")
	if err != nil {
		author = "unknown"
	}

	return &GitCommitResult{
		Hash:    strings.TrimSpace(hash)[:7],
		Message: req.Message,
		Author:  strings.TrimSpace(author),
	}, nil
}

// GitDiffRequest represents a request for git diff.
type GitDiffRequest struct {
	Dir     string   `json:"dir"`
	Files   []string `json:"files,omitempty"`
	Staged  bool     `json:"staged"`
	Commit1 string   `json:"commit1,omitempty"`
	Commit2 string   `json:"commit2,omitempty"`
}

// GitDiffResult represents the result of git diff.
type GitDiffResult struct {
	Diff     string `json:"diff"`
	NumFiles int    `json:"num_files"`
	Added    int    `json:"added"`
	Deleted  int    `json:"deleted"`
}

// Diff returns the diff for the specified parameters.
func (a *GitActivities) Diff(ctx context.Context, req GitDiffRequest) (*GitDiffResult, error) {
	logger := activity.GetLogger(ctx)
	logger.Info("Getting diff")

	workDir := a.resolveDir(req.Dir)

	args := []string{"diff"}
	if req.Staged {
		args = append(args, "--staged")
	}
	if req.Commit1 != "" {
		args = append(args, req.Commit1)
		if req.Commit2 != "" {
			args = append(args, req.Commit2)
		}
	}
	if len(req.Files) > 0 {
		args = append(args, "--")
		args = append(args, req.Files...)
	}

	diff, err := a.runGit(ctx, workDir, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to get diff: %w", err)
	}

	// Get stat
	statArgs := append([]string{"diff", "--stat"}, args[1:]...)
	stat, _ := a.runGit(ctx, workDir, statArgs...)

	result := &GitDiffResult{
		Diff: diff,
	}

	// Parse stats from last line
	lines := strings.Split(stat, "\n")
	for _, line := range lines {
		if strings.Contains(line, "files changed") || strings.Contains(line, "file changed") {
			fmt.Sscanf(line, " %d file", &result.NumFiles)
			if strings.Contains(line, "insertion") {
				fmt.Sscanf(line, "%*d %*s %d insertion", &result.Added)
			}
			if strings.Contains(line, "deletion") {
				fmt.Sscanf(line, "%*d %*s %*d %*s %d deletion", &result.Deleted)
			}
		}
	}

	return result, nil
}

// GitBranchRequest represents a request for branch operations.
type GitBranchRequest struct {
	Dir        string `json:"dir"`
	Name       string `json:"name,omitempty"`
	Create     bool   `json:"create"`
	Checkout   bool   `json:"checkout"`
	Delete     bool   `json:"delete"`
	StartPoint string `json:"start_point,omitempty"`
}

// GitBranchResult represents the result of branch operations.
type GitBranchResult struct {
	Name     string   `json:"name"`
	Branches []string `json:"branches,omitempty"`
	Current  string   `json:"current"`
}

// Branch performs branch operations.
func (a *GitActivities) Branch(ctx context.Context, req GitBranchRequest) (*GitBranchResult, error) {
	logger := activity.GetLogger(ctx)
	logger.Info("Branch operation", "name", req.Name, "create", req.Create, "checkout", req.Checkout)

	workDir := a.resolveDir(req.Dir)

	result := &GitBranchResult{}

	if req.Delete && req.Name != "" {
		if _, err := a.runGit(ctx, workDir, "branch", "-d", req.Name); err != nil {
			return nil, fmt.Errorf("failed to delete branch: %w", err)
		}
		result.Name = req.Name
	} else if req.Create && req.Name != "" {
		args := []string{"branch", req.Name}
		if req.StartPoint != "" {
			args = append(args, req.StartPoint)
		}
		if _, err := a.runGit(ctx, workDir, args...); err != nil {
			return nil, fmt.Errorf("failed to create branch: %w", err)
		}
		result.Name = req.Name

		if req.Checkout {
			if _, err := a.runGit(ctx, workDir, "checkout", req.Name); err != nil {
				return nil, fmt.Errorf("failed to checkout branch: %w", err)
			}
		}
	} else if req.Checkout && req.Name != "" {
		if _, err := a.runGit(ctx, workDir, "checkout", req.Name); err != nil {
			return nil, fmt.Errorf("failed to checkout branch: %w", err)
		}
		result.Name = req.Name
	}

	// Get current branch
	current, err := a.runGit(ctx, workDir, "branch", "--show-current")
	if err == nil {
		result.Current = strings.TrimSpace(current)
	}

	// Get all branches
	branches, err := a.runGit(ctx, workDir, "branch", "-a")
	if err == nil {
		for _, line := range strings.Split(branches, "\n") {
			line = strings.TrimSpace(line)
			if line != "" {
				line = strings.TrimPrefix(line, "* ")
				result.Branches = append(result.Branches, line)
			}
		}
	}

	return result, nil
}

// GitLogRequest represents a request for git log.
type GitLogRequest struct {
	Dir    string `json:"dir"`
	Limit  int    `json:"limit,omitempty"`
	Since  string `json:"since,omitempty"`
	Author string `json:"author,omitempty"`
	Path   string `json:"path,omitempty"`
}

// GitLogEntry represents a log entry.
type GitLogEntry struct {
	Hash    string `json:"hash"`
	Author  string `json:"author"`
	Email   string `json:"email"`
	Date    string `json:"date"`
	Message string `json:"message"`
}

// GitLogResult represents the result of git log.
type GitLogResult struct {
	Entries []GitLogEntry `json:"entries"`
}

// Log returns the git log.
func (a *GitActivities) Log(ctx context.Context, req GitLogRequest) (*GitLogResult, error) {
	logger := activity.GetLogger(ctx)
	logger.Info("Getting git log")

	workDir := a.resolveDir(req.Dir)

	// Use format that's easy to parse
	format := "%H|%an|%ae|%ai|%s"
	args := []string{"log", "--format=" + format}

	if req.Limit > 0 {
		args = append(args, fmt.Sprintf("-n%d", req.Limit))
	} else {
		args = append(args, "-n20") // Default limit
	}

	if req.Since != "" {
		args = append(args, "--since="+req.Since)
	}

	if req.Author != "" {
		args = append(args, "--author="+req.Author)
	}

	if req.Path != "" {
		args = append(args, "--", req.Path)
	}

	output, err := a.runGit(ctx, workDir, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to get log: %w", err)
	}

	result := &GitLogResult{Entries: []GitLogEntry{}}

	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		parts := strings.SplitN(line, "|", 5)
		if len(parts) != 5 {
			continue
		}

		result.Entries = append(result.Entries, GitLogEntry{
			Hash:    parts[0][:7],
			Author:  parts[1],
			Email:   parts[2],
			Date:    parts[3],
			Message: parts[4],
		})
	}

	return result, nil
}

// GitPullRequest represents a request to pull.
type GitPullRequest struct {
	Dir    string `json:"dir"`
	Remote string `json:"remote,omitempty"`
	Branch string `json:"branch,omitempty"`
	Rebase bool   `json:"rebase"`
}

// GitPullResult represents the result of git pull.
type GitPullResult struct {
	Updated bool   `json:"updated"`
	Message string `json:"message"`
}

// Pull performs git pull.
func (a *GitActivities) Pull(ctx context.Context, req GitPullRequest) (*GitPullResult, error) {
	logger := activity.GetLogger(ctx)
	logger.Info("Pulling changes")

	workDir := a.resolveDir(req.Dir)

	args := []string{"pull"}
	if req.Rebase {
		args = append(args, "--rebase")
	}
	if req.Remote != "" {
		args = append(args, req.Remote)
		if req.Branch != "" {
			args = append(args, req.Branch)
		}
	}

	output, err := a.runGit(ctx, workDir, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to pull: %w", err)
	}

	updated := !strings.Contains(output, "Already up to date")

	return &GitPullResult{
		Updated: updated,
		Message: strings.TrimSpace(output),
	}, nil
}

// GitPushRequest represents a request to push.
type GitPushRequest struct {
	Dir       string `json:"dir"`
	Remote    string `json:"remote,omitempty"`
	Branch    string `json:"branch,omitempty"`
	SetUpstream bool `json:"set_upstream"`
}

// GitPushResult represents the result of git push.
type GitPushResult struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// Push performs git push.
func (a *GitActivities) Push(ctx context.Context, req GitPushRequest) (*GitPushResult, error) {
	logger := activity.GetLogger(ctx)
	logger.Info("Pushing changes")

	workDir := a.resolveDir(req.Dir)

	args := []string{"push"}
	if req.SetUpstream {
		args = append(args, "-u")
	}
	if req.Remote != "" {
		args = append(args, req.Remote)
		if req.Branch != "" {
			args = append(args, req.Branch)
		}
	}

	output, err := a.runGit(ctx, workDir, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to push: %w", err)
	}

	return &GitPushResult{
		Success: true,
		Message: strings.TrimSpace(output),
	}, nil
}

// Add stages files.
func (a *GitActivities) Add(ctx context.Context, dir string, files []string) error {
	workDir := a.resolveDir(dir)
	args := append([]string{"add"}, files...)
	_, err := a.runGit(ctx, workDir, args...)
	return err
}

// runGit executes a git command.
func (a *GitActivities) runGit(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		errMsg := stderr.String()
		if errMsg == "" {
			errMsg = stdout.String()
		}
		return "", fmt.Errorf("%w: %s", err, errMsg)
	}

	return stdout.String(), nil
}

// resolveDir resolves the working directory.
func (a *GitActivities) resolveDir(dir string) string {
	if dir != "" {
		return dir
	}
	return a.workingDir
}
