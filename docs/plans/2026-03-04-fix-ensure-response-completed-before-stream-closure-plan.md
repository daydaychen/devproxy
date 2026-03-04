---
title: "fix: Ensure response.completed is sent before stream closure"
type: fix
status: completed
date: 2026-03-04
origin: docs/brainstorms/2026-03-04-fix-stream-disconnection-before-completion-brainstorm.md
---

# fix: Ensure response.completed is sent before stream closure

## Overview
This plan addresses a bug in the `OpenAIResponsesPlugin` where the target application reports `Stream disconnected before completion: stream closed before response.completed`. The issue occurs because the proxy server closes the connection immediately after forwarding `data: [DONE]` from the upstream, but before sending the required `response.completed` event in the converted Responses API format.

## Problem Statement / Motivation
The client (target program) expects the `response.completed` event to signal a successful completion of the stream. If the proxy closes the connection (EOF) after forwarding a `data: [DONE]` message but before sending `response.completed`, the client perceives it as a premature disconnection. Furthermore, if the upstream stream ends abruptly without a `finish_reason` in the last data chunk, the `response.completed` event is never triggered in the current implementation.

## Proposed Solution
Refactor the `handleStream` method in `pkg/proxy/plugin_openai_responses.go` to:
1.  **Track Completion State**: Introduce a `completedSent` boolean to ensure completion events are sent exactly once.
2.  **Process Partial Lines at EOF**: Fix the `bufio.Reader` loop to process the final line even if `io.EOF` is returned simultaneously.
3.  **Prioritize Completion Events**: Ensure `response.completed` (and its prerequisites like `response.output_text.done`) are sent *before* forwarding the `data: [DONE]` signal.
4.  **Robust Fallback**: Ensure that any unexpected stream closure (EOF) triggers the completion events if they haven't been sent yet.

## Technical Considerations
- **Architecture impacts**: None, this is a local logic fix within the plugin.
- **Performance implications**: Minimal, adding a few bytes and a boolean flag.
- **Security considerations**: None.

## System-Wide Impact
- **Interaction graph**: `ProxyServer` calls `OpenAIResponsesPlugin.ProcessResponse`, which calls `handleStream`. `handleStream` uses an `io.Pipe` to bridge the asynchronous processing with the `http.Response.Body`.
- **Error propagation**: Errors in `ReadString` are logged and trigger the fallback completion event sequence.
- **State lifecycle risks**: The `completedSent` flag prevents double-sending events if both a `finish_reason` and a `[DONE]` message are present.
- **API surface parity**: All stream-based responses transformed by this plugin will now follow the same robust completion logic.
- **Integration test scenarios**:
    - Stream ending with `finish_reason`.
    - Stream ending with `data: [DONE]` (no `finish_reason`).
    - Stream ending with raw EOF (no `[DONE]`, no `finish_reason`).
    - Stream ending with partial data at EOF (missing trailing newline).

## Acceptance Criteria
- [x] `response.completed` event is ALWAYS sent before the stream closes.
- [x] `data: [DONE]` is forwarded AFTER `response.completed`.
- [x] Streams ending without `finish_reason` still trigger `response.completed`.
- [x] Streams ending without a trailing newline before EOF still process the final line.
- [x] No duplicate `response.completed` events are sent.

## Success Metrics
- Target program no longer reports "stream closed before response.completed".
- All `go test ./pkg/proxy` tests pass.

## Dependencies & Risks
- **Dependencies**: Relies on `bufio.Reader` and `io.Pipe` behavior.
- **Risks**: If `responseID` is never established (e.g., empty response), no completion events are sent (correct behavior).

## Sources & References
- **Origin brainstorm:** [docs/brainstorms/2026-03-04-fix-stream-disconnection-before-completion-brainstorm.md](docs/brainstorms/2026-03-04-fix-stream-disconnection-before-completion-brainstorm.md) (see brainstorm: docs/brainstorms/2026-03-04-fix-stream-disconnection-before-completion-brainstorm.md)
- **Key decisions carried forward:**
    - Use `completedSent` flag for exactly-once delivery.
    - Process partial lines at EOF.
    - Send completion events before `[DONE]`.
- Similar implementations: `pkg/proxy/plugin_openai_responses.go:184`
- Best practices: SSE (Server-Sent Events) spec recommends terminating with a `[DONE]` signal but application logic often requires custom "end of object" events.
