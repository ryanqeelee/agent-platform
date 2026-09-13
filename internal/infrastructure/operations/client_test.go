package operations

import (
	"context"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestClientUsesTypedCenterEnterpriseEdgeContracts(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		require.Equal(t, "Bearer service-token", request.Header.Get("Authorization"))
		require.Equal(t, "system-admin-1", request.Header.Get("X-Product-Base-Actor-User-Id"))
		switch request.URL.Path {
		case "/api/internal/product-base/operations/enterprises/enterprise-1/edge":
			require.Equal(t, http.MethodGet, request.Method)
			_, _ = writer.Write([]byte(`{"schema":"CenterEnterpriseEdgeV1","enterpriseId":"enterprise-1","summary":{"connectionStatus":"online","nodeCount":1,"onlineNodeCount":1,"lastSeenAt":null},"nodes":[{"edgeNodeId":"edge-1","status":"online","controlRevision":7}]}`))
		case "/api/internal/product-base/operations/enterprises/enterprise-1/edge-enrollment-token/rotate":
			require.Equal(t, http.MethodPost, request.Method)
			_, _ = writer.Write([]byte(`{"schema":"CenterEnterpriseEdgeEnrollmentV1","enterpriseId":"enterprise-1","enrollmentToken":"one-time","rotatedAt":null}`))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	client := &Client{baseURL: server.URL, token: "service-token", http: &http.Client{Timeout: time.Second}}

	edge, err := client.GetEnterpriseEdge(context.Background(), "enterprise-1", "system-admin-1")
	require.NoError(t, err)
	require.Len(t, edge.Nodes, 1)
	require.Equal(t, int64(7), edge.Nodes[0].ControlRevision)

	enrollment, err := client.RotateEnterpriseEnrollmentToken(context.Background(), "enterprise-1", "system-admin-1")
	require.NoError(t, err)
	require.Equal(t, "one-time", enrollment.EnrollmentToken)
}

func TestDisableNodePreservesPendingRevocationAndChecksIdentity(t *testing.T) {
	for _, tc := range []struct {
		name    string
		status  int
		body    string
		pending bool
		valid   bool
	}{
		{"disabled", 200, `{"schema":"CenterEdgeNodeDisableV1","enterpriseId":"enterprise-1","edgeNodeId":"edge-1","status":"disabled","controlRevision":7}`, false, true},
		{"pending", 503, `{"detail":{"code":"edge_node_disabled_projection_pending","node_status":"disabled","control_revision":7}}`, true, false},
		{"foreign", 200, `{"schema":"CenterEdgeNodeDisableV1","enterpriseId":"enterprise-2","edgeNodeId":"edge-1","status":"disabled","controlRevision":7}`, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, http.MethodPost, r.Method)
				require.Equal(t, "/api/internal/product-base/operations/enterprises/enterprise-1/edge/nodes/edge-1/disable", r.URL.Path)
				require.Equal(t, "Bearer service-token", r.Header.Get("Authorization"))
				require.Equal(t, "admin", r.Header.Get("X-Product-Base-Actor-User-Id"))
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()
			client := &Client{baseURL: server.URL, token: "service-token", http: server.Client()}
			value, err := client.DisableEnterpriseEdgeNode(context.Background(), "enterprise-1", "edge-1", "admin")
			if tc.valid {
				require.NoError(t, err)
				require.Equal(t, int64(7), value.ControlRevision)
				return
			}
			require.Error(t, err)
			if tc.pending {
				var upstream *interfaces.PlatformOperationsUpstreamError
				require.ErrorAs(t, err, &upstream)
				require.Equal(t, "edge_node_disabled_projection_pending", upstream.Detail)
			}
		})
	}
}
