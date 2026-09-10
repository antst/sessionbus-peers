// SPDX-License-Identifier: MIT
package qwen

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/sys/unix"
)

// Bound both retained caller input and the resulting single argv entry. This
// stays below Linux's per-argument limit and does not create an on-disk config.
const maxInteractiveMCPConfig = 64 << 10

// composeInteractiveMCP preserves every other native argument. The one caller
// config stays in its original position; absence inserts one managed prefix.
// No second --mcp-config, native schema rewrite, or caller file mutation occurs.
func composeInteractiveMCP(args []string, managed json.RawMessage) ([]string, error) {
	position, attached := -1, false
	value := ""
	for i := 0; i < len(args); i++ {
		if args[i] == "--" {
			break
		}
		key, raw, hasValue := strings.Cut(args[i], "=")
		if key != "--mcp-config" {
			continue
		}
		if position != -1 {
			return nil, errors.New("qwen-peer accepts only one caller --mcp-config")
		}
		position, attached = i, hasValue
		if !hasValue {
			if i+1 == len(args) || args[i+1] == "--" || strings.HasPrefix(args[i+1], "--mcp-config") {
				return nil, errors.New("--mcp-config requires one value")
			}
			i++
			raw = args[i]
		}
		value = raw
	}
	config := map[string]json.RawMessage{}
	servers := config
	wrapped := false
	if position >= 0 && value != "" {
		data, err := readInteractiveMCPConfig(value)
		if err != nil {
			return nil, err
		}
		if err := json.Unmarshal(data, &config); err != nil || config == nil {
			return nil, errors.New("invalid --mcp-config: expected a JSON server map")
		}
		servers = config
		if raw, ok := config["mcpServers"]; ok && isJSONObjectValue(raw) {
			var nested map[string]json.RawMessage
			if err := json.Unmarshal(raw, &nested); err != nil || nested == nil {
				return nil, errors.New("invalid --mcp-config mcpServers: expected a server map")
			}
			servers = nested
			wrapped = true
		}
		// Match native shallow validation; native owns each server's schema.
		for _, raw := range servers {
			value := bytes.TrimSpace(raw)
			if len(value) == 0 || (value[0] != '{' && value[0] != '[') {
				return nil, errors.New("invalid --mcp-config: each server must be an object")
			}
		}
	}
	if _, exists := servers["sessionbus"]; exists {
		return nil, errors.New("caller --mcp-config reserves sessionbus; rename or remove that server before a managed launch")
	}
	if len(managed) > maxInteractiveMCPConfig || !json.Valid(managed) || len(bytes.TrimSpace(managed)) == 0 || bytes.TrimSpace(managed)[0] != '{' {
		return nil, errors.New("invalid managed Sessionbus MCP configuration")
	}
	servers["sessionbus"] = managed
	if wrapped {
		raw, err := json.Marshal(servers)
		if err != nil {
			return nil, err
		}
		config["mcpServers"] = raw
	}
	body, err := json.Marshal(config)
	if err != nil {
		return nil, err
	}
	if len(body) > maxInteractiveMCPConfig {
		return nil, errors.New("combined --mcp-config exceeds 65536 bytes")
	}
	if position < 0 {
		return append([]string{"--mcp-config", string(body)}, args...), nil
	}
	result := append([]string(nil), args...)
	if attached {
		result[position] = "--mcp-config=" + string(body)
	} else {
		result[position+1] = string(body)
	}
	return result, nil
}

func isJSONObjectValue(raw json.RawMessage) bool {
	v := bytes.TrimSpace(raw)
	return len(v) != 0 && (v[0] == '{' || v[0] == '[' || bytes.Equal(v, []byte("null")))
}

func readInteractiveMCPConfig(value string) ([]byte, error) {
	if len(value) > maxInteractiveMCPConfig {
		return nil, errors.New("--mcp-config input exceeds 65536 bytes")
	}
	// Native gives an existing file priority over inline JSON. Follow symlinks
	// for ordinary config files, but never block on a device or named pipe.
	if info, err := os.Stat(value); err == nil {
		if !info.Mode().IsRegular() {
			return nil, errors.New("--mcp-config file must be regular")
		}
		f, err := os.OpenFile(value, os.O_RDONLY|unix.O_NONBLOCK, 0)
		if err != nil {
			return nil, fmt.Errorf("read --mcp-config: %w", err)
		}
		opened, statErr := f.Stat()
		if statErr != nil || !opened.Mode().IsRegular() {
			return nil, errors.Join(errors.New("opened --mcp-config file must be regular"), statErr, f.Close())
		}
		data, readErr := io.ReadAll(io.LimitReader(f, maxInteractiveMCPConfig+1))
		err = errors.Join(readErr, f.Close())
		if err != nil {
			return nil, fmt.Errorf("read --mcp-config: %w", err)
		}
		if len(data) > maxInteractiveMCPConfig {
			return nil, errors.New("--mcp-config file exceeds 65536 bytes")
		}
		return stripMCPComments(data), nil
	}
	return []byte(value), nil
}

// File-only JSON comments follow native strip-json-comments: replace comments
// with whitespace, retaining line breaks and every byte within JSON strings.
// Trailing commas remain invalid; inline config never calls this function.
func stripMCPComments(data []byte) []byte {
	out := append([]byte(nil), data...)
	inString, escaped := false, false
	for i := 0; i < len(data); i++ {
		if inString {
			if escaped {
				escaped = false
			} else if data[i] == '\\' {
				escaped = true
			} else if data[i] == '"' {
				inString = false
			}
			continue
		}
		if data[i] == '"' {
			inString = true
			continue
		}
		if data[i] != '/' || i+1 == len(data) || (data[i+1] != '/' && data[i+1] != '*') {
			continue
		}
		block := data[i+1] == '*'
		start := i
		i += 2
		for i < len(data) {
			if !block && data[i] == '\n' {
				break
			}
			if block && data[i] == '*' && i+1 < len(data) && data[i+1] == '/' {
				i += 2
				break
			}
			i++
		}
		for j := start; j < i; j++ {
			if out[j] != '\n' && out[j] != '\r' {
				out[j] = ' '
			}
		}
		i--
	}
	return out
}
