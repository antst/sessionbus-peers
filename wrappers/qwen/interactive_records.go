// SPDX-License-Identifier: MIT
package qwen

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf16"

	"golang.org/x/sys/unix"
)

const maxInteractiveRecord = 8 << 20

// Native JSONL is observed incrementally. An incomplete final record is kept
// until a later append; replacement/truncation never becomes a new identity.
type nativeRecords struct {
	path   string
	info   os.FileInfo
	offset int64
	tail   []byte
}

func (r *nativeRecords) read(consume func([]byte) error) error {
	f, err := os.OpenFile(r.path, os.O_RDONLY|unix.O_NONBLOCK, 0)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return errors.New("native observation requires a regular file")
	}
	if r.info != nil && (!os.SameFile(r.info, info) || info.Size() < r.offset) {
		return errors.New("native observation file was replaced or truncated")
	}
	r.info = info
	if _, err = f.Seek(r.offset, io.SeekStart); err != nil {
		return err
	}
	buffer := make([]byte, 32<<10)
	for {
		n, readErr := f.Read(buffer)
		r.offset += int64(n)
		r.tail = append(r.tail, buffer[:n]...)
		for {
			end := bytes.IndexByte(r.tail, '\n')
			if end < 0 {
				break
			}
			if end > maxInteractiveRecord {
				return errors.New("native observation record exceeds 8 MiB")
			}
			line := bytes.TrimSpace(r.tail[:end])
			if len(line) > 0 {
				if err = consume(line); err != nil {
					return err
				}
			}
			r.tail = r.tail[end+1:]
		}
		if len(r.tail) > maxInteractiveRecord {
			return errors.New("native observation record exceeds 8 MiB")
		}
		if readErr != nil {
			if readErr == io.EOF {
				return nil
			}
			return readErr
		}
	}
}

type initialNativeSession struct{ ID, CWD string }

func (s *initialNativeSession) observe(line []byte) error {
	var event struct {
		Type    string `json:"type"`
		Subtype string `json:"subtype"`
		Data    struct {
			ID  string `json:"session_id"`
			CWD string `json:"cwd"`
		} `json:"data"`
	}
	if err := json.Unmarshal(line, &event); err != nil {
		return fmt.Errorf("native event: %w", err)
	}
	if event.Type != "system" || event.Subtype != "session_start" || s.ID != "" {
		return nil
	}
	if !qwenSessionID.MatchString(event.Data.ID) || !filepath.IsAbs(event.Data.CWD) {
		return errors.New("invalid native initial session_start")
	}
	s.ID, s.CWD = event.Data.ID, event.Data.CWD
	return nil
}

type nativeManualTitle struct {
	value    string
	observed bool
}

func (t *nativeManualTitle) observe(id string, line []byte) error {
	var event struct {
		ID      string `json:"sessionId"`
		Type    string `json:"type"`
		Subtype string `json:"subtype"`
		Payload struct {
			Title  *string `json:"customTitle"`
			Source string  `json:"titleSource"`
		} `json:"systemPayload"`
	}
	if err := json.Unmarshal(line, &event); err != nil {
		return fmt.Errorf("native history: %w", err)
	}
	if event.ID != id || event.Type != "system" || event.Subtype != "custom_title" || event.Payload.Source == "auto" {
		return nil
	}
	if event.Payload.Title == nil {
		return errors.New("native custom_title lacks a title")
	}
	t.value, t.observed = *event.Payload.Title, true
	return nil
}

// Qwen's POSIX sanitizeCwd replaces each non-ASCII-alphanumeric UTF-16 code
// unit. In particular a supplementary Unicode rune contributes two hyphens.
func nativeHistoryPath(home, cwd, id string) string {
	var slug strings.Builder
	for _, unit := range utf16.Encode([]rune(cwd)) {
		if unit >= 'a' && unit <= 'z' || unit >= 'A' && unit <= 'Z' || unit >= '0' && unit <= '9' {
			slug.WriteByte(byte(unit))
		} else {
			slug.WriteByte('-')
		}
	}
	return filepath.Join(home, "projects", slug.String(), "chats", id+".jsonl")
}

type nativeRegistry struct {
	Schema    int     `json:"schemaVersion"`
	PID       int     `json:"pid"`
	ProcStart *string `json:"procStart"`
	PIDNS     *uint64 `json:"pidNs"`
	ID        string  `json:"sessionId"`
	CWD       string  `json:"cwd"`
	StartedAt int64   `json:"startedAt"`
}

// The exact native parent PID selects the record; session-ID-only lookup
// would admit stale rows. Process identity is rechecked after reading it.
func readInitialRegistry(home string, parent nativeProcessIdentity, session initialNativeSession) (bool, error) {
	path := filepath.Join(home, "sessions", strconv.Itoa(parent.pid)+".json")
	f, err := os.OpenFile(path, os.O_RDONLY|unix.O_NONBLOCK, 0)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	info, statErr := f.Stat()
	if statErr != nil || !info.Mode().IsRegular() {
		return false, errors.Join(errors.New("native registry is not regular"), statErr, f.Close())
	}
	data, readErr := io.ReadAll(io.LimitReader(f, (64<<10)+1))
	err = errors.Join(readErr, f.Close())
	if err != nil {
		return false, err
	}
	if len(data) > 64<<10 {
		return false, errors.New("native registry exceeds 64 KiB")
	}
	var row nativeRegistry
	if json.Unmarshal(data, &row) != nil || row.Schema != 1 || row.PID != parent.pid || row.ID != session.ID || row.CWD != session.CWD {
		return false, errors.New("native registry contradicts initial session identity")
	}
	if err = parent.validateRegistry(row); err != nil {
		return false, err
	}
	current, err := inspectNativeProcess(parent.pid)
	if err != nil {
		return false, err
	}
	if current != parent {
		return false, errors.New("native parent identity changed during registry observation")
	}
	return true, nil
}
