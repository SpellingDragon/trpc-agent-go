//
// Tencent is pleased to support the open source community by making trpc-agent-go available.
//
// Copyright (C) 2025 Tencent.  All rights reserved.
//
// trpc-agent-go is licensed under the Apache License Version 2.0.
//
//

package toolcall

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"trpc.group/trpc-go/trpc-agent-go/model"
)

// downgradePayloadLimit pins the documented 4KB payload budget independently of
// the package constant so these tests fail if the limit itself drifts.
const downgradePayloadLimit = 4 << 10

// truncationMarkerPattern matches the annotation appended after a clamped payload.
const truncationMarkerPattern = `\[truncated \d+ bytes\]`

// wantInvalidToolCallFormat and wantOrphanToolCallFormat are golden renderings of
// the downgrade messages. They are duplicated here on purpose: a downgraded
// payload under the limit must stay byte-for-byte what it was before the limit
// was introduced.
const (
	wantInvalidToolCallFormat = "%s Tool call arguments were downgraded to a user message (%s)." +
		"\nname: %s\nid: %s\narguments:\n```text\n%s\n```"
	wantOrphanToolCallFormat = "%s Tool call was downgraded to a user message because " +
		"no matching tool result exists.\nname: %s\nid: %s\narguments:\n```text\n%s\n```"
)

// callMessages builds an assistant message holding a single tool call without a
// matching tool result, so sanitize downgrades it as invalid or orphaned
// depending on whether the arguments parse as JSON.
func callMessages(payload string) []model.Message {
	return []model.Message{
		{
			Role: model.RoleAssistant,
			ToolCalls: []model.ToolCall{
				{
					ID: "call_1",
					Function: model.FunctionDefinitionParam{
						Name:      "test_tool",
						Arguments: []byte(payload),
					},
				},
			},
		},
	}
}

// assertFencedAndMarked checks that a clamped downgrade message still closes its
// markdown code fence and carries the truncation marker after that fence.
func assertFencedAndMarked(t *testing.T, content string) {
	t.Helper()
	assert.Regexp(t, truncationMarkerPattern, content)
	assert.Equal(t, 2, strings.Count(content, "```"), "fenced block must stay balanced")
	closingFence := strings.LastIndex(content, "```")
	marker := strings.Index(content, "[truncated ")
	if assert.NotEqual(t, -1, marker) && assert.NotEqual(t, -1, closingFence) {
		assert.Less(t, closingFence, marker, "marker must follow the closing fence")
	}
	assert.True(t, strings.HasSuffix(content, "\n"), "message must end with a newline")
	assert.True(t, utf8.ValidString(content), "truncation must not split a UTF-8 sequence")
}

// markerOffset reports the byte offset where the truncation marker line starts,
// which equals the length of the message rendered before the marker.
func markerOffset(t *testing.T, content string) int {
	t.Helper()
	idx := strings.Index(content, "\n[truncated ")
	if !assert.NotEqual(t, -1, idx, "truncation marker must be present") {
		return -1
	}
	return idx
}

func TestSanitizeMessagesWithTools_TruncatesOversizedInvalidToolCallArguments(t *testing.T) {
	payload := `{"path":"/tmp/big.txt","content":"` + strings.Repeat("a", 8<<10)
	out := SanitizeMessagesWithTools(context.Background(), callMessages(payload), nil)
	if !assert.Len(t, out, 1) {
		return
	}
	assert.Equal(t, model.RoleUser, out[0].Role)
	content := out[0].Content
	assert.Contains(t, content, invalidToolCallTag)
	assert.Contains(t, content, "call_1")
	assert.Contains(t, content, `{"path":"/tmp/big.txt","content":"aaa`, "payload prefix must survive")
	assert.LessOrEqual(t, len(content), downgradePayloadLimit+32)
	assert.Equal(t, downgradePayloadLimit, markerOffset(t, content))
	assertFencedAndMarked(t, content)
}

func TestSanitizeMessagesWithTools_TruncatesOversizedOrphanToolCallArguments(t *testing.T) {
	payload := `{"text":"` + strings.Repeat("b", 8<<10) + `"}`
	out := SanitizeMessagesWithTools(context.Background(), callMessages(payload), nil)
	if !assert.Len(t, out, 1) {
		return
	}
	assert.Equal(t, model.RoleUser, out[0].Role)
	content := out[0].Content
	assert.Contains(t, content, orphanToolCallTag)
	assert.NotContains(t, content, invalidToolCallTag)
	assert.Contains(t, content, `{"text":"bbb`, "payload prefix must survive")
	assert.LessOrEqual(t, len(content), downgradePayloadLimit+32)
	assert.Equal(t, downgradePayloadLimit, markerOffset(t, content))
	assertFencedAndMarked(t, content)
}

func TestSanitizeMessagesWithTools_KeepsSmallToolCallArgumentsByteForByte(t *testing.T) {
	invalidPayload := `{"path":"/tmp/small.txt"` + strings.Repeat("a", 1024)
	orphanPayload := `{"text":"` + strings.Repeat("b", 1024) + `"}`
	cases := []struct {
		name    string
		payload string
		want    string
	}{
		{
			name:    "invalid call",
			payload: invalidPayload,
			want: fmt.Sprintf(
				wantInvalidToolCallFormat,
				invalidToolCallTag,
				errArgumentsNotValidJSON.Error(),
				"test_tool",
				"call_1",
				invalidPayload,
			),
		},
		{
			name:    "orphan call",
			payload: orphanPayload,
			want: fmt.Sprintf(
				wantOrphanToolCallFormat,
				orphanToolCallTag,
				"test_tool",
				"call_1",
				orphanPayload,
			),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := SanitizeMessagesWithTools(context.Background(), callMessages(tc.payload), nil)
			if !assert.Len(t, out, 1) {
				return
			}
			assert.Equal(t, tc.want, out[0].Content)
			assert.NotRegexp(t, truncationMarkerPattern, out[0].Content)
		})
	}
}

func TestSanitizeMessagesWithTools_TruncatesOnlyAbovePayloadBudget(t *testing.T) {
	overhead := len(fmt.Sprintf(wantOrphanToolCallFormat, orphanToolCallTag, "test_tool", "call_1", ""))
	budget := downgradePayloadLimit - overhead
	if !assert.Greater(t, budget, 0) {
		return
	}
	atLimit := `"` + strings.Repeat("a", budget-2) + `"`
	if !assert.Len(t, atLimit, budget) {
		return
	}
	out := SanitizeMessagesWithTools(context.Background(), callMessages(atLimit), nil)
	if assert.Len(t, out, 1) {
		assert.Equal(t, downgradePayloadLimit, len(out[0].Content))
		assert.NotRegexp(t, truncationMarkerPattern, out[0].Content)
	}

	overLimit := `"` + strings.Repeat("a", budget-1) + `"`
	if !assert.Len(t, overLimit, budget+1) {
		return
	}
	out = SanitizeMessagesWithTools(context.Background(), callMessages(overLimit), nil)
	if assert.Len(t, out, 1) {
		assert.Contains(t, out[0].Content, "[truncated 1 bytes]")
		assert.LessOrEqual(t, len(out[0].Content), downgradePayloadLimit+32)
		assertFencedAndMarked(t, out[0].Content)
	}
}

func TestSanitizeMessagesWithTools_TruncatesOnRuneBoundary(t *testing.T) {
	payload := `{"text":"` + strings.Repeat("好", 3000) + `"}`
	out := SanitizeMessagesWithTools(context.Background(), callMessages(payload), nil)
	if !assert.Len(t, out, 1) {
		return
	}
	assert.LessOrEqual(t, len(out[0].Content), downgradePayloadLimit+32)
	assertFencedAndMarked(t, out[0].Content)
}

// downgradeResultOf builds one tool result message of the given size.
func downgradeResultOf(size int) model.Message {
	return model.Message{Role: model.RoleTool, ToolID: "call-result", ToolName: "worker", Content: strings.Repeat("r", size)}
}

func TestTruncatesOversizedInvalidToolResultContent(t *testing.T) {
	msg := downgradeInvalidToolResult(context.Background(), downgradeResultOf(9000))
	if len(msg.Content) > 4<<10+512 {
		t.Fatalf("downgraded invalid result content = %d bytes, want at most the 4KB payload budget plus its header", len(msg.Content))
	}
	if !strings.Contains(msg.Content, "[truncated ") {
		t.Fatalf("downgraded invalid result carries no truncation marker")
	}
}

func TestTruncatesOversizedOrphanToolResultContent(t *testing.T) {
	msg := downgradeOrphanToolResult(context.Background(), downgradeResultOf(9000))
	if len(msg.Content) > 4<<10+512 {
		t.Fatalf("downgraded orphan result content = %d bytes, want at most the 4KB payload budget plus its header", len(msg.Content))
	}
	if !strings.Contains(msg.Content, "[truncated ") {
		t.Fatalf("downgraded orphan result carries no truncation marker")
	}
}
