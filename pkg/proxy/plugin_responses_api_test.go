package proxy

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestResponsesAPIPlugin_ProcessRequest(t *testing.T) {
	plugin := &ResponsesAPIPlugin{}

	reqBody := ResponsesAPIRequest{
		Model: "gpt-4o",
		Input: []interface{}{
			map[string]interface{}{"role": "user", "content": "Hello"},
		},
		MaxOutputTokens: 100,
		Stream:          true,
		ResponseFormat: &ResponseFmt{
			Type: "json_schema",
			JSONSchema: map[string]interface{}{
				"name": "foo",
			},
		},
	}
	bodyBytes, _ := json.Marshal(reqBody)

	req := httptest.NewRequest("POST", "/v1/responses", bytes.NewReader(bodyBytes))

	if err := plugin.ProcessRequest(req); err != nil {
		t.Fatalf("ProcessRequest failed: %v", err)
	}

	if req.URL.Path != "/v1/chat/completions" {
		t.Errorf("Path not rewritten, got %s", req.URL.Path)
	}

	if req.Header.Get("X-DevProxy-Responses-API") != "true" {
		t.Errorf("Header X-DevProxy-Responses-API not set")
	}

	// Check body
	newBodyBytes, _ := io.ReadAll(req.Body)
	var chatReq ChatCompletionRequest
	if err := json.Unmarshal(newBodyBytes, &chatReq); err != nil {
		t.Fatalf("Failed to unmarshal chat request: %v", err)
	}

	if chatReq.Model != "gpt-4o" {
		t.Errorf("Model mismatch: %s", chatReq.Model)
	}
	if chatReq.MaxTokens != 100 {
		t.Errorf("MaxTokens mismatch: %d", chatReq.MaxTokens)
	}
	if !chatReq.Stream {
		t.Errorf("Stream mismatch")
	}
	if chatReq.ResponseFormat == nil || chatReq.ResponseFormat.Type != "json_schema" {
		t.Errorf("ResponseFormat mismatch")
	}
}

func TestResponsesAPIPlugin_ProcessResponse_JSON(t *testing.T) {
	plugin := &ResponsesAPIPlugin{}

	// Mock request with marker header
	req := httptest.NewRequest("POST", "/v1/chat/completions", nil)
	req.Header.Set("X-DevProxy-Responses-API", "true")

	// Construct response body using a map to avoid anonymous struct issues
	chatRespMap := map[string]interface{}{
		"id":      "chatcmpl-123",
		"object":  "chat.completion",
		"created": 1234567890,
		"model":   "gpt-4o",
		"choices": []map[string]interface{}{
			{
				"index": 0,
				"message": map[string]interface{}{
					"role":    "assistant",
					"content": "Hi there",
				},
				"finish_reason": "stop",
			},
		},
		"usage": map[string]interface{}{
			"total_tokens": 10,
		},
	}
	respBody, _ := json.Marshal(chatRespMap)

	resp := &http.Response{
		StatusCode: 200,
		Header:     make(http.Header),
		Body:       io.NopCloser(bytes.NewReader(respBody)),
		Request:    req,
	}
	resp.Header.Set("Content-Type", "application/json")

	if err := plugin.ProcessResponse(resp, nil, true); err != nil {
		t.Fatalf("ProcessResponse failed: %v", err)
	}

	// Check response body
	newBytes, _ := io.ReadAll(resp.Body)
	var resResp ResponsesAPIResponse
	if err := json.Unmarshal(newBytes, &resResp); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if resResp.ID != "resp_123" {
		t.Errorf("ID mismatch: %s", resResp.ID)
	}
	if resResp.Object != "response" {
		t.Errorf("Object mismatch: %s", resResp.Object)
	}
	if len(resResp.Output) != 1 {
		t.Fatalf("Expected 1 output, got %d", len(resResp.Output))
	}
	if resResp.Output[0].Content[0].Text != "Hi there" {
		t.Errorf("Content mismatch: %s", resResp.Output[0].Content[0].Text)
	}
}

func TestResponsesAPIPlugin_ProcessResponse_Ignored(t *testing.T) {
	plugin := &ResponsesAPIPlugin{}

	// Request WITHOUT marker header
	req := httptest.NewRequest("POST", "/v1/chat/completions", nil)
	// req.Header.Set("X-DevProxy-Responses-API", "true") // MISSING

	chatRespMap := map[string]interface{}{
		"id": "chatcmpl-123",
	}
	respBody, _ := json.Marshal(chatRespMap)

	resp := &http.Response{
		StatusCode: 200,
		Header:     make(http.Header),
		Body:       io.NopCloser(bytes.NewReader(respBody)),
		Request:    req,
	}
	resp.Header.Set("Content-Type", "application/json")

	if err := plugin.ProcessResponse(resp, nil, true); err != nil {
		t.Fatalf("ProcessResponse failed: %v", err)
	}

	// Body should be unchanged (or at least same content)
	newBytes, _ := io.ReadAll(resp.Body)
	if !bytes.Contains(newBytes, []byte("chatcmpl-123")) {
		t.Errorf("Response should not have been transformed")
	}
}

func TestResponsesAPIPlugin_ProcessResponse_Stream(t *testing.T) {
	plugin := &ResponsesAPIPlugin{}

	req := httptest.NewRequest("POST", "/v1/chat/completions", nil)
	req.Header.Set("X-DevProxy-Responses-API", "true")

	// Mock Stream Data
	chunks := []string{
		`data: {"id":"chatcmpl-123","object":"chat.completion.chunk","created":1677649420,"model":"gpt-3.5-turbo","choices":[{"index":0,"delta":{"role":"assistant"},"finish_reason":null}]}`,
		`data: {"id":"chatcmpl-123","object":"chat.completion.chunk","created":1677649420,"model":"gpt-3.5-turbo","choices":[{"index":0,"delta":{"content":"Hello"},"finish_reason":null}]}`,
		`data: [DONE]`,
	}
	bodyStr := strings.Join(chunks, "\n\n") + "\n\n"

	resp := &http.Response{
		StatusCode: 200,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(bodyStr)),
		Request:    req,
	}
	resp.Header.Set("Content-Type", "text/event-stream")

	if err := plugin.ProcessResponse(resp, nil, true); err != nil {
		t.Fatalf("ProcessResponse failed: %v", err)
	}

	// Read stream
	reader := io.Reader(resp.Body)
	buf := new(bytes.Buffer)
	buf.ReadFrom(reader)
	output := buf.String()

	if !strings.Contains(output, "event: response.created") {
		t.Errorf("Missing response.created event")
	}
	if !strings.Contains(output, "event: response.output_text.delta") {
		t.Errorf("Missing response.output_text.delta event")
	}
	// Since we mock [DONE] but maybe not enough chunks to trigger completion if we implemented full logic?
	// Our `handleStream` implementation sends completion events when it sees `finish_reason` OR when `[DONE]` (if not sent yet).
	// In the mock chunks above, `finish_reason` is null. So it should trigger on `[DONE]`.
	if !strings.Contains(output, "event: response.completed") {
		t.Errorf("Missing response.completed event")
	}
}
