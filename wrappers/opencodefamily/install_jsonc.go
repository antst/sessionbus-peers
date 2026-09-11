// SPDX-License-Identifier: MIT

package opencodefamily

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"unicode/utf8"
)

const maxInstallConfig = 1 << 20

// Nodes retain source spans so edits never marshal unrelated configuration.
type configNode struct {
	start, end int
	kind       byte
	text       string
	items      []*configNode
	keys       []string
	commas     []int // comma following each item; -1 means absent
}

type configParser struct {
	data  []byte
	pos   int
	nodes int
}

func parseConfig(data []byte) (*configNode, error) {
	if len(data) > maxInstallConfig || !utf8.Valid(data) {
		return nil, errors.New("configuration exceeds 1 MiB or is not UTF-8")
	}
	p := configParser{data: data}
	n, err := p.value(0)
	if err == nil {
		err = p.space()
	}
	if err != nil {
		return nil, err
	}
	if p.pos != len(data) || n.kind != '{' {
		return nil, errors.New("configuration must be one JSONC object")
	}
	return n, nil
}

func (p *configParser) space() error {
	for p.pos < len(p.data) {
		switch p.data[p.pos] {
		case ' ', '\t', '\n', '\r':
			p.pos++
		case '/':
			if p.pos+1 == len(p.data) {
				return fmt.Errorf("incomplete comment at byte %d", p.pos)
			}
			switch p.data[p.pos+1] {
			case '/':
				p.pos += 2
				for p.pos < len(p.data) && p.data[p.pos] != '\n' && p.data[p.pos] != '\r' {
					p.pos++
				}
			case '*':
				p.pos += 2
				for p.pos+1 < len(p.data) && !(p.data[p.pos] == '*' && p.data[p.pos+1] == '/') {
					p.pos++
				}
				if p.pos+1 == len(p.data) || p.pos == len(p.data) {
					return errors.New("unterminated JSONC comment")
				}
				p.pos += 2
			default:
				return fmt.Errorf("invalid comment at byte %d", p.pos)
			}
		default:
			return nil
		}
	}
	return nil
}

func (p *configParser) value(depth int) (*configNode, error) {
	if depth > 128 || p.nodes >= 65536 {
		return nil, errors.New("configuration exceeds depth 128 or 65536 values")
	}
	p.nodes++
	if err := p.space(); err != nil {
		return nil, err
	}
	if p.pos == len(p.data) {
		return nil, errors.New("missing JSONC value")
	}
	n := &configNode{start: p.pos, kind: p.data[p.pos]}
	if n.kind == '[' || n.kind == '{' {
		p.pos++
		end := byte(']')
		if n.kind == '{' {
			end = '}'
		}
		for {
			if err := p.space(); err != nil {
				return nil, err
			}
			if p.pos == len(p.data) {
				return nil, errors.New("unterminated JSONC container")
			}
			if p.data[p.pos] == end {
				p.pos++
				n.end = p.pos
				return n, nil
			}
			if n.kind == '{' {
				key, err := p.value(depth + 1)
				if err != nil || key.kind != '"' {
					return nil, errors.New("JSONC object requires string keys")
				}
				if err := p.space(); err != nil {
					return nil, err
				}
				if p.pos == len(p.data) || p.data[p.pos] != ':' {
					return nil, errors.New("JSONC object requires colon")
				}
				p.pos++
				n.keys = append(n.keys, key.text)
			}
			child, err := p.value(depth + 1)
			if err != nil {
				return nil, err
			}
			n.items = append(n.items, child)
			if err := p.space(); err != nil {
				return nil, err
			}
			n.commas = append(n.commas, -1)
			if p.pos < len(p.data) && p.data[p.pos] == ',' {
				n.commas[len(n.commas)-1] = p.pos
				p.pos++
				continue
			}
			if p.pos == len(p.data) || p.data[p.pos] != end {
				return nil, errors.New("JSONC container requires comma or closing delimiter")
			}
		}
	}
	if n.kind == '"' {
		p.pos++
		for p.pos < len(p.data) {
			ch := p.data[p.pos]
			p.pos++
			if ch == '\\' {
				if p.pos < len(p.data) {
					p.pos++
				}
				continue
			}
			if ch == '"' {
				n.end = p.pos
				if err := json.Unmarshal(p.data[n.start:n.end], &n.text); err != nil {
					return nil, err
				}
				return n, nil
			}
		}
		return nil, errors.New("unterminated JSONC string")
	}
	for p.pos < len(p.data) && !slices.Contains([]byte{' ', '\t', '\r', '\n', ',', ']', '}', '/'}, p.data[p.pos]) {
		p.pos++
	}
	n.end = p.pos
	if !json.Valid(p.data[n.start:n.end]) {
		return nil, fmt.Errorf("invalid JSONC primitive at byte %d", n.start)
	}
	return n, nil
}

func (n *configNode) property(key string) (*configNode, error) {
	var found *configNode
	for i, k := range n.keys {
		if k == key {
			if found != nil {
				return nil, fmt.Errorf("ambiguous duplicate %q property", key)
			}
			found = n.items[i]
		}
	}
	return found, nil
}

type configEdit struct {
	start, end int
	text       []byte
}

func applyConfigEdits(data []byte, edits []configEdit) ([]byte, error) {
	slices.SortFunc(edits, func(a, b configEdit) int { return a.start - b.start })
	size, cursor := len(data), 0
	for _, e := range edits {
		if e.start < cursor || e.end < e.start || e.end > len(data) {
			return nil, errors.New("overlapping configuration edits")
		}
		size += len(e.text) - (e.end - e.start)
		cursor = e.end
	}
	if size > maxInstallConfig {
		return nil, errors.New("edited configuration exceeds 1 MiB")
	}
	result := make([]byte, 0, size)
	cursor = 0
	for _, e := range edits {
		result = append(result, data[cursor:e.start]...)
		result = append(result, e.text...)
		cursor = e.end
	}
	result = append(result, data[cursor:]...)
	if _, err := parseConfig(result); err != nil {
		return nil, fmt.Errorf("invalid edited configuration: %w", err)
	}
	return result, nil
}
