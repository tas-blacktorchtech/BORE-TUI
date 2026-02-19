package git

import "context"

// Status returns the short-format status of the working tree at dir.
func (r *Repo) Status(ctx context.Context, dir string) (string, error) {
	return r.runInDir(ctx, dir, "status", "--short")
}

// Diff returns the unstaged diff for the working tree at dir.
func (r *Repo) Diff(ctx context.Context, dir string) (string, error) {
	return r.runInDir(ctx, dir, "diff")
}

// DiffStaged returns the staged (index) diff for the working tree at dir.
func (r *Repo) DiffStaged(ctx context.Context, dir string) (string, error) {
	return r.runInDir(ctx, dir, "diff", "--staged")
}

// DiffAll returns the combined unstaged and staged diff for dir.
// The two diffs are separated by a blank line when both are non-empty.
func (r *Repo) DiffAll(ctx context.Context, dir string) (string, error) {
	unstaged, err := r.Diff(ctx, dir)
	if err != nil {
		return "", err
	}

	staged, err := r.DiffStaged(ctx, dir)
	if err != nil {
		return "", err
	}

	switch {
	case unstaged != "" && staged != "":
		return unstaged + "\n\n" + staged, nil
	case unstaged != "":
		return unstaged, nil
	default:
		return staged, nil
	}
}

// HasChanges reports whether the working tree at dir has any uncommitted
// modifications (staged or unstaged, including untracked files).
func (r *Repo) HasChanges(ctx context.Context, dir string) (bool, error) {
	out, err := r.Status(ctx, dir)
	if err != nil {
		return false, err
	}
	return out != "", nil
}
