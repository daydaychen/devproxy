package proxy

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/elazarl/goproxy"
)

func TestOpenAIResponsesPlugin_ProcessResponse(t *testing.T) {
	plugin := &OpenAIResponsesPlugin{}
	ctx := &goproxy.ProxyCtx{}

	// 模拟 Chat Completion 响应
	chatResp := ChatCompletionResponse{
		ID:      "chatcmpl-123",
		Object:  "chat.completion",
		Created: 1677649420,
		Model:   "gpt-3.5-turbo-0301",
		Choices: []struct {
			Index   int `json:"index"`
			Message struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		}{
			{
				Index: 0,
				Message: struct {
					Role    string `json:"role"`
					Content string `json:"content"`
				}{
					Role:    "assistant",
					Content: "Hello! How can I help you today?",
				},
				FinishReason: "stop",
			},
		},
		Usage: map[string]interface{}{
			"prompt_tokens":     9,
			"completion_tokens": 12,
			"total_tokens":      21,
		},
	}

	bodyBytes, _ := json.Marshal(chatResp)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(bytes.NewReader(bodyBytes)),
	}
	resp.Header.Set("Content-Type", "application/json")

	err := plugin.ProcessResponse(resp, ctx, false)
	if err != nil {
		t.Fatalf("ProcessResponse failed: %v", err)
	}

	// 读取并验证转换后的 Body
	newBodyBytes, _ := io.ReadAll(resp.Body)
	var resResp ResponsesAPIResponse
	if err := json.Unmarshal(newBodyBytes, &resResp); err != nil {
		t.Fatalf("Failed to unmarshal converted response: %v", err)
	}

	if resResp.ID != "resp_123" {
		t.Errorf("Expected ID resp_123, got %s", resResp.ID)
	}
	if resResp.Object != "response" {
		t.Errorf("Expected object response, got %s", resResp.Object)
	}
	if len(resResp.Output) != 1 {
		t.Errorf("Expected 1 output item, got %d", len(resResp.Output))
	}
	if resResp.Output[0].Type != "message" {
		t.Errorf("Expected output type message, got %s", resResp.Output[0].Type)
	}
	if resResp.Output[0].Content[0].Text != "Hello! How can I help you today?" {
		t.Errorf("Content mismatch")
	}
}

func TestOpenAIResponsesPlugin_HandleStream(t *testing.T) {
	plugin := &OpenAIResponsesPlugin{}
	ctx := &goproxy.ProxyCtx{}

	// 模拟流式响应
	chunks := []string{
		`data: {"id":"chatcmpl-123","object":"chat.completion.chunk","created":1677649420,"model":"gpt-3.5-turbo","choices":[{"index":0,"delta":{"role":"assistant"},"finish_reason":null}]}`,
		`data: {"id":"chatcmpl-123","object":"chat.completion.chunk","created":1677649420,"model":"gpt-3.5-turbo","choices":[{"index":0,"delta":{"content":"Hello"},"finish_reason":null}]}`,
		`data: {"id":"chatcmpl-123","object":"chat.completion.chunk","created":1677649420,"model":"gpt-3.5-turbo","choices":[{"index":0,"delta":{"content":" world"},"finish_reason":"stop"}]}`,
		`data: [DONE]`,
	}

	bodyStr := ""
	for _, c := range chunks {
		bodyStr += c + "\n\n"
	}

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(bodyStr)),
	}
	resp.Header.Set("Content-Type", "text/event-stream")

	err := plugin.ProcessResponse(resp, ctx, true)
	if err != nil {
		t.Fatalf("ProcessResponse failed: %v", err)
	}

	// 读取转换后的流
	reader := bufio.NewReader(resp.Body)
	var events []ResponsesAPIEvent
	var currentEvent string
	for {
		line, err := reader.ReadString('\n')
		if err == io.EOF {
			break
		}
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "event: ") {
			currentEvent = strings.TrimPrefix(line, "event: ")
		} else if strings.HasPrefix(line, "data: ") {
			data := strings.TrimPrefix(line, "data: ")
			if data == "[DONE]" {
				continue
			}
			var ev ResponsesAPIEvent
			if err := json.Unmarshal([]byte(data), &ev); err == nil {
				ev.Type = currentEvent // 确保 Type 与 event: 一致
				events = append(events, ev)
			}
		}
	}

	// 验证事件序列
	// 期望: response.created, response.output_item.added, response.content_part.added, 
	//       response.output_text.delta, response.output_text.delta,
	//       response.output_text.done, response.content_part.done, response.output_item.done, response.completed
	if len(events) < 8 {
		t.Errorf("Expected at least 8 events, got %d", len(events))
	}
	
	lastEvent := events[len(events)-1]
	if lastEvent.Type != "response.completed" {
		t.Errorf("Expected last event to be response.completed, got %s", lastEvent.Type)
	}
	if lastEvent.Response == nil || lastEvent.Response.Output == nil {
		t.Errorf("response.completed should contain full response object")
	}
}

func TestOpenAIResponsesPlugin_ReproEOFData(t *testing.T) {
	plugin := &OpenAIResponsesPlugin{}
	ctx := &goproxy.ProxyCtx{}

	// 模拟流式响应，最后一个 [DONE] 没有换行符就直接结束了
	bodyStr := "data: {\"id\":\"chatcmpl-123\",\"object\":\"chat.completion.chunk\",\"created\":1677649420,\"model\":\"gpt-3.5-turbo\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"Hello\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]"

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(bodyStr)),
	}
	resp.Header.Set("Content-Type", "text/event-stream")

	err := plugin.ProcessResponse(resp, ctx, true)
	if err != nil {
		t.Fatalf("ProcessResponse failed: %v", err)
	}

	// 读取转换后的流
	reader := bufio.NewReader(resp.Body)
	var hasDone bool
	var hasCompleted bool
	for {
		line, err := reader.ReadString('\n')
		if err == io.EOF {
			break
		}
		if strings.Contains(line, "data: [DONE]") {
			hasDone = true
		}
		if strings.Contains(line, "response.completed") {
			hasCompleted = true
		}
	}

	if !hasCompleted {
		t.Errorf("FAIL: response.completed event was not sent when stream ended without trailing newline")
	}
	if !hasDone {
		t.Errorf("FAIL: data: [DONE] was not forwarded when it was at the very end of stream without newline")
	}
}

func TestOpenAIResponsesPlugin_ReproNoFinishReason(t *testing.T) {
	plugin := &OpenAIResponsesPlugin{}
	ctx := &goproxy.ProxyCtx{}

	// 模拟流式响应，没有 finish_reason，直接 [DONE]
	bodyStr := "data: {\"id\":\"chatcmpl-123\",\"object\":\"chat.completion.chunk\",\"created\":1677649420,\"model\":\"gpt-3.5-turbo\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"Hello\"}}]}\n\ndata: [DONE]\n\n"

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(bodyStr)),
	}
	resp.Header.Set("Content-Type", "text/event-stream")

	err := plugin.ProcessResponse(resp, ctx, true)
	if err != nil {
		t.Fatalf("ProcessResponse failed: %v", err)
	}

	// 读取转换后的流
	reader := bufio.NewReader(resp.Body)
	var events []string
	for {
		line, err := reader.ReadString('\n')
		if err == io.EOF {
			break
		}
		if strings.HasPrefix(line, "event: ") {
			events = append(events, strings.TrimSpace(strings.TrimPrefix(line, "event: ")))
		}
		if strings.HasPrefix(line, "data: [DONE]") {
			events = append(events, "DONE")
		}
	}

	// 检查顺序
	completedIndex := -1
	doneIndex := -1
	for i, e := range events {
		if e == "response.completed" {
			completedIndex = i
		}
		if e == "DONE" {
			doneIndex = i
		}
	}

	if completedIndex == -1 {
		t.Errorf("FAIL: response.completed not found")
	}
	if doneIndex != -1 && completedIndex > doneIndex {
		t.Errorf("FAIL: response.completed (%d) sent AFTER [DONE] (%d), client might have closed connection", completedIndex, doneIndex)
	}
}
