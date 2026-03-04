---
date: 2026-03-04
topic: fix-stream-disconnection-before-completion
---

# Fix Stream Disconnection Before Completion

## What We're Building
A fix for the `OpenAIResponsesPlugin` to ensure that the `response.completed` event is always sent before the stream closes, even in cases where the upstream stream ends abruptly or without a `finish_reason` in the last data chunk.

## Why This Approach
The client (target program) expects the `response.completed` event to signal a successful completion of the stream. If the proxy closes the connection (EOF) after forwarding a `data: [DONE]` message but before sending `response.completed`, the client perceives it as an error ("stream closed before response.completed").

## Key Decisions
- **Track Completion State**: Use a `completedSent` boolean to track if the completion events have already been sent, avoiding duplicate or missing events.
- **Process Partial Lines at EOF**: Fix a bug in the stream reading loop where the last line of data would be ignored if it didn't end with a newline character before the connection closed.
- **Priority of Completion Events**: When a `data: [DONE]` message is received, ensure `response.completed` events are sent *before* the `[DONE]` message is forwarded.
- **Robust Fallback**: Ensure that any unexpected stream closure (EOF) triggers the completion events if they haven't been sent yet.

## Next Steps
→ Verify the fix with the target application (user feedback).
→ Monitor for any side effects of the revised stream handling logic.
