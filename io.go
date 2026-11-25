package handlefix

import (
	"encoding/json"
	"fmt"
	"os"
)

// LoadProject reads a JSON project document.
func LoadProject(path string) (Project, error) {
	var project Project
	if err := loadJSON(path, &project); err != nil {
		return Project{}, err
	}
	return project, nil
}

// LoadEdgeEdits reads a trim trace.
func LoadEdgeEdits(path string) ([]EdgeEdit, error) {
	var edits []EdgeEdit
	if err := loadJSON(path, &edits); err != nil {
		return nil, err
	}
	return edits, nil
}

func loadJSON(path string, target any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	if err := json.Unmarshal(data, target); err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}
	return nil
}

// WriteJSON writes v as indented JSON with a trailing newline.
func WriteJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("encode %s: %w", path, err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}
