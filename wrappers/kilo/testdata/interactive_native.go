// SPDX-License-Identifier: MIT
//go:build ignore

package main

import (
	"encoding/json"
	"net"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	signals := make(chan os.Signal, 2)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	conn, err := net.Dial("unix", os.Getenv("KILO_TEST_SOCKET"))
	if err != nil {
		panic(err)
	}
	defer conn.Close()
	cwd, _ := os.Getwd()
	var launch any
	if err := json.Unmarshal([]byte(os.Getenv("SESSIONBUS_KILO_LAUNCH")), &launch); err != nil {
		panic(err)
	}
	encode := json.NewEncoder(conn)
	if err := encode.Encode(map[string]any{"pid": os.Getpid(), "ppid": os.Getppid(), "cwd": cwd, "args": os.Args[1:], "launch": launch,
		"password": os.Getenv("KILO_SERVER_PASSWORD"), "username": os.Getenv("KILO_SERVER_USERNAME"), "old_id": os.Getenv("SESSIONBUS_SESSION_ID"), "old_launch": os.Getenv("SESSIONBUS_OPENCODE_LAUNCH"),
		"daemon": os.Getenv("KILO_NO_DAEMON"), "parent": os.Getenv("KILO_PARENT_PID"), "resource": os.Getenv("KILO_TREE_SITTER_WASM_DIR")}); err != nil {
		panic(err)
	}
	switch os.Getenv("KILO_TEST_MODE") {
	case "exit":
		return
	case "error":
		os.Exit(23)
	}
	sig := <-signals
	if sig == os.Interrupt {
		os.Exit(130)
	}
	if err := encode.Encode(map[string]string{"event": "term"}); err != nil {
		panic(err)
	}
	var release bool
	if err := json.NewDecoder(conn).Decode(&release); err != nil || !release {
		panic("cleanup release missing")
	}
	os.Exit(143)
}
