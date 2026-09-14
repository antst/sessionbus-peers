// SPDX-License-Identifier: MIT

package pluginstage

import (
	"bytes"
	"os"
	"path/filepath"
	"text/template"
)

func renderSkill(repo, product string) ([]byte, error) {
	body, err := os.ReadFile(filepath.Join(repo, "wrappers", "opencodefamily", "plugin", "skills", "sessionbus", "SKILL.md.tmpl"))
	if err != nil {
		return nil, err
	}
	source, err := template.New("sessionbus").Option("missingkey=error").Parse(string(body))
	if err != nil {
		return nil, err
	}
	data := struct {
		Label, Command string
		Kilo           bool
	}{Label: "OpenCode", Command: "opencode-peer"}
	if product == "kilo" {
		data.Label = "Kilo"
		data.Command = "kilo-peer"
		data.Kilo = true
	}
	var rendered bytes.Buffer
	if err := source.Execute(&rendered, data); err != nil {
		return nil, err
	}
	return rendered.Bytes(), nil
}
