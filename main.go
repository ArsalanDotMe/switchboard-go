package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/smtp"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	yaml "gopkg.in/yaml.v3"
)

type Config struct {
	ListenAddr          string
	UpstreamBaseURL     string
	ProxyAPIKey         string
	UpstreamAPIKeys     []string
	MaxRequestBodyBytes int64

	SMTP SMTPConfig
}

type SMTPConfig struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string
	To       string
	TLS      bool
	StartTLS bool
}

func loadConfig() (Config, error) {
	cfg := defaultConfig()
	if path, ok, err := resolveConfigPath(); err != nil {
		return Config{}, err
	} else if ok {
		fileCfg, err := loadYAMLConfig(path)
		if err != nil {
			return Config{}, err
		}
		mergeConfig(&cfg, fileCfg)
	}
	applyEnvOverrides(&cfg)
	if strings.TrimSpace(cfg.ProxyAPIKey) == "" {
		return Config{}, errors.New("PROXY_API_KEY is required")
	}
	if len(cfg.UpstreamAPIKeys) == 0 {
		return Config{}, errors.New("OPENCODE_GO_API_KEYS is required")
	}
	return cfg, nil
}

func defaultConfig() Config {
	return Config{ListenAddr: ":8080", UpstreamBaseURL: "https://opencode.ai/zen/go/v1", MaxRequestBodyBytes: 20 << 20, SMTP: SMTPConfig{Port: 25}}
}

func resolveConfigPath() (string, bool, error) {
	if explicit := strings.TrimSpace(os.Getenv("SWITCHBOARD_GO_CONFIG")); explicit != "" {
		if _, err := os.Stat(explicit); err != nil {
			return "", false, fmt.Errorf("read SWITCHBOARD_GO_CONFIG: %w", err)
		}
		return explicit, true, nil
	}
	home, _ := os.UserHomeDir()
	paths := []string{}
	if home != "" {
		paths = append(paths, filepath.Join(home, ".config", "switchboard-go", "config.yaml"))
	}
	paths = append(paths, "/etc/switchboard-go/config.yaml")
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			return p, true, nil
		}
	}
	return "", false, nil
}

type yamlConfig struct {
	Server struct {
		ListenAddr  string `yaml:"listen_addr"`
		ProxyAPIKey string `yaml:"proxy_api_key"`
	} `yaml:"server"`
	Upstream struct {
		BaseURL string   `yaml:"base_url"`
		APIKeys []string `yaml:"api_keys"`
	} `yaml:"upstream"`
	SMTP struct {
		Host     string `yaml:"host"`
		Username string `yaml:"username"`
		Password string `yaml:"password"`
		From     string `yaml:"from"`
		To       string `yaml:"to"`
		Port     int    `yaml:"port"`
		TLS      bool   `yaml:"tls"`
		StartTLS bool   `yaml:"starttls"`
	} `yaml:"smtp"`
	Limits struct {
		MaxRequestBodyBytes int64 `yaml:"max_request_body_bytes"`
	} `yaml:"limits"`
}

func loadYAMLConfig(path string) (Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config file: %w", err)
	}
	var yc yamlConfig
	if err := yaml.Unmarshal(b, &yc); err != nil {
		return Config{}, fmt.Errorf("parse config file: %w", err)
	}
	return Config{ListenAddr: yc.Server.ListenAddr, UpstreamBaseURL: yc.Upstream.BaseURL, ProxyAPIKey: yc.Server.ProxyAPIKey, UpstreamAPIKeys: yc.Upstream.APIKeys, MaxRequestBodyBytes: yc.Limits.MaxRequestBodyBytes, SMTP: SMTPConfig{Host: yc.SMTP.Host, Port: yc.SMTP.Port, Username: yc.SMTP.Username, Password: yc.SMTP.Password, From: yc.SMTP.From, To: yc.SMTP.To, TLS: yc.SMTP.TLS, StartTLS: yc.SMTP.StartTLS}}, nil
}

func mergeConfig(dst *Config, src Config) {
	if src.ListenAddr != "" {
		dst.ListenAddr = src.ListenAddr
	}
	if src.UpstreamBaseURL != "" {
		dst.UpstreamBaseURL = src.UpstreamBaseURL
	}
	if src.ProxyAPIKey != "" {
		dst.ProxyAPIKey = src.ProxyAPIKey
	}
	if len(src.UpstreamAPIKeys) > 0 {
		dst.UpstreamAPIKeys = append([]string(nil), src.UpstreamAPIKeys...)
	}
	if src.MaxRequestBodyBytes > 0 {
		dst.MaxRequestBodyBytes = src.MaxRequestBodyBytes
	}
	if src.SMTP.Host != "" {
		dst.SMTP.Host = src.SMTP.Host
	}
	if src.SMTP.Port != 0 {
		dst.SMTP.Port = src.SMTP.Port
	}
	if src.SMTP.Username != "" {
		dst.SMTP.Username = src.SMTP.Username
	}
	if src.SMTP.Password != "" {
		dst.SMTP.Password = src.SMTP.Password
	}
	if src.SMTP.From != "" {
		dst.SMTP.From = src.SMTP.From
	}
	if src.SMTP.To != "" {
		dst.SMTP.To = src.SMTP.To
	}
	dst.SMTP.TLS = src.SMTP.TLS || dst.SMTP.TLS
	dst.SMTP.StartTLS = src.SMTP.StartTLS || dst.SMTP.StartTLS
}

func applyEnvOverrides(cfg *Config) {
	if v := strings.TrimSpace(os.Getenv("LISTEN_ADDR")); v != "" {
		cfg.ListenAddr = v
	}
	if v := strings.TrimSpace(os.Getenv("UPSTREAM_BASE_URL")); v != "" {
		cfg.UpstreamBaseURL = strings.TrimRight(v, "/")
	}
	if v := strings.TrimSpace(os.Getenv("PROXY_API_KEY")); v != "" {
		cfg.ProxyAPIKey = v
	}
	if v := strings.TrimSpace(os.Getenv("OPENCODE_GO_API_KEYS")); v != "" {
		var keys []string
		for _, k := range strings.Split(v, ",") {
			if s := strings.TrimSpace(k); s != "" {
				keys = append(keys, s)
			}
		}
		if len(keys) > 0 {
			cfg.UpstreamAPIKeys = keys
		}
	}
	if v := strings.TrimSpace(os.Getenv("MAX_REQUEST_BODY_BYTES")); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
			cfg.MaxRequestBodyBytes = n
		}
	}
	if v := os.Getenv("SMTP_HOST"); v != "" {
		cfg.SMTP.Host = v
	}
	if v := os.Getenv("SMTP_PORT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.SMTP.Port = n
		}
	}
	if v := os.Getenv("SMTP_USERNAME"); v != "" {
		cfg.SMTP.Username = v
	}
	if v := os.Getenv("SMTP_PASSWORD"); v != "" {
		cfg.SMTP.Password = v
	}
	if v := os.Getenv("SMTP_FROM"); v != "" {
		cfg.SMTP.From = v
	}
	if v := os.Getenv("SMTP_TO"); v != "" {
		cfg.SMTP.To = v
	}
	if v := os.Getenv("SMTP_TLS"); v != "" {
		cfg.SMTP.TLS = parseBool(v)
	}
	if v := os.Getenv("SMTP_STARTTLS"); v != "" {
		cfg.SMTP.StartTLS = parseBool(v)
	}
}

func parseBool(v string) bool { b, _ := strconv.ParseBool(strings.TrimSpace(v)); return b }

type KeyState string

const (
	KeyUnknown   KeyState = "unknown"
	KeyAvailable KeyState = "available"
	KeyExhausted KeyState = "exhausted"
)

type KeyManager struct {
	mu             sync.Mutex
	keys           []string
	states         []KeyState
	last429        map[int]time.Time
	current        int
	allNotified    bool
	notifiedSwitch map[int]bool
}

func NewKeyManager(keys []string) *KeyManager {
	states := make([]KeyState, len(keys))
	for i := range states {
		states[i] = KeyUnknown
	}
	return &KeyManager{keys: append([]string(nil), keys...), states: states, last429: map[int]time.Time{}, notifiedSwitch: map[int]bool{}}
}

func (m *KeyManager) Current() (int, string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.keys) == 0 || m.allExhaustedLocked() {
		return 0, "", false
	}
	if m.states[m.current] == KeyExhausted {
		m.advanceLocked()
	}
	if m.states[m.current] == KeyExhausted {
		return 0, "", false
	}
	return m.current, m.keys[m.current], true
}

func (m *KeyManager) MarkExhausted(i int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if i < 0 || i >= len(m.keys) {
		return
	}
	m.states[i] = KeyExhausted
	m.last429[i] = time.Now().UTC()
	m.advanceLocked()
}

func (m *KeyManager) ShouldNotifySwitch(i int) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.notifiedSwitch[i] {
		return false
	}
	m.notifiedSwitch[i] = true
	return true
}

func (m *KeyManager) ShouldNotifyAllExhausted() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.allNotified {
		return false
	}
	if !m.allExhaustedLocked() {
		return false
	}
	m.allNotified = true
	return true
}

func (m *KeyManager) AdvanceOnExhaustion() (int, string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.keys) == 0 {
		return 0, "", false
	}
	m.advanceLocked()
	return m.current, m.keys[m.current], true
}

func (m *KeyManager) advanceLocked() {
	if len(m.keys) == 0 {
		return
	}
	start := m.current
	for step := 1; step <= len(m.keys); step++ {
		next := (start + step) % len(m.keys)
		if m.states[next] != KeyExhausted {
			m.current = next
			return
		}
	}
	m.current = start
}

func (m *KeyManager) AllExhausted() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.allExhaustedLocked()
}

func (m *KeyManager) allExhaustedLocked() bool {
	for _, st := range m.states {
		if st != KeyExhausted {
			return false
		}
	}
	return true
}

func (m *KeyManager) Status() StatusResponse {
	m.mu.Lock()
	defer m.mu.Unlock()
	states := make([]PerKeyStatus, len(m.keys))
	for i := range m.keys {
		state := m.states[i]
		if i == m.current && state != KeyExhausted {
			state = KeyAvailable
		}
		states[i] = PerKeyStatus{Index: i, State: string(state), Last429Time: m.last429String(i), Current: i == m.current}
	}
	return StatusResponse{CurrentKeyIndex: m.current, Keys: states, Note: "Remaining usage is unavailable from opencode-go API."}
}

func (m *KeyManager) last429String(i int) string {
	if t, ok := m.last429[i]; ok && !t.IsZero() {
		return t.Format(time.RFC3339)
	}
	return ""
}

type PerKeyStatus struct {
	Index       int    `json:"index"`
	State       string `json:"state"`
	Last429Time string `json:"last_429_time,omitempty"`
	Current     bool   `json:"current"`
}
type StatusResponse struct {
	CurrentKeyIndex int            `json:"current_key_index"`
	Keys            []PerKeyStatus `json:"keys"`
	Note            string         `json:"note"`
}

type App struct {
	config Config
	keys   *KeyManager
	client *http.Client
	sender *SMTPNotifier
}

func newApp(cfg Config) *App {
	return &App{config: cfg, keys: NewKeyManager(cfg.UpstreamAPIKeys), client: &http.Client{Transport: &http.Transport{Proxy: http.ProxyFromEnvironment, DialContext: (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext, TLSHandshakeTimeout: 10 * time.Second, ResponseHeaderTimeout: 30 * time.Second, ExpectContinueTimeout: 2 * time.Second}}, sender: NewSMTPNotifier(cfg.SMTP)}
}

func (a *App) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.URL.Path == "/healthz":
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	case strings.HasPrefix(r.URL.Path, "/admin/"):
		if !a.authOK(r) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		a.handleAdmin(w, r)
	case strings.HasPrefix(r.URL.Path, "/v1/"):
		if !a.authOK(r) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		a.proxyV1(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (a *App) authOK(r *http.Request) bool {
	if tok := bearerToken(r.Header.Get("Authorization")); tok != "" && tok == a.config.ProxyAPIKey {
		return true
	}
	return strings.TrimSpace(r.Header.Get("x-api-key")) == a.config.ProxyAPIKey
}

func bearerToken(v string) string {
	parts := strings.Fields(v)
	if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
		return strings.TrimSpace(parts[1])
	}
	return ""
}

func (a *App) handleAdmin(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(a.keys.Status())
}

func (a *App) proxyV1(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, a.config.MaxRequestBodyBytes)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		if strings.Contains(err.Error(), "request body too large") {
			http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, "read body failed", http.StatusBadRequest)
		return
	}
	_ = r.Body.Close()
	orig := r.Context()
	for attempts := 0; attempts < len(a.config.UpstreamAPIKeys); attempts++ {
		idx, key, ok := a.keys.Current()
		if !ok {
			break
		}
		resp, reqErr := a.doUpstream(orig, r, body, key)
		if reqErr != nil {
			http.Error(w, reqErr.Error(), http.StatusBadGateway)
			return
		}
		if resp.StatusCode == http.StatusTooManyRequests && isQuota429(resp) {
			_ = resp.Body.Close()
			a.keys.MarkExhausted(idx)
			if a.keys.ShouldNotifySwitch(idx) {
				a.sender.NotifySwitch(idx, a.keys.Status())
			}
			if a.keys.ShouldNotifyAllExhausted() {
				a.sender.NotifyAllExhausted(a.keys.Status())
				writeOpenAIError(w, 429, "rate_limit_exceeded", "all upstream keys exhausted")
				return
			}
			continue
		}
		copyResponse(w, resp)
		return
	}
	if a.keys.AllExhausted() {
		if a.keys.ShouldNotifyAllExhausted() {
			a.sender.NotifyAllExhausted(a.keys.Status())
		}
		writeOpenAIError(w, 429, "rate_limit_exceeded", "all upstream keys exhausted")
		return
	}
	writeOpenAIError(w, 502, "bad_gateway", "upstream unavailable")
}

func (a *App) doUpstream(ctx context.Context, r *http.Request, body []byte, key string) (*http.Response, error) {
	path := strings.TrimPrefix(r.URL.EscapedPath(), "/v1")
	if path == "" {
		path = "/"
	}
	u := a.config.UpstreamBaseURL + path
	if r.URL.RawQuery != "" {
		u += "?" + r.URL.RawQuery
	}
	req, err := http.NewRequestWithContext(ctx, r.Method, u, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	copyHeaders(req.Header, r.Header)
	req.Header.Set("Authorization", "Bearer "+key)
	if strings.TrimSpace(req.Header.Get("User-Agent")) == "" {
		req.Header.Set("User-Agent", "OpenAI/Python 1.0.0")
	}
	stripHopByHopHeaders(req.Header)
	return a.client.Do(req)
}

func copyHeaders(dst, src http.Header) {
	for k, v := range src {
		if strings.EqualFold(k, "Authorization") || strings.EqualFold(k, "x-api-key") {
			continue
		}
		for _, s := range v {
			dst.Add(k, s)
		}
	}
}

func stripHopByHopHeaders(h http.Header) {
	hop := map[string]struct{}{"Connection": {}, "Proxy-Authorization": {}, "Proxy-Authenticate": {}, "Keep-Alive": {}, "Te": {}, "Trailer": {}, "Transfer-Encoding": {}, "Upgrade": {}}
	for _, v := range h.Values("Connection") {
		for _, part := range strings.Split(v, ",") {
			if name := strings.TrimSpace(part); name != "" {
				hop[http.CanonicalHeaderKey(name)] = struct{}{}
			}
		}
	}
	for k := range hop {
		h.Del(k)
	}
}

func copyResponse(w http.ResponseWriter, resp *http.Response) {
	defer resp.Body.Close()
	stripHopByHopHeaders(resp.Header)
	for k, v := range resp.Header {
		for _, s := range v {
			w.Header().Add(k, s)
		}
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

func isQuota429(resp *http.Response) bool {
	if resp.StatusCode != 429 {
		return false
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	resp.Body.Close()
	resp.Body = io.NopCloser(bytes.NewReader(body))
	var m map[string]any
	if json.Unmarshal(body, &m) == nil {
		if e, ok := m["error"].(map[string]any); ok {
			if code, _ := e["code"].(string); code == "insufficient_quota" || code == "usage_not_included" {
				return true
			}
			typ, _ := e["type"].(string)
			msg, _ := e["message"].(string)
			lowType := strings.ToLower(typ)
			lowMsg := strings.ToLower(msg)
			if strings.Contains(lowType, "usage") || strings.Contains(lowType, "quota") || strings.Contains(lowType, "freeusagelimit") {
				return true
			}
			if strings.Contains(lowMsg, "quota") || strings.Contains(lowMsg, "exhausted") || strings.Contains(lowMsg, "usage limit") {
				return true
			}
		}
	}
	return strings.Contains(strings.ToLower(resp.Header.Get("X-RateLimit-Reason")), "quota")
}

func writeOpenAIError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"message": message, "type": "invalid_request_error", "param": nil, "code": code}})
}

type SMTPNotifier struct{ cfg SMTPConfig }

func NewSMTPNotifier(cfg SMTPConfig) *SMTPNotifier { return &SMTPNotifier{cfg: cfg} }
func (n *SMTPNotifier) NotifySwitch(idx int, st StatusResponse) {
	go n.send("Switchboard Go switched upstream key", fmt.Sprintf("Switched away from key %d\n\n%+v", idx, st))
}
func (n *SMTPNotifier) NotifyAllExhausted(st StatusResponse) {
	go n.send("Switchboard Go exhausted all upstream keys", fmt.Sprintf("All keys exhausted\n\n%+v", st))
}
func (n *SMTPNotifier) send(subject, body string) {
	if n.cfg.Host == "" || n.cfg.To == "" || n.cfg.From == "" {
		return
	}
	addr := net.JoinHostPort(n.cfg.Host, strconv.Itoa(n.cfg.Port))
	auth := smtp.Auth(nil)
	if strings.TrimSpace(n.cfg.Username) != "" {
		auth = smtp.PlainAuth("", n.cfg.Username, n.cfg.Password, n.cfg.Host)
	}
	msg := []byte("To: " + n.cfg.To + "\r\nSubject: " + subject + "\r\n\r\n" + body + "\r\n")
	if err := sendMail(addr, auth, n.cfg.From, []string{n.cfg.To}, msg, n.cfg.TLS, n.cfg.StartTLS); err != nil {
		log.Printf("smtp notification failed: %v", err)
	}
}

// tiny indirection to keep stdlib-only and testable.
var sendMail = func(addr string, auth smtp.Auth, from string, to []string, msg []byte, useTLS, useStartTLS bool) error {
	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		return err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(15 * time.Second))
	host, _, _ := net.SplitHostPort(addr)
	if useTLS {
		conn = tls.Client(conn, &tls.Config{ServerName: host})
	}
	c, err := smtp.NewClient(conn, host)
	if err != nil {
		return err
	}
	defer c.Quit()
	if useStartTLS {
		if ok, _ := c.Extension("STARTTLS"); ok {
			if err := c.StartTLS(&tls.Config{ServerName: host}); err != nil {
				return err
			}
		}
	}
	if auth != nil {
		if err := c.Auth(auth); err != nil {
			return err
		}
	}
	if err := c.Mail(from); err != nil {
		return err
	}
	for _, rcpt := range to {
		if err := c.Rcpt(rcpt); err != nil {
			return err
		}
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(msg); err != nil {
		return err
	}
	return w.Close()
}

func main() {
	cfg, err := loadConfig()
	if err != nil {
		log.Fatal(err)
	}
	a := newApp(cfg)
	srv := &http.Server{Addr: cfg.ListenAddr, Handler: a, ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 65 * time.Second, WriteTimeout: 0, IdleTimeout: 120 * time.Second}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shut, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(shut)
	}()
	log.Printf("listening on %s", cfg.ListenAddr)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}
