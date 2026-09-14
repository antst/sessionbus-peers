// SPDX-License-Identifier: MIT
package opencodefamily

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
)

const (
	maxNativeRequest  = 1 << 20
	maxNativeResponse = 8 << 20
	maxNativeRequests = 8
)

// laneHTTP owns bounded native HTTP work. There is no application retry. A
// response and a completed request write are distinct, reusable observations.
// Seven ordinary slots plus one reserved cancellation slot bound retained bodies; no buffers are preallocated to the caps.
type laneHTTP struct {
	kind                                    nativeKind
	endpoint, directory, username, password string
	dial                                    func(context.Context, string, string) (net.Conn, error)
	slots                                   chan struct{}
	control                                 chan struct{}
}

type httpOperation struct {
	written, done chan struct{}
	writeOnce     sync.Once
	writeErr      error  // published by written
	data          []byte // published by done
	header        http.Header
	status        int
	err           error
}

func (o *httpOperation) wrote(err error) {
	o.writeOnce.Do(func() { o.writeErr = err; close(o.written) })
}
func (o *httpOperation) wait() ([]byte, error) { <-o.done; return o.data, o.err }

func newLaneHTTP(endpoint, directory, username, password string) *laneHTTP {
	return &laneHTTP{endpoint: endpoint, directory: directory, username: username, password: password,
		slots: make(chan struct{}, maxNativeRequests-1), control: make(chan struct{}, 1), dial: (&net.Dialer{}).DialContext}
}

// Each native exchange owns a single loopback HTTP/1 connection. Request.Write
// includes its final flush; Transport.WroteRequest does not. This is necessary
// to distinguish a completed write from failure flushing Transport's buffer.
func (c *laneHTTP) exchange(r *http.Request, wrote func(error)) (*http.Response, error) {
	conn, err := c.dial(r.Context(), "tcp", r.URL.Host)
	if err != nil {
		wrote(err)
		return nil, err
	}
	stop := context.AfterFunc(r.Context(), func() { conn.Close() })
	cleanup := func() { stop(); conn.Close() }
	r.Close = true
	err = r.Write(conn)
	wrote(err)
	if err != nil {
		cleanup()
		return nil, err
	}
	headers := &io.LimitedReader{R: conn, N: 64 << 10}
	response, err := http.ReadResponse(bufio.NewReader(headers), r)
	if err != nil {
		cleanup()
		return nil, err
	}
	headers.N = 1<<63 - 1 // Headers bounded; bodies/individual SSE frames bounded by their consumer.
	response.Body = &nativeBody{ReadCloser: response.Body, close: cleanup}
	return response, nil
}

type nativeBody struct {
	io.ReadCloser
	close func()
}

func (b *nativeBody) Close() error { b.close(); return b.ReadCloser.Close() }

func encodeNativeFor(kind nativeKind, body any) ([]byte, error) {
	if body == nil {
		return nil, nil
	}
	b, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	if len(b) > maxNativeRequest {
		return nil, kind.err("request exceeds 1 MiB")
	}
	return b, nil
}

func (c *laneHTTP) prepare(ctx context.Context, method, path string, body []byte) (*http.Request, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(body) > maxNativeRequest {
		return nil, c.kind.err("request exceeds 1 MiB")
	}
	target, err := url.Parse(c.endpoint + path)
	if err != nil {
		return nil, err
	}
	q := target.Query()
	q.Set("directory", c.directory)
	target.RawQuery = q.Encode()
	r, err := http.NewRequestWithContext(ctx, method, target.String(), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	r.GetBody = nil // Never make a submitted mutating request replayable.
	r.SetBasicAuth(c.username, c.password)
	r.Header.Set("x-"+c.kind.name()+"-directory", c.directory)
	if body != nil {
		r.Header.Set("Content-Type", "application/json")
	}
	return r, nil
}

// begin returns a preflight error without submission. Once returned successfully,
// the operation is owned and potentially submitted, even if its eventual error
// happened before any native response. Callers must join it and never replay it.
func (c *laneHTTP) begin(r *http.Request, expected ...int) (*httpOperation, error) {
	return c.start(r, c.slots, expected...)
}
func (c *laneHTTP) beginControl(r *http.Request, expected ...int) (*httpOperation, error) {
	return c.start(r, c.control, expected...)
}
func (c *laneHTTP) start(r *http.Request, slots chan struct{}, expected ...int) (*httpOperation, error) {
	if err := r.Context().Err(); err != nil {
		return nil, err
	}
	select {
	case slots <- struct{}{}:
	default:
		return nil, c.kind.err("HTTP work limit reached")
	}
	o := &httpOperation{written: make(chan struct{}), done: make(chan struct{})}
	go func() {
		defer func() { <-slots; close(o.done) }()
		response, err := c.exchange(r, o.wrote)
		if err != nil {
			o.err = errors.Join(err, r.Context().Err())
			o.wrote(err)
			return
		}
		defer response.Body.Close()
		o.status, o.header = response.StatusCode, response.Header.Clone()
		o.data, o.err = io.ReadAll(io.LimitReader(response.Body, maxNativeResponse+1))
		if len(o.data) > maxNativeResponse {
			o.data = nil
			o.err = c.kind.err("response exceeds 8 MiB")
		}
		if o.err == nil {
			accepted := false
			for _, s := range expected {
				accepted = accepted || s == o.status
			}
			if !accepted {
				o.err = fmt.Errorf("%s %s %s returned HTTP %d", c.kind.title(), r.Method, r.URL.Path, o.status)
			}
		}
		// Exchange always settles the write before parsing any response.
		o.wrote(c.kind.err("request write was not confirmed"))
	}()
	return o, nil
}

func (c *laneHTTP) call(ctx context.Context, method, path string, body any, expected ...int) ([]byte, error) {
	b, err := encodeNativeFor(c.kind, body)
	if err != nil {
		return nil, err
	}
	r, err := c.prepare(ctx, method, path, b)
	if err != nil {
		return nil, err
	}
	o, err := c.begin(r, expected...)
	if err != nil {
		return nil, err
	}
	return o.wait()
}

func (c *laneHTTP) closeIdle() {}

// events establishes one scoped stream. It does not infer execution completion
// from session status; only native permission/question requests use this stream.
func (c *laneHTTP) events(ctx context.Context, observe func([]byte) error) (<-chan error, error) {
	r, err := c.prepare(ctx, http.MethodGet, "/event", nil)
	if err != nil {
		return nil, err
	}
	response, err := c.exchange(r, func(error) {})
	if err != nil {
		return nil, err
	}
	if response.StatusCode != http.StatusOK {
		response.Body.Close()
		return nil, fmt.Errorf("%s events HTTP %d", c.kind.title(), response.StatusCode)
	}
	done := make(chan error, 1)
	go func() {
		defer close(done)
		defer response.Body.Close()
		scanner := bufio.NewScanner(response.Body)
		scanner.Buffer(make([]byte, 4096), maxNativeResponse)
		var data []byte
		for scanner.Scan() {
			line := scanner.Text()
			if line == "" {
				if len(data) > 0 {
					if err := observe(data); err != nil {
						done <- err
						return
					}
					data = nil
				}
				continue
			}
			if strings.HasPrefix(line, "data:") {
				part := strings.TrimPrefix(line, "data:")
				part = strings.TrimPrefix(part, " ")
				if len(data)+len(part)+1 > maxNativeResponse {
					done <- c.kind.err("event exceeds 8 MiB")
					return
				}
				if len(data) > 0 {
					data = append(data, '\n')
				}
				data = append(data, part...)
			}
		}
		if err := scanner.Err(); err != nil {
			done <- err
		} else {
			done <- c.kind.err("event stream ended")
		}
	}()
	return done, nil
}
