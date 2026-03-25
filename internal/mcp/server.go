// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package mcp

import (
	"context"
	"fmt"
	"io"

	mcpserver "github.com/mark3labs/mcp-go/server"

	"github.com/micasa-dev/micasa/internal/data"
)

// Server wraps an MCP protocol server with access to the micasa data store.
type Server struct {
	store  *data.Store
	mcpSrv *mcpserver.MCPServer
}

// NewServer creates a new MCP server backed by the given data store.
func NewServer(store *data.Store) *Server {
	s := &Server{store: store}
	s.mcpSrv = mcpserver.NewMCPServer(
		"micasa",
		"1.0.0",
	)
	s.registerTools()
	return s
}

// MCPServer returns the underlying mcp-go server for direct access.
func (s *Server) MCPServer() *mcpserver.MCPServer {
	return s.mcpSrv
}

// Tools returns all registered server tools for use with mcptest.
func (s *Server) Tools() []mcpserver.ServerTool {
	listed := s.mcpSrv.ListTools()
	tools := make([]mcpserver.ServerTool, 0, len(listed))
	for _, t := range listed {
		tools = append(tools, *t)
	}
	return tools
}

// Serve runs the MCP server over the given reader/writer pair.
func (s *Server) Serve(ctx context.Context, stdin io.Reader, stdout io.Writer) error {
	stdio := mcpserver.NewStdioServer(s.mcpSrv)
	if err := stdio.Listen(ctx, stdin, stdout); err != nil {
		return fmt.Errorf("serve mcp: %w", err)
	}
	return nil
}

// ServeStdio runs the MCP server over os.Stdin/os.Stdout.
func (s *Server) ServeStdio() error {
	if err := mcpserver.ServeStdio(s.mcpSrv); err != nil {
		return fmt.Errorf("serve stdio: %w", err)
	}
	return nil
}
