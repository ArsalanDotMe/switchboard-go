package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSessionCompatibilityPreservesClientsAndRejectsGuessing(t *testing.T) {
	cases := []struct{ name, path, feature, thread, version, session, ua, body string }{
		{"existing-session", "/v1/chat/completions", "instance-ai", "new", "2.47.7", "existing-session", "hermes/0.21.6", `{"messages":[]}`},
		{"unlabelled-conversation", "/v1/chat/completions", "", "", "", "", "ai/7.0.150", `{"model":"test","messages":[{"role":"user","content":"hello"}]}`},
		{"not-a-probe", "/v1/chat/completions", "", "", "", "", "ai/7.0.150", `{"model":"test","max_tokens":17,"messages":[{"role":"user","content":"Reply with OK."}]}`},
		{"tools-not-probe", "/v1/chat/completions", "", "", "", "", "ai/7.0.150", `{"model":"test","max_tokens":16,"tools":[],"messages":[{"role":"user","content":"Reply with OK."}]}`},
		{"invalid-label", "/v1/chat/completions", "instance-ai", "bad thread", "2.47.7", "", "ai/7.0.150", `{}`},
		{"missing-thread", "/v1/chat/completions", "instance-ai", "", "2.47.7", "", "ai/7.0.150", `{}`},
		{"not-chat", "/v1/models", "instance-ai", "thread-a", "2.47.7", "", "ai/7.0.150", `{}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("x-opencode-session") != c.session || r.UserAgent() != c.ua {
					t.Error("existing headers changed or ambiguous session guessed")
				}
				b, _ := io.ReadAll(r.Body)
				if string(b) != c.body || r.Header.Get("Authorization") != "Bearer upstream-test" {
					t.Error("payload or authorization changed")
				}
				w.Header().Set("Content-Type", "text/event-stream")
				io.WriteString(w, "data: test\n\ndata: [DONE]\n\n")
			}))
			defer upstream.Close()
			app := newApp(Config{UpstreamBaseURL: upstream.URL, ProxyAPIKey: "local-test", UpstreamAPIKeys: []string{"upstream-test"}, MaxRequestBodyBytes: 1024})
			r := httptest.NewRequest("POST", c.path, strings.NewReader(c.body))
			r.Header.Set("Authorization", "Bearer local-test")
			r.Header.Set("User-Agent", c.ua)
			r.Header.Set("x-n8n-feature", c.feature)
			r.Header.Set("x-n8n-thread-id", c.thread)
			r.Header.Set("x-n8n-version", c.version)
			r.Header.Set("x-opencode-session", c.session)
			rec := httptest.NewRecorder()
			app.ServeHTTP(rec, r)
			if rec.Code != 200 || rec.Body.String() != "data: test\n\ndata: [DONE]\n\n" {
				t.Error("stream changed")
			}
		})
	}
}
