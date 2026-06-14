package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/tamnd/ant/ant"
	"github.com/tamnd/any-cli/kit"
)

// newMCPCmd serves the whole URI namespace to an agent as a tiny, uniform tool
// set (get/ls/links/resolve/url/domains) over stdio JSON-RPC, instead of the
// rich per-op surface a single-site `mcp` exposes (8000_uri §8).
func newMCPCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "mcp",
		Short: "MCP surface: get/ls/resolve tools over the whole namespace",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			e, err := engineFrom()
			if err != nil {
				return err
			}
			return serveMCP(c.Context(), e, os.Stdin, os.Stdout)
		},
	}
}

type mcpRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type mcpResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *mcpError       `json:"error,omitempty"`
}

type mcpError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func serveMCP(ctx context.Context, e *ant.Engine, in io.Reader, out io.Writer) error {
	dec := json.NewDecoder(bufio.NewReader(in))
	enc := json.NewEncoder(out)
	write := func(resp mcpResponse) {
		resp.JSONRPC = "2.0"
		_ = enc.Encode(resp)
	}

	for {
		var req mcpRequest
		if err := dec.Decode(&req); err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
		switch req.Method {
		case "initialize":
			write(mcpResponse{ID: req.ID, Result: mcpInit()})
		case "tools/list":
			write(mcpResponse{ID: req.ID, Result: map[string]any{"tools": mcpTools()}})
		case "tools/call":
			write(mcpCall(ctx, e, req))
		case "ping":
			write(mcpResponse{ID: req.ID, Result: map[string]any{}})
		case "notifications/initialized":
			// no response
		default:
			if len(req.ID) > 0 {
				write(mcpResponse{ID: req.ID, Error: &mcpError{Code: -32601, Message: "method not found: " + req.Method}})
			}
		}
	}
}

func mcpInit() map[string]any {
	return map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]any{"tools": map[string]any{}},
		"serverInfo":      map[string]any{"name": "ant", "version": Version},
		"instructions":    "Dereference resource URIs across every registered site. Resolve a messy URL or id to a URI, then get/ls/links it.",
	}
}

func uriArgSchema(desc string) map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"uri": map[string]any{"type": "string", "description": desc},
		},
		"required": []string{"uri"},
	}
}

func mcpTools() []map[string]any {
	return []map[string]any{
		{"name": "get", "description": "Dereference a URI to its record", "inputSchema": uriArgSchema("a resource URI, e.g. goodreads://book/2767052")},
		{"name": "ls", "description": "List the members of a collection URI", "inputSchema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"uri": map[string]any{"type": "string", "description": "a collection URI"},
				"n":   map[string]any{"type": "integer", "description": "max members (0 = default)"},
			},
			"required": []string{"uri"},
		}},
		{"name": "links", "description": "List a record's outbound link URIs", "inputSchema": uriArgSchema("a resource URI")},
		{"name": "url", "description": "The live https URL for a URI", "inputSchema": uriArgSchema("a resource URI")},
		{"name": "resolve", "description": "Normalize an id/URL/URI to a canonical URI", "inputSchema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"input": map[string]any{"type": "string", "description": "any id, URL, or URI"},
				"on":    map[string]any{"type": "string", "description": "domain scheme for a bare id"},
			},
			"required": []string{"input"},
		}},
		{"name": "domains", "description": "List the registered domains", "inputSchema": map[string]any{"type": "object", "properties": map[string]any{}}},
	}
}

func mcpCall(ctx context.Context, e *ant.Engine, req mcpRequest) mcpResponse {
	var params struct {
		Name      string         `json:"name"`
		Arguments map[string]any `json:"arguments"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return mcpResponse{ID: req.ID, Error: &mcpError{Code: -32602, Message: "invalid params: " + err.Error()}}
	}

	result, err := dispatchTool(ctx, e, params.Name, params.Arguments)
	if err != nil {
		return mcpResponse{ID: req.ID, Result: mcpToolError(err)}
	}
	return mcpResponse{ID: req.ID, Result: mcpToolResult(result)}
}

func dispatchTool(ctx context.Context, e *ant.Engine, name string, args map[string]any) (any, error) {
	str := func(k string) string {
		if v, ok := args[k].(string); ok {
			return v
		}
		return ""
	}
	switch name {
	case "get":
		u, err := kit.ParseURI(str("uri"))
		if err != nil {
			return nil, err
		}
		return e.Get(ctx, u)
	case "ls":
		u, err := kit.ParseURI(str("uri"))
		if err != nil {
			return nil, err
		}
		n := 0
		if f, ok := args["n"].(float64); ok {
			n = int(f)
		}
		return e.List(ctx, u, n)
	case "links":
		u, err := kit.ParseURI(str("uri"))
		if err != nil {
			return nil, err
		}
		links, err := e.Links(ctx, u)
		if err != nil {
			return nil, err
		}
		out := make([]string, 0, len(links))
		for _, lu := range links {
			out = append(out, lu.String())
		}
		return out, nil
	case "url":
		u, err := kit.ParseURI(str("uri"))
		if err != nil {
			return nil, err
		}
		loc, err := e.URL(u)
		if err != nil {
			return nil, err
		}
		return map[string]string{"url": loc}, nil
	case "resolve":
		u, err := e.Resolve(str("input"), str("on"))
		if err != nil {
			return nil, err
		}
		return map[string]string{"uri": u.String()}, nil
	case "domains":
		return e.Domains(), nil
	default:
		return nil, fmt.Errorf("unknown tool: %s", name)
	}
}

func mcpToolResult(v any) map[string]any {
	text, _ := json.MarshalIndent(v, "", "  ")
	return map[string]any{
		"content":           []map[string]any{{"type": "text", "text": string(text)}},
		"structuredContent": map[string]any{"result": v},
	}
}

func mcpToolError(err error) map[string]any {
	return map[string]any{
		"isError": true,
		"content": []map[string]any{{"type": "text", "text": err.Error()}},
	}
}
