package gitservice

import (
	"context"
	"strconv"
	"strings"
)

func branchHistory(ctx context.Context, root string, ref string, originRepoSlug string, limit int) BranchHistorySnapshot {
	if limit <= 0 {
		limit = 20
	}
	ref = strings.TrimSpace(ref)
	if ref == "" {
		ref = "HEAD"
	}

	output, err := gitOutput(ctx, root, "log", "--max-count", strconv.Itoa(limit), "--pretty=format:%H%x1f%s%x1f%b%x1f%an%x1f%aI%x1f%D%x1e", ref)
	if err != nil || strings.TrimSpace(output) == "" {
		return BranchHistorySnapshot{Entries: []BranchHistoryEntry{}}
	}

	entries := make([]BranchHistoryEntry, 0, limit)
	for _, record := range strings.Split(output, "\x1e") {
		record = strings.Trim(record, "\r\n")
		if strings.TrimSpace(record) == "" {
			continue
		}
		parts := strings.SplitN(record, "\x1f", 6)
		if len(parts) < 5 {
			continue
		}
		sha := strings.TrimSpace(parts[0])
		summary := strings.TrimSpace(parts[1])
		authoredAt := strings.TrimSpace(parts[4])
		if sha == "" || summary == "" || authoredAt == "" {
			continue
		}
		entries = append(entries, BranchHistoryEntry{
			SHA:         sha,
			Summary:     summary,
			Description: strings.TrimSpace(parts[2]),
			AuthorName:  strings.TrimSpace(parts[3]),
			AuthoredAt:  authoredAt,
			Tags:        tagsFromDecorations(parts),
			GitHubURL:   githubCommitURL(originRepoSlug, sha),
		})
	}

	return BranchHistorySnapshot{Entries: entries}
}

func tagsFromDecorations(parts []string) []string {
	if len(parts) < 6 {
		return []string{}
	}
	tags := []string{}
	for _, decoration := range strings.Split(parts[5], ",") {
		decoration = strings.TrimSpace(decoration)
		if tag := strings.TrimPrefix(decoration, "tag: "); tag != decoration && tag != "" {
			tags = append(tags, tag)
		}
	}
	return tags
}

func tagsForCommit(ctx context.Context, root string, sha string) []string {
	output, err := gitOutput(ctx, root, "tag", "--points-at", sha)
	if err != nil || strings.TrimSpace(output) == "" {
		return []string{}
	}
	tags := []string{}
	for _, line := range strings.Split(output, "\n") {
		tag := strings.TrimSpace(line)
		if tag != "" {
			tags = append(tags, tag)
		}
	}
	return tags
}

func githubCommitURL(originRepoSlug string, sha string) string {
	if originRepoSlug == "" || sha == "" {
		return ""
	}
	return "https://github.com/" + originRepoSlug + "/commit/" + sha
}

func firstNonEmptyGitRef(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
