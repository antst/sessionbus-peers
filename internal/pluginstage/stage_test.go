// SPDX-License-Identifier: MIT

package pluginstage

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func TestStageMatchesManifestAndCommonSource(t *testing.T) {
	repo := filepath.Join("..", "..")
	body, err := os.ReadFile(filepath.Join(repo, "opencode", "package.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Files []string `json:"files"`
	}
	if err := json.Unmarshal(body, &manifest); err != nil {
		t.Fatal(err)
	}
	common := filepath.Join(repo, "wrappers", "opencodefamily", "plugin")
	var modules, declared []string
	files, err := os.ReadDir(common)
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		name := file.Name()
		if strings.HasSuffix(name, ".mjs") && !strings.HasSuffix(name, ".test.mjs") && !strings.HasSuffix(name, "-fixture.mjs") {
			modules = append(modules, name)
		}
	}
	expected := []string{"package.json", "package-lock.json"}
	for _, name := range manifest.Files {
		if strings.HasSuffix(name, ".mjs") {
			declared = append(declared, name)
		}
		if name == "skills" {
			root := filepath.Join(repo, "opencode")
			if err := filepath.WalkDir(filepath.Join(root, name), func(path string, entry fs.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if entry.Type().IsRegular() {
					rel, err := filepath.Rel(root, path)
					if err != nil {
						return err
					}
					expected = append(expected, filepath.ToSlash(rel))
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
		} else {
			expected = append(expected, name)
		}
	}
	sort.Strings(modules)
	sort.Strings(declared)
	if !reflect.DeepEqual(modules, declared) {
		t.Fatalf("common modules %v differ from manifest %v", modules, declared)
	}
	stage := t.TempDir()
	if err := Stage(repo, "opencode", stage, false); err != nil {
		t.Fatal(err)
	}
	var got []string
	if err := filepath.WalkDir(stage, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type().IsRegular() {
			rel, err := filepath.Rel(stage, path)
			if err != nil {
				return err
			}
			got = append(got, filepath.ToSlash(rel))
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	sort.Strings(expected)
	sort.Strings(got)
	if !reflect.DeepEqual(expected, got) {
		t.Fatalf("staged files %v differ from declared files %v", got, expected)
	}
}

func TestStageIncludesEveryDevelopmentTest(t *testing.T) {
	repo := filepath.Join("..", "..")
	common := filepath.Join(repo, "wrappers", "opencodefamily", "plugin")
	sources, err := developmentFiles(common)
	if err != nil {
		t.Fatal(err)
	}
	if len(sources) == 0 {
		t.Fatal("no common plugin tests")
	}
	stage := t.TempDir()
	if err := Stage(repo, "opencode", stage, true); err != nil {
		t.Fatal(err)
	}
	staged, err := developmentFiles(stage)
	if err != nil {
		t.Fatal(err)
	}
	if len(staged) != len(sources) {
		t.Fatalf("staged %d tests, source has %d", len(staged), len(sources))
	}
	for _, source := range sources {
		want, err := os.ReadFile(source)
		if err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(filepath.Join(stage, filepath.Base(source)))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("staged test differs: %s", source)
		}
	}
}

func developmentFiles(directory string) ([]string, error) {
	files, err := filepath.Glob(filepath.Join(directory, "*.mjs"))
	if err != nil {
		return nil, err
	}
	var result []string
	for _, file := range files {
		if strings.HasSuffix(file, ".test.mjs") || strings.HasSuffix(file, "-fixture.mjs") {
			result = append(result, file)
		}
	}
	return result, nil
}
