package github

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/github/github-mcp-server/pkg/toolsets"
	"github.com/github/github-mcp-server/pkg/translations"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)


func Test_uploadToolsToProxy(t *testing.T) {
	// Create a read-only tool by setting ReadOnlyHint = true
	readOnly := true
	tool := mcp.NewTool("example",
		mcp.WithDescription("example"),
		mcp.WithString("x", mcp.Description("X")),
		mcp.WithToolAnnotation(mcp.ToolAnnotation{
			ReadOnlyHint: &readOnly,
		}),
	)

	// Wrap it in server.ServerTool
	serverTool := server.ServerTool{Tool: tool}

	// Create a Toolset and add the tool properly via AddReadTools
	toolset := &toolsets.Toolset{
		Name:    "test_toolset",
		Enabled: true,
	}
	toolset.AddReadTools(serverTool)

	// Create a ToolsetGroup and assign the toolset
	toolsetGroup := &toolsets.ToolsetGroup{
		Toolsets: map[string]*toolsets.Toolset{
			"test_toolset": toolset,
		},
	}

	t.Run("missing API key", func(t *testing.T) {
        err := uploadToolsToProxy(toolsetGroup, "")
        require.Error(t, err)
        assert.Contains(t, err.Error(), "missing Proxy API key")
    })

    t.Run("successful upload", func(t *testing.T) {
        mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            assert.Equal(t, "POST", r.Method)
            assert.Equal(t, "/functions/v1/upload", r.URL.Path)
            w.WriteHeader(http.StatusOK)
        }))
        defer mockServer.Close()

        err := uploadToolsToProxy(toolsetGroup, "test-api-key")
        require.NoError(t, err)
    })
}

func Test_ProxyToolSuggestion(t *testing.T) {
	// Create a read-only tool by setting ReadOnlyHint = true
	readOnly := true
	tool := mcp.NewTool("example",
		mcp.WithDescription("example"),
		mcp.WithString("x", mcp.Description("X")),
		mcp.WithToolAnnotation(mcp.ToolAnnotation{
			ReadOnlyHint: &readOnly,
		}),
	)

	// Wrap it in server.ServerTool
	serverTool := server.ServerTool{Tool: tool}

	// Create a Toolset and add the tool properly via AddReadTools
	toolset := &toolsets.Toolset{
		Name:    "test_toolset",
		Enabled: true,
	}
	toolset.AddReadTools(serverTool)

	// Create a ToolsetGroup and assign the toolset
	toolsetGroup := &toolsets.ToolsetGroup{
		Toolsets: map[string]*toolsets.Toolset{
			"test_toolset": toolset,
		},
	}

	t.Run("suggest tools via Proxy", func(t *testing.T) {
		callCounter := 0
		mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			callCounter++
			switch callCounter {
			case 1:
				assert.Equal(t, "/functions/v1/upload", r.URL.Path)
				w.WriteHeader(http.StatusOK)
			case 2:
				assert.Equal(t, "/functions/v1/search", r.URL.Path)
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"results": [{"name": "example"}]}`))
			default:
				t.Fatalf("unexpected request #%d", callCounter)
			}
		}))
		defer mockServer.Close()

		// Override URLs to mock server
		origUploadURL := proxyUploadURL
		origSearchURL := proxySearchURL
		proxyUploadURL = mockServer.URL + "/functions/v1/upload"
		proxySearchURL = mockServer.URL + "/functions/v1/search"
		defer func() {
			proxyUploadURL = origUploadURL
			proxySearchURL = origSearchURL
		}()

		_, handler := ProxyToolSuggestion(toolsetGroup, translations.NullTranslationHelper)
		req := createMCPRequest(map[string]interface{}{
			"proxy_api_key": "fake-key",
			"prompt":        "list commits",
		})

		result, err := handler(context.Background(), req)
		require.NoError(t, err)
		require.False(t, result.IsError)

		output := getTextResult(t, result)
		assert.Contains(t, output.Text, "example")
	})

	t.Run("missing prompt", func(t *testing.T) {
		_, handler := ProxyToolSuggestion(toolsetGroup, translations.NullTranslationHelper)
		req := createMCPRequest(map[string]interface{}{
			"proxy_api_key": "key",
		})
		result, err := handler(context.Background(), req)
		require.NoError(t, err)
		require.True(t, result.IsError)
		assert.Contains(t, getErrorResult(t, result).Text, "Missing 'prompt'")
	})
}
