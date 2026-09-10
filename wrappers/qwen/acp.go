// SPDX-License-Identifier: MIT

package qwen

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"sync"
)

const maxACPFrame = 1 << 20
const maxACPPending = 256
const maxACPRetained = 32 << 20
const maxACPID = int64(9007199254740991)

var errACPCapacity = errors.New("Qwen ACP capacity exhausted")

type acpError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

func (e *acpError) Error() string { return fmt.Sprintf("Qwen ACP error %d: %s", e.Code, e.Message) }

type acpFrame struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *acpError       `json:"error,omitempty"`
}
type acpReply struct {
	result json.RawMessage
	err    error
	bytes  int
}
type acpPending struct {
	reply   chan acpReply
	observe func(json.RawMessage, error) error
}

// A native request is prepared only after acquiring the serialized writer.
// finish settles the selected delivery batch after the actual write attempt.
type acpResponse struct {
	Result  any
	Prepare func() (any, error)
	Finish  func(error)
}
type acpAnswer func(string, json.RawMessage) (*acpResponse, error)

type acpClient struct {
	input         io.WriteCloser
	output        io.ReadCloser
	mu            sync.Mutex
	writeGate     chan struct{}
	next          int64
	pending       map[int64]*acpPending // nil retains an abandoned call's correlation only.
	failed        error
	ctx           context.Context
	cancel        context.CancelFunc
	done          chan struct{} // reader and every admitted native response writer have joined.
	workers       sync.WaitGroup
	requests      map[string]bool
	retained      int
	notify        func(string, json.RawMessage)
	answerRequest acpAnswer
}

func newDuplexACP(input io.WriteCloser, output io.ReadCloser, notify func(string, json.RawMessage), answer acpAnswer) *acpClient {
	ctx, cancel := context.WithCancel(context.Background())
	c := &acpClient{input: input, output: output, writeGate: make(chan struct{}, 1), pending: map[int64]*acpPending{}, requests: map[string]bool{}, ctx: ctx, cancel: cancel, done: make(chan struct{}), notify: notify, answerRequest: answer}
	c.writeGate <- struct{}{}
	go c.read()
	return c
}
func (c *acpClient) call(ctx context.Context, method string, params, result any) error {
	return c.request(ctx, method, params, result, nil, nil, nil)
}
func (c *acpClient) request(ctx context.Context, method string, params, result any, written chan<- error, submit func() error, observe func(json.RawMessage, error) error) error {
	id, reply, err := c.begin(ctx, method, params, submit, observe)
	if written != nil {
		written <- err
	}
	if err != nil {
		return err
	}
	var got acpReply
	select {
	case got = <-reply:
	case <-ctx.Done():
		c.mu.Lock()
		_, unclaimed := c.pending[id]
		if unclaimed {
			c.pending[id] = nil
		}
		c.mu.Unlock()
		if unclaimed {
			return ctx.Err()
		}
		got = <-reply // The reader's completion claim wins later cancellation.
	}
	c.release(got.bytes)
	if got.err != nil || result == nil {
		return got.err
	}
	if err = json.Unmarshal(got.result, result); err != nil {
		return fmt.Errorf("decode Qwen ACP %s result: %w", method, err)
	}
	return nil
}
func (c *acpClient) begin(ctx context.Context, method string, params any, submit func() error, observe func(json.RawMessage, error) error) (int64, chan acpReply, error) {
	if err := ctx.Err(); err != nil {
		return 0, nil, err
	}
	c.mu.Lock()
	if c.failed != nil {
		err := c.failed
		c.mu.Unlock()
		return 0, nil, err
	}
	if len(c.pending) >= maxACPPending || c.next == maxACPID {
		c.mu.Unlock()
		return 0, nil, errACPCapacity
	}
	c.next++
	id := c.next
	reply := make(chan acpReply, 1)
	c.pending[id] = &acpPending{reply: reply, observe: observe}
	c.mu.Unlock()
	err := c.send(ctx, map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params}, submit)
	if err != nil {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return 0, nil, err
	}
	return id, reply, nil
}
func (c *acpClient) reserve(n int) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.failed != nil || n > maxACPRetained-c.retained {
		return false
	}
	c.retained += n
	return true
}
func (c *acpClient) release(n int) { c.mu.Lock(); c.retained -= n; c.mu.Unlock() }
func (c *acpClient) gate(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-c.ctx.Done():
		return c.failure()
	case <-c.writeGate:
	}
	if err := ctx.Err(); err != nil {
		c.writeGate <- struct{}{}
		return err
	}
	if err := c.ctx.Err(); err != nil {
		c.writeGate <- struct{}{}
		return c.failure()
	}
	return nil
}
func (c *acpClient) send(ctx context.Context, value any, submit func() error) error {
	body, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if len(body)+1 > maxACPFrame {
		return errors.New("Qwen ACP frame exceeds size limit")
	}
	if !c.reserve(len(body)) {
		return errACPCapacity
	}
	defer c.release(len(body))
	if err = c.gate(ctx); err != nil {
		return err
	}
	defer func() { c.writeGate <- struct{}{} }()
	if submit != nil {
		if err = submit(); err != nil {
			return err
		}
	}
	return c.writeBytes(ctx, append(body, '\n'))
}

// Only actual closable native pipes are supported. An attempted interrupted
// write retires the transport and joins the writer; it is never retried.
func (c *acpClient) writeBytes(ctx context.Context, body []byte) error {
	done := make(chan error, 1)
	go func() {
		n, err := c.input.Write(body)
		if err == nil && n != len(body) {
			err = io.ErrShortWrite
		}
		done <- err
	}()
	var err error
	select {
	case err = <-done:
	case <-ctx.Done():
		select {
		case err = <-done:
		default:
			c.fail(ctx.Err())
			<-done
			err = ctx.Err()
		}
	case <-c.ctx.Done():
		<-done
		err = c.failure()
	}
	if err != nil {
		c.fail(err)
	}
	return err
}
func (c *acpClient) failure() error { c.mu.Lock(); defer c.mu.Unlock(); return c.failed }
func acpID(raw json.RawMessage) (string, bool) {
	if len(raw) == 0 || string(raw) == "null" {
		return "", false
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		b, _ := json.Marshal(s)
		return string(b), true
	}
	var id int64
	if json.Unmarshal(raw, &id) != nil || id < -maxACPID || id > maxACPID {
		return "", false
	}
	return strconv.FormatInt(id, 10), true
}
func (c *acpClient) read() {
	defer func() { c.workers.Wait(); close(c.done) }()
	scanner := bufio.NewScanner(c.output)
	scanner.Buffer(make([]byte, 4096), maxACPFrame)
	for c.ctx.Err() == nil && scanner.Scan() {
		var frame acpFrame
		if json.Unmarshal(scanner.Bytes(), &frame) != nil || frame.JSONRPC != "2.0" {
			c.fail(errors.New("malformed Qwen ACP frame"))
			return
		}
		if frame.Method != "" {
			if len(frame.Result) != 0 || frame.Error != nil {
				c.fail(errors.New("invalid Qwen ACP method envelope"))
				return
			}
			if len(frame.ID) == 0 {
				if c.notify != nil {
					c.notify(frame.Method, frame.Params)
				}
				continue
			}
			key, valid := acpID(frame.ID)
			if !valid {
				c.fail(errors.New("invalid Qwen ACP request id"))
				return
			}
			size := len(frame.ID) + len(frame.Method) + len(frame.Params)
			c.mu.Lock()
			if c.requests[key] || len(c.requests) >= maxACPPending || size > maxACPRetained-c.retained {
				c.mu.Unlock()
				c.fail(errACPCapacity)
				return
			}
			c.requests[key] = true
			c.retained += size
			c.workers.Add(1)
			c.mu.Unlock()
			response, failure := (*acpResponse)(nil), error(nil)
			if c.answerRequest == nil {
				failure = &acpError{Code: -32601, Message: "unsupported Qwen ACP client request " + frame.Method}
			} else {
				response, failure = c.answerRequest(frame.Method, frame.Params)
			}
			go func() {
				defer c.workers.Done()
				defer func() { c.mu.Lock(); delete(c.requests, key); c.retained -= size; c.mu.Unlock() }()
				c.answer(frame, response, failure)
			}()
			continue
		}
		var id int64
		if json.Unmarshal(frame.ID, &id) != nil || id < 1 || (frame.Error == nil) == (len(frame.Result) == 0) {
			c.fail(errors.New("invalid Qwen ACP response"))
			return
		}
		c.mu.Lock()
		pending, known := c.pending[id]
		if known {
			delete(c.pending, id)
		}
		var err error
		if frame.Error != nil {
			err = frame.Error
		}
		size := len(frame.Result)
		if frame.Error != nil {
			size += len(frame.Error.Message) + len(frame.Error.Data)
		}
		if pending != nil && size > maxACPRetained-c.retained {
			c.mu.Unlock()
			pending.reply <- acpReply{err: errACPCapacity}
			c.fail(errACPCapacity)
			return
		}
		if pending != nil {
			c.retained += size
		}
		c.mu.Unlock()
		// Claim and observe before dispatching a buffered later notification.
		var observerErr error
		if pending != nil && pending.observe != nil {
			observerErr = pending.observe(frame.Result, err)
			if observerErr != nil {
				err = observerErr
			}
		}
		if pending != nil {
			pending.reply <- acpReply{result: frame.Result, err: err, bytes: size}
		}
		if observerErr != nil {
			c.fail(observerErr)
			return
		}
		if !known {
			c.fail(errors.New("unknown Qwen ACP response id"))
			return
		}
	}
	err := scanner.Err()
	if err == nil {
		err = io.EOF
	}
	c.fail(err)
}
func (c *acpClient) answer(frame acpFrame, answer *acpResponse, failure error) {
	var writeErr error
	defer func() {
		if answer != nil && answer.Finish != nil {
			answer.Finish(writeErr)
		}
	}()
	if writeErr = c.gate(c.ctx); writeErr != nil {
		return
	}
	defer func() { c.writeGate <- struct{}{} }()
	var result any
	if answer != nil {
		result = answer.Result
		if failure == nil && answer.Prepare != nil {
			result, failure = answer.Prepare()
		}
	}
	response := map[string]any{"jsonrpc": "2.0", "id": frame.ID}
	if failure != nil {
		var native *acpError
		if !errors.As(failure, &native) {
			native = &acpError{Code: -32603, Message: failure.Error()}
		}
		response["error"] = native
	} else {
		response["result"] = result
	}
	body, writeErr := json.Marshal(response)
	if writeErr == nil && len(body)+1 > maxACPFrame {
		writeErr = errors.New("Qwen ACP response exceeds limit")
	}
	if writeErr == nil {
		writeErr = c.writeBytes(c.ctx, append(body, '\n'))
	} else {
		c.fail(writeErr)
	}
}
func (c *acpClient) fail(err error) {
	c.mu.Lock()
	if c.failed != nil {
		c.mu.Unlock()
		return
	}
	if err == nil {
		err = io.EOF
	}
	c.failed = err
	c.cancel()
	for id, pending := range c.pending {
		if pending != nil {
			pending.reply <- acpReply{err: err}
		}
		delete(c.pending, id)
	}
	c.mu.Unlock()
	_ = c.input.Close()
	_ = c.output.Close()
}
func (c *acpClient) close() error { c.fail(io.ErrClosedPipe); return nil }
