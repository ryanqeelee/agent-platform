package types

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestGovernedDataCredentialBoundToCallerAndWorkspace(t *testing.T) {
	base := context.WithValue(context.Background(), UserIDContextKey, "user-a")
	base = context.WithValue(base, TenantIDContextKey, uint64(10001))
	bound := WithGovernedDataUserCredential(base, "private-jwt")
	if _, _, ok := GovernedDataUserCredential(CopyGovernedDataTurnCredential(base, bound)); ok {
		t.Fatal("unmarked destination copied governed credential")
	}
	markedBase := WithGovernedDataObservability(base)
	copied := CopyGovernedDataTurnCredential(markedBase, bound)
	if token, tenant, ok := GovernedDataUserCredential(copied); !ok || token != "private-jwt" || tenant != 10001 {
		t.Fatal("marked governed turn did not copy its credential")
	}
	for _, tc := range []struct {
		name    string
		ctx     context.Context
		allowed bool
	}{
		{"same user", bound, true},
		{"native background turn", context.WithoutCancel(bound), true},
		{"no credential", base, false},
		{"changed user", context.WithValue(bound, UserIDContextKey, "user-b"), false},
		{"shared agent tenant", context.WithValue(bound, TenantIDContextKey, uint64(20002)), false},
		{"api key", WithTenantAPIKeyScope(bound, TenantAPIKeyScope{}), false},
		{"rebuilt worker", CopyPrivateAuthorizationContext(base, bound), false},
		{"empty token", WithGovernedDataUserCredential(base, ""), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			token, tenant, ok := GovernedDataUserCredential(tc.ctx)
			if ok != tc.allowed {
				t.Fatalf("allowed=%v want %v", ok, tc.allowed)
			}
			if ok && (token != "private-jwt" || tenant != 10001) {
				t.Fatal("lost caller credential")
			}
			if !ok && (token != "" || tenant != 0) {
				t.Fatal("rejected credential escaped")
			}
		})
	}
	credential := bound.Value(governedDataCredentialKey{})
	data, err := json.Marshal(credential)
	if err != nil {
		t.Fatal(err)
	}
	for _, printed := range []string{string(data), fmt.Sprintf("%v", bound), fmt.Sprintf("%+v", credential), fmt.Sprintf("%#v", credential)} {
		if strings.Contains(printed, "private-jwt") {
			t.Fatal("credential leaked in formatting")
		}
	}
}

func TestGovernedDataObservabilityIsMonotonicAndPayloadFree(t *testing.T) {
	base := context.Background()
	marked := WithGovernedDataObservability(base)
	if !GovernedDataObservability(marked) {
		t.Fatal("governed observability marker was not set")
	}
	if got := WithGovernedDataObservability(marked); got != marked {
		t.Fatal("setting an existing observability restriction should be idempotent")
	}
	if value := marked.Value(GovernedDataObservabilityContextKey); value != true {
		t.Fatalf("marker value = %#v, want true", value)
	}
}
