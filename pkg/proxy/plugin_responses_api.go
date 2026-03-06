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

// ResponsesAPIPlugin adapts the OpenAI Responses API (/v1/responses) to the Chat Completions API (/v1/chat/completions).
type ResponsesAPIPlugin struct{}

func (p *ResponsesAPIPlugin) Name() string {
	return "responses-api"
}

// ResponsesAPIRequest represents the incoming request body for /v1/responses
type ResponsesAPIRequest struct {
	Model           string        `json:"model"`
	Input           []interface{} `json:"input"` // messages
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
		// If unmarshal fails, maybe it's not a JSON body or malformed.
		// We'll log it and let it pass through (upstream might reject it).
		log.Printf("[responses-api] Warning: failed to parse request body: %v", err)
		req.Body = io.NopCloser(bytes.NewReader(bodyBytes))
		return nil
	}

	// 4. Transform to Chat Completion Request
	chatReq := ChatCompletionRequest{
		Model:       respReq.Model,
		Messages:    respReq.Input,
		Stream:      respReq.Stream,
		Temperature: respReq.Temperature,
		TopP:        respReq.TopP,
		N:           respReq.N,
		Tools:       respReq.Tools,
		ToolChoice:  respReq.ToolChoice,
	}

	// Map max_output_tokens -> max_tokens
	if respReq.MaxOutputTokens > 0 {
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

	log.Printf("[responses-api] Rewrote request /v1/responses -> /v1/chat/completions")
	return nil
}

func (p *ResponsesAPIPlugin) ProcessResponse(resp *http.Response, ctx *goproxy.ProxyCtx, verbose bool) error {
	// We only process responses if the original request was to /v1/responses.
	// However, since we rewrote the path in ProcessRequest, the req.URL.Path in context might be /v1/chat/completions.
	// But we can check if this plugin's ProcessRequest logic ran.
	// Actually, goproxy doesn't easily share state between Request and Response phases per request unless we use ctx.UserData.
	// But `ProcessRequest` modifies the `http.Request` object in place.
	// A robust way is to check if we are in a response to a request we modified.
	// Since we can't easily track that without UserData (which we might not have full control over in this interface signature if it varies),
	// we will assume that if we are enabled, we might want to process it.
	// BUT, `ProcessResponse` is called for *all* responses. We need to know if it was originally /v1/responses.
	
	// Wait, `req` in `ProcessResponse` is the *request that received the response*.
	// Since we rewrote the URL in `ProcessRequest`, `resp.Request.URL.Path` will likely be `/v1/chat/completions`.
	// We might need a way to flag this request.
	// We can add a custom header in `ProcessRequest` and check for it here, then remove it.
	
	// Let's rely on a custom internal header.
	if resp.Request != nil && resp.Request.Header.Get("X-DevProxy-Responses-API") != "true" {
		return nil
	}

	// Clean up the marker header from the response (it shouldn't be there, but good practice to ignore)
	// Actually, we can't easily modify the *request* headers at this stage for the client, but it doesn't matter.

	if resp.StatusCode != http.StatusOK {
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

	go func() {
		defer originalBody.Close()
		defer writer.Close()

		br := bufio.NewReaderSize(originalBody, 1024*1024)
		var responseID string
		var itemID string
		var seqNum int
		var createdAt int64
		var model string
		var fullContent strings.Builder
		var completedSent bool

		for {
			line, err := br.ReadString('\n')
			trimmedLine := strings.TrimRight(line, "\r\n")

			if trimmedLine == "" && line != "" {
				writer.Write([]byte("\n"))
			} else if trimmedLine != "" {
				if strings.HasPrefix(trimmedLine, "data: ") {
					data := strings.TrimPrefix(trimmedLine, "data: ")
					if data == "[DONE]" {
						if !completedSent && responseID != "" {
							p.sendCompletionEvents(writer, responseID, itemID, createdAt, model, fullContent.String(), nil)
							completedSent = true
						}
						writer.Write([]byte("data: [DONE]\n\n"))
					} else {
						var chunk ChatCompletionChunk
						if errUnmarshal := json.Unmarshal([]byte(data), &chunk); errUnmarshal == nil && len(chunk.ID) > 0 {
							if responseID == "" {
								responseID = strings.Replace(chunk.ID, "chatcmpl-", "resp_", 1)
								itemID = fmt.Sprintf("msg_%s_0", responseID)
								createdAt = chunk.Created
								model = chunk.Model

								p.writeEvent(writer, "response.created", ResponsesAPIEvent{
									Type: "response.created", ResponseID: responseID,
									Response: &ResponsesAPIResponse{ID: responseID, Object: "response", CreatedAt: createdAt, Model: model},
								})
								p.writeEvent(writer, "response.output_item.added", ResponsesAPIEvent{
									Type: "response.output_item.added", ResponseID: responseID,
									OutputIndex: 0,
									Item: &Item{ID: itemID, Type: "message", Status: "in_progress", Role: "assistant"},
								})
								p.writeEvent(writer, "response.content_part.added", ResponsesAPIEvent{
									Type: "response.content_part.added", ResponseID: responseID, ItemID: itemID,
									ContentPart: &ContentPart{Type: "output_text", Index: 0},
								})
							}

							if len(chunk.Choices) > 0 {
								delta := chunk.Choices[0].Delta.Content
								if delta != "" {
									fullContent.WriteString(delta)
									seqNum++
									p.writeEvent(writer, "response.output_text.delta", ResponsesAPIEvent{
										Type: "response.output_text.delta", ResponseID: responseID, ItemID: itemID, Delta: delta, SequenceNumber: seqNum,
									})
								}
								if chunk.Choices[0].FinishReason != nil {
									p.sendCompletionEvents(writer, responseID, itemID, createdAt, model, fullContent.String(), chunk.Usage)
									completedSent = true
								}
							}
						} else {
							// Pass through just in case
							writer.Write([]byte(trimmedLine + "\n\n"))
						}
					}
				} else {
					writer.Write([]byte(trimmedLine + "\n"))
				}
			}

			if err != nil {
				break
			}
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
		Type:       "response.output_item.done",
		ResponseID: responseID,
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

func (p *ResponsesAPIPlugin) writeEvent(w io.Writer, eventType string, data interface{}) {
	payload, _ := json.Marshal(data)
	w.Write([]byte(fmt.Sprintf("event: %s\ndata: %s\n\n", eventType, string(payload))))
}
