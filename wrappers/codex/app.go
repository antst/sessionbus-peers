// SPDX-License-Identifier: MIT

package codex

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"

	sessionkit "github.com/antst/sessionbus/bus/sdk/go"
)

type appError = sessionkit.ProtocolError

type appReply struct {
	result json.RawMessage
	err    error
}

type appTransport interface {
	Read(any) error
	Write(any) error
	Close() error
}

type streamTransport struct {
	input   io.WriteCloser
	output  io.ReadCloser
	decoder *json.Decoder
}

func (s *streamTransport) Read(value any) error { return s.decoder.Decode(value) }
func (s *streamTransport) Write(value any) error {
	body, err := json.Marshal(value)
	if err != nil {
		return err
	}
	body = append(body, '\n')
	n, err := s.input.Write(body)
	if err == nil && n != len(body) {
		err = io.ErrShortWrite
	}
	return err
}
func (s *streamTransport) Close() error { return s.input.Close() }

type appFrame struct {
	ID     json.RawMessage `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
	Result json.RawMessage `json:"result"`
	Error  *appError       `json:"error"`
}

type appPending struct {
	reply   chan appReply
	observe func(json.RawMessage) error
}
type appRequestHandler func(context.Context, string, json.RawMessage) (any, error)

type appClient struct {
	transport appTransport
	mu        sync.Mutex
	writeGate chan struct{}
	next      int64
	pending   map[int64]appPending
	failed    error
	done      chan struct{}
	notify    func(string, json.RawMessage)
	onFailure func(error)
	context   context.Context
	cancel    context.CancelFunc
	request   appRequestHandler
}

func newAppClient(input io.WriteCloser, output io.ReadCloser, notify func(string, json.RawMessage), failure func(error), handler ...appRequestHandler) *appClient {
	return newTransportClient(&streamTransport{input: input, output: output, decoder: json.NewDecoder(output)}, notify, failure, handler...)
}

func newTransportClient(transport appTransport, notify func(string, json.RawMessage), failure func(error), handler ...appRequestHandler) *appClient {
	ctx, cancel := context.WithCancel(context.Background())
	c := &appClient{transport: transport, pending: map[int64]appPending{}, done: make(chan struct{}), notify: notify, onFailure: failure, context: ctx, cancel: cancel, writeGate: make(chan struct{}, 1)}
	c.writeGate <- struct{}{}
	if len(handler) > 0 {
		c.request = handler[0]
	}
	go c.read()
	return c
}

func (c *appClient) initialize(ctx context.Context, title string) error {
	err := c.call(ctx, "initialize", map[string]any{
		"clientInfo":   map[string]string{"name": Product, "title": title, "version": "1"},
		"capabilities": map[string]bool{"experimentalApi": true},
	}, &struct{}{})
	if err == nil {
		err = c.notifyMethod("initialized", map[string]any{})
	}
	return err
}

func (c *appClient) call(ctx context.Context, method string, params, result any) error {
	return c.callObserved(ctx, method, params, result, nil)
}

// observe executes in native read order, before a later notification can change
// admission classification. It must not perform I/O or wait for a public receipt.
func (c *appClient) callObserved(ctx context.Context, method string, params, result any, observe func(json.RawMessage) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	c.mu.Lock()
	if c.failed != nil {
		err := c.failed
		c.mu.Unlock()
		return err
	}
	c.next++
	id, reply := c.next, make(chan appReply, 1)
	c.pending[id] = appPending{reply: reply, observe: observe}
	c.mu.Unlock()
	if _, err := c.writeContext(ctx, map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params}); err != nil {
		c.remove(id)
		return err
	}
	var response appReply
	select {
	case response = <-reply:
	case <-ctx.Done():
		select {
		case response = <-reply:
		default:
			return ctx.Err()
		}
	}
	if response.err != nil {
		return response.err
	}
	if result == nil || len(response.result) == 0 {
		return nil
	}
	if err := json.Unmarshal(response.result, result); err != nil {
		return fmt.Errorf("decode App Server %s result: %w", method, err)
	}
	return nil
}

func (c *appClient) notifyMethod(method string, params any) error {
	return c.write(map[string]any{"jsonrpc": "2.0", "method": method, "params": params})
}

func (c *appClient) write(value any) error {
	_, err := c.writeContext(c.context, value)
	return err
}
func (c *appClient) writeContext(ctx context.Context, value any) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	type written struct {
		attempted bool
		err       error
	}
	select {
	case <-ctx.Done():
		return false, ctx.Err()
	case <-c.writeGate:
	}
	defer func() { c.writeGate <- struct{}{} }()
	c.mu.Lock()
	failed := c.failed
	c.mu.Unlock()
	if failed != nil {
		return false, failed
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	done := make(chan written, 1)
	go func() { done <- written{attempted: true, err: c.transport.Write(value)} }()
	var result written
	select {
	case result = <-done:
	case <-ctx.Done():
		select {
		case result = <-done:
		default:
			c.fail(ctx.Err())
			result = <-done
			result.err = ctx.Err()
		}
	}
	if result.attempted && result.err != nil {
		c.fail(result.err)
	}
	return result.attempted, result.err
}
func (c *appClient) serveRequest(frame appFrame) {
	var value any
	var err error
	if c.request != nil {
		value, err = c.request(c.context, frame.Method, frame.Params)
	} else {
		err = &appError{Code: -32601, Message: "unsupported client exchange: " + frame.Method}
	}
	response := map[string]any{"jsonrpc": "2.0", "id": frame.ID}
	if err != nil {
		var native *appError
		if !errors.As(err, &native) {
			native = &appError{Code: -32603, Message: err.Error()}
		}
		response["error"] = native
	} else {
		response["result"] = value
	}
	_ = c.write(response)
}

func (c *appClient) read() {
	defer close(c.done)
	if stream, ok := c.transport.(*streamTransport); ok {
		defer stream.output.Close()
	}
	for !c.isFailed() {
		var frame appFrame
		if err := c.transport.Read(&frame); err != nil {
			if errors.Is(err, io.EOF) {
				err = fmt.Errorf("Codex App Server exited: %w", io.EOF)
			} else {
				err = fmt.Errorf("malformed Codex App Server frame: %w", err)
			}
			c.fail(err)
			return
		}
		if frame.Method != "" {
			if len(frame.ID) != 0 {
				go c.serveRequest(frame)
			} else if c.notify != nil {
				c.notify(frame.Method, frame.Params)
			}
			continue
		}
		var id int64
		if json.Unmarshal(frame.ID, &id) != nil || id < 1 {
			c.fail(errors.New("Codex App Server returned an invalid response id"))
			return
		}
		if (frame.Error == nil) == (len(frame.Result) == 0) {
			c.fail(errors.New("Codex App Server response must contain exactly one of result or error"))
			return
		}
		c.mu.Lock()
		reply := c.pending[id]
		delete(c.pending, id)
		c.mu.Unlock()
		if reply.reply == nil {
			c.fail(errors.New("Codex App Server returned an unknown response id"))
			return
		}
		if frame.Error != nil {
			reply.reply <- appReply{err: frame.Error}
		} else {
			var err error
			if reply.observe != nil {
				err = reply.observe(frame.Result)
			}
			reply.reply <- appReply{result: frame.Result, err: err}
		}
	}
}

func (c *appClient) isFailed() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.failed != nil
}

func (c *appClient) remove(id int64) {
	c.mu.Lock()
	delete(c.pending, id)
	c.mu.Unlock()
}

func (c *appClient) fail(err error) {
	c.mu.Lock()
	if c.failed != nil {
		c.mu.Unlock()
		return
	}
	c.failed = err
	pending := c.pending
	c.pending = map[int64]appPending{}
	c.mu.Unlock()
	for _, reply := range pending {
		reply.reply <- appReply{err: err}
	}
	c.cancel()
	_ = c.transport.Close()
	if stream, ok := c.transport.(*streamTransport); ok {
		_ = stream.output.Close()
	}
	if c.onFailure != nil {
		c.onFailure(err)
	}
}

func (c *appClient) close() error {
	return c.transport.Close()
}
