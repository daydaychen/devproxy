package proxy

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/elazarl/goproxy"
)

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// ResponsesAPIPlugin adapts the OpenAI Responses API (/v1/responses) to the Chat Completions API (/v1/chat/completions).
type ResponsesAPIPlugin struct{}

func (p *ResponsesAPIPlugin) Name() string {
	return "responses-api"
}

// ResponsesAPIRequest represents the incoming request body for /v1/responses
type ResponsesAPIRequest struct {
	Model           string        `json:"model"`
	Instructions    string        `json:"instructions,omitempty"`
	Input           []interface{} `json:"input,omitempty"`
	ResponseFormat  *ResponseFmt  `json:"response_format,omitempty"`
	Stream          bool          `json:"stream,omitempty"`
	MaxOutputTokens int           `json:"max_output_tokens,omitempty"`
	Temperature     *float64      `json:"temperature,omitempty"`
	TopP            *float64      `json:"top_p,omitempty"`
	N               *int          `json:"n,omitempty"`
	Tools           []interface{} `json:"tools,omitempty"`
	ToolChoice      interface{}   `json:"tool_choice,omitempty"`
}

type ResponseFmt struct {
	Type       string                 `json:"type"`
	JSONSchema map[string]interface{} `json:"json_schema,omitempty"`
}

// ChatCompletionRequest represents the standard OpenAI Chat Completion request
type ChatCompletionRequest struct {
	Model               string                 `json:"model"`
	Messages            []interface{}          `json:"messages"`
	ResponseFormat      *ChatResponseFormat    `json:"response_format,omitempty"`
	Stream              bool                   `json:"stream,omitempty"`
	MaxTokens           int                    `json:"max_tokens,omitempty"`
	MaxCompletionTokens int                    `json:"max_completion_tokens,omitempty"`
	Temperature         *float64               `json:"temperature,omitempty"`
	TopP                *float64               `json:"top_p,omitempty"`
	N                   *int                   `json:"n,omitempty"`
	Tools               []interface{}          `json:"tools,omitempty"`
	ToolChoice          interface{}            `json:"tool_choice,omitempty"`
}

type ChatResponseFormat struct {
	Type       string                 `json:"type"`
	JSONSchema map[string]interface{} `json:"json_schema,omitempty"`
}

func (p *ResponsesAPIPlugin) ProcessRequest(req *http.Request) error {
	// Only intercept /v1/responses
	if req.URL.Path != "/v1/responses" {
		return nil
	}

	// 1. Rewrite path
	req.URL.Path = "/v1/chat/completions"

	// 2. Read body
	bodyBytes, err := io.ReadAll(req.Body)
	if err != nil {
		return fmt.Errorf("responses-api: failed to read request body: %w", err)
	}
	req.Body.Close()

	if len(bodyBytes) == 0 {
		req.Body = io.NopCloser(bytes.NewReader(bodyBytes))
		return nil
	}

	// 3. Unmarshal Responses API request
	var respReq ResponsesAPIRequest
	if err := json.Unmarshal(bodyBytes, &respReq); err != nil {
		log.Printf("[responses-api] Warning: failed to parse request body: %v", err)
		req.Body = io.NopCloser(bytes.NewReader(bodyBytes))
		return nil
	}

	// 4. Transform to Chat Completion Request
	messages := p.transformMessages(respReq.Input)
	
	// Prepend instructions as a developer/system message if present
	if respReq.Instructions != "" {
		instrMsg := map[string]interface{}{
			"role":    "developer",
			"content": respReq.Instructions,
		}
		messages = append([]interface{}{instrMsg}, messages...)
	}

	chatReq := ChatCompletionRequest{
		Model:       respReq.Model,
		Messages:    messages,
		Stream:      respReq.Stream,
		Temperature: respReq.Temperature,
		TopP:        respReq.TopP,
		N:           respReq.N,
		Tools:       p.transformTools(respReq.Tools),
		ToolChoice:  p.transformToolChoice(respReq.ToolChoice),
	}

	// Map max_output_tokens -> max_completion_tokens (preferred for newer models)
	if respReq.MaxOutputTokens > 0 {
		chatReq.MaxCompletionTokens = respReq.MaxOutputTokens
		// Also set max_tokens for backward compatibility
		chatReq.MaxTokens = respReq.MaxOutputTokens
	}

	// Map response_format
	if respReq.ResponseFormat != nil {
		chatReq.ResponseFormat = &ChatResponseFormat{
			Type:       respReq.ResponseFormat.Type,
			JSONSchema: respReq.ResponseFormat.JSONSchema,
		}
	}

	// 5. Marshal new body
	newBodyBytes, err := json.Marshal(chatReq)
	if err != nil {
		return fmt.Errorf("responses-api: failed to marshal new request body: %w", err)
	}

	req.Body = io.NopCloser(bytes.NewReader(newBodyBytes))
	req.ContentLength = int64(len(newBodyBytes))
	req.Header.Set("Content-Length", fmt.Sprint(len(newBodyBytes)))

	// Ensure we ask for identity encoding to handle response body easily
	req.Header.Set("Accept-Encoding", "identity")

	// Mark this request as a Responses API request so ProcessResponse can handle it
	req.Header.Set("X-DevProxy-Responses-API", "true")

	log.Printf("[responses-api] Rewrote request /v1/responses -> /v1/chat/completions (model: %s)", respReq.Model)
	return nil
}

func (p *ResponsesAPIPlugin) transformMessages(input []interface{}) []interface{} {
	if input == nil {
		return nil
	}
	output := make([]interface{}, 0, len(input))
	for _, m := range input {
		item, ok := m.(map[string]interface{})
		if !ok {
			output = append(output, m)
			continue
		}

		itemType, _ := item["type"].(string)

		switch itemType {
		case "message":
			newMsg := make(map[string]interface{})
			// Standard Chat Completion roles: user, assistant, system, developer
			if role, ok := item["role"].(string); ok {
				newMsg["role"] = role
			}
			if content := item["content"]; content != nil {
				newMsg["content"] = p.transformContent(content)
			}
			// Copy other relevant fields (like name)
			for k, v := range item {
				if k != "type" && k != "role" && k != "content" && k != "phase" {
					newMsg[k] = v
				}
			}
			output = append(output, newMsg)

		case "function_call":
			// Map to assistant message with tool_calls for standard Chat Completion
			newMsg := map[string]interface{}{
				"role": "assistant",
				"tool_calls": []interface{}{
					map[string]interface{}{
						"id":   item["call_id"],
						"type": "function",
						"function": map[string]interface{}{
							"name":      item["name"],
							"arguments": item["arguments"],
						},
					},
				},
			}
			output = append(output, newMsg)

		case "function_call_output":
			// Map to tool message
			newMsg := map[string]interface{}{
				"role":         "tool",
				"tool_call_id": item["call_id"],
				"content":      item["output"],
			}
			output = append(output, newMsg)

		default:
			// Fallback: If it has role, assume it's already a standard message
			if _, hasRole := item["role"]; hasRole {
				newMsg := make(map[string]interface{})
				for k, v := range item {
					if k == "content" {
						newMsg[k] = p.transformContent(v)
					} else {
						newMsg[k] = v
					}
				}
				output = append(output, newMsg)
			} else {
				output = append(output, item)
			}
		}
	}
	return output
}

func (p *ResponsesAPIPlugin) transformContent(content interface{}) interface{} {
	switch v := content.(type) {
	case string:
		return v
	case []interface{}:
		newContent := make([]interface{}, len(v))
		for i, p := range v {
			part, ok := p.(map[string]interface{})
			if !ok {
				newContent[i] = p
				continue
			}
			newPart := make(map[string]interface{})
			for pk, pv := range part {
				// Responses API uses input_text/output_text, Chat uses text
				if pk == "type" && (pv == "input_text" || pv == "output_text") {
					newPart[pk] = "text"
				} else {
					newPart[pk] = pv
				}
			}
			newContent[i] = newPart
		}
		return newContent
	default:
		return content
	}
}

func (p *ResponsesAPIPlugin) transformTools(tools []interface{}) []interface{} {
	if tools == nil {
		return nil
	}
	output := make([]interface{}, 0, len(tools))
	for _, t := range tools {
		tool, ok := t.(map[string]interface{})
		if !ok {
			output = append(output, t)
			continue
		}

		toolType, _ := tool["type"].(string)

		// 1. Standard Function Tool (Responses API format)
		if toolType == "function" {
			newTool := map[string]interface{}{
				"type": "function",
			}

			var fnParams map[string]interface{}
			if fn, hasFn := tool["function"].(map[string]interface{}); hasFn {
				fnParams = fn
			} else {
				// Flat format: name/parameters at top level
				fnParams = tool
			}

			newTool["function"] = map[string]interface{}{
				"name":        fnParams["name"],
				"description": fnParams["description"],
				"parameters":  p.cleanParameters(fnParams["parameters"]),
			}
			output = append(output, newTool)
			continue
		}

		// 2. Built-in Tools (web_search, etc.) -> Map to Function
		if toolType != "" {
			newTool := map[string]interface{}{
				"type": "function",
				"function": map[string]interface{}{
					"name":        toolType,
					"description": fmt.Sprintf("Built-in tool: %s", toolType),
					"parameters": map[string]interface{}{
						"type":       "object",
						"properties": p.extractToolProperties(tool),
					},
				},
			}
			output = append(output, newTool)
			continue
		}

		output = append(output, tool)
	}
	return output
}

func (p *ResponsesAPIPlugin) extractToolProperties(tool map[string]interface{}) map[string]interface{} {
	props := make(map[string]interface{})
	for k, v := range tool {
		if k != "type" && k != "function" {
			props[k] = v
		}
	}
	return props
}

func (p *ResponsesAPIPlugin) cleanParameters(params interface{}) interface{} {
	m, ok := params.(map[string]interface{})
	if !ok {
		return params
	}

	newM := make(map[string]interface{})
	for k, v := range m {
		// Strip additionalProperties as many providers don't support it in tool parameters
		if k == "additionalProperties" {
			continue
		}

		// Recursively clean properties if it's a map
		if k == "properties" {
			if props, ok := v.(map[string]interface{}); ok {
				newProps := make(map[string]interface{})
				for pk, pv := range props {
					newProps[pk] = p.cleanParameters(pv)
				}
				newM[k] = newProps
				continue
			}
		}

		// Also clean items for arrays
		if k == "items" {
			newM[k] = p.cleanParameters(v)
			continue
		}

		newM[k] = v
	}
	return newM
}

func (p *ResponsesAPIPlugin) transformToolChoice(choice interface{}) interface{} {
	if choice == nil {
		return nil
	}
	// If it's a string like "auto", "none", "required", return as is
	if _, ok := choice.(string); ok {
		return choice
	}

	// If it's an object {"type": "function", "function": {"name": "..."}}
	m, ok := choice.(map[string]interface{})
	if !ok {
		return choice
	}

	// If it has name at top level (Responses API might support this in some versions)
	if name, ok := m["name"].(string); ok && m["function"] == nil {
		return map[string]interface{}{
			"type": "function",
			"function": map[string]interface{}{
				"name": name,
			},
		}
	}

	return choice
}

func (p *ResponsesAPIPlugin) ProcessResponse(resp *http.Response, ctx *goproxy.ProxyCtx, verbose bool) error {
	if resp.Request == nil {
		return nil
	}

	// Detection logic:
	// 1. Check for our internal marker header
	isResponsesAPI := resp.Request.Header.Get("X-DevProxy-Responses-API") == "true"
	
	// 2. Fallback: Check if it's a chat/completions request (which we might have rewritten from /v1/responses)
	// We only do this if we are SURE it was our plugin that did the rewrite.
	// Since we are now using matchedRule in server.go, this plugin will only be called if the rule matched.
	// Therefore, if it's a chat/completions request, it's highly likely it needs conversion back to responses format.
	if !isResponsesAPI && strings.HasSuffix(resp.Request.URL.Path, "/chat/completions") {
		isResponsesAPI = true
	}

	if !isResponsesAPI {
		return nil
	}

	if verbose {
		log.Printf("[responses-api] Processing response for %s (Status: %d)", resp.Request.URL.Path, resp.StatusCode)
	}

	if resp.StatusCode != http.StatusOK {
		if verbose {
			log.Printf("[responses-api] Non-200 status code: %d", resp.StatusCode)
		}
		return nil
	}

	contentType := resp.Header.Get("Content-Type")

	// Handle Gzip
	if resp.Header.Get("Content-Encoding") == "gzip" {
		gr, err := gzip.NewReader(resp.Body)
		if err == nil {
			resp.Body = &gzipReadCloser{Reader: gr, orig: resp.Body}
			resp.Header.Del("Content-Encoding")
			resp.Header.Del("Content-Length")
			resp.ContentLength = -1
		}
	}

	if strings.Contains(contentType, "application/json") {
		return p.handleJSON(resp, verbose)
	}

	if strings.Contains(contentType, "text/event-stream") {
		return p.handleStream(resp, verbose)
	}

	return nil
}

func (p *ResponsesAPIPlugin) handleJSON(resp *http.Response, verbose bool) error {
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("responses-api: failed to read response body: %w", err)
	}
	resp.Body.Close()

	if len(bodyBytes) == 0 {
		resp.Body = io.NopCloser(bytes.NewReader(bodyBytes))
		return nil
	}

	var chatResp ChatCompletionResponse
	if err := json.Unmarshal(bodyBytes, &chatResp); err != nil {
		if verbose {
			log.Printf("[responses-api] Warning: failed to parse response body: %v", err)
		}
		resp.Body = io.NopCloser(bytes.NewReader(bodyBytes))
		return nil
	}

	// Transform
	resResp := ResponsesAPIResponse{
		ID:        strings.Replace(chatResp.ID, "chatcmpl-", "resp_", 1),
		Object:    "response",
		CreatedAt: chatResp.Created,
		Model:     chatResp.Model,
		Usage:     chatResp.Usage,
		Output:    make([]Item, 0, len(chatResp.Choices)),
	}

	for i, choice := range chatResp.Choices {
		item := Item{
			ID:     fmt.Sprintf("msg_%s_%d", resResp.ID, i),
			Type:   "message",
			Status: "completed",
			Role:   choice.Message.Role,
			Content: []Content{
				{
					Type: "output_text",
					Text: choice.Message.Content,
				},
			},
		}
		resResp.Output = append(resResp.Output, item)
	}

	newBodyBytes, err := json.Marshal(resResp)
	if err != nil {
		return fmt.Errorf("responses-api: failed to marshal new response body: %w", err)
	}

	resp.Body = io.NopCloser(bytes.NewReader(newBodyBytes))
	resp.ContentLength = int64(len(newBodyBytes))
	resp.Header.Set("Content-Length", fmt.Sprint(len(newBodyBytes)))

	return nil
}

func (p *ResponsesAPIPlugin) handleStream(resp *http.Response, verbose bool) error {
	reader, writer := io.Pipe()
	originalBody := resp.Body
	resp.Body = reader
	resp.ContentLength = -1
	resp.Header.Del("Content-Length")
	resp.Header.Set("Cache-Control", "no-cache")
	resp.Header.Set("Connection", "keep-alive")
	resp.Header.Set("X-Accel-Buffering", "no")
	// Ensure Content-Type is set for SSE
	resp.Header.Set("Content-Type", "text/event-stream")

	go func() {
		defer originalBody.Close()
		defer writer.Close()

		// Use default 4KB buffer for SSE streaming to minimize TTFB latency.
		// 1MB buffer causes data to accumulate before being read, violating SSE real-time semantics.
		scanner := bufio.NewScanner(originalBody)
		var responseID string
		var createdAt int64
		var model string

		if verbose {
			log.Printf("[responses-api] handleStream: started with 4KB buffer")
		}

		// State for message (Item 0)
		var fullContent strings.Builder
		var messageItemID string
		var messageSeqNum int
		var messageAdded bool
		var messagePartAdded bool
		var messageDone bool

		// State for tool calls (Items 1, 2, ...)
		type toolCallState struct {
			ID        string
			Name      string
			Arguments strings.Builder
			Added     bool
			Done      bool
		}
		toolCalls := make(map[int]*toolCallState)

		var completedSent bool
		var hasError bool

		// writeEventWrapper wraps writeEvent and logs errors for client disconnect detection
		writeEventWrapper := func(eventType string, data interface{}) bool {
			if err := p.writeEvent(writer, eventType, data); err != nil {
				if verbose {
					log.Printf("[responses-api] Client disconnected while writing event %s: %v", eventType, err)
				}
				hasError = true
				return false
			}
			return true
		}

		// Helper to close the message item
		finishMessage := func() {
			if messageAdded && !messageDone && !hasError {
				if writeEventWrapper("response.output_text.done", ResponsesAPIEvent{
					Type: "response.output_text.done", ResponseID: responseID, ItemID: messageItemID,
					ContentPartIndex: 0, ContentPart: &ContentPart{Type: "output_text", Index: 0, Text: fullContent.String()},
				}) {
					writeEventWrapper("response.content_part.done", ResponsesAPIEvent{
						Type: "response.content_part.done", ResponseID: responseID, ItemID: messageItemID,
						ContentPartIndex: 0, ContentPart: &ContentPart{Type: "output_text", Index: 0, Text: fullContent.String()},
					})
					writeEventWrapper("response.output_item.done", ResponsesAPIEvent{
						Type: "response.output_item.done", ResponseID: responseID, OutputIndex: 0,
						Item: &Item{ID: messageItemID, Type: "message", Status: "completed", Role: "assistant", Content: []Content{{Type: "output_text", Text: fullContent.String()}}},
					})
				}
				messageDone = true
			}
		}

		// Helper to close a tool call item
		finishToolCall := func(idx int, tc *toolCallState) {
			if tc.Added && !tc.Done && !hasError {
				itemIdx := 0
				if messageAdded {
					itemIdx = 1
				}
				itemIdx += idx
				tItemID := fmt.Sprintf("msg_%s_%d", responseID, itemIdx)

				writeEventWrapper("response.output_item.done", ResponsesAPIEvent{
					Type: "response.output_item.done", ResponseID: responseID, OutputIndex: itemIdx,
					Item: &Item{ID: tItemID, Type: "function_call", Status: "completed", Name: tc.Name, Arguments: tc.Arguments.String(), CallID: tc.ID},
				})
				tc.Done = true
			}
		}

		sendFinalEvents := func(usage interface{}) {
			if completedSent || hasError {
				return
			}
			finishMessage()
			if hasError {
				return
			}

			finalItems := make([]Item, 0)
			if messageAdded {
				finalItems = append(finalItems, Item{
					ID: messageItemID, Type: "message", Status: "completed", Role: "assistant",
					Content: []Content{{Type: "output_text", Text: fullContent.String()}},
				})
			}

			// Finish all tool calls in order
			for i := 0; i < len(toolCalls); i++ {
				if tc, ok := toolCalls[i]; ok {
					finishToolCall(i, tc)
					if hasError {
						return
					}
					itemIdx := len(finalItems)
					tItemID := fmt.Sprintf("msg_%s_%d", responseID, itemIdx)
					finalItems = append(finalItems, Item{
						ID: tItemID, Type: "function_call", Status: "completed", Name: tc.Name, Arguments: tc.Arguments.String(), CallID: tc.ID,
					})
				}
			}

			if !hasError {
				p.writeEvent(writer, "response.completed", ResponsesAPIEvent{
					Type: "response.completed", ResponseID: responseID,
					Response: &ResponsesAPIResponse{ID: responseID, Object: "response", CreatedAt: createdAt, Model: model, Usage: usage, Output: finalItems},
				})
			}
			completedSent = true
		}

		for scanner.Scan() {
			line := scanner.Text()

			if verbose {
				logLen := len(line)
				if logLen > 50 {
					logLen = 50
				}
				log.Printf("[responses-api] Read line: %s", line[:logLen])
			}

			if line == "" {
				if _, err := writer.Write([]byte("\n")); err != nil {
					if verbose {
						log.Printf("[responses-api] Client disconnected: %v", err)
					}
					return
				}
			} else {
				if strings.HasPrefix(line, "data: ") {
					data := strings.TrimPrefix(line, "data: ")
					if data == "[DONE]" {
						if verbose {
							log.Printf("[responses-api] Received [DONE], sending final events")
						}
						sendFinalEvents(nil)
						if _, err := writer.Write([]byte("data: [DONE]\n\n")); err != nil {
							if verbose {
								log.Printf("[responses-api] Client disconnected: %v", err)
							}
							return
						}
					} else {
						var chunk ChatCompletionChunk
						if errUnmarshal := json.Unmarshal([]byte(data), &chunk); errUnmarshal == nil && len(chunk.ID) > 0 {
							if verbose {
								log.Printf("[responses-api] Parsed chunk: ID=%s, Choices=%d", chunk.ID, len(chunk.Choices))
							}
							if responseID == "" {
								responseID = strings.Replace(chunk.ID, "chatcmpl-", "resp_", 1)
								createdAt = chunk.Created
								model = chunk.Model
								writeEventWrapper("response.created", ResponsesAPIEvent{
									Type: "response.created", ResponseID: responseID,
									Response: &ResponsesAPIResponse{ID: responseID, Object: "response", CreatedAt: createdAt, Model: model},
								})
							}

							if len(chunk.Choices) > 0 {
								choice := chunk.Choices[0]
								delta := choice.Delta

								// 1. Process Text Content
								if delta.Content != "" {
									if !messageAdded {
										messageItemID = fmt.Sprintf("msg_%s_0", responseID)
										if !writeEventWrapper("response.output_item.added", ResponsesAPIEvent{
											Type: "response.output_item.added", ResponseID: responseID, OutputIndex: 0,
											Item: &Item{ID: messageItemID, Type: "message", Status: "in_progress", Role: "assistant"},
										}) {
											return
										}
										messageAdded = true
									}
									if !messagePartAdded {
										if !writeEventWrapper("response.content_part.added", ResponsesAPIEvent{
											Type: "response.content_part.added", ResponseID: responseID, ItemID: messageItemID,
											ContentPart: &ContentPart{Type: "output_text", Index: 0},
										}) {
											return
										}
										messagePartAdded = true
									}
									fullContent.WriteString(delta.Content)
									messageSeqNum++
									if !writeEventWrapper("response.output_text.delta", ResponsesAPIEvent{
										Type: "response.output_text.delta", ResponseID: responseID, ItemID: messageItemID,
										Delta: delta.Content, SequenceNumber: messageSeqNum,
									}) {
										return
									}
								}

								// 2. Process Tool Calls
								if len(delta.ToolCalls) > 0 {
									// If we were sending text, finish that item first to keep order
									finishMessage()
									if hasError {
										return
									}

									for _, tcChunk := range delta.ToolCalls {
										tc, ok := toolCalls[tcChunk.Index]
										if !ok {
											tc = &toolCallState{}
											toolCalls[tcChunk.Index] = tc
										}
										if tcChunk.ID != "" {
											tc.ID = tcChunk.ID
										}
										if tcChunk.Function.Name != "" {
											tc.Name = tcChunk.Function.Name
										}
										if tcChunk.Function.Arguments != "" {
											tc.Arguments.WriteString(tcChunk.Function.Arguments)
										}

										// Announce tool call once we have basic info (ID/Name) or some arguments
										if !tc.Added && (tc.ID != "" || tc.Name != "" || tc.Arguments.Len() > 0) {
											itemIdx := 0
											if messageAdded {
												itemIdx = 1
											}
											itemIdx += tcChunk.Index
											tItemID := fmt.Sprintf("msg_%s_%d", responseID, itemIdx)

											if !writeEventWrapper("response.output_item.added", ResponsesAPIEvent{
												Type: "response.output_item.added", ResponseID: responseID, OutputIndex: itemIdx,
												Item: &Item{ID: tItemID, Type: "function_call", Status: "in_progress", Name: tc.Name, CallID: tc.ID},
											}) {
												return
											}
											tc.Added = true
										}
									}
								}

								if choice.FinishReason != nil {
									sendFinalEvents(chunk.Usage)
								}
							}
						} else {
							if verbose {
								log.Printf("[responses-api] Failed to parse chunk or empty ID: %v, data: %s", errUnmarshal, line[:min(100, len(line))])
							}
							// Forward non-JSON or unparseable data as-is
							if _, err := writer.Write([]byte(line + "\n\n")); err != nil {
								if verbose {
									log.Printf("[responses-api] Client disconnected: %v", err)
								}
								return
							}
						}
					}
				} else {
					if _, err := writer.Write([]byte(line + "\n")); err != nil {
						if verbose {
							log.Printf("[responses-api] Client disconnected: %v", err)
						}
						return
					}
				}
			}
		}
		// After loop: check scanner errors
		if err := scanner.Err(); err != nil {
			log.Printf("[responses-api] Scanner error: %v", err)
		}
	}()

	return nil
}

func (p *ResponsesAPIPlugin) sendCompletionEvents(w io.Writer, responseID, itemID string, createdAt int64, model, text string, usage interface{}) {
	p.writeEvent(w, "response.output_text.done", ResponsesAPIEvent{
		Type:             "response.output_text.done",
		ResponseID:       responseID,
		ItemID:           itemID,
		ContentPartIndex: 0,
		ContentPart: &ContentPart{
			Type:  "output_text",
			Index: 0,
			Text:  text,
		},
	})
	p.writeEvent(w, "response.content_part.done", ResponsesAPIEvent{
		Type:             "response.content_part.done",
		ResponseID:       responseID,
		ItemID:           itemID,
		ContentPartIndex: 0,
		ContentPart: &ContentPart{
			Type:  "output_text",
			Index: 0,
			Text:  text,
		},
	})
	p.writeEvent(w, "response.output_item.done", ResponsesAPIEvent{
		Type:        "response.output_item.done",
		ResponseID:  responseID,
		OutputIndex: 0,
		Item: &Item{
			ID:     itemID,
			Type:   "message",
			Status: "completed",
			Role:   "assistant",
			Content: []Content{
				{Type: "output_text", Text: text},
			},
		},
	})
	p.writeEvent(w, "response.completed", ResponsesAPIEvent{
		Type:       "response.completed",
		ResponseID: responseID,
		Response: &ResponsesAPIResponse{
			ID:        responseID,
			Object:    "response",
			CreatedAt: createdAt,
			Model:     model,
			Usage:     usage,
			Output: []Item{
				{
					ID:     itemID,
					Type:   "message",
					Status: "completed",
					Role:   "assistant",
					Content: []Content{
						{Type: "output_text", Text: text},
					},
				},
			},
		},
	})
}

func (p *ResponsesAPIPlugin) writeEvent(w io.Writer, eventType string, data interface{}) error {
	payload, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("writeEvent: failed to marshal JSON: %w", err)
	}

	// Use strings.Builder to reduce allocations compared to fmt.Sprintf
	prefix := "event: "
	suffix := "\n\ndata: "
	end := "\n\n"

	var builder strings.Builder
	builder.Grow(len(prefix) + len(eventType) + len(suffix) + len(payload) + len(end))
	builder.WriteString(prefix)
	builder.WriteString(eventType)
	builder.WriteString(suffix)
	builder.Write(payload)
	builder.WriteString(end)

	_, err = w.Write([]byte(builder.String()))
	if err != nil {
		return fmt.Errorf("writeEvent: failed to write: %w", err)
	}
	return nil
}
