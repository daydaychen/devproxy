package proxy

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/elazarl/goproxy"
)

// AnthropicThinkingFixPlugin 把上游缺少收尾事件的 Anthropic 风格流式事件补齐。
//
// 关键策略：上游 thinking 块没有真实加密 signature。只要客户端不收到
// signature_delta 就不会触发签名校验，能照常把 thinking 渲染成思考块；一旦
// 收到 signature_delta（哪怕是空的），客户端会去校验，校验失败就静默丢弃
// 后续的 text/tool_use。所以插件只做最小干预：
//   - thinking 块原样透传，不做任何改写
//   - 丢弃上游所有 signature_delta
//   - 补齐缺失的 message_delta / message_stop / content_block_stop
type AnthropicThinkingFixPlugin struct{}

func (p *AnthropicThinkingFixPlugin) Name() string {
	return "anthropic-thinking-fix"
}

func (p *AnthropicThinkingFixPlugin) ProcessRequest(req *http.Request) error {
	req.Header.Set("Accept-Encoding", "identity")
	return nil
}

func (p *AnthropicThinkingFixPlugin) ProcessResponse(resp *http.Response, ctx *goproxy.ProxyCtx, verbose bool) error {
	if resp.StatusCode != http.StatusOK {
		return nil
	}
	if !strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream") {
		return nil
	}

	reader, writer := io.Pipe()
	originalBody := resp.Body
	resp.Body = reader

	resp.ContentLength = -1
	resp.Header.Del("Content-Length")
	resp.Header.Set("Cache-Control", "no-cache")
	resp.Header.Set("Connection", "keep-alive")
	resp.Header.Set("X-Accel-Buffering", "no")

	go p.rewrite(originalBody, writer, verbose)
	return nil
}

type thinkingFixState struct {
	openIndex       int
	hasOpenBlock    bool
	sawToolUse      bool
	sawMessageDelta bool
	sawMessageStop  bool
}

func (p *AnthropicThinkingFixPlugin) rewrite(src io.ReadCloser, dst *io.PipeWriter, verbose bool) {
	defer src.Close()
	defer dst.Close()

	br := bufio.NewReader(src)
	state := &thinkingFixState{}

	var pendingEventType string

	for {
		line, err := br.ReadString('\n')
		if err != nil && err != io.EOF {
			if verbose {
				log.Printf("[%s] 读取响应体出错: %v", p.Name(), err)
			}
			p.flushTrailing(dst, state, verbose)
			return
		}

		trimmed := strings.TrimRight(line, "\r\n")
		switch {
		case strings.HasPrefix(trimmed, "event: "):
			pendingEventType = strings.TrimPrefix(trimmed, "event: ")
		case strings.HasPrefix(trimmed, "data: "):
			data := strings.TrimPrefix(trimmed, "data: ")
			p.dispatch(dst, state, pendingEventType, data, verbose)
			pendingEventType = ""
		case trimmed == "":
			// 丢弃上游空行；writeEvent / writeRawEvent 会自动写 \n\n 分隔
		default:
			_, _ = dst.Write([]byte(line))
		}

		if err == io.EOF {
			p.flushTrailing(dst, state, verbose)
			return
		}
	}
}

func (p *AnthropicThinkingFixPlugin) dispatch(dst io.Writer, state *thinkingFixState, eventType, data string, verbose bool) {
	var event map[string]interface{}
	if err := json.Unmarshal([]byte(data), &event); err != nil {
		p.writeRawEvent(dst, eventType, data)
		return
	}

	if t, _ := event["type"].(string); eventType == "" && t != "" {
		eventType = t
	}

	indexVal, _ := event["index"].(float64)
	index := int(indexVal)

	switch eventType {
	case "message_start":
		p.fixMessageStart(event)
		p.writeEvent(dst, eventType, event)

	case "content_block_start":
		state.openIndex = index
		state.hasOpenBlock = true
		cb, _ := event["content_block"].(map[string]interface{})
		if blockType, _ := cb["type"].(string); blockType == "tool_use" || blockType == "server_tool_use" {
			state.sawToolUse = true
		}
		p.writeRawEvent(dst, eventType, data)

	case "content_block_delta":
		delta, _ := event["delta"].(map[string]interface{})
		if deltaType, _ := delta["type"].(string); deltaType == "signature_delta" {
			if verbose {
				log.Printf("[%s] 丢弃 signature_delta(index=%d)", p.Name(), index)
			}
			return
		}
		p.writeRawEvent(dst, eventType, data)

	case "content_block_stop":
		state.hasOpenBlock = false
		p.writeRawEvent(dst, eventType, data)

	case "message_delta":
		state.sawMessageDelta = true
		p.writeRawEvent(dst, eventType, data)

	case "message_stop":
		p.closeLingeringBlock(dst, state, verbose)
		if !state.sawMessageDelta {
			p.writeMessageDelta(dst, state)
			state.sawMessageDelta = true
			if verbose {
				log.Printf("[%s] message_stop 前补发 message_delta", p.Name())
			}
		}
		state.sawMessageStop = true
		p.writeRawEvent(dst, eventType, data)

	default:
		// ping / error / 未来新事件类型：透传
		p.writeRawEvent(dst, eventType, data)
	}
}

func (p *AnthropicThinkingFixPlugin) fixMessageStart(event map[string]interface{}) {
	msg, ok := event["message"].(map[string]interface{})
	if !ok {
		return
	}
	if _, ok := msg["stop_reason"]; !ok {
		msg["stop_reason"] = nil
	}
	if _, ok := msg["stop_sequence"]; !ok {
		msg["stop_sequence"] = nil
	}
	if _, ok := msg["content"]; !ok {
		msg["content"] = []interface{}{}
	}
	if _, ok := msg["usage"]; !ok {
		msg["usage"] = map[string]interface{}{
			"input_tokens":  0,
			"output_tokens": 0,
		}
	}
}

func (p *AnthropicThinkingFixPlugin) closeLingeringBlock(dst io.Writer, state *thinkingFixState, verbose bool) {
	if !state.hasOpenBlock {
		return
	}
	idx := state.openIndex
	p.writeEvent(dst, "content_block_stop", map[string]interface{}{
		"type":  "content_block_stop",
		"index": idx,
	})
	state.hasOpenBlock = false
	if verbose {
		log.Printf("[%s] 收尾时补发 content_block_stop(index=%d)", p.Name(), idx)
	}
}

func (p *AnthropicThinkingFixPlugin) flushTrailing(dst io.Writer, state *thinkingFixState, verbose bool) {
	if state.sawMessageStop {
		return
	}
	p.closeLingeringBlock(dst, state, verbose)
	if !state.sawMessageDelta {
		p.writeMessageDelta(dst, state)
		state.sawMessageDelta = true
	}
	p.writeEvent(dst, "message_stop", map[string]interface{}{
		"type": "message_stop",
	})
	state.sawMessageStop = true
	if verbose {
		log.Printf("[%s] 上游未发 message_stop，补发收尾事件", p.Name())
	}
}

func (p *AnthropicThinkingFixPlugin) writeMessageDelta(dst io.Writer, state *thinkingFixState) {
	stopReason := "end_turn"
	if state.sawToolUse {
		stopReason = "tool_use"
	}
	p.writeEvent(dst, "message_delta", map[string]interface{}{
		"type": "message_delta",
		"delta": map[string]interface{}{
			"stop_reason":   stopReason,
			"stop_sequence": nil,
		},
		"usage": map[string]interface{}{
			"output_tokens": 0,
		},
	})
}

func (p *AnthropicThinkingFixPlugin) writeEvent(dst io.Writer, eventType string, event interface{}) {
	payload, err := json.Marshal(event)
	if err != nil {
		return
	}
	if eventType != "" {
		fmt.Fprintf(dst, "event: %s\n", eventType)
	}
	fmt.Fprintf(dst, "data: %s\n\n", payload)
}

func (p *AnthropicThinkingFixPlugin) writeRawEvent(dst io.Writer, eventType, rawData string) {
	if eventType != "" {
		fmt.Fprintf(dst, "event: %s\n", eventType)
	}
	fmt.Fprintf(dst, "data: %s\n\n", rawData)
}
