// SPDX-License-Identifier: MIT
package opencodefamily

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"unicode"
	"unicode/utf8"
)

type nativeSession struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Directory string `json:"directory"`
	ParentID  string `json:"parentID"`
}
type nativeInfo struct {
	ID        string          `json:"id"`
	SessionID string          `json:"sessionID"`
	ParentID  string          `json:"parentID"`
	Role      string          `json:"role"`
	Finish    string          `json:"finish"`
	Summary   bool            `json:"summary,omitempty"`
	Error     json.RawMessage `json:"error"`
	Time      struct {
		Completed *float64 `json:"completed"`
	} `json:"time"`
}
type withParts struct {
	Info  nativeInfo        `json:"info"`
	Parts []json.RawMessage `json:"parts"`
}

func validNativeID(s string) bool {
	return strings.HasPrefix(s, "ses") && utf8.ValidString(s) && utf8.RuneCountInString(s) <= 128 && !strings.ContainsFunc(s, func(r rune) bool { return !unicode.IsPrint(r) || unicode.IsSpace(r) })
}
func validMessageID(s string) bool {
	return strings.HasPrefix(s, "msg") && len(s) <= 4096 && utf8.ValidString(s) && !strings.ContainsAny(s, "\x00\r\n")
}
func validPermissionID(s string) bool {
	return strings.HasPrefix(s, "per") && len(s) <= 4096 && !strings.ContainsAny(s, "\x00\r\n")
}
func randomMessageID() (string, error) {
	b := make([]byte, 16)
	_, e := rand.Read(b)
	return "msg_" + hex.EncodeToString(b), e
}
func sessionPath(id string) string { return "/session/" + url.PathEscape(id) }
func (c *laneHTTP) get(ctx context.Context, id string) (nativeSession, error) {
	if !validNativeID(id) {
		return nativeSession{}, errors.New("invalid native session ID")
	}
	b, e := c.call(ctx, "GET", sessionPath(id), nil, 200)
	if e != nil {
		return nativeSession{}, e
	}
	var s nativeSession
	if json.Unmarshal(b, &s) != nil || s.ID != id || s.Directory != c.directory {
		return s, c.kind.err("returned different session identity/directory")
	}
	return s, nil
}
func (c *laneHTTP) openSession(ctx context.Context, id, title, permission string) (nativeSession, error) {
	method, path := "POST", "/session"
	if id != "" {
		if _, err := c.get(ctx, id); err != nil {
			return nativeSession{}, err
		}
		method, path = "PATCH", sessionPath(id)
	}
	body := map[string]any{"title": title}
	if permission == "bypassPermissions" {
		body["permission"] = []map[string]string{{"permission": "*", "pattern": "*", "action": "allow"}}
	}
	b, e := c.call(ctx, method, path, body, 200)
	if e != nil {
		return nativeSession{}, e
	}
	var s nativeSession
	if json.Unmarshal(b, &s) != nil || !validNativeID(s.ID) || s.Directory != c.directory || s.Title != title || (id != "" && s.ID != id) {
		return s, c.kind.err("did not confirm opened native identity/settings")
	}
	return s, nil
}
func (c *laneHTTP) remove(ctx context.Context, id string) error {
	b, e := c.call(ctx, "DELETE", sessionPath(id), nil, 200)
	if e != nil {
		return e
	}
	var deleted bool
	if json.Unmarshal(b, &deleted) != nil || !deleted {
		return c.kind.err("did not confirm session deletion")
	}
	return nil
}
func (c *laneHTTP) ready(ctx context.Context) error {
	b, e := c.call(ctx, "GET", "/experimental/tool/ids", nil, 200)
	if e != nil {
		return e
	}
	var ids []string
	if json.Unmarshal(b, &ids) != nil || ids == nil {
		return c.kind.err("tool inventory malformed")
	}
	for _, id := range ids {
		if id == ToolName {
			return nil
		}
	}
	return c.kind.err("Sessionbus tool is not registered")
}
func decodeParts(b []byte, session string) (withParts, error) {
	var m withParts
	if json.Unmarshal(b, &m) != nil {
		return m, errors.New("malformed native message")
	}
	return m, validateParts(m, session)
}
func validateParts(m withParts, session string) error {
	if m.Info.SessionID != session || !validMessageID(m.Info.ID) || (m.Info.Role != "user" && m.Info.Role != "assistant") || m.Parts == nil {
		return errors.New("invalid native message identity/role/parts")
	}
	if m.Info.Role == "assistant" && !validMessageID(m.Info.ParentID) {
		return errors.New("native assistant missing parent")
	}
	for _, raw := range m.Parts {
		var part struct {
			Type      string  `json:"type"`
			Text      *string `json:"text"`
			SessionID string  `json:"sessionID"`
			MessageID string  `json:"messageID"`
		}
		if json.Unmarshal(raw, &part) != nil || part.Type == "" || part.SessionID != session || part.MessageID != m.Info.ID || (part.Type == "text" && part.Text == nil) {
			return errors.New("malformed native message part")
		}
	}
	return nil
}

var errPriorAssistant = errors.New("native terminal predates current input")

func (c *laneHTTP) historyResult(ctx context.Context, session, initial string, final withParts) (string, error) {
	if err := validateParts(final, session); err != nil {
		return "", err
	}
	if final.Info.Role != "assistant" {
		return "", errors.New("native terminal is not an assistant")
	}
	cursor := ""
	cursors := map[string]bool{}
	seen := map[string]bool{}
	var reversePages [][]withParts
	total, bytes := 0, 0
	foundInitial, foundFinal := false, false
	for !foundInitial {
		path := sessionPath(session) + "/message?limit=64"
		if cursor != "" {
			path += "&before=" + url.QueryEscape(cursor)
		}
		r, err := c.prepare(ctx, http.MethodGet, path, nil)
		if err != nil {
			return "", err
		}
		op, err := c.begin(r, 200)
		if err != nil {
			return "", err
		}
		b, err := op.wait()
		if err != nil {
			return "", err
		}
		bytes += len(b)
		if bytes > 16<<20 {
			return "", errors.New("native history exceeds 16 MiB")
		}
		var page []withParts
		if json.Unmarshal(b, &page) != nil || page == nil || len(page) > 64 {
			return "", errors.New("invalid native history page")
		}
		for _, m := range page {
			total++
			if total > 4096 {
				return "", errors.New("native history exceeds 4096 messages")
			}
			if err := validateParts(m, session); err != nil {
				return "", err
			}
			if seen[m.Info.ID] {
				return "", errors.New("native history repeats message")
			}
			seen[m.Info.ID] = true
			foundInitial = foundInitial || m.Info.ID == initial
			foundFinal = foundFinal || m.Info.ID == final.Info.ID
		}
		reversePages = append(reversePages, page)
		next := op.header.Get("X-Next-Cursor")
		if foundInitial {
			break
		}
		if next == "" || cursors[next] || len(next) > 4096 {
			return "", errors.New("native history missing initial input or repeated cursor")
		}
		cursors[next] = true
		cursor = next
	}
	if !foundFinal {
		return "", errors.New("native history lacks returned final assistant")
	}
	var out strings.Builder
	inside := false
	users := map[string]bool{}
	for i := len(reversePages) - 1; i >= 0; i-- {
		for _, m := range reversePages[i] {
			if m.Info.ID == initial {
				if m.Info.Role != "user" {
					return "", errors.New("native initial message not a user")
				}
				inside = true
				users[m.Info.ID] = true
				continue
			}
			if !inside {
				if m.Info.ID == final.Info.ID {
					return "", errPriorAssistant
				}
				continue
			}
			if m.Info.Role == "user" {
				users[m.Info.ID] = true
			}
			if m.Info.Role == "assistant" && !m.Info.Summary {
				for _, raw := range m.Parts {
					var part struct{ Type, Text string }
					_ = json.Unmarshal(raw, &part)
					if part.Type == "text" {
						if out.Len()+len(part.Text) > maxNativeRequest {
							return "", errors.New("native output exceeds 1 MiB")
						}
						out.WriteString(part.Text)
					}
				}
			}
			if m.Info.ID == final.Info.ID {
				if m.Info.Role != "assistant" || m.Info.ParentID != final.Info.ParentID {
					return "", errors.New("native final history disagrees")
				}
				if !users[m.Info.ParentID] {
					return "", errors.New("native final parent is not a user in the owned history interval")
				}
				return out.String(), nil
			}
		}
	}
	return "", fmt.Errorf("native final assistant outside owned history interval")
}
