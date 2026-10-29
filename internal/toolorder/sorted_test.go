//
// Tencent is pleased to support the open source community by making trpc-agent-go available.
//
// Copyright (C) 2025 Tencent.  All rights reserved.
//
// trpc-agent-go is licensed under the Apache License Version 2.0.
//
//

package toolorder

import (
	"testing"

	"github.com/stretchr/testify/require"
	"trpc.group/trpc-go/trpc-agent-go/tool"
)

func TestSortedTools(t *testing.T) {
	tools := map[string]tool.Tool{
		"gamma": testOrderTool{name: "gamma"},
		"alpha": testOrderTool{name: "alpha"},
		"skip":  nil,
		"delta": testOrderTool{name: "delta"},
		"beta":  testOrderTool{name: "beta"},
	}

	sorted := SortedTools(tools)
	require.Len(t, sorted, 4)
	require.Equal(t, "alpha", sorted[0].Declaration().Name)
	require.Equal(t, "beta", sorted[1].Declaration().Name)
	require.Equal(t, "delta", sorted[2].Declaration().Name)
	require.Equal(t, "gamma", sorted[3].Declaration().Name)
}

func TestSortedToolsFiltersNilDeclarationAndSortsByKey(t *testing.T) {
	tools := map[string]tool.Tool{
		"b-key": testOrderTool{name: "alpha"},
		"a-key": testOrderTool{name: "zeta"},
		"c-key": nilDeclarationTool{},
	}

	sorted := SortedTools(tools)
	require.Len(t, sorted, 2)
	require.Equal(t, "zeta", sorted[0].Declaration().Name)
	require.Equal(t, "alpha", sorted[1].Declaration().Name)
}

func TestSortedToolsWithOrder(t *testing.T) {
	tools := map[string]tool.Tool{
		"gamma": testOrderTool{name: "gamma"},
		"alpha": testOrderTool{name: "alpha"},
		"skip":  nil,
		"delta": testOrderTool{name: "delta"},
		"beta":  testOrderTool{name: "beta"},
	}
	names := func(list []tool.Tool) []string {
		out := make([]string, 0, len(list))
		for _, t := range list {
			out = append(out, t.Declaration().Name)
		}
		return out
	}

	t.Run("the named order leads and the rest follows in name order", func(t *testing.T) {
		sorted := SortedToolsWithOrder(tools, []string{"gamma", "alpha"})
		require.Equal(t, []string{"gamma", "alpha", "beta", "delta"}, names(sorted))
	})

	t.Run("an empty order is the plain name order", func(t *testing.T) {
		require.Equal(t, names(SortedTools(tools)), names(SortedToolsWithOrder(tools, nil)))
	})

	t.Run("an unknown or repeated name is skipped", func(t *testing.T) {
		sorted := SortedToolsWithOrder(tools, []string{"missing", "beta", "beta", "skip"})
		require.Equal(t, []string{"beta", "alpha", "delta", "gamma"}, names(sorted))
	})
}

type testOrderTool struct {
	name string
}

func (t testOrderTool) Declaration() *tool.Declaration {
	return &tool.Declaration{Name: t.name}
}

type nilDeclarationTool struct{}

func (nilDeclarationTool) Declaration() *tool.Declaration { return nil }
