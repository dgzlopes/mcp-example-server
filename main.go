package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var httpAddr = flag.String("http", "localhost:3001", "if set, use streamable HTTP at this address, instead of stdin/stdout")
var sseAddr = flag.String("sse", "localhost:3002", "if set, use SSE at this address, instead of stdin/stdout")
var stdioFlag = flag.Bool("stdio", false, "if set, use stdin/stdout transport instead of HTTP/SSE")

type HiArgs struct {
	Name string `json:"name" jsonschema:"the name to say hi to"`
}

func SayHi(ctx context.Context, ss *mcp.ServerSession, params *mcp.CallToolParamsFor[HiArgs]) (*mcp.CallToolResultFor[struct{}], error) {
	return &mcp.CallToolResultFor[struct{}]{
		Content: []mcp.Content{
			&mcp.TextContent{Text: "Hi " + params.Arguments.Name},
		},
	}, nil
}

func PromptHi(ctx context.Context, ss *mcp.ServerSession, params *mcp.GetPromptParams) (*mcp.GetPromptResult, error) {
	return &mcp.GetPromptResult{
		Description: "Code review prompt",
		Messages: []*mcp.PromptMessage{
			{Role: "user", Content: &mcp.TextContent{Text: "Say hi to " + params.Arguments["name"]}},
		},
	}, nil
}

func main() {
	flag.Parse()

	server := mcp.NewServer(&mcp.Implementation{Name: "greeter"}, nil)
	mcp.AddTool(server, &mcp.Tool{Name: "greet", Description: "say hi"}, SayHi)
	server.AddPrompt(&mcp.Prompt{Name: "greet"}, PromptHi)
	server.AddResource(&mcp.Resource{
		Name:     "info",
		MIMEType: "text/plain",
		URI:      "embedded:info",
	}, handleEmbeddedResource)

	errs := make(chan error, 2)

	if *stdioFlag {
		t := mcp.NewLoggingTransport(mcp.NewStdioTransport(), os.Stderr)
		if err := server.Run(context.Background(), t); err != nil {
			log.Printf("Server failed: %v", err)
		}
		return
	}

	if *httpAddr != "" {
		go func() {
			handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
				return server
			}, nil)
			log.Printf("MCP handler listening at %s", *httpAddr)
			errs <- http.ListenAndServe(*httpAddr, handler)
		}()
	}

	if *sseAddr != "" {
		go func() {
			handler := mcp.NewSSEHandler(func(*http.Request) *mcp.Server {
				return server
			})
			log.Printf("MCP SSE handler listening at %s", *sseAddr)
			errs <- http.ListenAndServe(*sseAddr, handler)
		}()
	}

	log.Printf("If you want to use stdin/stdout, pass the --stdio flag")
	log.Fatalf("Server exited: %v", <-errs)
}

var embeddedResources = map[string]string{
	"info": "This is the hello example server.",
}

func handleEmbeddedResource(_ context.Context, _ *mcp.ServerSession, params *mcp.ReadResourceParams) (*mcp.ReadResourceResult, error) {
	u, err := url.Parse(params.URI)
	if err != nil {
		return nil, err
	}
	if u.Scheme != "embedded" {
		return nil, fmt.Errorf("wrong scheme: %q", u.Scheme)
	}
	key := u.Opaque
	text, ok := embeddedResources[key]
	if !ok {
		return nil, fmt.Errorf("no embedded resource named %q", key)
	}
	return &mcp.ReadResourceResult{
		Contents: []*mcp.ResourceContents{
			{URI: params.URI, MIMEType: "text/plain", Text: text},
		},
	}, nil
}
