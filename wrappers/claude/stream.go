// SPDX-License-Identifier: MIT
package claude

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"

	kit "github.com/antst/sessionbus/bus/sdk/go"
)

type nativeFrame struct {
	Type             string   `json:"type"`
	Subtype          string   `json:"subtype"`
	SessionID        string   `json:"session_id"`
	UUID             string   `json:"uuid"`
	IsReplay         bool     `json:"isReplay"`
	IsError          bool     `json:"is_error"`
	Result           string   `json:"result"`
	Errors           []string `json:"errors"`
	TerminalReason   string   `json:"terminal_reason"`
	StopReason       string   `json:"stop_reason"`
	UserMessageUUID  string   `json:"user_message_uuid"`
	UserMessageUUIDs []string `json:"user_message_uuids"`
	RequestID        string   `json:"request_id"`
	Request          struct {
		Subtype string `json:"subtype"`
	} `json:"request"`
	Response struct {
		Subtype   string          `json:"subtype"`
		RequestID string          `json:"request_id"`
		Response  json.RawMessage `json:"response"`
		Error     string          `json:"error"`
	} `json:"response"`
}

type controlResult struct {
	value json.RawMessage
	err   error
}
type nativeRun struct {
	uuid, session string
	admitted      func()
	echoed        bool
	terminal      *nativeFrame
	result        chan runResult
}
type runResult struct {
	value kit.TurnResult
	err   error
}
type admission struct {
	session string
	done    chan error
}
type stream struct {
	input      io.WriteCloser
	output     io.ReadCloser
	mu, writes sync.Mutex
	closed     error
	controls   map[string]chan controlResult
	admissions map[string]*admission
	active     *nativeRun
	done       chan struct{}
	fatal      func(error)
}

func correlationID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = b[6]&15 | 64
	b[8] = b[8]&63 | 128
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[:4], b[4:6], b[6:8], b[8:10], b[10:]), nil
}
func newStream(input io.WriteCloser, output io.ReadCloser, fatal func(error)) *stream {
	s := &stream{input: input, output: output, controls: make(map[string]chan controlResult), admissions: make(map[string]*admission), done: make(chan struct{}), fatal: fatal}
	go s.read()
	return s
}
func (s *stream) stop(err error) {
	if err == nil {
		err = io.EOF
	}
	s.mu.Lock()
	if s.closed != nil {
		s.mu.Unlock()
		return
	}
	s.closed = err
	for _, c := range s.controls {
		c <- controlResult{err: err}
	}
	s.controls = make(map[string]chan controlResult)
	for _, a := range s.admissions {
		a.done <- err
	}
	s.admissions = make(map[string]*admission)
	if s.active != nil {
		s.active.result <- runResult{err: err}
		s.active = nil
	}
	close(s.done)
	s.mu.Unlock()
	_ = s.input.Close()
	_ = s.output.Close()
	if s.fatal != nil {
		s.fatal(err)
	}
}

// Writes serialize complete native frames, without holding the state mutex.
// Cancellation of a blocked pipe ends this transport; an attempted write is
// never relabelled as a native rejection.
func (s *stream) write(ctx context.Context, value any) (bool, error) {
	body, err := json.Marshal(value)
	if err != nil {
		return false, err
	}
	body = append(body, '\n')
	type written struct {
		attempted bool
		err       error
	}
	done := make(chan written, 1)
	go func() {
		s.writes.Lock()
		defer s.writes.Unlock()
		s.mu.Lock()
		closed := s.closed
		s.mu.Unlock()
		if closed != nil {
			done <- written{err: closed}
			return
		}
		if err := ctx.Err(); err != nil {
			done <- written{err: err}
			return
		}
		n, err := s.input.Write(body)
		if err == nil && n != len(body) {
			err = io.ErrShortWrite
		}
		done <- written{attempted: true, err: err}
	}()
	select {
	case result := <-done:
		if result.err != nil && result.attempted {
			s.stop(result.err)
		}
		if result.err != nil && ctx.Err() != nil {
			result.err = ctx.Err()
		}
		return result.attempted, result.err
	case <-ctx.Done():
		// A completed write wins over later request cancellation. Only an
		// outstanding write needs transport closure to unblock it.
		select {
		case result := <-done:
			return result.attempted, result.err
		default:
		}
		s.stop(ctx.Err())
		result := <-done
		result.err = ctx.Err()
		return result.attempted, result.err
	}
}
func (s *stream) control(ctx context.Context, request any) (json.RawMessage, error) {
	id, err := correlationID()
	if err != nil {
		return nil, err
	}
	result := make(chan controlResult, 1)
	s.mu.Lock()
	if s.closed != nil {
		err = s.closed
		s.mu.Unlock()
		return nil, err
	}
	s.controls[id] = result
	s.mu.Unlock()
	defer func() { s.mu.Lock(); delete(s.controls, id); s.mu.Unlock() }()
	if _, err = s.write(ctx, map[string]any{"type": "control_request", "request_id": id, "request": request}); err != nil {
		return nil, err
	}
	select {
	case r := <-result:
		return r.value, r.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
func userFrame(id, session, content string, query bool) any {
	return map[string]any{"type": "user", "uuid": id, "session_id": session, "message": map[string]string{"role": "user", "content": content}, "parent_tool_use_id": nil, "shouldQuery": query}
}
func (s *stream) run(ctx context.Context, session, input string, admitted func()) (kit.TurnResult, error) {
	id, err := correlationID()
	if err != nil {
		return kit.TurnResult{}, err
	}
	turn := &nativeRun{uuid: id, session: session, admitted: admitted, result: make(chan runResult, 1)}
	s.mu.Lock()
	if s.closed != nil {
		err = s.closed
	} else if s.active != nil {
		err = errors.New("native turn already active")
	} else {
		s.active = turn
	}
	s.mu.Unlock()
	if err != nil {
		return kit.TurnResult{}, err
	}
	if _, err = s.write(ctx, userFrame(id, session, input, true)); err != nil {
		s.mu.Lock()
		if s.active == turn {
			s.active = nil
		}
		s.mu.Unlock()
		return kit.TurnResult{}, err
	}
	select {
	case result := <-turn.result:
		return result.value, result.err
	case <-ctx.Done():
		s.stop(ctx.Err())
		return kit.TurnResult{}, ctx.Err()
	}
}
func (s *stream) append(ctx context.Context, session, content string) (kit.DeliveryReceipt, error) {
	id, err := correlationID()
	if err != nil {
		return kit.DeliveryReceipt{}, err
	}
	a := &admission{session: session, done: make(chan error, 1)}
	s.mu.Lock()
	if s.closed != nil {
		err = s.closed
	} else {
		s.admissions[id] = a
	}
	s.mu.Unlock()
	if err != nil {
		return kit.DeliveryReceipt{Disposition: "rejected", Reason: "native_unavailable"}, nil
	}
	defer func() { s.mu.Lock(); delete(s.admissions, id); s.mu.Unlock() }()
	attempted, err := s.write(ctx, userFrame(id, session, content, false))
	if err == nil {
		select {
		case err = <-a.done:
		case <-ctx.Done():
			err = ctx.Err()
		}
	}
	if err != nil {
		if attempted {
			return kit.DeliveryReceipt{}, &kit.ProtocolError{Code: -32603, Data: json.RawMessage(`"uncertain_native_admission"`)}
		}
		return kit.DeliveryReceipt{Disposition: "rejected", Reason: "not_submitted"}, nil
	}
	return kit.DeliveryReceipt{Disposition: "queued_for_next_turn"}, nil
}
func (s *stream) read() {
	decoder := json.NewDecoder(s.output)
	for {
		var f nativeFrame
		if err := decoder.Decode(&f); err != nil {
			s.stop(err)
			return
		}
		if f.Type == "control_request" {
			// No approval surface is declared by this adapter. Native headless policy
			// remains native; caller-enabled host-only control requests cannot be granted.
			go func() {
				_, _ = s.write(context.Background(), map[string]any{"type": "control_response", "response": map[string]string{"subtype": "error", "request_id": f.RequestID, "error": "Sessionbus lane has no handler for native control: " + f.Request.Subtype}})
			}()
			continue
		}
		s.mu.Lock()
		switch f.Type {
		case "control_response":
			if c := s.controls[f.Response.RequestID]; c != nil {
				delete(s.controls, f.Response.RequestID)
				r := controlResult{value: f.Response.Response}
				if f.Response.Subtype != "success" {
					r.err = fmt.Errorf("native control failed: %s", f.Response.Error)
				}
				c <- r
			}
		case "user":
			if f.IsReplay {
				if a := s.admissions[f.UUID]; a != nil && a.session == f.SessionID {
					delete(s.admissions, f.UUID)
					a.done <- nil
				}
				if t := s.active; t != nil && t.uuid == f.UUID && t.session == f.SessionID {
					t.echoed = true
					t.admitted()
					s.finishLocked()
				}
			}
		case "result":
			if t := s.active; t != nil && t.session == f.SessionID && containsUUID(f, t.uuid) {
				t.terminal = &f
				s.finishLocked()
			}
		}
		s.mu.Unlock()
	}
}
func containsUUID(f nativeFrame, id string) bool {
	if f.UserMessageUUID == id {
		return true
	}
	for _, v := range f.UserMessageUUIDs {
		if v == id {
			return true
		}
	}
	return false
}
func (s *stream) finishLocked() {
	t := s.active
	if t == nil || !t.echoed || t.terminal == nil {
		return
	}
	f := t.terminal
	reason := f.TerminalReason
	if f.StopReason != "" {
		if reason != "" {
			reason += "; "
		}
		reason += f.StopReason
	}
	outcome := "completed"
	if f.IsError || f.Subtype != "success" || f.StopReason == "refusal" {
		outcome = "failed"
	}
	if f.TerminalReason == "aborted_tools" || f.TerminalReason == "aborted_streaming" {
		outcome = "interrupted"
	}
	result := f.Result
	if result == "" && len(f.Errors) > 0 {
		result = strings.Join(f.Errors, "\n")
	}
	t.result <- runResult{value: kit.TurnResult{Outcome: outcome, Result: result, NativeStopReason: reason}}
	s.active = nil
}
