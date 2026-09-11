// SPDX-License-Identifier: MIT
package opencodefamily

import (
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"
)

// Native Identifier.ascending encodes 48 low bits of milliseconds*4096+counter
// plus fourteen base62 random bytes. This owns only this lane's ordering, not
// the native process's counter or other clients. Sessions remain native IDs.
type messageIDs struct {
	mu          sync.Mutex
	lastTime    int64
	lastPrefix  uint64
	counter     uint16
	initialized bool
	now         func() int64 // nil uses the wall clock; test seam only
	entropy     io.Reader    // nil uses crypto/rand; test seam only
}

func (g *messageIDs) next() (string, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	now := time.Now().UnixMilli()
	if g.now != nil {
		now = g.now()
	}
	if now < 0 || (g.initialized && now < g.lastTime) {
		return "", errors.New("Kilo message clock moved backward")
	}
	counter := uint16(1)
	if g.initialized && now == g.lastTime {
		if g.counter == 4095 {
			return "", errors.New("Kilo message counter exhausted in current millisecond")
		}
		counter = g.counter + 1
	}
	prefix := (uint64(now)<<12 | uint64(counter)) & ((1 << 48) - 1)
	if g.initialized && prefix <= g.lastPrefix {
		return "", errors.New("Kilo encoded message prefix did not increase")
	}
	random := g.entropy
	if random == nil {
		random = rand.Reader
	}
	var suffix [14]byte
	if _, err := io.ReadFull(random, suffix[:]); err != nil {
		return "", err
	}
	const alphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	for i, value := range suffix {
		suffix[i] = alphabet[int(value)%len(alphabet)]
	}
	g.lastTime, g.counter, g.lastPrefix, g.initialized = now, counter, prefix, true
	return fmt.Sprintf("msg_%012x%s", prefix, suffix[:]), nil
}

func (p *Wrapper) nextMessageID() (string, error) {
	if p.kind == kiloNative {
		return p.messageIDs.next()
	}
	return randomMessageID()
}

func (p *Wrapper) stagedMessageID() string {
	if p.kind == kiloNative {
		return "msg_00000000000000000000000000"
	}
	return "msg_00000000000000000000000000000000"
}
