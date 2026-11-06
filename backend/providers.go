package main

// providers.go: calling LLM APIs over plain HTTP.
//
// There is one function per provider. Each one takes the same LLMRequest and
// returns the same LLMResponse, so the runner doesn't care who it is talking to.
// No SDKs are used: every request and response shape is visible right here.

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type LLMRequest struct {
	Model       string
	System      string
	User        string
	Temperature *float64
	MaxTokens   int
	Images      []Image // attached to the user message (see images.go)
}

type LLMResponse struct {
	Text         string
	InputTokens  int
	OutputTokens int
	LatencyMs    int64
}

var httpClient = &http.Client{} // timeouts come from each request's context

// callLLM sends one prompt to the right provider and measures latency.
func callLLM(ctx context.Context, provider ProviderConfig, req LLMRequest) (LLMResponse, error) {
	if req.MaxTokens <= 0 {
		req.MaxTokens = 4096
	}
	start := time.Now()
	var resp LLMResponse
	var err error
	switch provider.Kind {
	case "openai", "openai_compatible":
		resp, err = callOpenAI(ctx, provider, req)
	case "anthropic":
		resp, err = callAnthropic(ctx, provider, req)
	case "gemini":
		resp, err = callGemini(ctx, provider, req)
	case "ollama":
		resp, err = callOllama(ctx, provider, req)
	case "typesafe":
		err = errors.New(tr("o TypeSafe só tem modelos de decisão: use-o em avaliadores do tipo “Motor de decisão”", "TypeSafe only has decision models: use it in “Decision engine” evaluators"))
	default:
		err = fmt.Errorf(tr("tipo de provedor desconhecido: %q", "unknown provider kind: %q"), provider.Kind)
	}
	resp.LatencyMs = time.Since(start).Milliseconds()
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		err = errors.New(tr("tempo limite excedido", "timed out"))
	}
	return resp, err
}

// ---------- OpenAI (and any OpenAI-compatible server) ----------

func callOpenAI(ctx context.Context, p ProviderConfig, req LLMRequest) (LLMResponse, error) {
	messages := []map[string]any{}
	if req.System != "" {
		messages = append(messages, map[string]any{"role": "system", "content": req.System})
	}
	if len(req.Images) == 0 {
		messages = append(messages, map[string]any{"role": "user", "content": req.User})
	} else {
		// With images, the content becomes a list of parts. Images go as data URLs.
		parts := []any{map[string]any{"type": "text", "text": req.User}}
		for _, img := range req.Images {
			dataURL := "data:" + img.MediaType + ";base64," + base64.StdEncoding.EncodeToString(img.Data)
			parts = append(parts, map[string]any{"type": "image_url", "image_url": map[string]string{"url": dataURL}})
		}
		messages = append(messages, map[string]any{"role": "user", "content": parts})
	}

	body := map[string]any{"model": req.Model, "messages": messages}
	if p.Kind == "openai" {
		body["max_completion_tokens"] = req.MaxTokens // newer OpenAI name
	} else {
		body["max_tokens"] = req.MaxTokens // what most compatible servers expect
	}
	if req.Temperature != nil {
		body["temperature"] = *req.Temperature
	}

	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
		} `json:"usage"`
	}
	headers := map[string]string{}
	if key := apiKey(p); key != "" {
		headers["Authorization"] = "Bearer " + key
	}
	if err := postJSON(ctx, baseURL(p, "https://api.openai.com")+"/v1/chat/completions", headers, body, &out); err != nil {
		return LLMResponse{}, err
	}
	if len(out.Choices) == 0 {
		return LLMResponse{}, errors.New(tr("resposta vazia do provedor", "empty answer from the provider"))
	}
	return LLMResponse{
		Text:         out.Choices[0].Message.Content,
		InputTokens:  out.Usage.PromptTokens,
		OutputTokens: out.Usage.CompletionTokens,
	}, nil
}

// ---------- Anthropic (Claude) ----------

func callAnthropic(ctx context.Context, p ProviderConfig, req LLMRequest) (LLMResponse, error) {
	// Images first, then the text: Claude works best with that order.
	content := []any{}
	for _, img := range req.Images {
		content = append(content, map[string]any{
			"type":   "image",
			"source": map[string]string{"type": "base64", "media_type": img.MediaType, "data": base64.StdEncoding.EncodeToString(img.Data)},
		})
	}
	content = append(content, map[string]any{"type": "text", "text": req.User})
	body := map[string]any{
		"model":      req.Model,
		"max_tokens": req.MaxTokens,
		"messages":   []map[string]any{{"role": "user", "content": content}},
	}
	if req.System != "" {
		body["system"] = req.System
	}
	// Current Claude models reject sampling parameters (temperature/top_p),
	// so temperature is intentionally not sent to Anthropic.

	var out struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		StopReason string `json:"stop_reason"`
		Usage      struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	}
	headers := map[string]string{
		"x-api-key":         apiKey(p),
		"anthropic-version": "2026-10-21",
	}
	if err := postJSON(ctx, baseURL(p, "https://api.anthropic.com")+"/v1/messages", headers, body, &out); err != nil {
		return LLMResponse{}, err
	}
	// The answer can come in several blocks (e.g. thinking + text). Keep only the text.
	var text strings.Builder
	for _, block := range out.Content {
		if block.Type == "text" {
			text.WriteString(block.Text)
		}
	}
	resp := LLMResponse{Text: text.String(), InputTokens: out.Usage.InputTokens, OutputTokens: out.Usage.OutputTokens}
	if out.StopReason == "refusal" {
		return resp, errors.New(tr("o modelo recusou responder (stop_reason: refusal)", "the model refused to answer (stop_reason: refusal)"))
	}
	return resp, nil
}

// ---------- Google Gemini ----------

func callGemini(ctx context.Context, p ProviderConfig, req LLMRequest) (LLMResponse, error) {
	genConfig := map[string]any{"maxOutputTokens": req.MaxTokens}
	if req.Temperature != nil {
		genConfig["temperature"] = *req.Temperature
	}
	parts := []any{}
	for _, img := range req.Images {
		parts = append(parts, map[string]any{
			"inlineData": map[string]string{"mimeType": img.MediaType, "data": base64.StdEncoding.EncodeToString(img.Data)},
		})
	}
	parts = append(parts, map[string]string{"text": req.User})
	body := map[string]any{
		"contents":         []any{map[string]any{"role": "user", "parts": parts}},
		"generationConfig": genConfig,
	}
	if req.System != "" {
		body["systemInstruction"] = map[string]any{"parts": []any{map[string]string{"text": req.System}}}
	}

	var out struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
			FinishReason string `json:"finishReason"`
		} `json:"candidates"`
		UsageMetadata struct {
			PromptTokenCount     int `json:"promptTokenCount"`
			CandidatesTokenCount int `json:"candidatesTokenCount"`
		} `json:"usageMetadata"`
	}
	endpoint := baseURL(p, "https://generativelanguage.googleapis.com") +
		"/v1beta/models/" + url.PathEscape(req.Model) + ":generateContent"
	headers := map[string]string{"x-goog-api-key": apiKey(p)}
	if err := postJSON(ctx, endpoint, headers, body, &out); err != nil {
		return LLMResponse{}, err
	}
	if len(out.Candidates) == 0 {
		return LLMResponse{}, errors.New(tr("o Gemini não retornou nenhuma resposta (possível bloqueio de segurança)", "Gemini returned no answer (possibly a safety block)"))
	}
	var text strings.Builder
	for _, part := range out.Candidates[0].Content.Parts {
		text.WriteString(part.Text)
	}
	return LLMResponse{
		Text:         text.String(),
		InputTokens:  out.UsageMetadata.PromptTokenCount,
		OutputTokens: out.UsageMetadata.CandidatesTokenCount,
	}, nil
}

// ---------- Ollama (local models) ----------

func callOllama(ctx context.Context, p ProviderConfig, req LLMRequest) (LLMResponse, error) {
	messages := []map[string]any{}
	if req.System != "" {
		messages = append(messages, map[string]any{"role": "system", "content": req.System})
	}
	user := map[string]any{"role": "user", "content": req.User}
	if len(req.Images) > 0 {
		// Ollama takes images as a plain list of base64 strings (needs a vision model, e.g. llava, gemma3).
		var images []string
		for _, img := range req.Images {
			images = append(images, base64.StdEncoding.EncodeToString(img.Data))
		}
		user["images"] = images
	}
	messages = append(messages, user)
	options := map[string]any{"num_predict": req.MaxTokens}
	if req.Temperature != nil {
		options["temperature"] = *req.Temperature
	}
	body := map[string]any{"model": req.Model, "messages": messages, "stream": false, "options": options}

	var out struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
		PromptEvalCount int `json:"prompt_eval_count"`
		EvalCount       int `json:"eval_count"`
	}
	if err := postJSON(ctx, baseURL(p, "http://localhost:11434")+"/api/chat", nil, body, &out); err != nil {
		if strings.Contains(err.Error(), "not found") {
			return LLMResponse{}, fmt.Errorf(tr("o modelo %s não está baixado no Ollama: baixe em Configurações › Baixar modelo", "model %s is not downloaded in Ollama: download it in Settings › Download model"), req.Model)
		}
		return LLMResponse{}, err
	}
	return LLMResponse{Text: out.Message.Content, InputTokens: out.PromptEvalCount, OutputTokens: out.EvalCount}, nil
}

// ---------- Listing models / connection test ----------

// listModels asks the provider which models exist. It doubles as a connection test.
func listModels(ctx context.Context, p ProviderConfig) ([]string, error) {
	names := []string{} // never nil: the UI expects a list, even an empty one
	switch p.Kind {
	case "ollama":
		var out struct {
			Models []struct {
				Name string `json:"name"`
			} `json:"models"`
		}
		if err := getJSON(ctx, baseURL(p, "http://localhost:11434")+"/api/tags", nil, &out); err != nil {
			return nil, err
		}
		for _, m := range out.Models {
			names = append(names, m.Name)
		}
	case "openai", "openai_compatible":
		var out struct {
			Data []struct {
				ID string `json:"id"`
			} `json:"data"`
		}
		headers := map[string]string{}
		if key := apiKey(p); key != "" {
			headers["Authorization"] = "Bearer " + key
		}
		if err := getJSON(ctx, baseURL(p, "https://api.openai.com")+"/v1/models", headers, &out); err != nil {
			return nil, err
		}
		for _, m := range out.Data {
			names = append(names, m.ID)
		}
	case "anthropic":
		var out struct {
			Data []struct {
				ID string `json:"id"`
			} `json:"data"`
		}
		headers := map[string]string{"x-api-key": apiKey(p), "anthropic-version": "2026-10-21"}
		if err := getJSON(ctx, baseURL(p, "https://api.anthropic.com")+"/v1/models?limit=100", headers, &out); err != nil {
			return nil, err
		}
		for _, m := range out.Data {
			names = append(names, m.ID)
		}
	case "typesafe":
		var out struct {
			Data []struct {
				ID string `json:"id"`
			} `json:"data"`
		}
		headers := map[string]string{"Authorization": "Bearer " + apiKey(p)}
		if err := getJSON(ctx, baseURL(p, "https://api.typesafe.ai")+"/v1/models", headers, &out); err != nil {
			return nil, err
		}
		for _, m := range out.Data {
			names = append(names, m.ID)
		}
	case "gemini":
		var out struct {
			Models []struct {
				Name string `json:"name"`
			} `json:"models"`
		}
		headers := map[string]string{"x-goog-api-key": apiKey(p)}
		if err := getJSON(ctx, baseURL(p, "https://generativelanguage.googleapis.com")+"/v1beta/models?pageSize=200", headers, &out); err != nil {
			return nil, err
		}
		for _, m := range out.Models {
			names = append(names, strings.TrimPrefix(m.Name, "models/"))
		}
	default:
		return nil, fmt.Errorf(tr("tipo de provedor desconhecido: %q", "unknown provider kind: %q"), p.Kind)
	}
	return names, nil
}

// pullOllamaModel downloads a model into Ollama (same as `ollama pull <model>`).
// It waits until the download finishes, which can take several minutes.
func pullOllamaModel(ctx context.Context, p ProviderConfig, model string) error {
	if p.Kind != "ollama" {
		return errors.New(tr("só é possível baixar modelos em provedores Ollama", "models can only be downloaded on Ollama providers"))
	}
	var out struct {
		Status string `json:"status"`
	}
	body := map[string]any{"model": model, "stream": false}
	if err := postJSON(ctx, baseURL(p, "http://localhost:11434")+"/api/pull", nil, body, &out); err != nil {
		if strings.Contains(err.Error(), "file does not exist") {
			return fmt.Errorf(tr("o modelo %q não existe na biblioteca do Ollama (veja ollama.com/library)", "model %q does not exist in the Ollama library (see ollama.com/library)"), model)
		}
		return err
	}
	if out.Status != "success" {
		return fmt.Errorf(tr("o Ollama respondeu: %s", "Ollama answered: %s"), out.Status)
	}
	return nil
}

// ---------- HTTP helpers ----------

func baseURL(p ProviderConfig, fallback string) string {
	if p.BaseURL == "" {
		return fallback
	}
	return strings.TrimRight(p.BaseURL, "/")
}

func postJSON(ctx context.Context, endpoint string, headers map[string]string, body, out any) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	return doJSON(req, headers, out)
}

func getJSON(ctx context.Context, endpoint string, headers map[string]string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	return doJSON(req, headers, out)
}

func doJSON(req *http.Request, headers map[string]string, out any) error {
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return friendlyNetworkError(err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 20<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode >= 300 {
		return friendlyHTTPError(resp.StatusCode, data)
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf(tr("resposta inesperada do provedor: %w", "unexpected answer from the provider: %w"), err)
	}
	return nil
}

// friendlyHTTPError turns an API error into a short Portuguese message,
// keeping the provider's own message so nothing is hidden.
func friendlyHTTPError(status int, body []byte) error {
	detail := extractErrorMessage(body)
	var prefix string
	switch status {
	case 401, 403:
		prefix = tr("chave de API inválida ou sem permissão", "invalid API key or no permission")
	case 404:
		prefix = tr("modelo ou endereço não encontrado", "model or address not found")
	case 429:
		prefix = tr("limite de uso atingido (rate limit), tente menos chamadas em paralelo", "rate limit reached, try fewer parallel calls")
	case 400:
		prefix = tr("requisição recusada pelo provedor", "request rejected by the provider")
	default:
		if status >= 500 {
			prefix = tr("o provedor está com problemas", "the provider is having problems")
		} else {
			prefix = tr("erro do provedor", "provider error")
		}
	}
	if detail == "" {
		return fmt.Errorf("%s (HTTP %d)", prefix, status)
	}
	return fmt.Errorf("%s (HTTP %d): %s", prefix, status, detail)
}

// extractErrorMessage finds the message in the common error formats:
// {"error": {"message": "..."}}, {"error": "..."}, {"message": "..."}.
func extractErrorMessage(body []byte) string {
	var parsed struct {
		Error   json.RawMessage `json:"error"`
		Message string          `json:"message"`
	}
	if json.Unmarshal(body, &parsed) == nil {
		var nested struct {
			Message string `json:"message"`
		}
		if json.Unmarshal(parsed.Error, &nested) == nil && nested.Message != "" {
			return nested.Message
		}
		var plain string
		if json.Unmarshal(parsed.Error, &plain) == nil && plain != "" {
			return plain
		}
		if parsed.Message != "" {
			return parsed.Message
		}
	}
	text := strings.TrimSpace(string(body))
	if len(text) > 300 {
		text = text[:300] + "…"
	}
	return text
}

func friendlyNetworkError(err error) error {
	if errors.Is(err, context.DeadlineExceeded) {
		return errors.New(tr("tempo limite excedido", "timed out"))
	}
	if errors.Is(err, context.Canceled) {
		return errors.New(tr("cancelado", "canceled"))
	}
	msg := err.Error()
	if strings.Contains(msg, "connection refused") {
		return errors.New(tr("não foi possível conectar (o servidor está ligado? Para Ollama, rode \"ollama serve\")", "could not connect (is the server running? For Ollama, run \"ollama serve\")"))
	}
	if strings.Contains(msg, "no such host") {
		return errors.New(tr("endereço não encontrado, verifique a URL e sua internet", "address not found, check the URL and your internet"))
	}
	return fmt.Errorf(tr("erro de conexão: %w", "connection error: %w"), err)
}
