package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"AICodeAgent/internal/mcp"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	cfgs := map[string]mcp.MCPConfigAdapter{
		"context7": {
			Type:    "stdio",
			Command: "npx",
			Args:    []string{"-y", "@upstash/context7-mcp@latest"},
			Timeout: 30,
		},
	}

	log.Println("Initializing Context7 MCP Server...")
	mcp.Initialize(ctx, cfgs)

	state, ok := mcp.GetState("context7")
	if !ok {
		log.Fatal("no state found for context7")
	}
	if state.State != mcp.StateConnected {
		log.Fatalf("state: %s, error: %v", state.State, state.Error)
	}
	log.Printf("State: %s, Tools: %d", state.State, state.Counts.Tools)

	session, ok := mcp.GetSession("context7")
	if !ok {
		log.Fatal("no session found")
	}

	tools, err := session.ListTools(ctx)
	if err != nil {
		log.Fatalf("list tools: %v", err)
	}

	fmt.Println("\n=== Available Context7 Tools ===")
	for i, tool := range tools {
		fmt.Printf("%d. %s - %s\n", i+1, tool.Name, tool.Description)
	}

	mcp.Close()
	log.Println("Done")
}
