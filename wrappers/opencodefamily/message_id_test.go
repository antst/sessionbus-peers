// SPDX-License-Identifier: MIT
package opencodefamily

import (
	"bytes"
	"errors"
	"io"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

func TestKiloMessageIDsNativeShapeAndOwnOrder(t *testing.T) {
	now := int64(123456)
	g := messageIDs{now: func() int64 { return now }}
	previous := ""
	shape := regexp.MustCompile("^msg_[0-9a-f]{12}[0-9A-Za-z]{14}$")
	for counter := uint64(1); counter <= 4095; counter++ {
		id, err := g.next()
		if err != nil || !shape.MatchString(id) || id <= previous {
			t.Fatalf("native ID=%q previous=%q error=%v", id, previous, err)
		}
		prefix, _ := strconv.ParseUint(id[4:16], 16, 64)
		if prefix != uint64(now)*4096+counter {
			t.Fatalf("native prefix=%x counter=%d", prefix, counter)
		}
		previous = id
	}
	if _, err := g.next(); err == nil || !strings.Contains(err.Error(), "counter exhausted") {
		t.Fatalf("same-ms exhaustion=%v", err)
	}
	now++
	id, err := g.next()
	if err != nil || id <= previous || g.counter != 1 {
		t.Fatalf("new native millisecond=%s/%v", id, err)
	}
}

func TestKiloMessageIDsRejectClockAndEncodedWrapWithoutStateChange(t *testing.T) {
	for _, tc := range []struct {
		name          string
		initial, next int64
	}{
		{"rollback", 1000, 999}, {"encoded wrap", (1 << 36) - 1, 1 << 36},
	} {
		t.Run(tc.name, func(t *testing.T) {
			now := tc.initial
			g := messageIDs{now: func() int64 { return now }}
			if _, err := g.next(); err != nil {
				t.Fatal(err)
			}
			last, prefix, counter := g.lastTime, g.lastPrefix, g.counter
			now = tc.next
			if _, err := g.next(); err == nil {
				t.Fatal("unorderable ID accepted")
			}
			if g.lastTime != last || g.lastPrefix != prefix || g.counter != counter {
				t.Fatal("rejected generation changed its ordering state")
			}
		})
	}
	g := messageIDs{now: func() int64 { return 1000 }, entropy: bytes.NewReader([]byte{1})}
	if _, err := g.next(); !errors.Is(err, io.ErrUnexpectedEOF) || g.initialized {
		t.Fatalf("entropy failure changed state: %v", err)
	}
}
