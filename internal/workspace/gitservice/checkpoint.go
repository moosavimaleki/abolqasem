package gitservice

import (
	"context"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

const (
	CodeCheckpointKindGit  = "git"
	CodeCheckpointKindNone = "none"
)

type CodeCheckpoint struct {
	Kind           string `json:"kind"`
	Status         string `json:"status"`
	RepositoryRoot string `json:"repositoryRoot,omitempty"`
	BranchName     string `json:"branchName,omitempty"`
	Commit         string `json:"commit,omitempty"`
	Ref            string `json:"ref,omitempty"`
	FileCount      int    `json:"fileCount,omitempty"`
	ByteCount      int64  `json:"byteCount,omitempty"`
	Warning        string `json:"warning,omitempty"`
}

func CreateCodeCheckpoint(ctx context.Context, localPath string, checkpointID string, message string) (CodeCheckpoint, error) {
	snapshot, err := Detect(ctx, localPath)
	if err != nil {
		return CodeCheckpoint{}, err
	}
	if snapshot.Status != StatusReady {
		return CodeCheckpoint{
			Kind:    CodeCheckpointKindNone,
			Status:  snapshot.Status,
			Warning: "Git repository is not ready.",
		}, nil
	}

	indexFile, err := os.CreateTemp("", "abolqasem-checkpoint-index-*")
	if err != nil {
		return CodeCheckpoint{}, err
	}
	indexPath := indexFile.Name()
	_ = indexFile.Close()
	_ = os.Remove(indexPath)
	defer os.Remove(indexPath)

	env := checkpointGitEnv(indexPath)
	head, headErr := gitOutput(ctx, snapshot.RepositoryRoot, "rev-parse", "--verify", "HEAD")
	if headErr == nil && strings.TrimSpace(head) != "" {
		if _, err := gitCheckpointOutput(ctx, snapshot.RepositoryRoot, env, "read-tree", head); err != nil {
			return CodeCheckpoint{}, err
		}
	} else if _, err := gitCheckpointOutput(ctx, snapshot.RepositoryRoot, env, "read-tree", "--empty"); err != nil {
		return CodeCheckpoint{}, err
	}
	if _, err := gitCheckpointOutput(ctx, snapshot.RepositoryRoot, env, "add", "-A", "--", "."); err != nil {
		return CodeCheckpoint{}, err
	}
	tree, err := gitCheckpointOutput(ctx, snapshot.RepositoryRoot, env, "write-tree")
	if err != nil {
		return CodeCheckpoint{}, err
	}

	commitArgs := []string{"commit-tree", strings.TrimSpace(tree), "-m", firstNonEmptyGitRef(message, "Abolqasem checkpoint")}
	if headErr == nil && strings.TrimSpace(head) != "" {
		commitArgs = append(commitArgs, "-p", strings.TrimSpace(head))
	}
	commit, err := gitCheckpointOutput(ctx, snapshot.RepositoryRoot, env, commitArgs...)
	if err != nil {
		return CodeCheckpoint{}, err
	}
	commit = strings.TrimSpace(commit)
	ref := "refs/abolqasem/checkpoints/" + checkpointID
	if _, err := gitOutput(ctx, snapshot.RepositoryRoot, "update-ref", ref, commit); err != nil {
		return CodeCheckpoint{}, err
	}

	fileCount := 0
	if output, err := gitOutput(ctx, snapshot.RepositoryRoot, "ls-tree", "-r", "--name-only", commit); err == nil && strings.TrimSpace(output) != "" {
		fileCount = len(strings.Split(output, "\n"))
	}
	return CodeCheckpoint{
		Kind:           CodeCheckpointKindGit,
		Status:         StatusReady,
		RepositoryRoot: snapshot.RepositoryRoot,
		BranchName:     snapshot.BranchName,
		Commit:         commit,
		Ref:            ref,
		FileCount:      fileCount,
	}, nil
}

func RestoreCodeCheckpoint(ctx context.Context, checkpoint CodeCheckpoint) (BranchActionResult, error) {
	result := BranchActionResult{
		BranchName:      checkpoint.BranchName,
		SnapshotChanged: true,
	}
	if checkpoint.Kind != CodeCheckpointKindGit || strings.TrimSpace(checkpoint.RepositoryRoot) == "" || strings.TrimSpace(checkpoint.Commit) == "" {
		return branchFailure(result, "Checkpoint unavailable", "This checkpoint does not contain a git code snapshot.", ""), nil
	}
	commit := strings.TrimSpace(checkpoint.Commit)
	if _, err := gitOutput(ctx, checkpoint.RepositoryRoot, "cat-file", "-e", commit+"^{commit}"); err != nil {
		return branchFailure(result, "Checkpoint unavailable", "The git checkpoint ref is missing or invalid.", err.Error()), nil
	}
	if output, err := gitOutput(ctx, checkpoint.RepositoryRoot, "read-tree", "--reset", "-u", commit); err != nil {
		return branchFailure(result, "Restore failed", err.Error(), output), nil
	}
	if output, err := gitOutput(ctx, checkpoint.RepositoryRoot, "clean", "-fd", "--", "."); err != nil {
		return branchFailure(result, "Restore cleanup failed", err.Error(), output), nil
	}
	if snapshot, err := Detect(ctx, checkpoint.RepositoryRoot); err == nil {
		result.BranchName = snapshot.BranchName
	}
	result.OK = true
	return result, nil
}

func checkpointGitEnv(indexPath string) []string {
	now := strconv.FormatInt(time.Now().Unix(), 10) + " +0000"
	return []string{
		"GIT_INDEX_FILE=" + indexPath,
		"GIT_AUTHOR_NAME=Abolqasem",
		"GIT_AUTHOR_EMAIL=abolqasem@localhost",
		"GIT_AUTHOR_DATE=" + now,
		"GIT_COMMITTER_NAME=Abolqasem",
		"GIT_COMMITTER_EMAIL=abolqasem@localhost",
		"GIT_COMMITTER_DATE=" + now,
	}
}

func gitCheckpointOutput(ctx context.Context, dir string, env []string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	output, err := cmd.CombinedOutput()
	text := strings.TrimRight(string(output), "\r\n")
	if err != nil {
		return "", gitCommandError{err: err, output: text}
	}
	return strings.TrimSpace(text), nil
}
