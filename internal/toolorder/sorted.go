//
// Tencent is pleased to support the open source community by making trpc-agent-go available.
//
// Copyright (C) 2025 Tencent.  All rights reserved.
//
// trpc-agent-go is licensed under the Apache License Version 2.0.
//
//

// Package toolorder provides deterministic ordering helpers for model tools.
package toolorder

import (
	"sort"

	"trpc.group/trpc-go/trpc-agent-go/tool"
)

// SortedTools returns tools in deterministic request order.
//
// Model adapters and telemetry both use this helper so the exported tool
// definitions match the order sent to model providers.
func SortedTools(tools map[string]tool.Tool) []tool.Tool {
	return SortedToolsWithOrder(tools, nil)
}

// SortedToolsWithOrder returns tools in deterministic request order with the
// named order honored first: the names that order carries and the tool map
// holds lead in that order, the names it omits follow in name order. An unknown
// name in order is skipped, a nil tool or a tool without a declaration is never
// emitted, and an empty order is exactly SortedTools. The order exists so a
// caller can keep stable tools ahead of tools it loaded later, which keeps the
// provider-side prefix cache valid for the larger stable part of the request.
func SortedToolsWithOrder(tools map[string]tool.Tool, order []string) []tool.Tool {
	held := func(name string) bool {
		t, ok := tools[name]
		return ok && t != nil && t.Declaration() != nil
	}
	names := make([]string, 0, len(tools))
	seen := make(map[string]bool, len(tools))
	for _, name := range order {
		if seen[name] || !held(name) {
			continue
		}
		seen[name] = true
		names = append(names, name)
	}
	rest := make([]string, 0, len(tools))
	for name := range tools {
		if seen[name] || !held(name) {
			continue
		}
		rest = append(rest, name)
	}
	sort.Strings(rest)
	names = append(names, rest...)

	result := make([]tool.Tool, 0, len(names))
	for _, name := range names {
		result = append(result, tools[name])
	}
	return result
}
