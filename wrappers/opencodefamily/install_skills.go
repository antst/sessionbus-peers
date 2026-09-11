// SPDX-License-Identifier: MIT
package opencodefamily

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"golang.org/x/sys/unix"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// Native global config order, distinct from plugin target preference. Both
// native loaders replace skills.paths arrays when merging a later document.
func (kind nativeKind) skillConfigOrder() []string {
	if kind == kiloNative {
		return []string{"config.json", "kilo.json", "kilo.jsonc", "opencode.json", "opencode.jsonc"}
	}
	return []string{"config.json", "opencode.json", "opencode.jsonc"}
}

func packageSkillRoot(kind nativeKind, options InstallOptions) (string, error) {
	if options.Remove || !strings.HasPrefix(options.Specifier, "file:") {
		return "", nil
	}
	u, err := url.Parse(options.Specifier)
	if err != nil {
		return "", err
	}
	root := u.Path
	if u.Opaque != "" {
		root = u.Opaque
	}
	// Archive specifiers retain plugin-only behavior; there is no extracted root.
	if strings.HasSuffix(root, ".tgz") {
		return "", nil
	}
	if !filepath.IsAbs(root) {
		root = filepath.Join(options.Directory, root)
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return "", err
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return "", err
	}
	skills := filepath.Join(root, "skills")
	if !ownedSkillPath(kind, skills) {
		return "", errors.New("local skill root must belong to the selected Sessionbus package")
	}
	if err := onlySkillEntry(skills, "sessionbus", true); err != nil {
		return "", err
	}
	if err := onlySkillEntry(filepath.Join(skills, "sessionbus"), "SKILL.md", false); err != nil {
		return "", err
	}
	skillFile := filepath.Join(skills, "sessionbus", "SKILL.md")
	physical, err := filepath.EvalSymlinks(skillFile)
	if err != nil {
		return "", fmt.Errorf("local package generic skill: %w", err)
	}
	if physical != skillFile {
		return "", errors.New("local package generic skill leaves its rendered location")
	}
	body, _, err := readInstallFile(skillFile)
	if err != nil {
		return "", fmt.Errorf("local package generic skill: %w", err)
	}
	if len(bytes.TrimSpace(body)) == 0 {
		return "", errors.New("local package generic skill is empty")
	}
	return skills, nil
}

func ownedSkillPath(kind nativeKind, path string) bool {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path || filepath.Base(path) != "skills" {
		return false
	}
	body, _, err := readInstallFile(filepath.Join(filepath.Dir(path), "package.json"))
	var manifest struct {
		Name string `json:"name"`
	}
	return err == nil && json.Unmarshal(body, &manifest) == nil && manifest.Name == kind.installPackage()
}

func skillNodes(doc *configDocument) (skills, paths *configNode, err error) {
	skills, err = doc.tree.property("skills")
	if err != nil || skills == nil {
		return
	}
	if skills.kind != '{' {
		return nil, nil, errors.New("skills must be an object")
	}
	paths, err = skills.property("paths")
	if err != nil || paths == nil {
		return
	}
	if paths.kind != '[' {
		return nil, nil, errors.New("skills.paths must be an array")
	}
	for _, item := range paths.items {
		if item.kind != '"' || !validInstallText(item.text) {
			return nil, nil, errors.New("skills.paths must contain nonempty strings")
		}
	}
	return
}

func skillTarget(kind nativeKind, docs []*configDocument) (*configDocument, error) {
	var selected *configDocument
	for _, doc := range docs {
		if doc.selected {
			selected = doc
		}
		if _, _, err := skillNodes(doc); err != nil {
			return nil, fmt.Errorf("%s: %w", doc.file, err)
		}
	}
	for _, name := range kind.skillConfigOrder() {
		for _, doc := range docs {
			if filepath.Base(doc.file) != name {
				continue
			}
			_, paths, _ := skillNodes(doc)
			if paths != nil {
				selected = doc
			}
		}
	}
	return selected, nil
}

func insertConfigProperty(target *configNode, text []byte) configEdit {
	if len(target.items) > 0 && target.commas[len(target.commas)-1] < 0 {
		text = append([]byte{','}, text...)
	}
	return configEdit{target.end - 1, target.end - 1, text}
}

func editSkillConfig(doc *configDocument, root string, selected bool) ([]byte, error) {
	skills, paths, err := skillNodes(doc)
	if err != nil {
		return nil, err
	}
	add := root != "" && selected
	encoded, _ := json.Marshal(root)
	if paths == nil {
		if !add {
			return doc.body, nil
		}
		text := append([]byte(`"paths":[`), encoded...)
		text = append(text, ']')
		target := skills
		if target == nil {
			target = doc.tree
			text = append([]byte(`"skills":{`), text...)
			text = append(text, '}')
		}
		return applyConfigEdits(doc.body, []configEdit{insertConfigProperty(target, text)})
	}
	count, exact := 0, false
	for _, item := range paths.items {
		if ownedSkillPath(doc.kind, item.text) {
			count++
			exact = item.text == root
		}
	}
	if count == 0 && !add || count == 1 && exact && add {
		return doc.body, nil
	}
	var edits []configEdit
	last := -1
	for i, item := range paths.items {
		if ownedSkillPath(doc.kind, item.text) {
			edits = append(edits, configEdit{item.start, item.end, nil})
			if paths.commas[i] >= 0 {
				edits = append(edits, configEdit{paths.commas[i], paths.commas[i] + 1, nil})
			}
		} else {
			last = i
		}
	}
	if add {
		if last >= 0 && paths.commas[last] < 0 {
			encoded = append([]byte{','}, encoded...)
		}
		edits = append(edits, configEdit{paths.end - 1, paths.end - 1, encoded})
	}
	return applyConfigEdits(doc.body, edits)
}

// Inspect at most two entries: the shipped tree contains exactly one child at
// each level. Opening directories without following links keeps a local package
// from redirecting this registration to another skill collection.
func onlySkillEntry(directory, name string, wantDir bool) error {
	fd, err := unix.Open(directory, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return fmt.Errorf("local rendered skill directory: %w", err)
	}
	file := os.NewFile(uintptr(fd), directory)
	entries, readErr := file.ReadDir(2)
	closeErr := file.Close()
	if readErr != nil && !errors.Is(readErr, io.EOF) {
		return errors.Join(readErr, closeErr)
	}
	if closeErr != nil {
		return closeErr
	}
	if len(entries) != 1 || entries[0].Name() != name {
		return fmt.Errorf("%s: expected only %s", directory, name)
	}
	info, err := entries[0].Info()
	if err != nil {
		return err
	}
	if wantDir && !info.IsDir() || !wantDir && (!info.Mode().IsRegular() || info.Size() > maxInstallConfig) {
		return fmt.Errorf("%s: invalid rendered skill entry", filepath.Join(directory, name))
	}
	return nil
}
