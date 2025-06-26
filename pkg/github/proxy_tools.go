package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/github/github-mcp-server/pkg/toolsets"
	"github.com/github/github-mcp-server/pkg/translations"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

var proxyUploadURL = "https://api.tryproxy.ai/functions/v1/upload"
var proxySearchURL = "https://api.tryproxy.ai/functions/v1/search"

// Uploads all tools from all toolsets to Proxy. Proxy will dynamically discover relevant tools based on the prompt, regardless of the toolset each tool is in
func uploadToolsToProxy(toolsetGroup *toolsets.ToolsetGroup, proxyAPIKey string) error {
	if proxyAPIKey == "" {
		return fmt.Errorf("missing Proxy API key")
	}

	for _, ts := range toolsetGroup.Toolsets {
		for _, st := range ts.GetAvailableTools() {
			tool := st.Tool

			payload := map[string]interface{}{
				"tool": map[string]interface{}{
					"name":        tool.Name,
					"description": tool.Description,
					"inputSchema": tool.InputSchema,
				},
			}

			body, err := json.Marshal(payload)
			if err != nil {
				return fmt.Errorf("marshal error: %w", err)
			}

			req, err := http.NewRequest("POST", proxyUploadURL, bytes.NewBuffer(body))
			if err != nil {
				return fmt.Errorf("request creation error: %w", err)
			}

			req.Header.Set("Authorization", proxyAPIKey)
			req.Header.Set("Content-Type", "application/json")

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				return fmt.Errorf("upload failed for %s: %w", tool.Name, err)
			}
			defer func() {
				if err := resp.Body.Close(); err != nil {
					fmt.Printf("failed to close response body: %v", err)
				}
			}()

			if resp.StatusCode != http.StatusOK {
				var errResp map[string]interface{}
				if decodeErr := json.NewDecoder(resp.Body).Decode(&errResp); decodeErr != nil {
					fmt.Printf("failed to decode error response: %v", decodeErr)
				}
				return fmt.Errorf("proxy upload error for %s: %v", tool.Name, errResp)
			}
		}
	}

	fmt.Println("✅ Proxy: All tools uploaded successfully")
	return nil
}

// ProxyToolSuggestion searches tools via Proxy based on the user's prompt.
func ProxyToolSuggestion(toolsetGroup *toolsets.ToolsetGroup, t translations.TranslationHelperFunc) (mcp.Tool, server.ToolHandlerFunc) {
	return mcp.NewTool("proxy_tool_suggestion",
			mcp.WithDescription(t("TOOL_PROXY_DESCRIPTION", "Suggest GitHub tools via Proxy based on a prompt")),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        t("TOOL_PROXY_TITLE", "Suggest a GitHub tool"),
				ReadOnlyHint: ToBoolPtr(true),
			}),
			mcp.WithString("proxy_api_key",
				mcp.Required(),
				mcp.Description("Your Proxy API key"),
			),
			mcp.WithString("prompt",
				mcp.Required(),
				mcp.Description("Describe what you want to do"),
			),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			// Get the API key
			apiKey, err := RequiredParam[string](request, "proxy_api_key")
			if err != nil {
				return mcp.NewToolResultError("Missing proxy API key"), nil
			}

			// Step 1: Upload all tools to Proxy
			err = uploadToolsToProxy(toolsetGroup, apiKey)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Proxy upload failed: %v", err)), nil
			}

			// Step 2: Get user prompt
			prompt, err := RequiredParam[string](request, "prompt")
			if err != nil {
				return mcp.NewToolResultError("Missing 'prompt'"), nil
			}

			// Step 3: Call Proxy search API
			query := map[string]interface{}{
				"query": prompt,
				"limit": 5, // can change the limit here to however many tools you want returned from Proxy
			}

			body, err := json.Marshal(query)
			if err != nil {
				return mcp.NewToolResultError("Failed to marshal query"), nil
			}

			req, err := http.NewRequest("POST", proxySearchURL, bytes.NewBuffer(body))
			if err != nil {
				return mcp.NewToolResultError("Failed to create request"), nil
			}

			req.Header.Set("Authorization", apiKey)
			req.Header.Set("Content-Type", "application/json")

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				return mcp.NewToolResultError("Failed to call Proxy API"), nil
			}
			defer func() {
				if err := resp.Body.Close(); err != nil {
					fmt.Printf("failed to close response body: %v", err)
				}
			}()

			if resp.StatusCode != http.StatusOK {
				var errResp map[string]interface{}
				if decodeErr := json.NewDecoder(resp.Body).Decode(&errResp); decodeErr != nil {
					fmt.Printf("failed to decode error response: %v", decodeErr)
				}
				return mcp.NewToolResultError(fmt.Sprintf("Proxy API error: %v", errResp)), nil
			}

			var result map[string]interface{}
			if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
				return mcp.NewToolResultError("Failed to decode response"), nil
			}

			pretty, err := json.MarshalIndent(result, "", "  ")
			if err != nil {
				return mcp.NewToolResultError("Failed to format output"), nil
			}

			return mcp.NewToolResultText(string(pretty)), nil
		}
}
