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

// OpenAIResponsesPlugin 将 OpenAI Chat Completions API 的响应转换为 Responses API 响应格式
type OpenAIResponsesPlugin struct{}

type gzipReadCloser struct {
	io.Reader
	orig io.ReadCloser
}

func (g *gzipReadCloser) Close() error {
	return g.orig.Close()
}

func (p *OpenAIResponsesPlugin) Name() string {
	return "openai-responses"
}

// ProcessRequest 在请求发送前执行，确保上游返回明文以供插件处理
func (p *OpenAIResponsesPlugin) ProcessRequest(req *http.Request) error {
	// 针对 OpenAI 请求，强制要求明文响应，以便后续 ProcessResponse 处理
	req.Header.Set("Accept-Encoding", "identity")
	return nil
}

// ChatCompletionChunk 为 Chat Completion 流式响应块结构
type ChatCompletionChunk struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	Model   string `json:"model"`
	Choices []struct {
		Index int `json:"index"`
		Delta struct {
			Role    string `json:"role,omitempty"`
			Content string `json:"content,omitempty"`
		} `json:"delta"`
		FinishReason *string `json:"finish_reason,omitempty"`
	} `json:"choices"`
	Usage map[string]interface{} `json:"usage,omitempty"`
}

// ResponsesAPIEvent 为 Responses API 流式事件结构
type ResponsesAPIEvent struct {
	Type             string                `json:"type"`
	ResponseID       string                `json:"response_id,omitempty"`
	ItemID           string                `json:"item_id,omitempty"`
	Item             *Item                 `json:"item,omitempty"`
	ContentPart      *ContentPart          `json:"content_part,omitempty"`
	ContentPartIndex int                   `json:"content_part_index,omitempty"`
	Delta            string                `json:"delta,omitempty"`
	SequenceNumber   int                   `json:"sequence_number,omitempty"`
	OutputIndex      int                   `json:"output_index"`
	Usage            interface{}           `json:"usage,omitempty"`
	Response         *ResponsesAPIResponse `json:"response,omitempty"`
}

type ContentPart struct {
	Type  string `json:"type"`
	Index int    `json:"index"`
	Text  string `json:"text,omitempty"`
}

// ResponsesAPIResponse 为 Responses API 响应结构
type ResponsesAPIResponse struct {
	ID        string `json:"id"`
	Object    string `json:"object"`
	CreatedAt int64  `json:"created_at"`
	Model     string `json:"model"`
	Output    []Item `json:"output"`
	Usage     interface{} `json:"usage,omitempty"`
}

type Item struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"` // "message" or "function_call"
	Status    string    `json:"status,omitempty"`
	Role      string    `json:"role,omitempty"`
	Content   []Content `json:"content,omitempty"`
	Name      string    `json:"name,omitempty"`
	Arguments string    `json:"arguments,omitempty"`
	CallID    string    `json:"call_id,omitempty"`
}

type Content struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func (p *OpenAIResponsesPlugin) ProcessResponse(resp *http.Response, ctx *goproxy.ProxyCtx, verbose bool) error {
	// 仅处理状态码为 200 的请求
	if resp.StatusCode != http.StatusOK {
		if verbose {
			log.Printf("[openai-responses] 跳过: 状态码为 %d", resp.StatusCode)
		}
		return nil
	}
	contentType := resp.Header.Get("Content-Type")

	// 无论处理还是直连，只要匹配了本插件，就应该处理可能的压缩并删除对应头部
	// 因为我们的处理逻辑目前仅输出解压后的内容
	if resp.Header.Get("Content-Encoding") == "gzip" {
		gr, err := gzip.NewReader(resp.Body)
		if err == nil {
			// 将解压后的 Reader 替换回去，并移除编码头部
			resp.Body = &gzipReadCloser{Reader: gr, orig: resp.Body}
			resp.Header.Del("Content-Encoding")
			resp.Header.Del("Content-Length") // 长度已变
			resp.ContentLength = -1
		} else if verbose {
			log.Printf("[openai-responses] 无法创建 gzip reader: %v", err)
		}
	}

	// 处理普通 JSON
	if strings.Contains(contentType, "application/json") {
		return p.handleJSON(resp, verbose)
	}

	// 处理流式
	if strings.Contains(contentType, "text/event-stream") {
		return p.handleStream(resp, verbose)
	}

	if verbose {
		log.Printf("[openai-responses] 跳过: 未知 Content-Type: %s", contentType)
	}
	return nil
}

func (p *OpenAIResponsesPlugin) handleJSON(resp *http.Response, verbose bool) error {

	// 读取原始 Body
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("openai-responses: 读取响应体失败: %w", err)
	}
	resp.Body.Close()

	if len(bodyBytes) == 0 {
		resp.Body = io.NopCloser(bytes.NewReader(bodyBytes))
		return nil
	}

	var chatResp ChatCompletionResponse
	if err := json.Unmarshal(bodyBytes, &chatResp); err != nil {
		if verbose {
			log.Printf("[openai-responses] 跳过: 无法解析为 ChatCompletionResponse: %v", err)
		}
		// 如果无法解析为 ChatCompletionResponse，可能不是目标请求，原样回写
		resp.Body = io.NopCloser(bytes.NewReader(bodyBytes))
		return nil
	}

	// 如果 object 不是 chat.completion，大概率不是我们要转的
	if chatResp.Object != "chat.completion" {
		if verbose {
			log.Printf("[openai-responses] 跳过: Object 为 %s (期望 chat.completion)", chatResp.Object)
		}
		resp.Body = io.NopCloser(bytes.NewReader(bodyBytes))
		return nil
	}

	// 转换逻辑
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

	// 序列化新 Body
	newBodyBytes, err := json.Marshal(resResp)
	if err != nil {
		return fmt.Errorf("openai-responses: 序列化新响应体失败: %w", err)
	}

	// 更新响应
	resp.Body = io.NopCloser(bytes.NewReader(newBodyBytes))
	resp.ContentLength = int64(len(newBodyBytes))
	resp.Header.Set("Content-Length", fmt.Sprint(len(newBodyBytes)))
	
	log.Printf("[openai-responses] 转换成功: %s (%s) -> %s (response), Items: %d", 
		chatResp.ID, chatResp.Object, resResp.ID, len(resResp.Output))
	return nil
}

func (p *OpenAIResponsesPlugin) handleStream(resp *http.Response, verbose bool) error {
	reader, writer := io.Pipe()
	originalBody := resp.Body
	resp.Body = reader
	// 确保流式响应的头部正确
	resp.ContentLength = -1
	resp.Header.Del("Content-Length")
	// 禁用缓存和缓冲区，确保实时性
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
						// 1. 首先尝试检测是否已经是 Responses API 格式 (含有 "type":)
						if strings.Contains(data, "\"type\":\"") {
							// 手动提取类型以保留所有原始字段 (避免 Unmarshal 丢弃未知字段)
							eventType := "message"
							if typeIdx := strings.Index(data, "\"type\":\""); typeIdx != -1 {
								typeStart := typeIdx + 8
								typeEnd := strings.Index(data[typeStart:], "\"")
								if typeEnd != -1 {
									eventType = data[typeStart : typeStart+typeEnd]
								}
							}

							if eventType == "response.completed" {
								completedSent = true
								// 核心修复：补全工具调用的生命周期事件
								var compEvent ResponsesAPIEvent
								if errJson := json.Unmarshal([]byte(data), &compEvent); errJson == nil && compEvent.Response != nil {
									for i, item := range compEvent.Response.Output {
										if item.Type == "function_call" {
											// 确保 SDK 收到 added 和 done 事件
											p.writeEvent(writer, "response.output_item.added", ResponsesAPIEvent{
												Type:       "response.output_item.added",
												ResponseID: responseID,
												OutputIndex: i,
												Item:       &item,
											})
											p.writeEvent(writer, "response.output_item.done", ResponsesAPIEvent{
												Type:       "response.output_item.done",
												ResponseID: responseID,
												OutputIndex: i,
												Item:       &item,
											})
										}
									}
								}
							}

							// 记录元数据用于兜底
							if responseID == "" && strings.Contains(data, "\"id\":\"") {
								idIdx := strings.Index(data, "\"id\":\"") + 6
								idEnd := strings.Index(data[idIdx:], "\"")
								if idEnd != -1 {
									responseID = data[idIdx : idIdx+idEnd]
								}
							}

							// 规范化：修正 object 字段 (手术级替换)
							modifiedData := strings.Replace(data, "\"object\":\"chat.completion\"", "\"object\":\"response\"", 1)
							
							// 补全 event: 头部并转发原始 JSON (保留所有未知字段)
							writer.Write([]byte(fmt.Sprintf("event: %s\ndata: %s\n\n", eventType, modifiedData)))
						} else {
							// 2. 降级尝试解析为标准 ChatCompletionChunk并转换
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
								// 兜底转发
								writer.Write([]byte(trimmedLine + "\n\n"))
							}
						}
					}
				} else {
					writer.Write([]byte(trimmedLine + "\n"))
				}
			}

			if err != nil {
				if responseID != "" && !completedSent {
					p.sendCompletionEvents(writer, responseID, itemID, createdAt, model, fullContent.String(), nil)
					completedSent = true
					writer.Write([]byte("data: [DONE]\n\n"))
				}
				break
			}
		}

		if verbose {
			log.Printf("[openai-responses] 流式转换完成并关闭 Pipe: %s", responseID)
		}
	}()

	return nil
}

// sendCompletionEvents 发送流式结束时的完整事件序列
func (p *OpenAIResponsesPlugin) sendCompletionEvents(w io.Writer, responseID, itemID string, createdAt int64, model, text string, usage interface{}) {
	// 4. response.output_text.done
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
	// 5. response.content_part.done
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
	// 6. response.output_item.done
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
	// 7. response.completed
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

func (p *OpenAIResponsesPlugin) writeEvent(w io.Writer, eventType string, data interface{}) {
	payload, _ := json.Marshal(data)
	// OpenAI Responses API 强制要求 event 头部，否则 SDK 可能无法解析语义
	w.Write([]byte(fmt.Sprintf("event: %s\ndata: %s\n\n", eventType, string(payload))))
}

// 定义一个非流式的 ChatCompletionResponse 用于 handleJSON
type ChatCompletionResponse struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	Model   string `json:"model"`
	Choices []struct {
		Index   int `json:"index"`
		Message struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage map[string]interface{} `json:"usage"`
}
