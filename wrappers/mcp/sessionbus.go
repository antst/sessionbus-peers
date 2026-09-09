// SPDX-License-Identifier: MIT

package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"sync"

	kit "github.com/antst/sessionbus/bus/sdk/go"
)

// SessionbusOwner owns public calls and their connection lifetime.
type SessionbusOwner interface {
	Action(context.Context, string, json.RawMessage) (json.RawMessage, error)
	End()
}

// ReportHandler is an optional native hook capability, never model-advertised.
// Native validation and identity state stay in the product callback.
type ReportHandler struct {
	Name  string
	Begin func(json.RawMessage) (<-chan error, error)
}

func requestID(raw json.RawMessage) (string, bool) {
	var id any
	if json.Unmarshal(raw, &id) != nil {
		return "", false
	}
	switch x := id.(type) {
	case string:
		b, _ := json.Marshal(x)
		return string(b), true
	case float64:
		if math.Trunc(x) == x && math.Abs(x) <= 9007199254740991 {
			b, _ := json.Marshal(x)
			return string(b), true
		}
	}
	return "", false
}

func toolResult(value json.RawMessage, err error) any {
	result := map[string]any{}
	if err != nil {
		var protocolError *kit.ProtocolError
		if errors.As(err, &protocolError) {
			value, _ = json.Marshal(protocolError)
		} else {
			value, _ = json.Marshal(map[string]string{"error": err.Error()})
		}
		result["isError"] = true
	}
	if len(value) == 0 {
		value = json.RawMessage(`{}`)
	}
	result["content"] = []any{map[string]string{"type": "text", "text": string(value)}}
	return result
}

// Serve keeps native reports, cancellation and EOF independent of pending public
// calls. Only in-flight public request IDs are retained; results live in Caller.
func ServeSessionbus(owner SessionbusOwner, input io.ReadCloser, output io.Writer, report ReportHandler) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var state, writes sync.Mutex
	var workers sync.WaitGroup
	pending := map[string]context.CancelFunc{}
	var once sync.Once
	stop := func() { once.Do(func() { cancel(); owner.End(); _ = input.Close() }) }
	defer stop()
	write := func(frame any) {
		writes.Lock()
		defer writes.Unlock()
		if ctx.Err() != nil {
			return
		}
		body, err := json.Marshal(frame)
		if err == nil {
			body = append(body, '\n')
			var n int
			n, err = output.Write(body)
			if err == nil && n != len(body) {
				err = io.ErrShortWrite
			}
		}
		if err != nil {
			stop()
		}
	}
	failure := func(id json.RawMessage, code int, message string) {
		if len(id) == 0 {
			id = json.RawMessage(`null`)
		}
		write(map[string]any{"jsonrpc": "2.0", "id": id, "error": map[string]any{"code": code, "message": message}})
	}
	respond := func(id json.RawMessage, value any) {
		write(map[string]any{"jsonrpc": "2.0", "id": id, "result": value})
	}
	reader := bufio.NewReader(input)
	for ctx.Err() == nil {
		line, readErr := reader.ReadBytes('\n')
		if len(line) == 0 {
			break
		}
		var frame map[string]json.RawMessage
		if !json.Valid(line) {
			failure(nil, -32700, "Parse error")
			if readErr != nil {
				break
			}
			continue
		}
		if json.Unmarshal(line, &frame) != nil || frame == nil {
			failure(nil, -32600, "Invalid Request")
			continue
		}
		id, hasID := frame["id"]
		key, validID := requestID(id)
		var version, method string
		if json.Unmarshal(frame["jsonrpc"], &version) != nil || version != "2.0" || json.Unmarshal(frame["method"], &method) != nil || string(frame["method"]) == "null" || hasID && !validID {
			if !validID {
				id = nil
			}
			failure(id, -32600, "Invalid Request")
			continue
		}
		var params map[string]json.RawMessage
		p := frame["params"]
		if p == nil || string(p) == "null" {
			p = json.RawMessage(`{}`)
		}
		paramsErr := json.Unmarshal(p, &params)
		if !hasID {
			if method == "notifications/cancelled" && paramsErr == nil {
				target, valid := requestID(params["requestId"])
				if valid {
					state.Lock()
					if abort := pending[target]; abort != nil {
						abort()
					}
					state.Unlock()
				}
			}
			continue
		}
		if paramsErr != nil || params == nil {
			failure(id, -32602, "Invalid parameters")
			continue
		}
		switch method {
		case "initialize":
			var version string
			if string(params["protocolVersion"]) == "null" || json.Unmarshal(params["protocolVersion"], &version) != nil {
				failure(id, -32602, "Invalid initialize parameters")
				continue
			}
			respond(id, map[string]any{"protocolVersion": version, "capabilities": map[string]any{"tools": map[string]any{}}, "serverInfo": map[string]string{"name": "sessionbus", "version": "0.5.0"}})
		case "ping":
			respond(id, map[string]any{})
		case "tools/list":
			respond(id, map[string]any{"tools": []any{Tool()}})
		case "tools/call":
			var name string
			if json.Unmarshal(params["name"], &name) != nil || name != "sessionbus" && (report.Begin == nil || name != report.Name) {
				failure(id, -32602, "Unknown tool")
				continue
			}
			state.Lock()
			_, duplicate := pending[key]
			state.Unlock()
			if duplicate {
				failure(id, -32600, "Request ID already in flight")
				continue
			}
			if report.Begin != nil && name == report.Name {
				done, err := report.Begin(params["arguments"])
				if err != nil || done == nil {
					respond(id, toolResult(nil, err))
					continue
				}
				workers.Add(1)
				go func() { defer workers.Done(); respond(id, toolResult(nil, <-done)) }()
			} else {
				callCtx, abort := context.WithCancel(ctx)
				state.Lock()
				pending[key] = abort
				state.Unlock()
				workers.Add(1)
				go func() {
					defer workers.Done()
					defer abort()
					value, err := CallTool(callCtx, owner, params["arguments"])
					state.Lock()
					delete(pending, key)
					state.Unlock()
					// Completion wins: never suppress a fulfilled consuming result.
					if err != nil && callCtx.Err() != nil && errors.Is(err, callCtx.Err()) {
						return
					}
					respond(id, toolResult(value, err))
				}()
			}
		default:
			failure(id, -32601, "Method not found")
		}
		if readErr != nil {
			break
		}
	}
	stop()
	workers.Wait()
	return nil
}
