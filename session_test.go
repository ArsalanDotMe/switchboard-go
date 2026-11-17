package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestN8nThreadLabelsBecomeStableIsolatedSessions(t *testing.T) {
	var sessions, agents []string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sessions = append(sessions, r.Header.Get("x-opencode-session"))
		agents = append(agents, r.UserAgent())
		w.Write([]byte(`{}`))
	}))
	defer upstream.Close()
	app := newApp(Config{UpstreamBaseURL: upstream.URL})
	for _, thread := range []string{"thread-a", "thread-b", "thread-a"} {
		r := httptest.NewRequest("POST", "/v1/chat/completions", nil)
		r.Header.Set("x-n8n-feature", "instance-ai")
		r.Header.Set("x-n8n-thread-id", thread)
		r.Header.Set("x-n8n-version", "2.47.7")
		r.Header.Set("User-Agent", "ai/7.0.150")
		resp, err := app.doUpstream(r.Context(), r, []byte(`{"messages":[]}`), "test", APIStyleOpenAI)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
	}
	if sessions[0] == "" || sessions[0] != sessions[2] || sessions[0] == sessions[1] {
		t.Fatalf("not isolated/stable: %v", sessions)
	}
	for _, ua := range agents {
		if ua != "switchboard-go/1.4.0 n8n-assistant/2.47.7" {
			t.Fatalf("untruthful/generic client: %s", ua)
		}
	}
}

func TestN8nVerificationGetsUniqueBoundedSession(t *testing.T) {
	var sessions []string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sessions = append(sessions, r.Header.Get("x-opencode-session"))
		io.Copy(io.Discard, r.Body)
		w.Write([]byte(`{}`))
	}))
	defer upstream.Close()
	app := newApp(Config{UpstreamBaseURL: upstream.URL})
	for i := 0; i < 2; i++ {
		r := httptest.NewRequest("POST", "/v1/chat/completions", nil)
		r.Header.Set("User-Agent", "ai/7.0.150 ai-sdk-provider-utils/5.0.67 node.js/26")
		resp, err := app.doUpstream(r.Context(), r, []byte(`{"model":"deepseek-v4.1-flash","max_tokens":16,"messages":[{"role":"user","content":"Reply with OK."}]}`), "test", APIStyleOpenAI)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
	}
	if !strings.HasPrefix(sessions[0], "switchboard-verification-") || sessions[0] == sessions[1] {
		t.Fatalf("invalid probe sessions: %v", sessions)
	}
}
