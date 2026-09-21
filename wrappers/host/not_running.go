// SPDX-License-Identifier: MIT
package host

import (
	kit "github.com/antst/sessionbus/bus/sdk/go"
	"github.com/antst/sessionbus/bus/sdk/go/protocol"
)

// NotRunning reports that a delivery reached a product only after its native
// turn stopped accepting input. Callers may retry the original, unmodified
// delivery as a new managed Run because no native or local queue submission was
// attempted before this error.
func NotRunning() error {
	return &kit.ProtocolError{Code: protocol.NotRunning, Message: "not_running"}
}
