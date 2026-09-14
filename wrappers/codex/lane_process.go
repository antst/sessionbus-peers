// SPDX-License-Identifier: MIT
package codex

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
)

// nativeChild owns only its direct process. Pipe reads outlive command.Wait.
type nativeChild struct {
	command *exec.Cmd
	done    chan struct{}
	err     error
}

func startNative(command *exec.Cmd) (*nativeChild, *os.File, *os.File, error) {
	stdin, input, err := os.Pipe()
	if err != nil {
		return nil, nil, nil, err
	}
	output, stdout, err := os.Pipe()
	if err != nil {
		_ = stdin.Close()
		_ = input.Close()
		return nil, nil, nil, err
	}
	command.Stdin, command.Stdout = stdin, stdout
	err = command.Start()
	_ = stdin.Close()
	_ = stdout.Close()
	if err != nil {
		_ = input.Close()
		_ = output.Close()
		return nil, nil, nil, err
	}
	c := &nativeChild{command: command, done: make(chan struct{})}
	go func() { c.err = command.Wait(); close(c.done) }()
	return c, input, output, nil
}
func (c *nativeChild) Done() <-chan struct{} { return c.done }
func (c *nativeChild) Wait() error           { <-c.done; return c.err }
func (c *nativeChild) abort()                { _ = c.command.Process.Kill() }

func (p *Wrapper) startLifetime(ctx context.Context) (func() bool, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.ctx != nil || p.closing {
		return nil, errors.New("Codex worker already opened or closed")
	}
	p.ctx, p.cancel = context.WithCancel(context.WithoutCancel(ctx))
	return context.AfterFunc(ctx, func() {
		p.mu.Lock()
		defer p.mu.Unlock()
		if !p.opened {
			p.cancel()
		}
	}), nil
}
func (p *Wrapper) transportEnd(err error) {
	p.mu.Lock()
	closing := p.closing
	p.mu.Unlock()
	if closing {
		return
	}
	p.fail(err)
}
func (p *Wrapper) nativeFailure(err error) {
	if errors.Is(err, io.EOF) {
		p.transportEnd(err)
	} else {
		p.fail(err)
	}
}
