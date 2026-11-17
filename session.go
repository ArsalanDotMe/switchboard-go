package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
)

// sessionToken deliberately excludes whitespace/control characters and bounds
// forwarded labels. No message content or caller IP is used as conversation ID.
func sessionToken(s string) bool {
	if len(s) == 0 || len(s) > 256 {
		return false
	}
	for _, c := range s {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.ContainsRune("._:-", c)) {
			return false
		}
	}
	return true
}

// addSessionCompatibility runs only after gateway authentication. Stock n8n's
// opt-in model request labels carry the actual Assistant thread across turns.
// Preserve existing clients' session and identity verbatim. Never infer a
// conversation from message prefixes, IP, key, model, or a global fallback.
func addSessionCompatibility(req *http.Request, body []byte) {
	if req.Method != "POST" || (req.URL.Path != "/zen/go/v1/chat/completions" && !strings.HasSuffix(req.URL.Path, "/chat/completions")) {
		return
	}
	if req.Header.Get("x-opencode-session") != "" {
		return
	}
	if req.Header.Get("x-n8n-feature") == "instance-ai" && sessionToken(req.Header.Get("x-n8n-thread-id")) && sessionToken(req.Header.Get("x-n8n-version")) {
		req.Header.Set("x-opencode-session", "n8n-"+req.Header.Get("x-n8n-thread-id"))
		req.Header.Set("User-Agent", "switchboard-go/1.4.0 n8n-assistant/"+req.Header.Get("x-n8n-version"))
		return
	}
	// n8n verifyModel does not use model request labels. Its exact innocuous,
	// one-message 16-token probe is a standalone verification, not a conversation.
	// A fresh ID per request is correct here, not for general unlabeled inference.
	if stockVerificationProbe(req, body) {
		var id [16]byte
		if _, err := rand.Read(id[:]); err != nil {
			return
		}
		req.Header.Set("x-opencode-session", "switchboard-verification-"+hex.EncodeToString(id[:]))
		req.Header.Set("User-Agent", "switchboard-go/1.4.0 connection-verification/1")
	}
}

func stockVerificationProbe(req *http.Request, body []byte) bool {
	if !strings.HasPrefix(req.UserAgent(), "ai/") || req.Header.Get("x-n8n-thread-id") != "" {
		return false
	}
	var raw map[string]json.RawMessage
	if json.Unmarshal(body, &raw) != nil || len(raw) != 3 {
		return false
	}
	var model string
	var limit int
	if json.Unmarshal(raw["model"], &model) != nil || model == "" || json.Unmarshal(raw["max_tokens"], &limit) != nil || limit != 16 {
		return false
	}
	var messages []map[string]json.RawMessage
	if json.Unmarshal(raw["messages"], &messages) != nil || len(messages) != 1 || len(messages[0]) != 2 {
		return false
	}
	var role, content string
	return json.Unmarshal(messages[0]["role"], &role) == nil && role == "user" && json.Unmarshal(messages[0]["content"], &content) == nil && content == "Reply with OK."
}
