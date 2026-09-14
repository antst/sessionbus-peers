// SPDX-License-Identifier: MIT

package codex

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"sync"

	"github.com/coder/websocket"
)

// brokerStdio bounds a native JSON line before decoding. Close owns only stdin;
// the process owner continues draining stdout and reaps after native EOF exit.
type brokerStdio struct {
	input  io.WriteCloser
	reader *bufio.Reader
}

func (s *brokerStdio) Read(v any) error {
	var line []byte
	for {
		part, err := s.reader.ReadSlice('\n')
		if len(line)+len(part) > brokerMessageLimit {
			return errors.New("native App Server frame exceeds message limit")
		}
		line = append(line, part...)
		if err == bufio.ErrBufferFull {
			continue
		}
		if err != nil && (err != io.EOF || len(line) == 0) {
			return err
		}
		return json.Unmarshal(line, v)
	}
}
func (s *brokerStdio) Write(v any) error { return json.NewEncoder(s.input).Encode(v) }
func (s *brokerStdio) Close() error      { return s.input.Close() }

// serveBrokerTUI accepts one native client for this launch. A second connection
// cannot initialize a second session or replace the owning TUI transport.
func serveBrokerTUI(m *brokerMux, listener net.Listener) (*http.Server, <-chan struct{}) {
	var mu sync.Mutex
	accepted := false
	var conn *websocket.Conn
	done := make(chan struct{})
	server := &http.Server{}
	server.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		if accepted {
			mu.Unlock()
			http.Error(w, "this launch already has a TUI", http.StatusConflict)
			return
		}
		if m.ctx.Err() != nil {
			mu.Unlock()
			http.Error(w, "broker is closing", http.StatusServiceUnavailable)
			return
		}
		c, err := websocket.Accept(w, r, &websocket.AcceptOptions{CompressionMode: websocket.CompressionDisabled})
		if err != nil {
			mu.Unlock()
			return
		}
		accepted = true
		conn = c
		mu.Unlock()
		c.SetReadLimit(brokerMessageLimit)
		defer c.CloseNow()
		writerDone := make(chan struct{})
		go func() {
			defer close(writerDone)
			for {
				select {
				case <-m.ctx.Done():
					return
				case f := <-m.tuiOut:
					err := c.Write(m.ctx, websocket.MessageText, f.body)
					m.release(len(f.body))
					if err != nil {
						m.fail(err)
						return
					}
				}
			}
		}()
		for m.ctx.Err() == nil {
			kind, b, err := c.Read(m.ctx)
			if err != nil {
				m.fail(err)
				break
			}
			if kind != websocket.MessageText {
				m.fail(errors.New("native TUI sent non-text WebSocket frame"))
				break
			}
			var f brokerFrame
			if err = json.Unmarshal(b, &f); err == nil {
				if f == nil {
					err = errors.New("invalid TUI frame")
				} else {
					err = m.fromTUI(f)
				}
			}
			if err != nil {
				m.fail(err)
				break
			}
		}
		m.fail(errors.New("native TUI connection closed"))
		<-writerDone
	})
	go func() {
		err := server.Serve(listener)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			m.fail(err)
		}
		close(done)
	}()
	go func() {
		<-m.ctx.Done()
		_ = server.Close()
		mu.Lock()
		c := conn
		mu.Unlock()
		if c != nil {
			_ = c.CloseNow()
		}
	}()
	return server, done
}

// Interface assertion also prevents accidental addition of initialize ownership.
var _ appTransport = (*brokerStdio)(nil)
