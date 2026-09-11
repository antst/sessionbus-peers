// SPDX-License-Identifier: MIT
package opencode

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	kit "github.com/antst/sessionbus/bus/sdk/go"
)

func TestLegacyToolReadinessRequiresExactInventory(t *testing.T) {
	for _, tc := range []struct {
		body string
		ok   bool
	}{
		{`["sessionbus"]`, true}, {`["other","sessionbus"]`, true},
		{`null`, false}, {`[]`, false}, {`["prefix_sessionbus"]`, false}, {`{}`, false}, {`[null]`, false},
	} {
		t.Run(tc.body, func(t *testing.T) {
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/experimental/tool/ids" {
					t.Errorf("unexpected discovery route %s", r.URL.Path)
				}
				_, _ = io.WriteString(w, tc.body)
			}))
			defer s.Close()
			c := newLaneHTTP(s.URL, "/work", "u", "p")
			if err := c.ready(context.Background()); (err == nil) != tc.ok {
				t.Fatalf("ready=%v, want success=%v", err, tc.ok)
			}
		})
	}
}
func TestLegacyDeleteRequiresTrueJSONConfirmation(t *testing.T) {
	for _, tc := range []struct {
		body string
		ok   bool
	}{
		{`true`, true}, {" \ntrue\n", true}, {`false`, false}, {`null`, false}, {`"true"`, false}, {`true false`, false},
	} {
		t.Run(tc.body, func(t *testing.T) {
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "DELETE" || r.URL.Path != "/session/ses_test" {
					t.Errorf("unexpected deletion %s %s", r.Method, r.URL.Path)
				}
				_, _ = io.WriteString(w, tc.body)
			}))
			defer s.Close()
			c := newLaneHTTP(s.URL, "/work", "u", "p")
			if err := c.remove(context.Background(), "ses_test"); (err == nil) != tc.ok {
				t.Fatalf("delete=%v, want success=%v", err, tc.ok)
			}
		})
	}
}
func TestLegacyWorkerExplicitForgetAcceptsNativeJSON(t *testing.T) {
	for _, forget := range []bool{false, true} {
		t.Run(map[bool]string{false: "ordinary", true: "forget"}[forget], func(t *testing.T) {
			f := newWorkerFixture(t)
			f.call(t, "session.close", kit.SessionCloseRequest{SessionID: "ses_native@local", Forget: forget}, nil)
		})
	}
}
