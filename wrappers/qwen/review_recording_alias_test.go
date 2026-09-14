// SPDX-License-Identifier: MIT
package qwen

import "testing"

func TestReviewNativeRecordingAliasesCannotBypassRequiredRecording(t *testing.T) {
	for _, flag := range []string{"--no-chat-recording", "--chatRecording=false"} {
		t.Run(flag, func(t *testing.T) {
			if _, err := InteractivePlan([]string{flag}, nil); err == nil {
				t.Fatalf("native recording-disable alias accepted: %s", flag)
			}
		})
	}
}
