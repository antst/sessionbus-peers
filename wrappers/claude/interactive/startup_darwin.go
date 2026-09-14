// SPDX-License-Identifier: MIT
package interactive

import (
	"errors"
	"fmt"
	"golang.org/x/sys/unix"
	"time"
)

func inspectStartupProcess(pid int) (startupProcess, error) {
	value, err := unix.SysctlKinfoProc("kern.proc.pid", pid)
	if err != nil {
		return startupProcess{}, err
	}
	if value == nil || int(value.Proc.P_pid) != pid || value.Proc.P_stat == 5 {
		return startupProcess{}, errors.New("native process is not live")
	}
	sec, usec := value.Proc.P_starttime.Sec, value.Proc.P_starttime.Usec
	return startupProcess{start: time.Unix(sec, int64(usec)*int64(time.Microsecond)).UTC().Format("Mon Jan _2 15:04:05 2006"), generation: fmt.Sprintf("%d:%06d", sec, usec), startedMillis: sec*1000 + int64(usec)/1000}, nil
}
