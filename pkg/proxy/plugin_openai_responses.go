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
	OutputItem       *Item                 `json:"output_item,omitempty"`
	ContentPart      *ContentPart          `json:"content_part,omitempty"`
	ContentPartIndex int                   `json:"content_part_index,omitempty"`
	Delta            string                `json:"delta,omitempty"`
	SequenceNumber   int                   `json:"sequence_number,omitempty"`
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
	ID      string    `json:"id"`
	Type    string    `json:"type"`
	Status  string    `json:"status,omitempty"`
	Role    string    `json:"role,omitempty"`
	Content []Content `json:"content,omitempty"`
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
	// 禁用缓存，确保实时性
	resp.Header.Set("Cache-Control", "no-cache")
	resp.Header.Set("Connection", "keep-alive")

	go func() {
		defer originalBody.Close()
		defer writer.Close()

		// 使用 bufio.Reader 替代 Scanner，避免 64KB 行长限制
		br := bufio.NewReaderSize(originalBody, 1024*1024) // 1MB buffer
		var responseID string
		var itemID string
		var seqNum int
		var createdAt int64
		var model string
		var fullContent strings.Builder

		var completedSent bool
		for {
			line, err := br.ReadString('\n')
			// 即使有 err (如 io.EOF)，ReadString 也可能返回已读取的数据
			// 处理当前行
			trimmedLine := strings.TrimRight(line, "\r\n")
			
			if trimmedLine == "" && line != "" {
				// 这是一个空行，SSE 事件分隔符，原样转发
				writer.Write([]byte("\n"))
			} else if trimmedLine != "" {
				if strings.HasPrefix(trimmedLine, "data: ") {
					data := strings.TrimPrefix(trimmedLine, "data: ")
					if data == "[DONE]" {
						// 收到 [DONE] 时，如果还没发送完成事件，先补发
						if !completedSent && responseID != "" {
							p.sendCompletionEvents(writer, responseID, itemID, createdAt, model, fullContent.String(), nil)
							completedSent = true
						}
						writer.Write([]byte("data: [DONE]\n\n"))
					} else {
						var chunk ChatCompletionChunk
						if errUnmarshal := json.Unmarshal([]byte(data), &chunk); errUnmarshal != nil {
							if verbose {
								log.Printf("[openai-responses] 流式 chunk 解析失败 (原样转发): %v", errUnmarshal)
							}
							writer.Write([]byte(trimmedLine + "\n\n"))
						} else {
							if responseID == "" {
								responseID = strings.Replace(chunk.ID, "chatcmpl-", "resp_", 1)
								itemID = fmt.Sprintf("msg_%s_0", responseID)
								createdAt = chunk.Created
								model = chunk.Model
								
								// 1. response.created
								p.writeEvent(writer, "response.created", ResponsesAPIEvent{
									Type:       "response.created",
									ResponseID: responseID,
									Response: &ResponsesAPIResponse{
										ID:        responseID,
										Object:    "response",
										CreatedAt: createdAt,
										Model:     model,
									},
								})
								// 2. response.output_item.added
								p.writeEvent(writer, "response.output_item.added", ResponsesAPIEvent{
									Type:       "response.output_item.added",
									ResponseID: responseID,
									OutputItem: &Item{
										ID:     itemID,
										Type:   "message",
										Status: "in_progress",
										Role:   "assistant",
									},
								})
								// 3. response.content_part.added
								p.writeEvent(writer, "response.content_part.added", ResponsesAPIEvent{
									Type:       "response.content_part.added",
									ResponseID: responseID,
									ItemID:     itemID,
									ContentPart: &ContentPart{
										Type:  "output_text",
										Index: 0,
									},
								})
							}

							// 发送内容增量
							if len(chunk.Choices) > 0 {
								delta := chunk.Choices[0].Delta.Content
								if delta != "" {
									fullContent.WriteString(delta)
									seqNum++
									p.writeEvent(writer, "response.output_text.delta", ResponsesAPIEvent{
										Type:             "response.output_text.delta",
										ResponseID:       responseID,
										ItemID:           itemID,
										ContentPartIndex: 0,
										Delta:            delta,
										SequenceNumber:   seqNum,
									})
								}

								// 检查结束
								if chunk.Choices[0].FinishReason != nil {
									p.sendCompletionEvents(writer, responseID, itemID, createdAt, model, fullContent.String(), chunk.Usage)
									completedSent = true
								}
							}
						}
					}
				} else {
					// 非 data 行 (如 event: 头部或其他注释)，原样转发
					writer.Write([]byte(trimmedLine + "\n"))
				}
			}

			if err != nil {
				if err != io.EOF {
					log.Printf("[openai-responses] 流读取错误: %v", err)
				}
				// 最终兜底：如果有未发送的 response.completed，在这里补发
				if responseID != "" && !completedSent {
					p.sendCompletionEvents(writer, responseID, itemID, createdAt, model, fullContent.String(), nil)
					completedSent = true
				}
				break
			}
		}

		if verbose {
			log.Printf("[openai-responses] 流式转换完成: %s", responseID)
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
		OutputItem: &Item{
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
