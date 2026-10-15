package main

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

const draftFile = "draft.json"

// PRDraft is the PR composer's unpublished text, kept beside the change
// rather than in it so saving never races an operation rewriting change.json.
type PRDraft struct {
	Title     string `json:"title"`
	Body      string `json:"body"`
	Reviewers string `json:"reviewers"`
}

// Draft returns the change's saved draft, or nil when there is none.
func (a *App) Draft(id string) (*PRDraft, error) {
	data, err := os.ReadFile(filepath.Join(a.store.Dir(id), draftFile))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var d PRDraft
	if err := json.Unmarshal(data, &d); err != nil {
		return nil, nil
	}
	return &d, nil
}

func (a *App) SaveDraft(id string, d PRDraft) error {
	dir := a.store.Dir(id)
	if _, err := os.Stat(dir); err != nil {
		return err
	}
	data, err := json.Marshal(d)
	if err != nil {
		return err
	}
	path := filepath.Join(dir, draftFile)
	if err := os.WriteFile(path+".tmp", data, 0o644); err != nil {
		return err
	}
	return os.Rename(path+".tmp", path)
}

func (a *App) DiscardDraft(id string) error {
	err := os.Remove(filepath.Join(a.store.Dir(id), draftFile))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}
