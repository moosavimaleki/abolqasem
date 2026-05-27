package gitservice

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func TestGetGitHubPublishInfoWhenGHMissing(t *testing.T) {
	restore := stubGitHubCLI(t, false, nil)
	defer restore()

	info, err := GetGitHubPublishInfo(context.Background(), filepath.Join(t.TempDir(), "My Project"))
	if err != nil {
		t.Fatalf("GetGitHubPublishInfo returned error: %v", err)
	}
	if info.GHInstalled || info.Authenticated {
		t.Fatalf("expected missing gh, got %#v", info)
	}
	if info.SuggestedRepoName != "My-Project" {
		t.Fatalf("unexpected suggested repo name: %q", info.SuggestedRepoName)
	}
}

func TestGetGitHubPublishInfoWithAuthenticatedGH(t *testing.T) {
	restore := stubGitHubCLI(t, true, map[string]string{
		"auth status -h github.com":    "",
		"api user --jq .login":         "alice",
		"api user/orgs --jq .[].login": "team-one\nteam-two",
	})
	defer restore()

	info, err := GetGitHubPublishInfo(context.Background(), filepath.Join(t.TempDir(), "repo"))
	if err != nil {
		t.Fatalf("GetGitHubPublishInfo returned error: %v", err)
	}
	if !info.GHInstalled || !info.Authenticated {
		t.Fatalf("expected authenticated gh, got %#v", info)
	}
	if info.ActiveAccountLogin != "alice" {
		t.Fatalf("expected active account alice, got %q", info.ActiveAccountLogin)
	}
	expectedOwners := []string{"alice", "team-one", "team-two"}
	if strings.Join(info.Owners, ",") != strings.Join(expectedOwners, ",") {
		t.Fatalf("expected owners %#v, got %#v", expectedOwners, info.Owners)
	}
}

func TestCheckGitHubRepoAvailability(t *testing.T) {
	restore := stubGitHubCLI(t, true, map[string]string{
		"repo view alice/free --json name":  "__ERROR__:not found",
		"repo view alice/taken --json name": `{"name":"taken"}`,
	})
	defer restore()

	available, err := CheckGitHubRepoAvailability(context.Background(), "alice", "free")
	if err != nil {
		t.Fatalf("CheckGitHubRepoAvailability returned error: %v", err)
	}
	if !available.Available {
		t.Fatalf("expected available repo, got %#v", available)
	}
	taken, err := CheckGitHubRepoAvailability(context.Background(), "alice", "taken")
	if err != nil {
		t.Fatalf("CheckGitHubRepoAvailability returned error: %v", err)
	}
	if taken.Available {
		t.Fatalf("expected taken repo, got %#v", taken)
	}
}

func TestPublishToGitHubCreatesRepoAndPushes(t *testing.T) {
	requireGit(t)
	remote := filepath.Join(t.TempDir(), "origin.git")
	runGit(t, "", "init", "--bare", remote)
	root := canonicalGitserviceTestPath(t, t.TempDir())
	initRepoWithIdentity(t, root)
	runGit(t, root, "remote", "add", "origin", remote)
	writeFile(t, filepath.Join(root, "a.txt"), "a\n")
	runGit(t, root, "add", ".")
	runGit(t, root, "commit", "-m", "initial")

	restore := stubGitHubCLI(t, true, map[string]string{
		"repo create alice/repo --source " + root + " --remote origin --description Test repo --private": "",
	})
	defer restore()

	result, err := PublishToGitHub(context.Background(), root, "alice", "repo", "private", "Test repo")
	if err != nil {
		t.Fatalf("PublishToGitHub returned error: %v", err)
	}
	if !result.OK || !result.SnapshotChanged {
		t.Fatalf("expected publish success, got %#v", result)
	}
}

func stubGitHubCLI(t *testing.T, installed bool, outputs map[string]string) func() {
	t.Helper()
	oldLookPath := lookPath
	oldGHOutput := ghOutput
	lookPath = func(file string) (string, error) {
		if !installed {
			return "", errors.New("not found")
		}
		return "/usr/bin/gh", nil
	}
	ghOutput = func(_ context.Context, args ...string) (string, error) {
		key := strings.Join(args, " ")
		value, ok := outputs[key]
		if !ok {
			return "", errors.New("command failed")
		}
		if strings.HasPrefix(value, "__ERROR__:") {
			return "", errors.New(strings.TrimPrefix(value, "__ERROR__:"))
		}
		return value, nil
	}
	return func() {
		lookPath = oldLookPath
		ghOutput = oldGHOutput
	}
}
