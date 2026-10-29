package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestKeyManagerNoCurrentWhenExhausted(t *testing.T) {
	km := NewKeyManager([]string{"a"}, time.Hour)
	km.MarkExhausted(0)
	if _, _, ok := km.Current(); ok {
		t.Fatal("expected no current key")
	}
}

func TestBearerTokenCaseInsensitive(t *testing.T) {
	if got := bearerToken("bearer abc"); got != "abc" {
		t.Fatalf("got %q", got)
	}
	if got := bearerToken("BEARER abc"); got != "abc" {
		t.Fatalf("got %q", got)
	}
}

func TestIsQuota429(t *testing.T) {
	resp := &http.Response{StatusCode: 429, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"error":{"message":"quota exceeded","code":"insufficient_quota"}}`))}
	if !isQuota429(resp) {
		t.Fatal("expected quota 429")
	}
}

func TestIsQuota429NotGenericRateLimit(t *testing.T) {
	resp := &http.Response{StatusCode: 429, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"error":{"message":"try again","code":"rate_limit_exceeded"}}`))}
	if isQuota429(resp) {
		t.Fatal("expected generic rate_limit_exceeded not to count as quota")
	}
}

func TestIsQuota429RestoresBody(t *testing.T) {
	const body = `{"error":{"message":"quota exceeded","code":"insufficient_quota"}}`
	resp := &http.Response{StatusCode: 429, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}
	_ = isQuota429(resp)
	got, _ := io.ReadAll(resp.Body)
	if string(got) != body {
		t.Fatalf("body not restored: %q", string(got))
	}
}

func TestIsQuota429AnthropicUsageLimit(t *testing.T) {
	resp := &http.Response{StatusCode: 429, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"type":"error","error":{"type":"rate_limit_error","message":"credit balance is too low"}}`))}
	if !isQuota429(resp) {
		t.Fatal("expected anthropic credit balance 429 to count as quota")
	}
}

func TestIsQuota429AnthropicGenericRateLimit(t *testing.T) {
	resp := &http.Response{StatusCode: 429, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"type":"error","error":{"type":"rate_limit_error","message":"rate limit reached"}}`))}
	if isQuota429(resp) {
		t.Fatal("expected generic anthropic rate limit not to count as quota")
	}
}

func TestRequestTooLargeReturns413(t *testing.T) {
	app := newApp(Config{ProxyAPIKey: "p", UpstreamAPIKeys: []string{"u"}, MaxRequestBodyBytes: 4})
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader([]byte("12345")))
	req.Header.Set("Authorization", "Bearer p")
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("got %d", rec.Code)
	}
}

func TestRootOpenAIPathProxies(t *testing.T) {
	var gotPath, gotAuth string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer upstream.Close()

	app := newApp(Config{ProxyAPIKey: "p", UpstreamAPIKeys: []string{"u"}, UpstreamBaseURL: upstream.URL, MaxRequestBodyBytes: 1024})
	req := httptest.NewRequest(http.MethodPost, "/chat/completions", strings.NewReader(`{"model":"glm-5.1","messages":[]}`))
	req.Header.Set("Authorization", "Bearer p")
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d body %s", rec.Code, rec.Body.String())
	}
	if gotPath != "/chat/completions" || gotAuth != "Bearer u" {
		t.Fatalf("unexpected upstream path=%q auth=%q", gotPath, gotAuth)
	}
}

func TestDoUpstreamSetsDefaultUserAgent(t *testing.T) {
	var gotUA string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	app := newApp(Config{ProxyAPIKey: "p", UpstreamAPIKeys: []string{"u"}, UpstreamBaseURL: upstream.URL})
	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	resp, err := app.doUpstream(req.Context(), req, nil, "u", APIStyleOpenAI)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if gotUA != "OpenAI/Python 1.0.0" {
		t.Fatalf("got user-agent %q", gotUA)
	}
}

func TestDoUpstreamAnthropicSetsHeaders(t *testing.T) {
	var gotPath, gotKey, gotAuth, gotVersion, gotUA string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotKey = r.Header.Get("x-api-key")
		gotAuth = r.Header.Get("Authorization")
		gotVersion = r.Header.Get("anthropic-version")
		gotUA = r.Header.Get("User-Agent")
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	app := newApp(Config{ProxyAPIKey: "p", UpstreamAPIKeys: []string{"u"}, UpstreamBaseURL: upstream.URL})
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{"model":"minimax-m3","messages":[]}`))
	req.Header.Set("x-api-key", "p")
	req.Header.Set("anthropic-version", "2023-06-01")
	resp, err := app.doUpstream(req.Context(), req, []byte(`{"model":"minimax-m3","messages":[]}`), "u", APIStyleAnthropic)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if gotPath != "/messages" || gotKey != "u" || gotAuth != "" || gotVersion != "2023-06-01" || gotUA != "anthropic-sdk-go/1.0.0" {
		t.Fatalf("unexpected upstream request path=%q key=%q auth=%q version=%q ua=%q", gotPath, gotKey, gotAuth, gotVersion, gotUA)
	}
}

func TestProxyAnthropicMessagesCyclesKeys(t *testing.T) {
	var gotKeys []string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/messages" {
			http.NotFound(w, r)
			return
		}
		gotKeys = append(gotKeys, r.Header.Get("x-api-key"))
		if r.Header.Get("x-api-key") == "bad" {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"type":"error","error":{"type":"rate_limit_error","message":"usage limit exhausted"}}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"msg_1","type":"message","content":[]}`))
	}))
	defer upstream.Close()

	app := newApp(Config{ProxyAPIKey: "p", UpstreamAPIKeys: []string{"bad", "good"}, UpstreamBaseURL: upstream.URL, MaxRequestBodyBytes: 1024})
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{"model":"minimax-m3","messages":[]}`))
	req.Header.Set("x-api-key", "p")
	req.Header.Set("anthropic-version", "2023-06-01")
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d body %s", rec.Code, rec.Body.String())
	}
	if strings.Join(gotKeys, ",") != "bad,good" {
		t.Fatalf("unexpected keys: %v", gotKeys)
	}
}

func TestAuthFailureReturnsOpenAIJSON(t *testing.T) {
	app := newApp(Config{ProxyAPIKey: "p", UpstreamAPIKeys: []string{"u"}, UpstreamBaseURL: "http://example.com", MaxRequestBodyBytes: 1})
	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("got %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("ct %q", ct)
	}
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["error"] == nil {
		t.Fatal("missing error object")
	}
}

func TestAuthOKConstantTimeCompare(t *testing.T) {
	app := newApp(Config{ProxyAPIKey: "p", UpstreamAPIKeys: []string{"u"}, UpstreamBaseURL: "http://example.com", MaxRequestBodyBytes: 1})
	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req.Header.Set("Authorization", "Bearer p")
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code == http.StatusUnauthorized {
		t.Fatalf("expected non-401 for correct bearer, got %d", rec.Code)
	}
	req2 := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req2.Header.Set("Authorization", "Bearer wrong")
	rec2 := httptest.NewRecorder()
	app.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for wrong bearer, got %d", rec2.Code)
	}
	req3 := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req3.Header.Set("x-api-key", "p")
	rec3 := httptest.NewRecorder()
	app.ServeHTTP(rec3, req3)
	if rec3.Code == http.StatusUnauthorized {
		t.Fatalf("expected non-401 for correct x-api-key, got %d", rec3.Code)
	}
}

func TestAdminResetKeyUnExhausts(t *testing.T) {
	app := newApp(Config{ProxyAPIKey: "p", UpstreamAPIKeys: []string{"a", "b"}, UpstreamBaseURL: "http://example.com", MaxRequestBodyBytes: 1})
	app.keys.MarkExhausted(0)
	app.keys.MarkExhausted(1)
	body := bytes.NewReader([]byte(`{"index":0}`))
	req := httptest.NewRequest(http.MethodPost, "/admin/reset-key", body)
	req.Header.Set("Authorization", "Bearer p")
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d body %s", rec.Code, rec.Body.String())
	}
	var st StatusResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &st); err != nil {
		t.Fatal(err)
	}
	if st.Keys[0].State != string(KeyAvailable) {
		t.Fatalf("key 0 not available: %+v", st.Keys[0])
	}
	if st.Keys[1].State != string(KeyExhausted) {
		t.Fatalf("key 1 should still be exhausted: %+v", st.Keys[1])
	}
}

func TestAdminResetKeyRejectsBadIndex(t *testing.T) {
	app := newApp(Config{ProxyAPIKey: "p", UpstreamAPIKeys: []string{"a"}, UpstreamBaseURL: "http://example.com", MaxRequestBodyBytes: 1})
	body := bytes.NewReader([]byte(`{"index":99}`))
	req := httptest.NewRequest(http.MethodPost, "/admin/reset-key", body)
	req.Header.Set("Authorization", "Bearer p")
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("got %d", rec.Code)
	}
}

func TestAdminResetAllKeys(t *testing.T) {
	app := newApp(Config{ProxyAPIKey: "p", UpstreamAPIKeys: []string{"a", "b", "c"}, UpstreamBaseURL: "http://example.com", MaxRequestBodyBytes: 1})
	for i := range app.config.UpstreamAPIKeys {
		app.keys.MarkExhausted(i)
	}
	req := httptest.NewRequest(http.MethodPost, "/admin/reset-all-keys", nil)
	req.Header.Set("Authorization", "Bearer p")
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d", rec.Code)
	}
	var st StatusResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &st); err != nil {
		t.Fatal(err)
	}
	for i, k := range st.Keys {
		if k.State != string(KeyAvailable) {
			t.Fatalf("key %d not available: %+v", i, k)
		}
	}
}
func TestAuthFailureReturnsAnthropicJSON(t *testing.T) {
	app := newApp(Config{ProxyAPIKey: "p", UpstreamAPIKeys: []string{"u"}, UpstreamBaseURL: "http://example.com", MaxRequestBodyBytes: 1})
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	req.Header.Set("anthropic-version", "2023-06-01")
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("got %d", rec.Code)
	}
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	errObj, ok := payload["error"].(map[string]any)
	if payload["type"] != "error" || !ok || errObj["type"] != "authentication_error" {
		t.Fatalf("unexpected payload: %#v", payload)
	}
}

func TestRootAnthropicAuthFailureReturnsAnthropicJSON(t *testing.T) {
	app := newApp(Config{ProxyAPIKey: "p", UpstreamAPIKeys: []string{"u"}, UpstreamBaseURL: "http://example.com", MaxRequestBodyBytes: 1})
	req := httptest.NewRequest(http.MethodPost, "/messages", nil)
	req.Header.Set("anthropic-version", "2023-06-01")
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("got %d", rec.Code)
	}
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	errObj, ok := payload["error"].(map[string]any)
	if payload["type"] != "error" || !ok || errObj["type"] != "authentication_error" {
		t.Fatalf("unexpected payload: %#v", payload)
	}
}

func TestValidateConfigHelper(t *testing.T) {
	if err := validateConfig(Config{}); err == nil {
		t.Fatal("expected error")
	}
	if err := validateConfig(Config{ProxyAPIKey: "p", UpstreamAPIKeys: []string{"u"}, UpstreamBaseURL: "http://x", MaxRequestBodyBytes: 1}); err != nil {
		t.Fatal(err)
	}
}

func TestValidateKeysEndpoint(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models" {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("User-Agent") != "OpenAI/Python 1.0.0" {
			t.Fatalf("unexpected user-agent %q", r.Header.Get("User-Agent"))
		}
		if r.Header.Get("Authorization") == "Bearer good" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"data":[]}`))
			return
		}
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":{"message":"quota exceeded","code":"insufficient_quota"}}`))
	}))
	defer upstream.Close()
	app := newApp(Config{ProxyAPIKey: "p", UpstreamAPIKeys: []string{"good", "bad"}, UpstreamBaseURL: upstream.URL, MaxRequestBodyBytes: 1})
	req := httptest.NewRequest(http.MethodPost, "/admin/validate-keys", nil)
	req.Header.Set("Authorization", "Bearer p")
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d", rec.Code)
	}
	var out ValidateKeysResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Results) != 2 {
		t.Fatalf("got %d results", len(out.Results))
	}
	if out.Results[0].State != string(KeyAvailable) || out.Results[1].State != string(KeyExhausted) {
		t.Fatalf("unexpected results: %+v", out.Results)
	}
}

func TestReadyzUnauthenticated(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/models" {
			w.WriteHeader(http.StatusOK)
			return
		}
		http.NotFound(w, r)
	}))
	defer upstream.Close()
	app := newApp(Config{ProxyAPIKey: "p", UpstreamAPIKeys: []string{"good"}, UpstreamBaseURL: upstream.URL, MaxRequestBodyBytes: 1})
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d", rec.Code)
	}
}

func TestLoadConfigFromYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := []byte(`server:
  listen_addr: "127.0.0.1:9090"
  proxy_api_key: "yaml-proxy"
upstream:
  base_url: "https://example.com/v1"
  api_keys: ["k1", "k2"]
smtp:
  host: "smtp.example.com"
  port: 587
  tls: false
  starttls: true
limits:
  max_request_body_bytes: 1234
`)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SWITCHBOARD_GO_CONFIG", path)
	t.Setenv("PROXY_API_KEY", "env-proxy")
	t.Setenv("OPENCODE_GO_API_KEYS", "env1,env2")
	cfg, err := loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ListenAddr != "127.0.0.1:9090" || cfg.ProxyAPIKey != "env-proxy" || cfg.UpstreamBaseURL != "https://example.com/v1" || cfg.MaxRequestBodyBytes != 1234 {
		t.Fatalf("unexpected cfg: %+v", cfg)
	}
}

func TestLoadConfigExplicitInvalidPathErrors(t *testing.T) {
	t.Setenv("SWITCHBOARD_GO_CONFIG", "/does/not/exist.yaml")
	if _, err := loadConfig(); err == nil {
		t.Fatal("expected error")
	}
}

func TestLoadConfigExplicitInvalidYAMLErrors(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("server: ["), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SWITCHBOARD_GO_CONFIG", path)
	if _, err := loadConfig(); err == nil {
		t.Fatal("expected error")
	}
}

func TestEnvOverridesYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(`server: {listen_addr: "127.0.0.1:1", proxy_api_key: "yaml"}
upstream: {base_url: "https://yaml", api_keys: ["yaml1"]}
`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SWITCHBOARD_GO_CONFIG", path)
	t.Setenv("PROXY_API_KEY", "env")
	t.Setenv("OPENCODE_GO_API_KEYS", "e1,e2")
	t.Setenv("LISTEN_ADDR", "0.0.0.0:9999")
	t.Setenv("MAX_REQUEST_BODY_BYTES", "99")
	cfg, err := loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ListenAddr != "0.0.0.0:9999" || cfg.ProxyAPIKey != "env" || len(cfg.UpstreamAPIKeys) != 2 || cfg.MaxRequestBodyBytes != 99 {
		t.Fatalf("unexpected cfg: %+v", cfg)
	}
}

func TestKeyManagerRetryEligibleAfterCooldown(t *testing.T) {
	now := time.Unix(1000, 0).UTC()
	km := NewKeyManager([]string{"a"}, time.Minute)
	km.now = func() time.Time { return now }
	km.MarkExhausted(0)
	if _, _, ok := km.Current(); ok {
		t.Fatal("expected no eligible key immediately after exhaustion")
	}
	now = now.Add(59 * time.Second)
	if _, _, ok := km.Current(); ok {
		t.Fatal("expected key still in cooldown at 59s")
	}
	now = now.Add(2 * time.Second) // 61s since exhaustion
	idx, key, ok := km.Current()
	if !ok || idx != 0 || key != "a" {
		t.Fatalf("expected key eligible after cooldown, got idx=%d key=%q ok=%v", idx, key, ok)
	}
}

func TestKeyManagerZeroCooldownImmediatelyEligible(t *testing.T) {
	km := NewKeyManager([]string{"a"}, 0)
	km.MarkExhausted(0)
	idx, key, ok := km.Current()
	if !ok || idx != 0 || key != "a" {
		t.Fatalf("expected exhausted key immediately eligible with zero cooldown, got ok=%v", ok)
	}
}

func TestKeyManagerRetryAfterSeconds(t *testing.T) {
	now := time.Unix(2000, 0).UTC()
	km := NewKeyManager([]string{"a", "b"}, 2*time.Minute)
	km.now = func() time.Time { return now }
	km.MarkExhausted(0) // eligible at t=2120
	now = now.Add(30 * time.Second)
	km.MarkExhausted(1) // eligible at t=2150
	secs, ok := km.RetryAfterSeconds()
	if !ok || secs != 90 { // soonest 2120 - now 2030
		t.Fatalf("expected retry-after 90s, got %d ok=%v", secs, ok)
	}

	km2 := NewKeyManager([]string{"a"}, 0)
	km2.MarkExhausted(0)
	if _, ok := km2.RetryAfterSeconds(); ok {
		t.Fatal("expected no retry-after hint when cooldown is disabled")
	}
}

func TestKeyManagerReArmsNotificationsOnRecovery(t *testing.T) {
	km := NewKeyManager([]string{"a", "b"}, time.Minute)
	km.MarkExhausted(0)
	km.MarkExhausted(1)
	if !km.ShouldNotifyAllExhausted() {
		t.Fatal("expected first all-exhausted notification")
	}
	if km.ShouldNotifyAllExhausted() {
		t.Fatal("expected all-exhausted notification suppressed the second time")
	}
	if !km.ShouldNotifySwitch(0) {
		t.Fatal("expected switch notification for key 0")
	}
	// Recovery of a key must re-arm both the switch flag and the all-exhausted flag.
	km.MarkAvailable(0)
	if !km.ShouldNotifySwitch(0) {
		t.Fatal("expected switch notification re-armed after recovery")
	}
	km.MarkExhausted(0)
	if !km.ShouldNotifyAllExhausted() {
		t.Fatal("expected all-exhausted notification re-armed after a recovery round")
	}
}

func doProxyReq(app *App) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"glm-5.1","messages":[]}`))
	req.Header.Set("Authorization", "Bearer p")
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	return rec
}

func TestProxyRetriesExhaustedKeyAfterCooldown(t *testing.T) {
	var replenished atomic.Bool
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if replenished.Load() {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok":true}`))
			return
		}
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":{"message":"quota exceeded","code":"insufficient_quota"}}`))
	}))
	defer upstream.Close()

	app := newApp(Config{ProxyAPIKey: "p", UpstreamAPIKeys: []string{"k1"}, UpstreamBaseURL: upstream.URL, MaxRequestBodyBytes: 1024, RetryExhaustedAfter: time.Minute})
	now := time.Unix(5000, 0).UTC()
	app.keys.now = func() time.Time { return now }

	// First request: the only key quota-fails, so the proxy returns a local 429
	// carrying a Retry-After hint pointing at the next probe window.
	rec := doProxyReq(app)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d body %s", rec.Code, rec.Body.String())
	}
	if ra := rec.Header().Get("Retry-After"); ra != "60" {
		t.Fatalf("expected Retry-After 60, got %q", ra)
	}

	// Within the cooldown the proxy fast-fails without probing upstream again.
	rec = doProxyReq(app)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 during cooldown, got %d", rec.Code)
	}

	// Cooldown elapses and the account is replenished: the next real request acts
	// as a probe and succeeds.
	now = now.Add(2 * time.Minute)
	replenished.Store(true)
	rec = doProxyReq(app)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 after cooldown+replenish, got %d body %s", rec.Code, rec.Body.String())
	}
	st := app.keys.Status()
	if st.Keys[0].State != string(KeyAvailable) || !st.Keys[0].Eligible {
		t.Fatalf("expected key available and eligible after recovery, got %+v", st.Keys[0])
	}
	if st.RetryExhaustedAfterSeconds != 60 {
		t.Fatalf("expected retry_exhausted_after_seconds 60, got %d", st.RetryExhaustedAfterSeconds)
	}
}

func TestProxyZeroCooldownNoRetryAfterHeader(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":{"message":"quota exceeded","code":"insufficient_quota"}}`))
	}))
	defer upstream.Close()

	app := newApp(Config{ProxyAPIKey: "p", UpstreamAPIKeys: []string{"k1"}, UpstreamBaseURL: upstream.URL, MaxRequestBodyBytes: 1024, RetryExhaustedAfter: 0})
	rec := doProxyReq(app)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", rec.Code)
	}
	if ra := rec.Header().Get("Retry-After"); ra != "" {
		t.Fatalf("expected no Retry-After header with cooldown disabled, got %q", ra)
	}
}

func TestLoadConfigRetryExhaustedAfterParsedAndOverridden(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := []byte("server: {proxy_api_key: \"p\"}\nupstream: {base_url: \"https://x\", api_keys: [\"k\"], retry_exhausted_after: \"90s\"}\n")
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SWITCHBOARD_GO_CONFIG", path)
	cfg, err := loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.RetryExhaustedAfter != 90*time.Second {
		t.Fatalf("expected 90s from yaml, got %s", cfg.RetryExhaustedAfter)
	}
	t.Setenv("RETRY_EXHAUSTED_AFTER", "5m")
	cfg, err = loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.RetryExhaustedAfter != 5*time.Minute {
		t.Fatalf("expected env override to 5m, got %s", cfg.RetryExhaustedAfter)
	}
}

func TestLoadConfigRetryExhaustedAfterDefaultAndExplicitZero(t *testing.T) {
	dir := t.TempDir()
	omitted := filepath.Join(dir, "omitted.yaml")
	if err := os.WriteFile(omitted, []byte("server: {proxy_api_key: \"p\"}\nupstream: {base_url: \"https://x\", api_keys: [\"k\"]}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SWITCHBOARD_GO_CONFIG", omitted)
	cfg, err := loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.RetryExhaustedAfter != 5*time.Minute {
		t.Fatalf("expected default 5m when omitted, got %s", cfg.RetryExhaustedAfter)
	}

	zero := filepath.Join(dir, "zero.yaml")
	if err := os.WriteFile(zero, []byte("server: {proxy_api_key: \"p\"}\nupstream: {base_url: \"https://x\", api_keys: [\"k\"], retry_exhausted_after: \"0\"}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SWITCHBOARD_GO_CONFIG", zero)
	cfg, err = loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.RetryExhaustedAfter != 0 {
		t.Fatalf("expected explicit 0 to disable cooldown, got %s", cfg.RetryExhaustedAfter)
	}
}

func TestLoadConfigRejectsInvalidRetryExhaustedAfterEnv(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := []byte("server: {proxy_api_key: \"p\"}\nupstream: {base_url: \"https://x\", api_keys: [\"k\"]}\n")
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SWITCHBOARD_GO_CONFIG", path)
	t.Setenv("RETRY_EXHAUSTED_AFTER", "not-a-duration")
	if _, err := loadConfig(); err == nil || !strings.Contains(err.Error(), "RETRY_EXHAUSTED_AFTER") {
		t.Fatalf("expected RETRY_EXHAUSTED_AFTER validation error, got %v", err)
	}
}

func TestLoadConfigResponseHeaderTimeoutParsedAndOverridden(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := []byte("server: {proxy_api_key: \"p\"}\nupstream: {base_url: \"https://x\", api_keys: [\"k\"], response_header_timeout: \"90s\"}\n")
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SWITCHBOARD_GO_CONFIG", path)
	cfg, err := loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.UpstreamResponseHeaderTimeout != 90*time.Second {
		t.Fatalf("expected 90s from yaml, got %s", cfg.UpstreamResponseHeaderTimeout)
	}
	t.Setenv("UPSTREAM_RESPONSE_HEADER_TIMEOUT", "5m")
	cfg, err = loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.UpstreamResponseHeaderTimeout != 5*time.Minute {
		t.Fatalf("expected env override to 5m, got %s", cfg.UpstreamResponseHeaderTimeout)
	}
}

func TestLoadConfigResponseHeaderTimeoutDefaultAndExplicitZero(t *testing.T) {
	dir := t.TempDir()
	omitted := filepath.Join(dir, "omitted.yaml")
	if err := os.WriteFile(omitted, []byte("server: {proxy_api_key: \"p\"}\nupstream: {base_url: \"https://x\", api_keys: [\"k\"]}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SWITCHBOARD_GO_CONFIG", omitted)
	cfg, err := loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.UpstreamResponseHeaderTimeout != 30*time.Second {
		t.Fatalf("expected default 30s when omitted, got %s", cfg.UpstreamResponseHeaderTimeout)
	}

	zero := filepath.Join(dir, "zero.yaml")
	if err := os.WriteFile(zero, []byte("server: {proxy_api_key: \"p\"}\nupstream: {base_url: \"https://x\", api_keys: [\"k\"], response_header_timeout: \"0\"}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SWITCHBOARD_GO_CONFIG", zero)
	cfg, err = loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.UpstreamResponseHeaderTimeout != 0 {
		t.Fatalf("expected explicit 0 to disable header timeout, got %s", cfg.UpstreamResponseHeaderTimeout)
	}
}

func TestLoadConfigRejectsInvalidResponseHeaderTimeoutEnv(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := []byte("server: {proxy_api_key: \"p\"}\nupstream: {base_url: \"https://x\", api_keys: [\"k\"]}\n")
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SWITCHBOARD_GO_CONFIG", path)
	t.Setenv("UPSTREAM_RESPONSE_HEADER_TIMEOUT", "not-a-duration")
	if _, err := loadConfig(); err == nil || !strings.Contains(err.Error(), "UPSTREAM_RESPONSE_HEADER_TIMEOUT") {
		t.Fatalf("expected UPSTREAM_RESPONSE_HEADER_TIMEOUT validation error, got %v", err)
	}
}

type flushRecorder struct {
	*httptest.ResponseRecorder
	flushCount int
}

func (f *flushRecorder) Flush() {
	f.flushCount++
	f.ResponseRecorder.Flush()
}

type chunkedReader struct {
	chunks [][]byte
	idx    int
}

func (cr *chunkedReader) Read(p []byte) (n int, err error) {
	if cr.idx >= len(cr.chunks) {
		return 0, io.EOF
	}
	n = copy(p, cr.chunks[cr.idx])
	cr.idx++
	return n, nil
}

func TestCopyResponseFlushesEventStream(t *testing.T) {
	rec := &flushRecorder{ResponseRecorder: httptest.NewRecorder()}

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type": []string{"text/event-stream; charset=utf-8"},
		},
		Body: io.NopCloser(&chunkedReader{
			chunks: [][]byte{
				[]byte("data: {\"choices\": [{\"delta\": {\"content\": \"Hello\"}}]}\n\n"),
				[]byte("data: {\"choices\": [{\"delta\": {\"content\": \" World\"}}]}\n\n"),
				[]byte("data: [DONE]\n\n"),
			},
		}),
	}

	copyResponse(rec, resp)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	if rec.flushCount != 3 {
		t.Fatalf("expected exactly 3 flushes, got %d", rec.flushCount)
	}

	expectedBody := "data: {\"choices\": [{\"delta\": {\"content\": \"Hello\"}}]}\n\ndata: {\"choices\": [{\"delta\": {\"content\": \" World\"}}]}\n\ndata: [DONE]\n\n"
	if rec.Body.String() != expectedBody {
		t.Fatalf("unexpected body: got %q", rec.Body.String())
	}
}

func TestCopyResponseDoesNotFlushNormalJSON(t *testing.T) {
	rec := &flushRecorder{ResponseRecorder: httptest.NewRecorder()}

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type": []string{"application/json"},
		},
		Body: io.NopCloser(&chunkedReader{
			chunks: [][]byte{
				[]byte(`{"choices": `),
				[]byte(`[{"message": {"content": "Hello World"}}]}`),
			},
		}),
	}

	copyResponse(rec, resp)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	if rec.flushCount != 0 {
		t.Fatalf("expected 0 explicit flushes for non-stream JSON, got %d", rec.flushCount)
	}

	expectedBody := `{"choices": [{"message": {"content": "Hello World"}}]}`
	if rec.Body.String() != expectedBody {
		t.Fatalf("unexpected body: got %q", rec.Body.String())
	}
}

const subscriptionRequired403 = `{"error":{"message":"Upstream request failed: An active OpenCode Go subscription is required to use Go models"}}`

func TestIsKeyDead403Subscription(t *testing.T) {
	resp := &http.Response{StatusCode: 403, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(subscriptionRequired403))}
	if !isKeyDead(resp) {
		t.Fatal("expected 403 subscription-required to be a dead key")
	}
	other := `{"error":{"message":"forbidden"}}`
	resp = &http.Response{StatusCode: 403, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(other))}
	if isKeyDead(resp) {
		t.Fatal("expected other 403 not to be a dead key")
	}
	if got, _ := io.ReadAll(resp.Body); string(got) != other {
		t.Fatalf("body not restored: %q", got)
	}
	resp = &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(subscriptionRequired403))}
	if isKeyDead(resp) {
		t.Fatal("expected 200 not to be a dead key")
	}
}

func TestProxyRotatesOn403SubscriptionRequired(t *testing.T) {
	var gotKeys []string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKeys = append(gotKeys, r.Header.Get("Authorization"))
		if r.Header.Get("Authorization") == "Bearer bad" {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(subscriptionRequired403))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer upstream.Close()

	app := newApp(Config{ProxyAPIKey: "p", UpstreamAPIKeys: []string{"bad", "good"}, UpstreamBaseURL: upstream.URL, MaxRequestBodyBytes: 1024})
	rec := doProxyReq(app)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body %s", rec.Code, rec.Body.String())
	}
	if strings.Join(gotKeys, ",") != "Bearer bad,Bearer good" {
		t.Fatalf("unexpected keys: %v", gotKeys)
	}
	st := app.keys.Status()
	if st.Keys[0].State != string(KeyExhausted) || st.Keys[1].State != string(KeyAvailable) {
		t.Fatalf("unexpected key states: %+v", st.Keys)
	}
}

func TestProxyPassesThroughOther403(t *testing.T) {
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":{"message":"model not allowed"}}`))
	}))
	defer upstream.Close()

	app := newApp(Config{ProxyAPIKey: "p", UpstreamAPIKeys: []string{"k1", "k2"}, UpstreamBaseURL: upstream.URL, MaxRequestBodyBytes: 1024})
	rec := doProxyReq(app)
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "model not allowed") {
		t.Fatalf("expected 403 passthrough, got %d body %s", rec.Code, rec.Body.String())
	}
	if calls.Load() != 1 {
		t.Fatalf("expected a single upstream call, got %d", calls.Load())
	}
	if st := app.keys.Status(); st.Keys[0].State != string(KeyAvailable) {
		t.Fatalf("expected key 0 available, got %+v", st.Keys[0])
	}
}

func TestProxyAllKeys403SubscriptionExhausted(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(subscriptionRequired403))
	}))
	defer upstream.Close()

	app := newApp(Config{ProxyAPIKey: "p", UpstreamAPIKeys: []string{"k1", "k2"}, UpstreamBaseURL: upstream.URL, MaxRequestBodyBytes: 1024})
	rec := doProxyReq(app)
	if rec.Code != http.StatusTooManyRequests || !strings.Contains(rec.Body.String(), "all upstream keys exhausted") {
		t.Fatalf("expected 429 all exhausted, got %d body %s", rec.Code, rec.Body.String())
	}
}

func TestValidateKeysMarksExhaustedOn403Subscription(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "Bearer good" {
			_, _ = w.Write([]byte(`{"data":[]}`))
			return
		}
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(subscriptionRequired403))
	}))
	defer upstream.Close()
	app := newApp(Config{ProxyAPIKey: "p", UpstreamAPIKeys: []string{"good", "bad"}, UpstreamBaseURL: upstream.URL, MaxRequestBodyBytes: 1})
	req := httptest.NewRequest(http.MethodPost, "/admin/validate-keys", nil)
	req.Header.Set("Authorization", "Bearer p")
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	var out ValidateKeysResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Results) != 2 || out.Results[0].State != string(KeyAvailable) || out.Results[1].State != string(KeyExhausted) {
		t.Fatalf("unexpected results: %+v", out.Results)
	}
}

func TestProxyRotatesOn403SubscriptionCompressed(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer bad" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok":true}`))
			return
		}
		if strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			w.Header().Set("Content-Encoding", "gzip")
			w.WriteHeader(http.StatusForbidden)
			zw := gzip.NewWriter(w)
			_, _ = zw.Write([]byte(subscriptionRequired403))
			_ = zw.Close()
			return
		}
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(subscriptionRequired403))
	}))
	defer upstream.Close()

	app := newApp(Config{ProxyAPIKey: "p", UpstreamAPIKeys: []string{"bad", "good"}, UpstreamBaseURL: upstream.URL, MaxRequestBodyBytes: 1024})
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"glm-5.1","messages":[]}`))
	req.Header.Set("Authorization", "Bearer p")
	req.Header.Set("Accept-Encoding", "gzip, deflate, br")
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body %q", rec.Code, rec.Body.String())
	}
}

func TestProxyDecompressesUpstreamBodyForClient(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Encoding", "gzip")
		zw := gzip.NewWriter(w)
		_, _ = zw.Write([]byte(`{"ok":true}`))
		_ = zw.Close()
	}))
	defer upstream.Close()

	app := newApp(Config{ProxyAPIKey: "p", UpstreamAPIKeys: []string{"k1"}, UpstreamBaseURL: upstream.URL, MaxRequestBodyBytes: 1024})
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Bearer p")
	req.Header.Set("Accept-Encoding", "gzip, deflate, br")
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Body.String() != `{"ok":true}` || rec.Header().Get("Content-Encoding") != "" {
		t.Fatalf("expected plain body without Content-Encoding, got %q (%q)", rec.Body.String(), rec.Header().Get("Content-Encoding"))
	}
}

func TestProxyStreamsSSEFromGzipUpstream(t *testing.T) {
	release := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Content-Encoding", "gzip")
		zw := gzip.NewWriter(w)
		_, _ = zw.Write([]byte("data: one\n\n"))
		_ = zw.Flush()
		w.(http.Flusher).Flush()
		select {
		case <-release:
		case <-time.After(5 * time.Second):
		}
		_, _ = zw.Write([]byte("data: two\n\n"))
		_ = zw.Close()
	}))
	defer upstream.Close()

	srv := httptest.NewServer(newApp(Config{ProxyAPIKey: "p", UpstreamAPIKeys: []string{"k1"}, UpstreamBaseURL: upstream.URL, MaxRequestBodyBytes: 1024}))
	defer srv.Close()
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/v1/chat/completions", strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Bearer p")
	req.Header.Set("Accept-Encoding", "gzip, deflate, br")
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	first := make(chan string, 1)
	go func() {
		buf := make([]byte, len("data: one\n\n"))
		_, _ = io.ReadFull(resp.Body, buf)
		first <- string(buf)
	}()
	select {
	case got := <-first:
		if got != "data: one\n\n" {
			t.Fatalf("unexpected first event %q", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("first SSE event not streamed before upstream finished")
	}
	close(release)
	rest, _ := io.ReadAll(resp.Body)
	if string(rest) != "data: two\n\n" {
		t.Fatalf("unexpected rest %q", rest)
	}
}

const fakeUsageBody = `{"usage":{"rolling":{"percent":12.5,"resetsAt":"2026-10-25T12:00:00Z"},"weekly":{"percent":40,"resetsAt":"2026-10-27T00:00:00Z"},"monthly":{"percent":7,"resetsAt":"2026-11-01T00:00:00Z"}}}`

// fakeUsageServer answers /usage per Bearer key: "sk-good*" gets windows, "sk-bad*" gets 401.
func fakeUsageServer(t *testing.T, hits *int32) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(hits, 1)
		if strings.HasPrefix(r.Header.Get("Authorization"), "Bearer sk-good") {
			_, _ = w.Write([]byte(fakeUsageBody))
			return
		}
		http.Error(w, "nope", http.StatusUnauthorized)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func getKeysReport(t *testing.T, app *App) (int, string, KeysReportResponse) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/admin/keys", nil)
	req.Header.Set("Authorization", "Bearer p")
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	var out KeysReportResponse
	if rec.Code == http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
	}
	return rec.Code, rec.Body.String(), out
}

func TestAdminKeysUsageSuccess(t *testing.T) {
	var hits int32
	srv := fakeUsageServer(t, &hits)
	app := newApp(Config{ProxyAPIKey: "p", UpstreamAPIKeys: []string{"sk-good-1", "sk-good-2"}, KeyNames: []string{"principal", "reserva"}, UsageURL: srv.URL, UsageCacheTTL: time.Minute, RetryExhaustedAfter: time.Hour})
	app.keys.MarkExhausted(1)
	code, body, out := getKeysReport(t, app)
	if code != http.StatusOK {
		t.Fatalf("got %d %s", code, body)
	}
	if len(out.Keys) != 2 || out.GeneratedAt == "" || out.UsageCachedSeconds != 60 {
		t.Fatalf("unexpected report: %s", body)
	}
	k0, k1 := out.Keys[0], out.Keys[1]
	if k0.Name != "principal" || !k0.Eligible {
		t.Fatalf("key 0: %+v", k0)
	}
	if k1.Name != "reserva" || k1.State != "exhausted" || k1.Eligible || k1.Last429Time == "" {
		t.Fatalf("key 1: %+v", k1)
	}
	u := k0.Usage
	if u == nil || u.Rolling.Percent != 12.5 || u.Weekly.Percent != 40 || u.Monthly.Percent != 7 {
		t.Fatalf("usage: %+v", u)
	}
	if string(u.Weekly.ResetsAt) != `"2026-10-27T00:00:00Z"` {
		t.Fatalf("resets_at: %s", u.Weekly.ResetsAt)
	}
	if !strings.Contains(body, `"resets_at"`) || strings.Contains(body, "resetsAt") {
		t.Fatalf("expected snake_case resets_at: %s", body)
	}
}

func TestAdminKeysOneKeyErrorOneGood(t *testing.T) {
	var hits int32
	srv := fakeUsageServer(t, &hits)
	app := newApp(Config{ProxyAPIKey: "p", UpstreamAPIKeys: []string{"sk-bad-secret", "sk-good-secret"}, UsageURL: srv.URL})
	_, body, out := getKeysReport(t, app)
	if out.Keys[0].UsageError != "HTTP 401" || out.Keys[0].Usage != nil {
		t.Fatalf("key 0: %+v", out.Keys[0])
	}
	if out.Keys[1].UsageError != "" || out.Keys[1].Usage == nil {
		t.Fatalf("key 1: %+v", out.Keys[1])
	}
	if out.Keys[0].Name != "" || out.Keys[1].Name != "" {
		t.Fatalf("names should be empty: %s", body)
	}
}

func TestAdminKeysNeverLeaksKeyValue(t *testing.T) {
	var hits int32
	srv := fakeUsageServer(t, &hits)
	dead := httptest.NewServer(http.NotFoundHandler())
	deadURL := dead.URL
	dead.Close() // connection refused: the url.Error text would carry the URL, not the key
	var logs bytes.Buffer
	log.SetOutput(&logs)
	defer log.SetOutput(os.Stderr)
	for _, usageURL := range []string{srv.URL, deadURL} {
		app := newApp(Config{ProxyAPIKey: "p", UpstreamAPIKeys: []string{"sk-bad-SEGREDO123", "sk-good-SEGREDO456"}, KeyNames: []string{"a", "b"}, UsageURL: usageURL})
		_, body, _ := getKeysReport(t, app)
		for _, secret := range []string{"SEGREDO123", "SEGREDO456", "sk-bad", "sk-good"} {
			if strings.Contains(body, secret) || strings.Contains(logs.String(), secret) {
				t.Fatalf("leaked %q\nbody: %s\nlogs: %s", secret, body, logs.String())
			}
		}
	}
	if !strings.Contains(logs.String(), "usage query failed") {
		t.Fatalf("expected the failure to be logged: %q", logs.String())
	}
}

func TestAdminKeysCache(t *testing.T) {
	var hits int32
	srv := fakeUsageServer(t, &hits)
	now := time.Now()
	app := newApp(Config{ProxyAPIKey: "p", UpstreamAPIKeys: []string{"sk-good-1", "sk-good-2"}, UsageURL: srv.URL, UsageCacheTTL: time.Minute})
	app.usage.now = func() time.Time { return now }
	getKeysReport(t, app)
	getKeysReport(t, app)
	if got := atomic.LoadInt32(&hits); got != 2 {
		t.Fatalf("expected 2 upstream hits (1 per key), got %d", got)
	}
	now = now.Add(61 * time.Second)
	getKeysReport(t, app)
	if got := atomic.LoadInt32(&hits); got != 4 {
		t.Fatalf("expected refresh after TTL (4 hits), got %d", got)
	}
}

func TestAdminKeysNamesMissingOrShort(t *testing.T) {
	var hits int32
	srv := fakeUsageServer(t, &hits)
	long := strings.Repeat("x", 60)
	cfg := Config{ProxyAPIKey: "p", UpstreamAPIKeys: []string{"sk-good-1", "sk-good-2", "sk-good-3"}, UsageURL: srv.URL, KeyNames: parseKeyNames("principal,\n" + long + "\n")}
	_, _, out := getKeysReport(t, newApp(cfg))
	if out.Keys[0].Name != "principal" || out.Keys[1].Name != long[:40] || out.Keys[2].Name != "" {
		t.Fatalf("names: %q %q %q", out.Keys[0].Name, out.Keys[1].Name, out.Keys[2].Name)
	}
	if got := parseKeyNames("a\r\nb, c"); got[0] != "ab" || got[1] != "c" {
		t.Fatalf("got %q", got)
	}
}

func TestAdminKeysAuthAndMethod(t *testing.T) {
	app := newApp(Config{ProxyAPIKey: "p", UpstreamAPIKeys: []string{"sk-good-1"}})
	req := httptest.NewRequest(http.MethodGet, "/admin/keys", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized || !strings.Contains(rec.Body.String(), "invalid_api_key") {
		t.Fatalf("no bearer: %d %s", rec.Code, rec.Body.String())
	}
	req = httptest.NewRequest(http.MethodPost, "/admin/keys", nil)
	req.Header.Set("Authorization", "Bearer p")
	rec = httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("POST should not match (same as other admin routes), got %d", rec.Code)
	}
}

func getStatusPage(app *App) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/status", nil))
	return rec
}

func TestStatusPageOpenAndNoKeyLeak(t *testing.T) {
	var hits int32
	srv := fakeUsageServer(t, &hits)
	app := newApp(Config{ProxyAPIKey: "p", UpstreamAPIKeys: []string{"sk-good-SEGREDO1", "sk-bad-SEGREDO2"}, KeyNames: []string{"principal"}, UsageURL: srv.URL, UsageCacheTTL: time.Minute, RetryExhaustedAfter: time.Hour})
	app.keys.MarkExhausted(1)
	rec := getStatusPage(app)
	body := rec.Body.String()
	if rec.Code != http.StatusOK || !strings.HasPrefix(rec.Header().Get("Content-Type"), "text/html") {
		t.Fatalf("got %d %s", rec.Code, rec.Header().Get("Content-Type"))
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("Cache-Control: %q", rec.Header().Get("Cache-Control"))
	}
	for _, want := range []string{"principal", "#2", "exhausted", "<td>12.5%</td>", "<td>40.0%</td>", "<td>7.0%</td>"} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q in %s", want, body)
		}
	}
	rows := strings.Split(body, "<tr>")
	if !strings.Contains(rows[2], "(atual)") || regexp.MustCompile(`<td>\d+s</td>`).MatchString(rows[2]) {
		t.Fatalf("current key row: %s", rows[2])
	}
	if !regexp.MustCompile(`<td>\d+s</td>`).MatchString(rows[3]) || strings.Contains(rows[3], "(atual)") {
		t.Fatalf("exhausted key row: %s", rows[3])
	}
	head := httptest.NewRecorder()
	app.ServeHTTP(head, httptest.NewRequest(http.MethodHead, "/status", nil))
	if head.Code != http.StatusOK {
		t.Fatalf("HEAD /status: %d", head.Code)
	}
	for _, secret := range []string{"SEGREDO1", "SEGREDO2", "sk-good", "sk-bad"} {
		if strings.Contains(body, secret) {
			t.Fatalf("leaked %q: %s", secret, body)
		}
	}
	req := httptest.NewRequest(http.MethodGet, "/admin/keys", nil)
	rec = httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("/admin/keys without key: %d", rec.Code)
	}
}

func TestStatusPageUsageErrorOnKeyRow(t *testing.T) {
	var hits int32
	srv := fakeUsageServer(t, &hits)
	app := newApp(Config{ProxyAPIKey: "p", UpstreamAPIKeys: []string{"sk-bad-1", "sk-good-2"}, UsageURL: srv.URL})
	rec := getStatusPage(app)
	body := rec.Body.String()
	rows := strings.Split(body, "<tr>")
	if rec.Code != http.StatusOK || len(rows) != 4 {
		t.Fatalf("got %d %s", rec.Code, body)
	}
	if !strings.Contains(rows[2], "erro de uso: HTTP 401") || strings.Contains(rows[2], "%</td>") {
		t.Fatalf("key #1 row: %s", rows[2])
	}
	if !strings.Contains(rows[3], "<td>12.5%</td>") || strings.Contains(rows[3], "erro de uso") {
		t.Fatalf("key #2 row: %s", rows[3])
	}
}

func TestStatusPageEscapesKeyName(t *testing.T) {
	var hits int32
	srv := fakeUsageServer(t, &hits)
	app := newApp(Config{ProxyAPIKey: "p", UpstreamAPIKeys: []string{"sk-good-1"}, KeyNames: []string{"<b>x</b>"}, UsageURL: srv.URL})
	body := getStatusPage(app).Body.String()
	if !strings.Contains(body, "&lt;b&gt;x&lt;/b&gt;") || strings.Contains(body, "<b>x</b>") {
		t.Fatalf("name not escaped: %s", body)
	}
}

func TestStatusPagePartialWindows(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"usage":{"weekly":{"percent":40}}}`))
	}))
	t.Cleanup(srv.Close)
	app := newApp(Config{ProxyAPIKey: "p", UpstreamAPIKeys: []string{"sk-good-1"}, UsageURL: srv.URL})
	rec := getStatusPage(app)
	body := rec.Body.String()
	if rec.Code != http.StatusOK || !strings.Contains(body, "<td>40.0%</td>") || !strings.Contains(body, "<td>—</td>") {
		t.Fatalf("got %d %s", rec.Code, body)
	}
}

func TestUsageForCancelledRequestDoesNotPoisonCache(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&hits, 1) == 1 {
			<-r.Context().Done() // slow upstream: hold until the client gives up
			return
		}
		_, _ = w.Write([]byte(fakeUsageBody))
	}))
	t.Cleanup(srv.Close)
	app := newApp(Config{ProxyAPIKey: "p", UpstreamAPIKeys: []string{"sk-good-1"}, UsageURL: srv.URL, UsageCacheTTL: time.Minute})
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(50*time.Millisecond, cancel)
	if got := app.usageFor(ctx); len(got) != 1 || got[0].err == "" {
		t.Fatalf("expected a cancel error for this response only: %+v", got)
	}
	got := app.usageFor(context.Background())
	if got[0].err != "" || got[0].usage == nil || atomic.LoadInt32(&hits) != 2 {
		t.Fatalf("expected a refetch after cancel: %+v hits=%d", got, atomic.LoadInt32(&hits))
	}
}

func TestUsageCacheTTLEnv(t *testing.T) {
	t.Setenv("USAGE_CACHE_TTL", "5x")
	cfg := defaultConfig()
	applyEnvOverrides(&cfg)
	cfg.ProxyAPIKey, cfg.UpstreamAPIKeys = "p", []string{"k"}
	if err := validateConfig(cfg); err == nil {
		t.Fatal("expected invalid USAGE_CACHE_TTL to fail validation")
	}
	if d := defaultConfig(); d.UsageCacheTTL != 60*time.Second || d.UsageURL != "https://opencode.ai/zen/go/v1/usage" {
		t.Fatalf("defaults: %+v", d)
	}
}
