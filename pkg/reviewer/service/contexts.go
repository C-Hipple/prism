package service

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// ContextDefinition represents a single context JSON definition file.
// Required fields: Name, Description, Repo, Files (non-empty slice of non-empty strings).
type ContextDefinition struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Repo        string   `json:"repo"`
	Files       []string `json:"files"`
}

// Validate ensures the definition has all required fields in valid form.
func (c ContextDefinition) Validate() error {
	if c.Name == "" {
		return fmt.Errorf("context 'name' is required")
	}
	if c.Description == "" {
		return fmt.Errorf("context 'description' is required for %q", c.Name)
	}
	if c.Repo == "" {
		return fmt.Errorf("context 'repo' is required for %q", c.Name)
	}
	if len(c.Files) == 0 {
		return fmt.Errorf("context 'files' must contain at least one entry for %q", c.Name)
	}
	for i, p := range c.Files {
		if p == "" {
			return fmt.Errorf("context 'files' contains an empty path at index %d for %q", i, c.Name)
		}
	}
	return nil
}

// LoadContextDefinitions scans dirPath for .json files, validates each contains exactly
// one JSON object with the required fields, and returns them keyed by Name.
func LoadContextDefinitions(dirPath string) (map[string]ContextDefinition, error) {
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return nil, fmt.Errorf("reading contexts dir: %w", err)
	}

	result := make(map[string]ContextDefinition)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if filepath.Ext(e.Name()) != ".json" {
			continue
		}

		fullPath := filepath.Join(dirPath, e.Name())
		f, err := os.Open(fullPath)
		if err != nil {
			return nil, fmt.Errorf("opening %s: %w", fullPath, err)
		}
		dec := json.NewDecoder(f)

		var def ContextDefinition
		if err := dec.Decode(&def); err != nil {
			f.Close()
			return nil, fmt.Errorf("decoding %s: %w", fullPath, err)
		}
		// Ensure there's no trailing JSON value beyond the single object
		if err := ensureSingleJSONValue(dec); err != nil {
			f.Close()
			return nil, fmt.Errorf("validating single JSON object in %s: %w", fullPath, err)
		}
		if err := f.Close(); err != nil {
			return nil, fmt.Errorf("closing %s: %w", fullPath, err)
		}

		if err := def.Validate(); err != nil {
			return nil, fmt.Errorf("validation failed for %s: %w", fullPath, err)
		}
		if _, exists := result[def.Name]; exists {
			return nil, fmt.Errorf("duplicate context name %q found in %s", def.Name, fullPath)
		}
		result[def.Name] = def
	}

	return result, nil
}

func ensureSingleJSONValue(dec *json.Decoder) error {
	// After first successful Decode, any additional non-whitespace tokens indicate extra data.
	var extra any
	if err := dec.Decode(&extra); err == io.EOF {
		return nil
	} else if err != nil {
		return err
	}
	return fmt.Errorf("extra JSON content found")
}
