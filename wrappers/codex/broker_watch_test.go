// SPDX-License-Identifier: MIT

package codex

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestBrokerParentWatchAcrossExecAndExit(t *testing.T) {
	if os.Getenv("SESSIONBUS_WATCH_TEST") != "" {
		t.Skip("fixture")
	}
	gateRead, gateWrite, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer gateRead.Close()
	defer gateWrite.Close()
	cmd := exec.Command(os.Args[0], "-test.run=^TestBrokerWatchFixture$")
	cmd.Env = append(os.Environ(), "SESSIONBUS_WATCH_TEST=parent")
	cmd.ExtraFiles = []*os.File{gateRead}
	output, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stderr = os.Stderr
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer cmd.Process.Kill()
	lines := make(chan string, 4)
	go func() {
		s := bufio.NewScanner(output)
		for s.Scan() {
			lines <- s.Text()
		}
		close(lines)
	}()
	next := func() string {
		t.Helper()
		select {
		case line := <-lines:
			return line
		case <-time.After(5 * time.Second):
			t.Fatal("watch fixture did not progress")
		}
		return ""
	}
	if got := next(); got != "ARMED" {
		t.Fatal(got)
	}
	if got := next(); got != "EXEC" {
		t.Fatal(got)
	}
	if _, err = gateWrite.Write([]byte{1}); err != nil {
		t.Fatal(err)
	}
	if got := next(); got != "EXIT" {
		t.Fatal(got)
	}
	if err = cmd.Wait(); err != nil {
		t.Fatal(err)
	}
}

func TestBrokerWatchFixture(t *testing.T) {
	switch os.Getenv("SESSIONBUS_WATCH_TEST") {
	case "parent":
		r, w, err := os.Pipe()
		if err != nil {
			os.Exit(2)
		}
		child := exec.Command(os.Args[0], "-test.run=^TestBrokerWatchFixture$")
		child.Env = append(os.Environ(), "SESSIONBUS_WATCH_TEST=watcher", "SESSIONBUS_WATCH_PID="+strconv.Itoa(os.Getpid()))
		child.Stdout = os.Stdout
		child.Stderr = os.Stderr
		child.ExtraFiles = []*os.File{w}
		if child.Start() != nil {
			os.Exit(3)
		}
		_ = w.Close()
		if _, err = io.ReadFull(r, make([]byte, 1)); err != nil {
			os.Exit(4)
		}
		_ = r.Close()
		_ = child.Process.Release()
		env := os.Environ()
		for i, value := range env {
			if strings.HasPrefix(value, "SESSIONBUS_WATCH_TEST=") {
				env[i] = "SESSIONBUS_WATCH_TEST=exec"
			}
		}
		if syscall.Exec(os.Args[0], []string{os.Args[0], "-test.run=^TestBrokerWatchFixture$"}, env) != nil {
			os.Exit(5)
		}
	case "exec":
		fmt.Println("EXEC")
		gate := os.NewFile(3, "gate")
		if _, err := io.ReadFull(gate, make([]byte, 1)); err != nil {
			os.Exit(6)
		}
		os.Exit(0)
	case "watcher":
		pid, _ := strconv.Atoi(os.Getenv("SESSIONBUS_WATCH_PID"))
		done, stop, err := watchBrokerParent(pid)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(7)
		}
		defer stop()
		fmt.Println("ARMED")
		ready := os.NewFile(3, "ready")
		_, _ = ready.Write([]byte{1})
		_ = ready.Close()
		if err, ok := <-done; !ok || err == nil {
			os.Exit(8)
		}
		fmt.Println("EXIT")
		stop()
		os.Exit(0)
	}
}
func TestBrokerParentWatchRejectsWrongParent(t *testing.T) {
	if _, _, err := watchBrokerParent(os.Getpid()); err == nil {
		t.Fatal("accepted non-parent")
	}
}
func TestBrokerParentWatchCancellation(t *testing.T) {
	done, stop, err := watchBrokerParent(os.Getppid())
	if err != nil {
		t.Fatal(err)
	}
	stop()
	stop()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("cancelled watch did not exit")
	}
}
