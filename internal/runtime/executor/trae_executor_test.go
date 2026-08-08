package executor

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	traeauth "github.com/router-for-me/CLIProxyAPI/v7/internal/auth/trae"
	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/executor"
)

func TestPrepareTraePayloadBuildsCLIRawChatRequest(t *testing.T) {
	req := cliproxyexecutor.Request{
		Model: "trae/gpt-4o",
		Payload: []byte(`{
			"model":"trae/gpt-4o",
			"messages":[{"role":"system","content":"Be concise"},{"role":"user","content":"hello"}],
			"tools":[{"type":"function","function":{"name":"lookup","description":"Look up a value","parameters":{"type":"object"}}}]
		}`),
	}
	prepared, err := prepareTraePayload(req, cliproxyexecutor.Options{}, true)
	if err != nil {
		t.Fatal(err)
	}
	if !prepared.cliRaw || prepared.upstreamModel != "DeepSeek-V4-Pro" {
		t.Fatalf("unexpected prepared model: %#v", prepared)
	}
	var body map[string]any
	if err = json.Unmarshal(prepared.body, &body); err != nil {
		t.Fatal(err)
	}
	if body["config_name"] != "deepseek-V4-Pro" || body["model_name"] != "deepseek-V4-Pro__v2" || body["stream"] != true {
		t.Fatalf("unexpected body: %#v", body)
	}
	if body["conversation_id"] == "" || body["conversation_id"] != body["session_id"] {
		t.Fatalf("unexpected session identifiers: %#v", body)
	}
	messages, _ := body["messages"].([]any)
	if len(messages) != 2 {
		t.Fatalf("messages length = %d, want 2", len(messages))
	}
	encodedMessages, _ := json.Marshal(messages)
	if !strings.Contains(string(encodedMessages), "tool_call") || !strings.Contains(string(encodedMessages), "lookup") {
		t.Fatalf("tool prompt missing from messages: %s", encodedMessages)
	}
}

func TestPrepareTraeIDEPayloadRemainsAvailable(t *testing.T) {
	prepared, err := prepareTraePayloadForCredential(cliproxyexecutor.Request{
		Model:   "trae/gpt-4o",
		Payload: []byte(`{"messages":[{"role":"user","content":"hello"}]}`),
	}, cliproxyexecutor.Options{}, true, &traeauth.TokenStorage{AuthKind: traeauth.AuthKindIDE})
	if err != nil {
		t.Fatal(err)
	}
	if prepared.cliRaw || prepared.upstreamModel != "DeepSeek-V4-Pro" {
		t.Fatalf("unexpected IDE prepared payload: %#v", prepared)
	}
	var body map[string]any
	if err = json.Unmarshal(prepared.body, &body); err != nil {
		t.Fatal(err)
	}
	if body["model"] != "DeepSeek-V4-Pro" || body["function"] != "inline_chat" {
		t.Fatalf("unexpected IDE body: %#v", body)
	}
}

func TestCollectTraeRawSSEUsesCumulativeOutputAndUsage(t *testing.T) {
	sse := strings.Join([]string{
		"event: output",
		`data: {"response":"hel","tool_calls":null}`,
		"",
		"event: output",
		`data: {"response":"hello","tool_calls":[{"id":"call-1","function":{"name":"lookup","arguments":"{\"key\":\"value\"}"}}]}`,
		"",
		"event: token_usage",
		`data: {"prompt_tokens":3,"completion_tokens":2,"total_tokens":5}`,
		"",
	}, "\n")
	collected, err := collectTraeSSE(strings.NewReader(sse), true)
	if err != nil {
		t.Fatal(err)
	}
	if collected.Content != "hello" || collected.InputTokens != 3 || collected.OutputTokens != 2 || collected.TotalTokens != 5 {
		t.Fatalf("unexpected collection: %#v", collected)
	}
	raw, _, err := buildTraeOpenAIResponse(collected, "trae/GLM-5.1", 7)
	if err != nil {
		t.Fatal(err)
	}
	var response map[string]any
	if err = json.Unmarshal(raw, &response); err != nil {
		t.Fatal(err)
	}
	choice := response["choices"].([]any)[0].(map[string]any)
	if choice["finish_reason"] != "tool_calls" {
		t.Fatalf("finish_reason = %#v", choice["finish_reason"])
	}
	message := choice["message"].(map[string]any)
	function := message["tool_calls"].([]any)[0].(map[string]any)["function"].(map[string]any)
	if function["name"] != "lookup" || function["arguments"] != `{"key":"value"}` {
		t.Fatalf("unexpected function: %#v", function)
	}
	usage := response["usage"].(map[string]any)
	if usage["total_tokens"] != float64(5) {
		t.Fatalf("unexpected usage: %#v", usage)
	}
}

func TestCollectTraeIDESSEAppendsOutputChunks(t *testing.T) {
	sse := strings.Join([]string{
		"event: output",
		`data:{"response":"<"}`,
		"",
		"event: output",
		`data:{"response":"tool_call>\n{\"name\":\"lookup\",\""}`,
		"",
		"event: output",
		`data:{"response":"arguments\":{\"key\":\"value\"}}\n</tool"}`,
		"",
		"event: output",
		`data:{"response":"_call>"}`,
		"",
		"event: done",
		`data:{"finish_reason":"stop"}`,
		"",
	}, "\n")
	collected, err := collectTraeSSE(strings.NewReader(sse), false)
	if err != nil {
		t.Fatal(err)
	}
	raw, _, err := buildTraeOpenAIResponse(collected, "trae/GLM-5.1", 7)
	if err != nil {
		t.Fatal(err)
	}
	var response map[string]any
	if err = json.Unmarshal(raw, &response); err != nil {
		t.Fatal(err)
	}
	choice := response["choices"].([]any)[0].(map[string]any)
	if choice["finish_reason"] != "tool_calls" {
		t.Fatalf("finish_reason = %#v", choice["finish_reason"])
	}
	message := choice["message"].(map[string]any)
	function := message["tool_calls"].([]any)[0].(map[string]any)["function"].(map[string]any)
	if function["name"] != "lookup" || function["arguments"] != `{"key":"value"}` {
		t.Fatalf("unexpected function: %#v", function)
	}
}

func TestApplyTraeCLIHeaders(t *testing.T) {
	req, err := http.NewRequest(http.MethodPost, "https://example.com/chat", nil)
	if err != nil {
		t.Fatal(err)
	}
	model := resolveTraeRawModel("GLM-5.1")
	applyTraeCLIHeaders(req, &traeauth.TokenStorage{Token: "token-1"}, model, "session-1", "https://api.enterprise.trae.cn")
	if req.Header.Get("Authorization") != "Cloud-IDE-JWT token-1" {
		t.Fatalf("Authorization = %q", req.Header.Get("Authorization"))
	}
	if req.Header.Get("X-App-Id") != traeCLIAppID || req.Header.Get("X-Ide-Function") != "chat" {
		t.Fatalf("unexpected CLI headers: %#v", req.Header)
	}
	var extra map[string]any
	if err = json.Unmarshal([]byte(req.Header.Get("Extra")), &extra); err != nil {
		t.Fatal(err)
	}
	if extra["api_key"] != "token-1" || extra["model_name"] != "glm-5__v2" || extra["session_id"] != "session-1" {
		t.Fatalf("unexpected Extra header: %#v", extra)
	}
}

func TestApplyTraeIDEHeaders(t *testing.T) {
	req, err := http.NewRequest(http.MethodPost, "https://example.com/chat", nil)
	if err != nil {
		t.Fatal(err)
	}
	applyTraeIDEHeaders(req, &traeauth.TokenStorage{Token: "token-1", UserID: "user-1"})
	for _, name := range []string{"X-Cloudide-Token", "X-Ide-Token", "X-Uid", "X-App-Id", "X-Device-Id", "X-Machine-Id", "X-Request-Id"} {
		if req.Header.Get(name) == "" {
			t.Fatalf("missing header %s", name)
		}
	}
}
