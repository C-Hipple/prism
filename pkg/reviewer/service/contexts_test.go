package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
	require.NoError(t, os.WriteFile(p, []byte(content), 0o644))
	return p
}

func TestLoadContextDefinitions_ValidSingle(t *testing.T) {
	tmp := t.TempDir()
	json := `{
		"name": "TempName",
		"description": "Core URL state context",
		"repo": "https://github.com/example/repo",
		"files": ["src/a.ts", "src/b.ts"]
	}`
	writeFile(t, tmp, "TempName.json", json)
	// Non-json should be ignored
	writeFile(t, tmp, "README.txt", "ignore me")

	defs, err := LoadContextDefinitions(tmp)
	require.NoError(t, err)
	require.Len(t, defs, 1)
	def, ok := defs["TempName"]
	require.True(t, ok)
	require.Equal(t, "TempName", def.Name)
	require.Equal(t, "Core URL state context", def.Description)
	require.Equal(t, "https://github.com/example/repo", def.Repo)
	require.Equal(t, []string{"src/a.ts", "src/b.ts"}, def.Files)
}

func TestLoadContextDefinitions_DuplicateNames(t *testing.T) {
	tmp := t.TempDir()
	jsonA := `{"name":"Dup","description":"A","repo":"r","files":["a"]}`
	jsonB := `{"name":"Dup","description":"B","repo":"r","files":["b"]}`
	writeFile(t, tmp, "a.json", jsonA)
	writeFile(t, tmp, "b.json", jsonB)

	_, err := LoadContextDefinitions(tmp)
	require.Error(t, err)
	require.Contains(t, err.Error(), "duplicate context name")
}

func TestLoadContextDefinitions_MissingFields(t *testing.T) {
	tmp := t.TempDir()
	cases := map[string]string{
		"missing_name.json":        `{"description":"d","repo":"r","files":["a"]}`,
		"missing_description.json": `{"name":"N","repo":"r","files":["a"]}`,
		"missing_repo.json":        `{"name":"N","description":"d","files":["a"]}`,
		"missing_files.json":       `{"name":"N","description":"d","repo":"r"}`,
		"empty_files.json":         `{"name":"N","description":"d","repo":"r","files":[]}`,
		"empty_file_entry.json":    `{"name":"N","description":"d","repo":"r","files":[""]}`,
	}
	for fn, content := range cases {
		writeFile(t, tmp, fn, content)
	}

	_, err := LoadContextDefinitions(tmp)
	require.Error(t, err)
	// Error message varies by which file is read first; assert it's about validation or files
	require.True(t, strings.Contains(err.Error(), "validation failed") || strings.Contains(err.Error(), "context 'files'"))
}

func TestLoadContextDefinitions_ExtraJSONInFile(t *testing.T) {
	tmp := t.TempDir()
	content := `{"name":"A","description":"d","repo":"r","files":["a"]} {"extra":1}`
	writeFile(t, tmp, "extra.json", content)

	_, err := LoadContextDefinitions(tmp)
	require.Error(t, err)
	require.Contains(t, err.Error(), "single JSON object")
}

func TestLoadContextDefinitions_NonObjectJSON(t *testing.T) {
	tmp := t.TempDir()
	writeFile(t, tmp, "arr.json", `[]`)

	_, err := LoadContextDefinitions(tmp)
	require.Error(t, err)
	require.Contains(t, err.Error(), "decoding")
	require.Contains(t, err.Error(), "cannot unmarshal")
}

func TestLoadContextDefinitions_NonexistentDir(t *testing.T) {
	_, err := LoadContextDefinitions(filepath.Join(t.TempDir(), "does-not-exist"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "reading contexts dir")
}
