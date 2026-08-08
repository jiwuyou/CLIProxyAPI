package executor

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	traeauth "github.com/router-for-me/CLIProxyAPI/v7/internal/auth/trae"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/runtime/executor/helps"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/thinking"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/util"
	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/executor"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/usage"
	sdktranslator "github.com/router-for-me/CLIProxyAPI/v7/sdk/translator"
	log "github.com/sirupsen/logrus"
)

const (
	traeIDEAppID             = "6eefa01c-1036-4c7e-9ca5-d891f63bfcd8"
	traeIDEVersion           = "3.3.67"
	traeIDEVersionCode       = "20260401"
	traeCLIAppID             = "7b3f9dc2-8a4e-5c6d-2f1b-9e4a3c5b7df0"
	traeCLIVersionCode       = "20260206"
	traeCLIRawChatPath       = "/api/ide/v2/llm_raw_chat"
	traeToolOpenTag          = "<tool_call>"
	traeToolCloseTag         = "</tool_call>"
	traeMaxScannerBufferSize = 16 * 1024 * 1024
)

var traeChatPaths = []string{
	"/api/agent/v3/llm_utils_chat",
	"/api/ide/v1/chat",
	"/api/agent/v3/create_agent_task",
}

var traeIDEModelAliases = map[string]string{
	"auto":              "glm-5.2",
	"claude-opus-4-7":   "glm-5.2",
	"claude-opus-4-6":   "glm-5.2",
	"claude-opus-4-5":   "glm-5.2",
	"claude-sonnet-4-6": "glm-5.2",
	"claude-sonnet-4-5": "glm-5.2",
	"claude-sonnet-4":   "glm-5.2",
	"claude-3.5-sonnet": "glm-5.2",
	"claude-3.7-sonnet": "glm-5.2",
	"claude-haiku-4-5":  "glm-5.1",
	"gpt-4o":            "DeepSeek-V4-Pro",
	"gpt-4o-mini":       "DeepSeek-V4-Flash",
	"gpt-4.1":           "DeepSeek-V4-Pro",
}

type traeRawModel struct {
	ConfigName  string
	ModelName   string
	DisplayName string
}

var traeRawModels = map[string]traeRawModel{
	"auto":             {ConfigName: "glm-5.1", ModelName: "glm-5__v2", DisplayName: "GLM-5.1"},
	"coding":           {ConfigName: "glm-5.1", ModelName: "glm-5__v2", DisplayName: "GLM-5.1"},
	"glm-5.1":          {ConfigName: "glm-5.1", ModelName: "glm-5__v2", DisplayName: "GLM-5.1"},
	"kimi-k2.6":        {ConfigName: "kimi-k2.6", ModelName: "kimi-k2.6__v2", DisplayName: "Kimi-K2.6"},
	"deepseek-v4-pro":  {ConfigName: "deepseek-V4-Pro", ModelName: "deepseek-V4-Pro__v2", DisplayName: "DeepSeek-V4-Pro"},
	"gpt-4o":           {ConfigName: "deepseek-V4-Pro", ModelName: "deepseek-V4-Pro__v2", DisplayName: "DeepSeek-V4-Pro"},
	"gpt-4.1":          {ConfigName: "deepseek-V4-Pro", ModelName: "deepseek-V4-Pro__v2", DisplayName: "DeepSeek-V4-Pro"},
	"minimax-m2.7":     {ConfigName: "MiniMax-M2.7", ModelName: "MiniMax-M2.7", DisplayName: "MiniMax-M2.7"},
	"doubao-seed-code": {ConfigName: "Doubao-Seed-Code", ModelName: "Doubao-Seed-Code", DisplayName: "Doubao-Seed-Code"},
}

func resolveTraeRawModel(name string) traeRawModel {
	raw := strings.TrimSpace(strings.TrimPrefix(name, "trae/"))
	key := strings.ToLower(raw)
	switch key {
	case "", "default", "balanced", "claude-sonnet-4", "claude-sonnet-4-5", "claude-sonnet-4-6", "claude-opus-4-5", "claude-opus-4-6", "claude-opus-4-7":
		key = "auto"
	case "strong":
		key = "kimi-k2.6"
	case "fast", "gpt-4o-mini":
		key = "minimax-m2.7"
	}
	if model, ok := traeRawModels[key]; ok {
		return model
	}
	if raw == "" {
		raw = "GLM-5.1"
	}
	return traeRawModel{ConfigName: raw, ModelName: raw, DisplayName: raw}
}

// TraeExecutor executes requests against Trae CLI raw chat or the legacy IDE chat service.
type TraeExecutor struct {
	cfg *config.Config
}

func NewTraeExecutor(cfg *config.Config) *TraeExecutor {
	return &TraeExecutor{cfg: cfg}
}

func (e *TraeExecutor) Identifier() string { return traeauth.Provider }

func (e *TraeExecutor) PrepareRequest(req *http.Request, auth *cliproxyauth.Auth) error {
	if req == nil {
		return nil
	}
	credential, err := traeCredential(auth)
	if err != nil {
		return err
	}
	if credential.UsesCLIRawChat() {
		applyTraeCLIHeaders(req, credential, resolveTraeRawModel("auto"), uuid.NewString(), traeChatBaseURL(credential))
	} else {
		applyTraeIDEHeaders(req, credential)
	}
	var attrs map[string]string
	if auth != nil {
		attrs = auth.Attributes
	}
	util.ApplyCustomHeadersFromAttrs(req, attrs)
	return nil
}

func (e *TraeExecutor) HttpRequest(ctx context.Context, auth *cliproxyauth.Auth, req *http.Request) (*http.Response, error) {
	if req == nil {
		return nil, fmt.Errorf("trae executor: request is nil")
	}
	if ctx == nil {
		ctx = req.Context()
	}
	httpReq := req.WithContext(ctx)
	if err := e.PrepareRequest(httpReq, auth); err != nil {
		return nil, err
	}
	return helps.NewProxyAwareHTTPClient(ctx, e.cfg, auth, 0).Do(httpReq)
}

func (e *TraeExecutor) Execute(ctx context.Context, auth *cliproxyauth.Auth, req cliproxyexecutor.Request, opts cliproxyexecutor.Options) (resp cliproxyexecutor.Response, err error) {
	credential, err := traeCredential(auth)
	if err != nil {
		return resp, err
	}
	prepared, err := prepareTraePayloadForCredential(req, opts, false, credential)
	if err != nil {
		return resp, err
	}
	reporter := helps.NewExecutorUsageReporter(ctx, e, prepared.upstreamModel, auth)
	defer reporter.TrackFailure(ctx, &err)

	httpResp, err := e.send(ctx, auth, credential, prepared)
	if err != nil {
		return resp, err
	}
	defer func() {
		if errClose := httpResp.Body.Close(); errClose != nil {
			log.Errorf("trae executor: close response body: %v", errClose)
		}
	}()
	raw, err := io.ReadAll(httpResp.Body)
	if err != nil {
		helps.RecordAPIResponseError(ctx, e.cfg, err)
		return resp, err
	}
	helps.AppendAPIResponseChunk(ctx, e.cfg, raw)
	collected, err := collectTraeSSE(bytes.NewReader(raw), prepared.cliRaw)
	if err != nil {
		return resp, err
	}
	responseRaw, detail, err := buildTraeOpenAIResponse(collected, req.Model, prepared.inputTokens)
	if err != nil {
		return resp, err
	}
	reporter.Publish(ctx, detail)
	var param any
	responseFormat := cliproxyexecutor.ResponseFormatOrSource(opts)
	out := sdktranslator.TranslateNonStream(ctx, sdktranslator.FormatOpenAI, responseFormat, req.Model, opts.OriginalRequest, prepared.openAIRequest, responseRaw, &param)
	return cliproxyexecutor.Response{Payload: out, Headers: httpResp.Header.Clone()}, nil
}

func (e *TraeExecutor) ExecuteStream(ctx context.Context, auth *cliproxyauth.Auth, req cliproxyexecutor.Request, opts cliproxyexecutor.Options) (_ *cliproxyexecutor.StreamResult, err error) {
	credential, err := traeCredential(auth)
	if err != nil {
		return nil, err
	}
	prepared, err := prepareTraePayloadForCredential(req, opts, true, credential)
	if err != nil {
		return nil, err
	}
	reporter := helps.NewExecutorUsageReporter(ctx, e, prepared.upstreamModel, auth)
	defer reporter.TrackFailure(ctx, &err)
	httpResp, err := e.send(ctx, auth, credential, prepared)
	if err != nil {
		return nil, err
	}

	out := make(chan cliproxyexecutor.StreamChunk)
	go func() {
		defer close(out)
		defer func() {
			if errClose := httpResp.Body.Close(); errClose != nil {
				log.Errorf("trae executor: close stream body: %v", errClose)
			}
		}()
		collected, errCollect := collectTraeSSE(io.TeeReader(httpResp.Body, &responseLogWriter{ctx: ctx, cfg: e.cfg}), prepared.cliRaw)
		if errCollect != nil {
			helps.RecordAPIResponseError(ctx, e.cfg, errCollect)
			reporter.PublishFailure(ctx)
			out <- cliproxyexecutor.StreamChunk{Err: errCollect}
			return
		}
		lines, detail, errBuild := buildTraeOpenAIStream(collected, req.Model, prepared.inputTokens)
		if errBuild != nil {
			reporter.PublishFailure(ctx)
			out <- cliproxyexecutor.StreamChunk{Err: errBuild}
			return
		}
		reporter.Publish(ctx, detail)
		responseFormat := cliproxyexecutor.ResponseFormatOrSource(opts)
		var param any
		for _, line := range lines {
			chunks := sdktranslator.TranslateStream(ctx, sdktranslator.FormatOpenAI, responseFormat, req.Model, opts.OriginalRequest, prepared.openAIRequest, line, &param)
			for _, chunk := range chunks {
				out <- cliproxyexecutor.StreamChunk{Payload: append([]byte(nil), chunk...)}
			}
		}
	}()
	return &cliproxyexecutor.StreamResult{Headers: httpResp.Header.Clone(), Chunks: out}, nil
}

func (e *TraeExecutor) Refresh(ctx context.Context, auth *cliproxyauth.Auth) (*cliproxyauth.Auth, error) {
	credential, err := traeCredential(auth)
	if err != nil {
		return nil, err
	}
	updatedStorage, err := traeauth.Refresh(ctx, e.cfg, credential)
	if err != nil {
		return nil, err
	}
	updated := auth.Clone()
	if updated.Metadata == nil {
		updated.Metadata = make(map[string]any)
	}
	updated.Storage = updatedStorage
	updated.Metadata["auth_kind"] = updatedStorage.AuthKind
	updated.Metadata["token"] = updatedStorage.Token
	updated.Metadata["access_token"] = updatedStorage.Token
	updated.Metadata["personal_access_token"] = updatedStorage.PersonalAccessToken
	updated.Metadata["refresh_token"] = updatedStorage.RefreshToken
	updated.Metadata["expired"] = updatedStorage.ExpiredAt
	updated.Metadata["refresh_expired"] = updatedStorage.RefreshExpiredAt
	updated.Metadata["auth_base_url"] = updatedStorage.AuthBaseURL
	updated.Metadata["chat_base_url"] = updatedStorage.ChatBaseURL
	now := time.Now()
	updated.UpdatedAt = now
	updated.LastRefreshedAt = now
	return updated, nil
}

func (e *TraeExecutor) CountTokens(_ context.Context, _ *cliproxyauth.Auth, req cliproxyexecutor.Request, opts cliproxyexecutor.Options) (cliproxyexecutor.Response, error) {
	payload := req.Payload
	if opts.SourceFormat != "" && opts.SourceFormat != sdktranslator.FormatOpenAI {
		payload = sdktranslator.TranslateRequest(opts.SourceFormat, sdktranslator.FormatOpenAI, req.Model, payload, false)
	}
	codec, err := helps.TokenizerForModel(req.Model)
	if err != nil {
		return cliproxyexecutor.Response{}, err
	}
	count, err := helps.CountOpenAIChatTokens(codec, payload)
	if err != nil {
		return cliproxyexecutor.Response{}, err
	}
	return cliproxyexecutor.Response{Payload: helps.BuildOpenAIUsageJSON(count)}, nil
}

type traePreparedPayload struct {
	body          []byte
	openAIRequest []byte
	upstreamModel string
	inputTokens   int64
	cliRaw        bool
	sessionID     string
	rawModel      traeRawModel
}

func prepareTraePayloadForCredential(req cliproxyexecutor.Request, opts cliproxyexecutor.Options, stream bool, credential *traeauth.TokenStorage) (*traePreparedPayload, error) {
	if credential != nil && credential.UsesCLIRawChat() {
		return prepareTraePayload(req, opts, stream)
	}
	return prepareTraeIDEPayload(req, opts, stream)
}

// prepareTraePayload builds Trae CLI's raw-chat request payload.
func prepareTraePayload(req cliproxyexecutor.Request, opts cliproxyexecutor.Options, stream bool) (*traePreparedPayload, error) {
	baseModel := thinking.ParseSuffix(req.Model).ModelName
	openAIRequest := req.Payload
	if opts.SourceFormat != "" && opts.SourceFormat != sdktranslator.FormatOpenAI {
		openAIRequest = sdktranslator.TranslateRequest(opts.SourceFormat, sdktranslator.FormatOpenAI, baseModel, req.Payload, stream)
	}
	var root map[string]any
	if err := json.Unmarshal(openAIRequest, &root); err != nil {
		return nil, fmt.Errorf("trae: parse OpenAI request: %w", err)
	}
	model := strings.TrimPrefix(strings.TrimSpace(baseModel), "trae/")
	if requested, ok := root["model"].(string); ok && strings.TrimSpace(requested) != "" {
		model = strings.TrimPrefix(strings.TrimSpace(requested), "trae/")
	}
	messages, err := buildTraeMessages(root)
	if err != nil {
		return nil, err
	}
	sessionID := uuid.NewString()
	rawModel := resolveTraeRawModel(model)
	body := map[string]any{
		"config_name":     rawModel.ConfigName,
		"conversation_id": sessionID,
		"messages":        messages,
		"model_name":      rawModel.ModelName,
		"session_id":      sessionID,
		"stream":          true,
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("trae: encode request: %w", err)
	}
	var inputTokens int64
	if codec, errCodec := helps.TokenizerForModel(rawModel.DisplayName); errCodec == nil {
		if count, errCount := helps.CountOpenAIChatTokens(codec, openAIRequest); errCount == nil {
			inputTokens = count
		}
	}
	return &traePreparedPayload{
		body: raw, openAIRequest: openAIRequest, upstreamModel: rawModel.DisplayName, inputTokens: inputTokens,
		cliRaw: true, sessionID: sessionID, rawModel: rawModel,
	}, nil
}

func prepareTraeIDEPayload(req cliproxyexecutor.Request, opts cliproxyexecutor.Options, stream bool) (*traePreparedPayload, error) {
	baseModel := thinking.ParseSuffix(req.Model).ModelName
	openAIRequest := req.Payload
	if opts.SourceFormat != "" && opts.SourceFormat != sdktranslator.FormatOpenAI {
		openAIRequest = sdktranslator.TranslateRequest(opts.SourceFormat, sdktranslator.FormatOpenAI, baseModel, req.Payload, stream)
	}
	var root map[string]any
	if err := json.Unmarshal(openAIRequest, &root); err != nil {
		return nil, fmt.Errorf("trae: parse OpenAI request: %w", err)
	}
	model := strings.TrimPrefix(strings.TrimSpace(baseModel), "trae/")
	if requested, ok := root["model"].(string); ok && strings.TrimSpace(requested) != "" {
		model = strings.TrimPrefix(strings.TrimSpace(requested), "trae/")
	}
	if mapped := traeIDEModelAliases[model]; mapped != "" {
		model = mapped
	}
	messages, err := buildTraeMessages(root)
	if err != nil {
		return nil, err
	}
	sessionID := uuid.NewString()
	body := map[string]any{
		"messages": messages, "model": model, "function": "inline_chat", "stream": true,
		"request_id": sessionID, "session_id": sessionID,
	}
	if maxTokens, ok := numericValue(root["max_tokens"]); ok && maxTokens > 0 {
		body["max_tokens"] = maxTokens
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("trae: encode request: %w", err)
	}
	var inputTokens int64
	if codec, errCodec := helps.TokenizerForModel(model); errCodec == nil {
		if count, errCount := helps.CountOpenAIChatTokens(codec, openAIRequest); errCount == nil {
			inputTokens = count
		}
	}
	return &traePreparedPayload{body: raw, openAIRequest: openAIRequest, upstreamModel: model, inputTokens: inputTokens, sessionID: sessionID}, nil
}

func buildTraeMessages(root map[string]any) ([]map[string]any, error) {
	rawMessages, ok := root["messages"].([]any)
	if !ok || len(rawMessages) == 0 {
		return nil, fmt.Errorf("trae: messages are required")
	}
	systemParts := make([]string, 0)
	if toolPrompt := traeToolsPrompt(root["tools"]); toolPrompt != "" {
		systemParts = append(systemParts, toolPrompt)
	}
	messages := make([]map[string]any, 0, len(rawMessages)+1)
	for _, rawMessage := range rawMessages {
		message, ok := rawMessage.(map[string]any)
		if !ok {
			continue
		}
		role, _ := message["role"].(string)
		content := flattenTraeContent(message["content"])
		if role == "assistant" {
			content = strings.TrimSpace(strings.Join([]string{content, renderOpenAIToolCalls(message["tool_calls"])}, "\n\n"))
		}
		switch role {
		case "system", "developer":
			if content != "" {
				systemParts = append(systemParts, content)
			}
			continue
		case "tool":
			name, _ := message["name"].(string)
			content = fmt.Sprintf("[Tool Result: %s]\n%s", name, content)
			role = "user"
		case "assistant", "user":
		default:
			role = "user"
		}
		if content == "" {
			continue
		}
		messages = append(messages, map[string]any{
			"role":    role,
			"content": []map[string]any{{"type": "text", "text": content}},
		})
	}
	if len(systemParts) > 0 {
		system := map[string]any{
			"role":    "system",
			"content": []map[string]any{{"type": "text", "text": strings.Join(systemParts, "\n\n")}},
		}
		messages = append([]map[string]any{system}, messages...)
	}
	if len(messages) == 0 {
		return nil, fmt.Errorf("trae: request did not contain usable messages")
	}
	return messages, nil
}

func traeToolsPrompt(raw any) string {
	tools, ok := raw.([]any)
	if !ok || len(tools) == 0 {
		return ""
	}
	var out strings.Builder
	out.WriteString("You have access to the following tools. To call one, output exactly:\n")
	out.WriteString(traeToolOpenTag + "\n")
	out.WriteString(`{"name":"tool_name","arguments":{"parameter":"value"}}`)
	out.WriteString("\n" + traeToolCloseTag + "\n\nAvailable tools:")
	for _, rawTool := range tools {
		tool, ok := rawTool.(map[string]any)
		if !ok {
			continue
		}
		function, _ := tool["function"].(map[string]any)
		if function == nil {
			function = tool
		}
		name, _ := function["name"].(string)
		if name == "" {
			continue
		}
		description, _ := function["description"].(string)
		parameters, _ := json.Marshal(function["parameters"])
		out.WriteString("\n\n- " + name)
		if description != "" {
			out.WriteString(": " + description)
		}
		if len(parameters) > 0 && string(parameters) != "null" {
			out.WriteString("\n  Parameters: " + string(parameters))
		}
	}
	return out.String()
}

func flattenTraeContent(raw any) string {
	switch value := raw.(type) {
	case string:
		return value
	case []any:
		parts := make([]string, 0, len(value))
		for _, rawPart := range value {
			part, ok := rawPart.(map[string]any)
			if !ok {
				continue
			}
			partType, _ := part["type"].(string)
			switch partType {
			case "text", "input_text", "output_text":
				if text, _ := part["text"].(string); text != "" {
					parts = append(parts, text)
				}
			case "tool_result":
				parts = append(parts, "[Tool Result]\n"+flattenTraeContent(part["content"]))
			case "image_url", "image":
				parts = append(parts, "[Image]")
			}
		}
		return strings.Join(parts, "\n")
	default:
		return ""
	}
}

func renderOpenAIToolCalls(raw any) string {
	calls, ok := raw.([]any)
	if !ok {
		return ""
	}
	parts := make([]string, 0, len(calls))
	for _, rawCall := range calls {
		call, ok := rawCall.(map[string]any)
		if !ok {
			continue
		}
		function, _ := call["function"].(map[string]any)
		name, _ := function["name"].(string)
		arguments, _ := function["arguments"].(string)
		if name != "" {
			parts = append(parts, fmt.Sprintf("[Called tool: %s]\n%s", name, arguments))
		}
	}
	return strings.Join(parts, "\n\n")
}

func (e *TraeExecutor) send(ctx context.Context, auth *cliproxyauth.Auth, credential *traeauth.TokenStorage, prepared *traePreparedPayload) (*http.Response, error) {
	if prepared == nil {
		return nil, fmt.Errorf("trae: prepared request is nil")
	}
	if prepared.cliRaw {
		return e.sendCLI(ctx, auth, credential, prepared)
	}
	return e.sendIDE(ctx, auth, credential, prepared.body)
}

func (e *TraeExecutor) sendCLI(ctx context.Context, auth *cliproxyauth.Auth, credential *traeauth.TokenStorage, prepared *traePreparedPayload) (*http.Response, error) {
	baseURL := traeChatBaseURL(credential)
	url := baseURL + traeCLIRawChatPath
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(prepared.body))
	if err != nil {
		return nil, err
	}
	applyTraeCLIHeaders(req, credential, prepared.rawModel, prepared.sessionID, baseURL)
	if auth != nil {
		util.ApplyCustomHeadersFromAttrs(req, auth.Attributes)
	}
	e.recordTraeRequest(ctx, auth, req, prepared.body)
	resp, err := helps.NewProxyAwareHTTPClient(ctx, e.cfg, auth, 0).Do(req)
	if err != nil {
		helps.RecordAPIResponseError(ctx, e.cfg, err)
		return nil, err
	}
	helps.RecordAPIResponseMetadata(ctx, e.cfg, resp.StatusCode, resp.Header.Clone())
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return resp, nil
	}
	raw, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	helps.AppendAPIResponseChunk(ctx, e.cfg, raw)
	return nil, statusErr{code: resp.StatusCode, msg: string(raw)}
}

func (e *TraeExecutor) sendIDE(ctx context.Context, auth *cliproxyauth.Auth, credential *traeauth.TokenStorage, body []byte) (*http.Response, error) {
	baseURL := traeChatBaseURL(credential)
	client := helps.NewProxyAwareHTTPClient(ctx, e.cfg, auth, 0)
	var lastStatus int
	var lastBody []byte
	for _, path := range traeChatPaths {
		url := baseURL + path
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		applyTraeIDEHeaders(req, credential)
		if auth != nil {
			util.ApplyCustomHeadersFromAttrs(req, auth.Attributes)
		}
		e.recordTraeRequest(ctx, auth, req, body)
		resp, err := client.Do(req)
		if err != nil {
			helps.RecordAPIResponseError(ctx, e.cfg, err)
			continue
		}
		helps.RecordAPIResponseMetadata(ctx, e.cfg, resp.StatusCode, resp.Header.Clone())
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return resp, nil
		}
		lastStatus = resp.StatusCode
		lastBody, _ = io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		helps.AppendAPIResponseChunk(ctx, e.cfg, lastBody)
	}
	if lastStatus == 0 {
		return nil, fmt.Errorf("trae: all chat endpoints failed")
	}
	return nil, statusErr{code: lastStatus, msg: string(lastBody)}
}

func (e *TraeExecutor) recordTraeRequest(ctx context.Context, auth *cliproxyauth.Auth, req *http.Request, body []byte) {
	authID, authLabel, authType, authValue := "", "", "", ""
	if auth != nil {
		authID, authLabel = auth.ID, auth.Label
		authType, authValue = auth.AccountInfo()
	}
	helps.RecordAPIRequest(ctx, e.cfg, helps.UpstreamRequestLog{
		URL: req.URL.String(), Method: req.Method, Headers: req.Header.Clone(), Body: body,
		Provider: e.Identifier(), AuthID: authID, AuthLabel: authLabel, AuthType: authType, AuthValue: authValue,
	})
}

func traeChatBaseURL(credential *traeauth.TokenStorage) string {
	if credential == nil {
		return traeauth.DefaultCLIBaseURL
	}
	baseURL := strings.TrimRight(strings.TrimSpace(credential.ChatBaseURL), "/")
	if baseURL != "" {
		return baseURL
	}
	if credential.UsesCLIRawChat() {
		return traeauth.DefaultCLIBaseURL
	}
	return traeauth.DefaultChatBaseURL(credential.Edition)
}

func applyTraeCLIHeaders(req *http.Request, credential *traeauth.TokenStorage, model traeRawModel, sessionID, baseURL string) {
	extra, _ := json.Marshal(map[string]any{
		"agent_loop_id": sessionID, "api_host": baseURL, "api_key": credential.Token,
		"base_url": baseURL + "/trae-cli/api/v1/llm/proxy", "config_name": model.ConfigName,
		"config_source": 1, "display_name": model.DisplayName, "model_name": model.ModelName,
		"real_api_key": "", "real_base_url": "", "session_id": sessionID,
		"user_prompt_submit_id": sessionID,
	})
	req.Header.Set("Authorization", "Cloud-IDE-JWT "+credential.Token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("X-App-Id", traeCLIAppID)
	req.Header.Set("X-Ide-Version-Code", traeCLIVersionCode)
	req.Header.Set("X-Ide-Function", "chat")
	req.Header.Set("Extra", string(extra))
}

func applyTraeIDEHeaders(req *http.Request, credential *traeauth.TokenStorage) {
	machineID := strings.ReplaceAll(uuid.NewString(), "-", "") + strings.ReplaceAll(uuid.NewString(), "-", "")
	deviceHash := sha256.Sum256([]byte(machineID))
	req.Header.Set("Authorization", "Cloud-IDE-JWT "+credential.Token)
	req.Header.Set("X-Cloudide-Token", credential.Token)
	req.Header.Set("x-ide-token", credential.Token)
	req.Header.Set("x-uid", credential.UserID)
	req.Header.Set("x-app-id", traeIDEAppID)
	req.Header.Set("x-device-id", fmt.Sprintf("%x", deviceHash[:16]))
	req.Header.Set("x-machine-id", machineID)
	req.Header.Set("x-request-id", uuid.NewString())
	req.Header.Set("x-ide-version", traeIDEVersion)
	req.Header.Set("x-ide-version-code", traeIDEVersionCode)
	req.Header.Set("x-device-type", "windows")
	req.Header.Set("x-os-version", "Windows 10")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
}

func traeCredential(auth *cliproxyauth.Auth) (*traeauth.TokenStorage, error) {
	if auth == nil {
		return nil, fmt.Errorf("trae: missing auth")
	}
	if storage, ok := auth.Storage.(*traeauth.TokenStorage); ok && strings.TrimSpace(storage.Token) != "" {
		storage.AuthKind = traeauth.NormalizeAuthKind(storage.AuthKind, storage.PersonalAccessToken)
		return storage, nil
	}
	credential := &traeauth.TokenStorage{
		Type:                traeauth.Provider,
		AuthKind:            metaStringValue(auth.Metadata, "auth_kind"),
		Edition:             metaStringValue(auth.Metadata, "edition"),
		Token:               metaStringValue(auth.Metadata, "token"),
		PersonalAccessToken: metaStringValue(auth.Metadata, "personal_access_token"),
		RefreshToken:        metaStringValue(auth.Metadata, "refresh_token"),
		UserID:              metaStringValue(auth.Metadata, "user_id"),
		Email:               metaStringValue(auth.Metadata, "email"),
		Username:            metaStringValue(auth.Metadata, "username"),
		ExpiredAt:           metaStringValue(auth.Metadata, "expired"),
		RefreshExpiredAt:    metaStringValue(auth.Metadata, "refresh_expired"),
		AuthBaseURL:         metaStringValue(auth.Metadata, "auth_base_url"),
		ChatBaseURL:         metaStringValue(auth.Metadata, "chat_base_url"),
	}
	credential.AuthKind = traeauth.NormalizeAuthKind(credential.AuthKind, credential.PersonalAccessToken)
	if credential.Token == "" {
		credential.Token = metaStringValue(auth.Metadata, "access_token")
	}
	credential.Edition = traeauth.NormalizeEdition(credential.Edition)
	if credential.Token == "" {
		return nil, fmt.Errorf("trae: credential token is missing")
	}
	return credential, nil
}

type traeCollectedResponse struct {
	Content      string
	Reasoning    string
	FinishReason string
	ToolCalls    []traeToolCall
	InputTokens  int64
	OutputTokens int64
	TotalTokens  int64
}

func collectTraeSSE(reader io.Reader, cumulativeOutput bool) (traeCollectedResponse, error) {
	var result traeCollectedResponse
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(nil, traeMaxScannerBufferSize)
	event := ""
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			event = ""
			continue
		}
		if strings.HasPrefix(line, "event:") {
			event = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			continue
		}
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" || data == "[DONE]" {
			continue
		}
		var value map[string]any
		if err := json.Unmarshal([]byte(data), &value); err != nil {
			continue
		}
		if event == "error" {
			message := metaStringValue(value, "message")
			if message == "" {
				message = metaStringValue(value, "error")
			}
			if message == "" {
				message = data
			}
			return result, fmt.Errorf("trae: raw chat stream error: %s", message)
		}
		if event == "token_usage" {
			result.InputTokens = traeUsageNumber(value, "input_tokens", "inputTokens", "prompt_tokens")
			result.OutputTokens = traeUsageNumber(value, "output_tokens", "outputTokens", "completion_tokens")
			result.TotalTokens = traeUsageNumber(value, "total_tokens", "totalTokens")
			if result.TotalTokens == 0 {
				result.TotalTokens = result.InputTokens + result.OutputTokens
			}
			continue
		}
		if response, _ := value["response"].(string); response != "" {
			if event == "output" && cumulativeOutput {
				result.Content = response
			} else {
				result.Content += response
			}
		}
		if reasoning, _ := value["reasoning_content"].(string); reasoning != "" {
			if event == "output" && cumulativeOutput {
				result.Reasoning = reasoning
			} else {
				result.Reasoning += reasoning
			}
		}
		if finish, _ := value["finish_reason"].(string); finish != "" {
			result.FinishReason = finish
		}
		if choices, ok := value["choices"].([]any); ok && len(choices) > 0 {
			if choice, okChoice := choices[0].(map[string]any); okChoice {
				if delta, okDelta := choice["delta"].(map[string]any); okDelta {
					if content, _ := delta["content"].(string); content != "" {
						result.Content += content
					}
					if reasoning, _ := delta["reasoning_content"].(string); reasoning != "" {
						result.Reasoning += reasoning
					}
				}
				if finish, _ := choice["finish_reason"].(string); finish != "" {
					result.FinishReason = finish
				}
			}
		}
		if event == "output" {
			if calls := normalizeTraeRawToolCalls(value["tool_calls"]); len(calls) > 0 {
				result.ToolCalls = mergeTraeRawToolCalls(result.ToolCalls, calls)
				result.FinishReason = "tool_calls"
			}
		}
		if event == "done" && result.FinishReason == "" {
			result.FinishReason = "stop"
		}
	}
	if err := scanner.Err(); err != nil {
		return result, fmt.Errorf("trae: read SSE stream: %w", err)
	}
	if result.FinishReason == "" {
		result.FinishReason = "stop"
	}
	return result, nil
}

func traeUsageNumber(value map[string]any, keys ...string) int64 {
	for _, key := range keys {
		if number, ok := numericValue(value[key]); ok {
			return number
		}
	}
	return 0
}

type traeToolCall struct {
	Index     int
	ID        string
	Name      string
	Arguments string
}

func normalizeTraeRawToolCalls(raw any) []traeToolCall {
	values, ok := raw.([]any)
	if !ok {
		return nil
	}
	calls := make([]traeToolCall, 0, len(values))
	for _, item := range values {
		record, ok := item.(map[string]any)
		if !ok {
			continue
		}
		function, _ := record["function"].(map[string]any)
		if function == nil {
			function = record
		}
		name, _ := function["name"].(string)
		arguments := function["arguments"]
		if arguments == nil {
			arguments = record["arguments"]
		}
		if arguments == nil {
			arguments = record["input"]
		}
		argumentText, _ := arguments.(string)
		if argumentText == "" && arguments != nil {
			if encoded, err := json.Marshal(arguments); err == nil {
				argumentText = string(encoded)
			}
		}
		if name == "" && argumentText == "" {
			continue
		}
		id, _ := record["id"].(string)
		if id == "" {
			id, _ = record["tool_call_id"].(string)
		}
		index := len(calls)
		if value, ok := numericValue(record["index"]); ok {
			index = int(value)
		}
		calls = append(calls, traeToolCall{Index: index, ID: id, Name: name, Arguments: argumentText})
	}
	return calls
}

func mergeTraeRawToolCalls(existing, next []traeToolCall) []traeToolCall {
	merged := append([]traeToolCall(nil), existing...)
	for _, candidate := range next {
		match := -1
		for index := range merged {
			if candidate.ID != "" && merged[index].ID == candidate.ID {
				match = index
				break
			}
			if merged[index].Index == candidate.Index {
				match = index
				break
			}
		}
		if match < 0 {
			merged = append(merged, candidate)
			continue
		}
		if candidate.Name != "" {
			merged[match].Name = candidate.Name
		}
		if candidate.Arguments != "" {
			if strings.HasPrefix(candidate.Arguments, merged[match].Arguments) {
				merged[match].Arguments = candidate.Arguments
			} else {
				merged[match].Arguments += candidate.Arguments
			}
		}
	}
	return merged
}

func parseTraeToolCalls(content string) (string, []traeToolCall) {
	var text strings.Builder
	calls := make([]traeToolCall, 0)
	rest := content
	for {
		start := strings.Index(rest, traeToolOpenTag)
		if start < 0 {
			text.WriteString(rest)
			break
		}
		text.WriteString(rest[:start])
		afterOpen := rest[start+len(traeToolOpenTag):]
		end := strings.Index(afterOpen, traeToolCloseTag)
		if end < 0 {
			text.WriteString(rest[start:])
			break
		}
		payload := strings.TrimSpace(afterOpen[:end])
		var call struct {
			Name       string          `json:"name"`
			Arguments  json.RawMessage `json:"arguments"`
			Input      json.RawMessage `json:"input"`
			Parameters json.RawMessage `json:"parameters"`
		}
		if json.Unmarshal([]byte(payload), &call) == nil && call.Name != "" {
			arguments := call.Arguments
			if len(arguments) == 0 {
				arguments = call.Input
			}
			if len(arguments) == 0 {
				arguments = call.Parameters
			}
			if len(arguments) == 0 {
				arguments = json.RawMessage("{}")
			}
			calls = append(calls, traeToolCall{
				ID:        "call_" + strings.ReplaceAll(uuid.NewString(), "-", ""),
				Name:      call.Name,
				Arguments: string(arguments),
			})
		} else {
			text.WriteString(rest[start : start+len(traeToolOpenTag)+end+len(traeToolCloseTag)])
		}
		rest = afterOpen[end+len(traeToolCloseTag):]
	}
	return strings.TrimSpace(text.String()), calls
}

func buildTraeOpenAIResponse(collected traeCollectedResponse, model string, inputTokens int64) ([]byte, usage.Detail, error) {
	content, textCalls := parseTraeToolCalls(collected.Content)
	calls := finalizeTraeToolCalls(append(append([]traeToolCall(nil), collected.ToolCalls...), textCalls...))
	detail := traeUsageDetail(collected, inputTokens, content+collected.Reasoning)
	message := map[string]any{"role": "assistant", "content": content}
	if collected.Reasoning != "" {
		message["reasoning_content"] = collected.Reasoning
	}
	finish := collected.FinishReason
	if len(calls) > 0 {
		finish = "tool_calls"
		toolCalls := make([]map[string]any, 0, len(calls))
		for _, call := range calls {
			toolCalls = append(toolCalls, map[string]any{
				"id": call.ID, "type": "function",
				"function": map[string]any{"name": call.Name, "arguments": call.Arguments},
			})
		}
		message["tool_calls"] = toolCalls
	}
	response := map[string]any{
		"id":     "chatcmpl-trae-" + strings.ReplaceAll(uuid.NewString(), "-", ""),
		"object": "chat.completion", "created": time.Now().Unix(), "model": model,
		"choices": []map[string]any{{"index": 0, "message": message, "finish_reason": finish}},
		"usage":   map[string]any{"prompt_tokens": detail.InputTokens, "completion_tokens": detail.OutputTokens, "total_tokens": detail.TotalTokens},
	}
	raw, err := json.Marshal(response)
	return raw, detail, err
}

func buildTraeOpenAIStream(collected traeCollectedResponse, model string, inputTokens int64) ([][]byte, usage.Detail, error) {
	content, textCalls := parseTraeToolCalls(collected.Content)
	calls := finalizeTraeToolCalls(append(append([]traeToolCall(nil), collected.ToolCalls...), textCalls...))
	detail := traeUsageDetail(collected, inputTokens, content+collected.Reasoning)
	id := "chatcmpl-trae-" + strings.ReplaceAll(uuid.NewString(), "-", "")
	created := time.Now().Unix()
	lines := make([][]byte, 0, 6+len(calls))
	appendChunk := func(delta map[string]any, finish any, usageValue any) error {
		chunk := map[string]any{
			"id": id, "object": "chat.completion.chunk", "created": created, "model": model,
			"choices": []map[string]any{{"index": 0, "delta": delta, "finish_reason": finish}},
		}
		if usageValue != nil {
			chunk["usage"] = usageValue
		}
		raw, err := json.Marshal(chunk)
		if err != nil {
			return err
		}
		lines = append(lines, append([]byte("data: "), append(raw, '\n', '\n')...))
		return nil
	}
	if err := appendChunk(map[string]any{"role": "assistant"}, nil, nil); err != nil {
		return nil, detail, err
	}
	if collected.Reasoning != "" {
		if err := appendChunk(map[string]any{"reasoning_content": collected.Reasoning}, nil, nil); err != nil {
			return nil, detail, err
		}
	}
	if content != "" {
		if err := appendChunk(map[string]any{"content": content}, nil, nil); err != nil {
			return nil, detail, err
		}
	}
	for index, call := range calls {
		delta := map[string]any{"tool_calls": []map[string]any{{
			"index": index, "id": call.ID, "type": "function",
			"function": map[string]any{"name": call.Name, "arguments": call.Arguments},
		}}}
		if err := appendChunk(delta, nil, nil); err != nil {
			return nil, detail, err
		}
	}
	finish := collected.FinishReason
	if len(calls) > 0 {
		finish = "tool_calls"
	}
	if err := appendChunk(map[string]any{}, finish, nil); err != nil {
		return nil, detail, err
	}
	if err := appendChunk(map[string]any{}, finish, map[string]any{
		"prompt_tokens": detail.InputTokens, "completion_tokens": detail.OutputTokens, "total_tokens": detail.TotalTokens,
	}); err != nil {
		return nil, detail, err
	}
	lines = append(lines, []byte("data: [DONE]\n\n"))
	return lines, detail, nil
}

func finalizeTraeToolCalls(calls []traeToolCall) []traeToolCall {
	for index := range calls {
		if calls[index].ID == "" {
			calls[index].ID = "call_" + strings.ReplaceAll(uuid.NewString(), "-", "")
		}
		if calls[index].Arguments == "" {
			calls[index].Arguments = "{}"
		}
	}
	return calls
}

func traeUsageDetail(collected traeCollectedResponse, fallbackInput int64, outputText string) usage.Detail {
	inputTokens := collected.InputTokens
	if inputTokens == 0 {
		inputTokens = fallbackInput
	}
	outputTokens := collected.OutputTokens
	if outputTokens == 0 {
		outputTokens = estimateTraeTokens(outputText)
	}
	totalTokens := collected.TotalTokens
	if totalTokens == 0 {
		totalTokens = inputTokens + outputTokens
	}
	return usage.Detail{InputTokens: inputTokens, OutputTokens: outputTokens, TotalTokens: totalTokens}
}

func estimateTraeTokens(text string) int64 {
	if text == "" {
		return 0
	}
	var estimate float64
	for len(text) > 0 {
		r, size := utf8.DecodeRuneInString(text)
		text = text[size:]
		if r > 0x2000 {
			estimate += 1.5
		} else {
			estimate += 0.25
		}
	}
	return int64(estimate + 0.999)
}

func numericValue(value any) (int64, bool) {
	switch typed := value.(type) {
	case float64:
		return int64(typed), true
	case int:
		return int64(typed), true
	case int64:
		return typed, true
	case json.Number:
		parsed, err := typed.Int64()
		return parsed, err == nil
	default:
		return 0, false
	}
}

type responseLogWriter struct {
	ctx context.Context
	cfg *config.Config
}

func (w *responseLogWriter) Write(p []byte) (int, error) {
	helps.AppendAPIResponseChunk(w.ctx, w.cfg, p)
	return len(p), nil
}
