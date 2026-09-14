// SPDX-License-Identifier: MIT
package interactive

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

func inspectStartupProcess(pid int) (startupProcess, error) {
	body, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return startupProcess{}, err
	}
	end := strings.LastIndexByte(string(body), ')')
	if end < 0 {
		return startupProcess{}, errors.New("malformed native process stat")
	}
	fields := strings.Fields(string(body[end+1:]))
	if len(fields) <= 19 || fields[0] == "Z" || fields[0] == "X" {
		return startupProcess{}, errors.New("native process is not live")
	}
	return startupProcess{start: fields[19], generation: fields[19]}, nil
}
