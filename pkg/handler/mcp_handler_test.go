package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/agent-sandbox/agent-sandbox/pkg/config"
	"github.com/agent-sandbox/agent-sandbox/pkg/sandbox"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	fakersinformer "knative.dev/pkg/client/injection/kube/informers/apps/v1/replicaset/fake"
	reconcilertesting "knative.dev/pkg/reconciler/testing"

	_ "knative.dev/pkg/client/injection/kube/client/fake"
)

// apiKeyTransport attaches X-Api-Key to every MCP request, like a real MCP client config would.
type apiKeyTransport struct{ key string }

func (t apiKeyTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	if t.key != "" {
		r.Header.Set("X-Api-Key", t.key)
	}
	return http.DefaultTransport.RoundTrip(r)
}

func newSandboxRS(name, user string) *appsv1.ReplicaSet {
	data, _ := json.Marshal(&sandbox.Sandbox{SandboxBase: sandbox.SandboxBase{Name: name}, User: user})
	return &appsv1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   "default",
			Labels:      map[string]string{sandbox.UserLabel: user, "owner": "agent-sandbox"},
			Annotations: map[string]string{sandbox.AnnotationSandboxData: string(data)},
		},
	}
}

// setupMCPServer serves /mcp behind the real auth middleware, backed by a fake ReplicaSet informer.
func setupMCPServer(t *testing.T, rss ...*appsv1.ReplicaSet) *httptest.Server {
	t.Helper()
	prev := config.Cfg
	config.Cfg = &config.Config{APIBaseURL: "/api/v1", SandboxNamespace: "default", APITokens: []string{"user-a", "user-b"}}
	t.Cleanup(func() { config.Cfg = prev })

	ctx, _ := reconcilertesting.SetupFakeContext(t)
	indexer := fakersinformer.Get(ctx).Informer().GetIndexer()
	for _, rs := range rss {
		if err := indexer.Add(rs); err != nil {
			t.Fatal(err)
		}
	}

	h := NewHandler(ctx, sandbox.NewController(ctx, nil, nil, nil), nil)
	mux := http.NewServeMux()
	mux.Handle("/mcp", h.McpSseHandler())
	srv := httptest.NewServer(ApiKeyAuthMiddleware(mux))
	t.Cleanup(srv.Close)
	return srv
}

func connectMCP(t *testing.T, srv *httptest.Server, key string) (*mcp.ClientSession, error) {
	t.Helper()
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "1.0.0"}, nil)
	return client.Connect(context.Background(), &mcp.StreamableClientTransport{
		Endpoint:   srv.URL + "/mcp",
		HTTPClient: &http.Client{Transport: apiKeyTransport{key: key}},
	}, nil)
}

func callListSandbox(t *testing.T, cs *mcp.ClientSession) (string, bool) {
	t.Helper()
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "listSandbox", Arguments: map[string]any{}})
	if err != nil {
		t.Fatalf("CallTool transport error: %v", err)
	}
	var sb strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			sb.WriteString(tc.Text)
		}
	}
	return sb.String(), res.IsError
}

func TestListSandboxTool_RequiresAPIKey(t *testing.T) {
	srv := setupMCPServer(t)
	if cs, err := connectMCP(t, srv, ""); err == nil {
		cs.Close()
		t.Fatal("expected connect without X-Api-Key to fail with 401")
	}
}

func TestListSandboxTool_EmptyList(t *testing.T) {
	srv := setupMCPServer(t)
	cs, err := connectMCP(t, srv, "user-a")
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer cs.Close()

	text, isErr := callListSandbox(t, cs)
	if isErr {
		t.Fatalf("listSandbox returned tool error for an empty list: %s", text)
	}
	if !strings.Contains(text, "Existing Sandboxes") {
		t.Fatalf("unexpected result: %s", text)
	}
}

func TestListSandboxTool_OnlyCallerSandboxes(t *testing.T) {
	srv := setupMCPServer(t, newSandboxRS("sb-a", "user-a"), newSandboxRS("sb-b", "user-b"))
	cs, err := connectMCP(t, srv, "user-a")
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer cs.Close()

	text, isErr := callListSandbox(t, cs)
	if isErr {
		t.Fatalf("listSandbox returned tool error: %s", text)
	}
	if !strings.Contains(text, "sb-a") {
		t.Errorf("expected caller's sandbox sb-a in result, got: %s", text)
	}
	if strings.Contains(text, "sb-b") {
		t.Errorf("result leaks another user's sandbox sb-b: %s", text)
	}
}
