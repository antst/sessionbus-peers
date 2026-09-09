// SPDX-License-Identifier: MIT
package interactive

import (
	"context"
	"encoding/json"
	"github.com/antst/sessionbus-peers/wrappers/mcp"
	"io"
)

const HiddenTool = "native_identity_event"

type MCPOwner interface {
	BeginReport(json.RawMessage) (<-chan error, error)
	Action(context.Context, string, json.RawMessage) (json.RawMessage, error)
	End()
}

func Serve(owner MCPOwner, input io.ReadCloser, output io.Writer) error {
	return mcp.ServeSessionbus(owner, input, output, mcp.ReportHandler{Name: HiddenTool, Begin: owner.BeginReport})
}
