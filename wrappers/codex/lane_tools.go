// SPDX-License-Identifier: MIT
package codex

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

const laneServer = "sessionbus"
const lanePlugin = "codex@sessionbus-peers"

func (p *Wrapper) receiveStartup(raw json.RawMessage) {
	var event struct{ ThreadID, Name, Status string }
	if json.Unmarshal(raw, &event) != nil || event.Name != laneServer {
		return
	}
	if event.ThreadID == "" {
		return
	} // Global catalog events do not bind the lane.
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.startup == nil {
		p.startup = map[string]string{}
	}
	p.startup[event.ThreadID] = event.Status
	if p.startupChanged != nil {
		close(p.startupChanged)
	}
	p.startupChanged = make(chan struct{})
}

type laneServerStatus struct {
	Name, PluginID, RuntimeStatus string
	Tools                         map[string]json.RawMessage
}

func (p *Wrapper) toolStatus(ctx context.Context, id string) (laneServerStatus, error) {
	cursor := ""
	seen := map[string]bool{}
	for {
		var page struct {
			Data       []laneServerStatus
			NextCursor *string
		}
		params := map[string]any{"threadId": id, "limit": 100, "detail": "toolsAndAuthOnly"}
		if cursor != "" {
			params["cursor"] = cursor
		}
		if err := p.app.call(ctx, "mcpServerStatus/list", params, &page); err != nil {
			return laneServerStatus{}, err
		}
		for _, server := range page.Data {
			if server.Name == laneServer {
				if server.PluginID != lanePlugin {
					return server, errors.New("required Sessionbus server is not supplied by the installed Codex plugin")
				}
				return server, nil
			}
		}
		if page.NextCursor == nil || *page.NextCursor == "" {
			return laneServerStatus{}, errors.New("required Sessionbus plugin server absent")
		}
		cursor = *page.NextCursor
		if seen[cursor] {
			return laneServerStatus{}, errors.New("native tool catalog repeated cursor")
		}
		seen[cursor] = true
	}
}
func (p *Wrapper) awaitTools(ctx context.Context, id string) error {
	status, err := p.toolStatus(ctx, id)
	if err != nil {
		return fmt.Errorf("integration open unavailable: %w", err)
	}
	switch status.RuntimeStatus {
	case "disabled", "failed", "cancelled", "authenticationRequired":
		return fmt.Errorf("integration open unavailable: MCP %s", status.RuntimeStatus)
	}
	for {
		p.mu.Lock()
		state, changed := p.startup[id], p.startupChanged
		p.mu.Unlock()
		if state == "ready" {
			break
		}
		if state == "failed" || state == "cancelled" {
			return fmt.Errorf("integration open unavailable: native MCP startup %s", state)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-changed:
		}
	}
	// One event-driven refresh after readiness when the initial catalog preceded
	// it. This is neither a retry loop nor a catalog-as-readiness assumption.
	if status.RuntimeStatus != "connected" {
		status, err = p.toolStatus(ctx, id)
		if err != nil {
			return err
		}
	}
	if status.RuntimeStatus != "connected" || status.Tools["sessionbus"] == nil {
		return errors.New("integration open unavailable: native Sessionbus tool not connected")
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-p.endpoint.ready:
	}
	return nil
}

// Server requests run separately from stdout dispatch. Human approval has no
// recipient here; failure is an unsupported client exchange, never user denial.
func (p *Wrapper) serverRequest(ctx context.Context, method string, raw json.RawMessage) (any, error) {
	switch method {
	case "item/tool/call":
		var call struct {
			ThreadID, TurnID, CallID, Tool string
			Namespace                      *string
			Arguments                      json.RawMessage
		}
		if json.Unmarshal(raw, &call) != nil || call.ThreadID == "" || call.TurnID == "" || call.CallID == "" {
			return nil, &appError{Code: -32602, Message: "invalid native dynamic tool request"}
		}
		p.mu.Lock()
		id, t := p.id, p.active
		matches := t != nil && t.id == call.TurnID
		p.mu.Unlock()
		if call.ThreadID != id || !matches {
			return nil, &appError{Code: -32602, Message: "native dynamic tool does not belong to current lane turn"}
		}
		name := strings.TrimPrefix(call.Tool, "tools.")
		qualified := name == "mcp__sessionbus__sessionbus" || name == "sessionbus" && call.Namespace != nil && *call.Namespace == "mcp__sessionbus"
		if !qualified {
			return nil, &appError{Code: -32601, Message: "unsupported native dynamic tool: " + call.Tool}
		}
		var result struct {
			Content           []struct{ Type, Text string }
			StructuredContent json.RawMessage
			IsError           bool
		}
		err := p.app.call(ctx, "mcpServer/tool/call", map[string]any{"threadId": id, "server": laneServer, "tool": "sessionbus", "arguments": call.Arguments, "_meta": map[string]string{"threadId": id}}, &result)
		if err != nil {
			return nil, err
		}
		items := []map[string]string{}
		for _, part := range result.Content {
			if part.Type != "text" {
				return nil, &appError{Code: -32603, Message: "unsupported Sessionbus tool content type"}
			}
			items = append(items, map[string]string{"type": "inputText", "text": part.Text})
		}
		if len(items) == 0 && len(result.StructuredContent) > 0 {
			items = append(items, map[string]string{"type": "inputText", "text": string(result.StructuredContent)})
		}
		return map[string]any{"contentItems": items, "success": !result.IsError}, nil
	case "item/commandExecution/requestApproval", "item/fileChange/requestApproval", "item/tool/requestUserInput", "item/permissions/requestApproval", "mcpServer/elicitation/request":
		return nil, &appError{Code: -32601, Message: "unsupported client exchange (no approval recipient): " + method}
	default:
		return nil, &appError{Code: -32601, Message: "unsupported client exchange: " + method}
	}
}
