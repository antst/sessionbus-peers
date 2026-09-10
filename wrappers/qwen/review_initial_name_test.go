// SPDX-License-Identifier: MIT
package qwen

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestReviewInitialNameCannotSelectNativeAutoRename(t *testing.T) {
	b, _, publish := ownerFixture(t, "--auto")
	watch, e := newInteractiveWatch(b.parent)
	must(t, e)
	defer watch.close()
	input := filepath.Join(b.launch.Directory, "input.jsonl")
	must(t, watch.add(input))
	publish()
	b.Initialized()
	waitFileCondition(t, watch, func() bool { data, err := os.ReadFile(input); return err == nil && len(data) > 0 })
	data, e := os.ReadFile(input)
	must(t, e)
	var cmd struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	must(t, json.Unmarshal(data, &cmd))
	if cmd.Text != "/rename -- --auto" {
		t.Fatalf("initial name became native rename options: %q", cmd.Text)
	}
}
