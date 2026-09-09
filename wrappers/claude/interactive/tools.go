// SPDX-License-Identifier: MIT
package interactive

import (
	"context"
	"encoding/json"
	"github.com/antst/sessionbus-peers/wrappers/mcp"
)

func Tool() any { return mcp.Tool() }
func CallTool(ctx context.Context, owner interface {
	Action(context.Context, string, json.RawMessage) (json.RawMessage, error)
}, raw json.RawMessage) (json.RawMessage, error) {
	return mcp.CallTool(ctx, owner, raw)
}
