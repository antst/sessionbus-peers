// SPDX-License-Identifier: MIT

package interactive

import (
	"context"
	"encoding/json"
	"io"
	"net"

	kit "github.com/antst/sessionbus/bus/sdk/go"
)

type Recipient struct {
	SessionID, Socket string
	Context           context.Context
}
type Dial func(context.Context, string, string) (net.Conn, error)

func DeliverNative(captured Recipient, request kit.DeliveryRequest, dial Dial) (kit.DeliveryReceipt, error) {
	rejected := func(reason string) (kit.DeliveryReceipt, error) {
		return kit.DeliveryReceipt{Disposition: "rejected", Reason: reason}, nil
	}
	if captured.Context.Err() != nil {
		return rejected("identity_unavailable_before_submission")
	}
	if captured.Socket == "" {
		return rejected("native_socket_missing_before_submission")
	}
	from := request.From.Name
	if from == "" {
		from = request.From.SessionID
	}
	frame, err := json.Marshal(map[string]any{"msgV": 1, "msg_id": request.MessageID, "type": "user", "priority": "next", "from": from, "session_id": captured.SessionID, "message": map[string]string{"role": "user", "content": request.Body}})
	if err != nil {
		return kit.DeliveryReceipt{}, err
	}
	conn, err := dial(captured.Context, "unix", captured.Socket)
	if err != nil {
		return rejected("native_connect_failed_before_submission")
	}
	defer conn.Close()
	stop := context.AfterFunc(captured.Context, func() { _ = conn.Close() })
	defer stop()
	if captured.Context.Err() != nil {
		return rejected("identity_unavailable_before_submission")
	}
	// Once Write is attempted, an error/short write cannot prove non-submission.
	frame = append(frame, '\n')
	n, err := conn.Write(frame)
	if err == nil && n != len(frame) {
		err = io.ErrShortWrite
	}
	if err != nil {
		return kit.DeliveryReceipt{}, &kit.ProtocolError{Code: -32603, Message: "internal", Data: json.RawMessage(`"uncertain_submission"`)}
	}
	return kit.DeliveryReceipt{Disposition: "written"}, nil
}
