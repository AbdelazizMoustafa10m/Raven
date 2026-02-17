//go:build tools

// Package tools declares tool dependencies to keep them in go.mod.
// These dependencies are used by internal packages that are not yet implemented.
package tools

import (
	_ "github.com/BurntSushi/toml"
	_ "github.com/bmatcuk/doublestar/v4"
	_ "github.com/cespare/xxhash/v2"
	_ "github.com/charmbracelet/bubbles/viewport"
	_ "github.com/charmbracelet/bubbletea"
	_ "github.com/charmbracelet/huh"
	_ "github.com/charmbracelet/lipgloss"
	_ "github.com/charmbracelet/log"
	_ "github.com/spf13/cobra"
	_ "github.com/stretchr/testify/assert"
	_ "golang.org/x/sync/errgroup"
)
