---
title: "feat: Add ResponsesAPIPlugin for OpenAI Responses API Adaptation"
type: feat
status: completed
date: 2026-03-06
---

# feat: Add ResponsesAPIPlugin for OpenAI Responses API Adaptation

## Overview

We need a new plugin, `ResponsesAPIPlugin`, to act as an adapter for the new "OpenAI Responses API". This plugin will allow clients to use the `/v1/responses` endpoint with its associated request format, while the proxy transparently maps these requests to the standard `/v1/chat/completions` endpoint and transforms the responses back to the "Responses API" format.

## Problem Statement / Motivation

The "OpenAI Responses API" is a new, structured-output-first API that some modern SDKs and clients use. However, some upstream providers or local models only support the legacy (but standard) `/v1/chat/completions` API. To support these clients without modifying the upstream, we need a transparent proxy layer that handles the bidirectional transformation.

## Proposed Solution

Create a new plugin `ResponsesAPIPlugin` that implements both `RequestPlugin` and `ResponsePlugin`.

### Request Transformation (`ProcessRequest`):
- **Endpoint**: Intercept requests to `/v1/responses`.
- **Path Rewrite**: Change `req.URL.Path` to `/v1/chat/completions`.
- **Body Transformation**:
    - `input` (array of messages) -> `messages`
    - `model` -> `model`
    - `max_output_tokens` -> `max_tokens` (or `max_completion_tokens`)
    - `text.format.type == "json_schema"` -> `response_format: { type: "json_schema", json_schema: { ... } }`
    - `stream` (if present) -> `stream`

### Response Transformation (`ProcessResponse`):
- **Upstream Response**: The response from upstream will be a standard `Chat Completions` response.
- **Back-Transformation**: Transform it back to `Responses API` format (JSON or Stream).
- **Event Mapping (Streaming)**:
    - Map `chat.completion.chunk` to `response.*` events (e.g., `response.created`, `response.output_text.delta`, `response.completed`).
    - This part can leverage or share logic from the existing `OpenAIResponsesPlugin`.

## Technical Considerations

- **Path Rewriting**: Ensure `req.URL.Path` is updated before the request is forwarded to upstream.
- **Body Rewriting**: The `ProxyServer` already handles body caching if a `RequestPlugin` is active.
- **Overlap with `OpenAIResponsesPlugin`**: `OpenAIResponsesPlugin` currently handles some response-side mapping. `ResponsesAPIPlugin` should either:
    - Be a superset of `OpenAIResponsesPlugin` for the `/v1/responses` path.
    - Or use a shared library for the response transformation logic.
    - Since the user asked for a *new* plugin, we'll implement it as a standalone adapter that handles the full lifecycle for `/v1/responses`.

## System-Wide Impact

- **Interaction Graph**:
    1. Client `POST /v1/responses` -> `ProxyServer`
    2. `ProxyServer` matches rule -> calls `ResponsesAPIPlugin.ProcessRequest`
    3. `ResponsesAPIPlugin` transforms request -> `ProxyServer` forwards `POST /v1/chat/completions` to Upstream.
    4. Upstream responds `chat.completion` -> `ProxyServer`
    5. `ProxyServer` matches rule -> calls `ResponsesAPIPlugin.ProcessResponse`
    6. `ResponsesAPIPlugin` transforms response -> Client receives `Responses API` response.

## Acceptance Criteria

- [x] New file `pkg/proxy/plugin_responses_api.go` created.
- [x] `ResponsesAPIPlugin` implements `RequestPlugin` and `ResponsePlugin`.
- [x] Requests to `/v1/responses` are rewritten to `/v1/chat/completions`.
- [x] Request body is correctly transformed from `Responses API` format to `Chat Completions` format.
- [x] Response body (JSON) is correctly transformed back to `Responses API` format.
- [x] Streaming response (SSE) is correctly transformed to `response.*` events.
- [x] Unit tests in `pkg/proxy/plugin_responses_api_test.go` cover all scenarios.
- [x] Plugin is registered in `pkg/proxy/plugin.go`.

## MVP

### `pkg/proxy/plugin_responses_api.go` (Pseudo-code)

```go
type ResponsesAPIPlugin struct{}

func (p *ResponsesAPIPlugin) Name() string { return "responses-api" }

func (p *ResponsesAPIPlugin) ProcessRequest(req *http.Request) error {
    if req.URL.Path != "/v1/responses" {
        return nil
    }
    // 1. Rewrite path
    req.URL.Path = "/v1/chat/completions"
    
    // 2. Read and unmarshal body (ResponsesRequest)
    // 3. Transform to ChatCompletionRequest
    // 4. Marshal and update body
    return nil
}

func (p *ResponsesAPIPlugin) ProcessResponse(resp *http.Response, ctx *goproxy.ProxyCtx, verbose bool) error {
    // 1. Detect if this was a responses-api request (maybe via context or path)
    // 2. Transform ChatCompletion response back to ResponsesAPI format
    // (Similar logic to OpenAIResponsesPlugin but specifically for this flow)
    return nil
}
```

## Sources & References

- OpenAI Documentation for Responses API: [developers.openai.com/api/docs/guides/structured-outputs_api-mode=responses](https://developers.openai.com/api/docs/guides/structured-outputs_api-mode=responses)
- Existing implementation for response mapping: `pkg/proxy/plugin_openai_responses.go`
