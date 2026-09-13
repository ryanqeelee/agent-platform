package handler

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCanonicalEnterpriseActivationV2MatchesCenterIdentityBytes(t *testing.T) {
	request := enterpriseActivationRequest{
		Name: "Acme", Description: "a\n\"b", SeatsTotal: 2, StorageQuota: 0,
		InitialAdministratorUserID: "owner-1",
	}
	require.Equal(t,
		`{"aiCapabilityPlanVersionId":"plan-1","description":"a\n\"b","initialAdministratorUserId":"owner-1","name":"Acme","schema":"EnterpriseActivationV2","seatsTotal":2,"storageQuota":0}`,
		string(canonicalEnterpriseActivationV2(request, "plan-1")),
	)
}
