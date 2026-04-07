// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package mcp

import mcpserver "github.com/mark3labs/mcp-go/server"

// Tools returns every tool registered on the server. It exists so the
// external mcp_test package can drive the registered tools through an
// in-process mcptest.UnstartedServer without spinning up the real
// stdio transport. The production binary registers tools through
// registerTools and then exposes them only via the MCP protocol, so
// this enumeration is never needed at runtime.
func Tools(s *Server) []mcpserver.ServerTool {
	listed := s.mcpSrv.ListTools()
	tools := make([]mcpserver.ServerTool, 0, len(listed))
	for _, t := range listed {
		tools = append(tools, *t)
	}
	return tools
}
