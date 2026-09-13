package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOperatingBriefProjectionRequiresExactAnchorQuestionSet(t *testing.T) {
	key := `["kpi","net_sales_amount","net_sales_amount","current",null]`
	data := map[string]any{
		"kpis": []any{map[string]any{"observationAnchorRef": key}},
		"salesTrend": []any{map[string]any{"observationAnchorRef": nil}},
	}
	rewritten, questions, err := rewriteBriefProjection(data, map[string]any{key: "分析销售变化"})
	require.NoError(t, err)
	anchor := rewritten["kpis"].([]any)[0].(map[string]any)["observationAnchorRef"].(string)
	require.NotEmpty(t, anchor)
	require.NotEqual(t, key, anchor)
	require.Equal(t, map[string]string{anchor: "分析销售变化"}, questions)

	_, _, err = rewriteBriefProjection(data, map[string]any{})
	require.Error(t, err)
	_, _, err = rewriteBriefProjection(data, map[string]any{key: "分析销售变化", "extra": "额外问题"})
	require.Error(t, err)
}
