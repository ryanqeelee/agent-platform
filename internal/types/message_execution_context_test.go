package types

import "testing"

func TestMessageExecutionContextGovernedAnalysisRunValueScanRoundTrip(t *testing.T) {
	original := MessageExecutionContext{
		AgentConfigHash: "config-hash",
		GovernedAnalysisRun: &GovernedAnalysisRunReference{
			ContractVersion: "governed-analysis-result/1",
			RunID:           "run-accepted",
		},
	}
	value, err := original.Value()
	if err != nil {
		t.Fatal(err)
	}
	var decoded MessageExecutionContext
	if err := decoded.Scan(value); err != nil {
		t.Fatal(err)
	}
	if decoded.GovernedAnalysisRun == nil {
		t.Fatal("accepted governed run provenance was dropped")
	}
	if got := decoded.GovernedAnalysisRun.ContractVersion; got != "governed-analysis-result/1" {
		t.Fatalf("contract version = %q", got)
	}
	if got := decoded.GovernedAnalysisRun.RunID; got != "run-accepted" {
		t.Fatalf("run ID = %q", got)
	}
}
