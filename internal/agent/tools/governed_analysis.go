package tools

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Tencent/WeKnora/internal/types"
)

var (
	hexDigestPattern      = regexp.MustCompile(`^[a-f0-9]{64}$`)
	prefixedDigestPattern = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)
)

const GovernedAnalysisDraftInstruction = `For this governed operating-analysis turn, your final response MUST be one AnalysisDraftV1 JSON object and nothing else. Use this shape (camelCase keys): {"contractVersion":"analysis-draft/1","summary":{"text":"qualitative summary without copied business quantities","evidence":[{"queryExecutionId":"current-query-id"}]},"claims":[{"claimId":"unique-claim-id","kind":"observation","essential":true,"segments":[{"type":"text","text":"description and unit: "},{"type":"evidenceValue","reference":{"queryExecutionId":"current-query-id","row":0,"column":"measure-column"}}],"counterEvidence":[]}],"observations":[],"explanations":[],"actions":[],"limitations":[]}.
Use claims for observations, interpretations and hypotheses (kind is observation, interpretation or hypothesis); each claimId must be unique. Claim segments may be text, evidenceValue, or calculationValue. Never copy business quantities or calculate a value into text: reference persisted measure cells with evidenceValue and let Center bind their exact values. An observation needs at least one evidenceValue or calculationValue segment. Keep interpretations and hypotheses distinct from observations and include counterEvidence when available. Keep legacy observations/explanations empty to avoid duplicate claims. Actions may use the existing {"text":"...","evidence":[{"queryExecutionId":"..."}],"verification":"..."} shape without inventing quantities.
For an eligible period comparison only, a segment may be {"type":"calculationValue","calculatorId":"difference","calculatorVersion":"1","inputs":{"current":{"queryExecutionId":"current-query-id","row":0,"column":"measure-column"},"baseline":{"queryExecutionId":"baseline-query-id","row":0,"column":"measure-column"}}}. Only difference, relative_change and percentage_point_change are registered. Center requires the same resolved metric key/version/unit/grain and a proven comparable population and period scope; missing context is rejected, and a zero baseline is invalid for relative_change. Do not invent metric context or compute locally; obtain supporting governed queries or state the limitation. relative_change is returned as a ratio, so label it as a ratio and do not append a percent sign. percentage_point_change requires both governed input units to be ratio and returns percentage points; label the result as percentage points, not relative percent change. Center validates and computes it. Share calculators are not available.
Read the Center-issued analysis_scope and its checks entry (kind=analysis_scope) in each query response before drafting comparisons. Only Center/query execution can resolve scope. A resolved scope names table, timeColumn, the half-open period [start,endExclusive), authorityScopeDigest, populationScopeDigest, and normalized populationFilters; unresolved carries limitationCodes, and not_applicable supplies no range. authorityScopeDigest binds the authorized run scope; populationScopeDigest identifies the query population for comparisons and can differ when populationFilters are present. The filters describe only Center-proven Catalog identifier/dimension string equality or IN sets. Preserve all three fields exactly, never assume the two digests are equal. Preserve these diagnostics in your reasoning; do not self-report or recompute scope and do not add scope fields to AnalysisDraftV1. Center additionally requires the same table/time column/actor population and equal-length, non-overlapping periods for difference, relative_change or percentage_point_change. If scope is unresolved or not_applicable, report its limitations instead of asserting a supported period comparison.
Every reference must use a successful governed_data_query queryExecutionId from this same turn. Value references require both zero-based row and column of a non-null persisted measure cell. Cell rows must be within evidence_receipt [0,citableRowEndExclusive); reading another file page does not extend that range. Run a focused query when necessary. Summary evidence is required when any query succeeds. Do not wrap JSON in Markdown or emit prose outside it.`

type governedAnalysisClientContextKey struct{}

func WithGovernedAnalysisClient(ctx context.Context, client *GovernedDataClient) context.Context {
	return context.WithValue(ctx, governedAnalysisClientContextKey{}, client)
}

func GovernedAnalysisClientFromContext(ctx context.Context) (*GovernedDataClient, bool) {
	if ctx == nil {
		return nil, false
	}
	client, ok := ctx.Value(governedAnalysisClientContextKey{}).(*GovernedDataClient)
	if !ok || client == nil {
		return nil, false
	}
	bearer, tenantID, credentialOK := types.GovernedDataUserCredential(ctx)
	if !credentialOK || bearer != client.bearer || fmt.Sprint(tenantID) != client.tenantID {
		return nil, false
	}
	return client, true
}

func CopyGovernedAnalysisClient(dst, src context.Context) context.Context {
	client, ok := GovernedAnalysisClientFromContext(src)
	if !ok {
		return dst
	}
	sourceBearer, sourceTenant, sourceOK := types.GovernedDataUserCredential(src)
	destBearer, destTenant, destOK := types.GovernedDataUserCredential(dst)
	if !sourceOK || !destOK || sourceBearer != destBearer || sourceTenant != destTenant || client.bearer != sourceBearer || client.tenantID != fmt.Sprint(sourceTenant) {
		return dst
	}
	return WithGovernedAnalysisClient(dst, client)
}

func (c *GovernedDataClient) Start(ctx context.Context, conversationID, turnID, question string) error {
	c.mu.Lock()
	if c.starting || c.runID != "" {
		c.mu.Unlock()
		return fmt.Errorf("governed analysis run already admitted")
	}
	c.starting = true
	c.mu.Unlock()
	body := map[string]any{
		"contractVersion": "governed-analysis-start/1",
		"conversationId":  conversationID,
		"turnId":          turnID,
		"question":        question,
	}
	data, err := c.rawAnalysisRequest(ctx, "runs", body)
	if err != nil {
		c.mu.Lock()
		c.starting = false
		c.mu.Unlock()
		return err
	}
	var receipt governedAnalysisRunReceipt
	if err := decodeExactJSON(data, &receipt); err != nil || !validGovernedRunReceipt(receipt, "running") {
		c.mu.Lock()
		c.starting = false
		c.mu.Unlock()
		return fmt.Errorf("invalid governed analysis run receipt")
	}
	c.mu.Lock()
	if c.runID != "" {
		c.mu.Unlock()
		return fmt.Errorf("governed analysis run already admitted")
	}
	c.runID = receipt.RunID
	c.starting = false
	c.catalogVersion = receipt.CatalogVersion
	c.catalogDigest = receipt.CatalogDigest
	c.scopeDigest = receipt.ScopeDigest
	heartbeatCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	c.heartbeatCancel = cancel
	c.mu.Unlock()
	go c.heartbeatLoop(heartbeatCtx)
	return nil
}

type governedAnalysisRunReceipt struct {
	ContractVersion string `json:"contractVersion"`
	RunID           string `json:"runId"`
	State           string `json:"state"`
	CatalogVersion  string `json:"catalogVersion"`
	CatalogDigest   string `json:"catalogDigest"`
	ScopeDigest     string `json:"scopeDigest"`
	BindingDigest   string `json:"bindingDigest"`
}

func validGovernedRunReceipt(receipt governedAnalysisRunReceipt, state string) bool {
	if receipt.ContractVersion != "governed-analysis-run/1" || receipt.RunID == "" || receipt.State != state || receipt.CatalogVersion == "" ||
		!hexDigestPattern.MatchString(receipt.CatalogDigest) || !prefixedDigestPattern.MatchString(receipt.ScopeDigest) || !prefixedDigestPattern.MatchString(receipt.BindingDigest) {
		return false
	}
	canonical, err := canonicalGovernedRunMaterial(receipt)
	if err != nil {
		return false
	}
	want := fmt.Sprintf("sha256:%x", sha256.Sum256(canonical))
	return receipt.BindingDigest == want
}

func canonicalGovernedRunMaterial(receipt governedAnalysisRunReceipt) ([]byte, error) {
	keys := []string{"catalogDigest", "catalogVersion", "contractVersion", "runId", "scopeDigest", "state"}
	values := []string{receipt.CatalogDigest, receipt.CatalogVersion, receipt.ContractVersion, receipt.RunID, receipt.ScopeDigest, receipt.State}
	var out bytes.Buffer
	out.WriteByte('{')
	for i := range keys {
		if i > 0 {
			out.WriteByte(',')
		}
		out.WriteString(`"` + keys[i] + `":`)
		if err := appendCanonicalJSONString(&out, values[i]); err != nil {
			return nil, err
		}
	}
	out.WriteByte('}')
	return out.Bytes(), nil
}

// appendCanonicalJSONString matches Center's ensure_ascii=False JSON string
// encoding for the string-only receipt material. In particular, U+2028 and
// U+2029 remain UTF-8 while a literal "\\u2028" remains a quoted backslash.
func appendCanonicalJSONString(out *bytes.Buffer, value string) error {
	const hex = "0123456789abcdef"
	out.WriteByte('"')
	for len(value) > 0 {
		r, size := utf8.DecodeRuneInString(value)
		if r == utf8.RuneError && size == 1 {
			return fmt.Errorf("invalid UTF-8 in governed analysis run receipt")
		}
		value = value[size:]
		switch r {
		case '"', '\\':
			out.WriteByte('\\')
			out.WriteRune(r)
		case '\b':
			out.WriteString(`\b`)
		case '\f':
			out.WriteString(`\f`)
		case '\n':
			out.WriteString(`\n`)
		case '\r':
			out.WriteString(`\r`)
		case '\t':
			out.WriteString(`\t`)
		default:
			if r < 0x20 {
				out.WriteString(`\u00`)
				out.WriteByte(hex[byte(r)>>4])
				out.WriteByte(hex[byte(r)&0x0f])
			} else {
				out.WriteRune(r)
			}
		}
	}
	out.WriteByte('"')
	return nil
}

func (c *GovernedDataClient) receiptMatchesBinding(receipt governedAnalysisRunReceipt, state string) bool {
	return validGovernedRunReceipt(receipt, state) && receipt.RunID == c.runID && receipt.CatalogVersion == c.catalogVersion &&
		receipt.CatalogDigest == c.catalogDigest && receipt.ScopeDigest == c.scopeDigest
}

func decodeExactJSON(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if decoder.Decode(&trailing) != io.EOF {
		return fmt.Errorf("trailing JSON")
	}
	return nil
}

func (c *GovernedDataClient) heartbeatLoop(ctx context.Context) {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := c.heartbeat(ctx); err != nil {
				c.failHeartbeat()
				return
			}
		}
	}
}

func (c *GovernedDataClient) failHeartbeat() {
	c.mu.Lock()
	if c.terminal {
		c.mu.Unlock()
		return
	}
	c.lifecycleErr = fmt.Errorf("governed analysis lease renewal failed")
	cancel := c.turnCancel
	c.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (c *GovernedDataClient) heartbeat(ctx context.Context) error {
	c.mu.Lock()
	runID, terminal := c.runID, c.terminal
	c.mu.Unlock()
	if terminal || runID == "" {
		return nil
	}
	data, err := c.analysisRequest(ctx, "analysis/heartbeat", map[string]any{"contractVersion": "governed-analysis-heartbeat/1", "runId": runID})
	if err != nil {
		return err
	}
	var receipt governedAnalysisRunReceipt
	if err := decodeExactJSON(data, &receipt); err != nil {
		return fmt.Errorf("invalid governed analysis heartbeat receipt")
	}
	c.mu.Lock()
	matches := c.receiptMatchesBinding(receipt, "running")
	c.mu.Unlock()
	if !matches {
		return fmt.Errorf("invalid governed analysis heartbeat receipt")
	}
	return nil
}

func (c *GovernedDataClient) RunID() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.runID
}

func (c *GovernedDataClient) IsTerminal() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.terminal
}

func (c *GovernedDataClient) LifecycleError() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.lifecycleErr
}

func (c *GovernedDataClient) SetTurnCancel(cancel context.CancelFunc) {
	c.mu.Lock()
	c.turnCancel = cancel
	failed := c.lifecycleErr != nil
	c.mu.Unlock()
	if failed && cancel != nil {
		cancel()
	}
}

func (c *GovernedDataClient) Finalize(ctx context.Context, draftText string) (string, map[string]any, error) {
	c.terminalMu.Lock()
	defer c.terminalMu.Unlock()
	trimmed := strings.TrimSpace(draftText)
	if trimmed == "" || trimmed[0] != '{' {
		return "", nil, fmt.Errorf("model did not return AnalysisDraftV1 JSON")
	}
	var draft map[string]any
	decoder := json.NewDecoder(strings.NewReader(trimmed))
	decoder.UseNumber()
	if err := decoder.Decode(&draft); err != nil || draft["contractVersion"] != "analysis-draft/1" {
		return "", nil, fmt.Errorf("model returned invalid AnalysisDraftV1 JSON")
	}
	var trailing any
	if decoder.Decode(&trailing) != io.EOF {
		return "", nil, fmt.Errorf("model returned trailing content after AnalysisDraftV1")
	}
	c.mu.Lock()
	if c.terminal || c.runID == "" || c.lifecycleErr != nil {
		c.mu.Unlock()
		return "", nil, fmt.Errorf("governed analysis run is not running")
	}
	runID := c.runID
	c.mu.Unlock()
	data, err := c.analysisRequest(ctx, "analysis/finalize", map[string]any{"contractVersion": "governed-analysis-finalize/1", "runId": runID, "draft": draft})
	if err != nil {
		return "", nil, err
	}
	var result map[string]any
	decode := json.NewDecoder(bytes.NewReader(data))
	decode.UseNumber()
	if err := decode.Decode(&result); err != nil {
		return "", nil, fmt.Errorf("invalid governed analysis result")
	}
	if decode.Decode(&trailing) != io.EOF {
		return "", nil, fmt.Errorf("invalid governed analysis result")
	}
	run, _ := result["run"].(map[string]any)
	answer, _ := result["answer"].(map[string]any)
	runBytes, _ := json.Marshal(run)
	var completedReceipt governedAnalysisRunReceipt
	if result["contractVersion"] != "governed-analysis-result/1" || decodeExactJSON(runBytes, &completedReceipt) != nil || answer["contractVersion"] != "analytical-answer/1" || answer["runId"] != runID {
		return "", nil, fmt.Errorf("governed analysis result does not match completed run")
	}
	c.mu.Lock()
	matched := c.receiptMatchesBinding(completedReceipt, "completed")
	if matched {
		c.markTerminalLocked()
	}
	c.mu.Unlock()
	if !matched {
		return "", nil, fmt.Errorf("governed analysis result does not match completed run")
	}
	presentation, err := renderGovernedAnalysisAnswer(answer)
	if err != nil {
		return "", result, err
	}
	return presentation, result, nil
}

func (c *GovernedDataClient) Terminate(ctx context.Context, state, code, message string) error {
	c.terminalMu.Lock()
	defer c.terminalMu.Unlock()
	if state != "failed" && state != "cancelled" {
		return fmt.Errorf("invalid governed analysis terminal state")
	}
	c.mu.Lock()
	if c.terminal || c.runID == "" {
		c.mu.Unlock()
		return nil
	}
	runID := c.runID
	c.mu.Unlock()
	c.markTerminal()
	data, err := c.rawAnalysisRequest(context.WithoutCancel(ctx), "analysis/terminate", map[string]any{"contractVersion": "governed-analysis-terminate/1", "runId": runID, "state": state, "code": code, "message": message})
	if err != nil {
		return err
	}
	var receipt governedAnalysisRunReceipt
	c.mu.Lock()
	matched := decodeExactJSON(data, &receipt) == nil && c.receiptMatchesBinding(receipt, state)
	c.mu.Unlock()
	if !matched {
		return fmt.Errorf("invalid governed analysis termination receipt")
	}
	return nil
}

func (c *GovernedDataClient) markTerminal() {
	c.mu.Lock()
	c.markTerminalLocked()
	c.mu.Unlock()
}

func (c *GovernedDataClient) markTerminalLocked() {
	c.terminal = true
	if c.heartbeatCancel != nil {
		c.heartbeatCancel()
		c.heartbeatCancel = nil
	}
}

func renderGovernedAnalysisAnswer(answer map[string]any) (string, error) {
	presentation, ok := answer["answer"].(map[string]any)
	if !ok {
		return "", fmt.Errorf("governed analysis completed without an accepted presentation")
	}
	manifest, ok := answer["factManifest"].(map[string]any)
	if !ok {
		return "", fmt.Errorf("governed analysis presentation has no fact manifest")
	}
	facts := map[string]string{}
	for _, item := range asAnySlice(manifest["facts"]) {
		fact, _ := item.(map[string]any)
		id, _ := fact["factId"].(string)
		value, _ := fact["value"].(map[string]any)
		exact, _ := value["exact"].(string)
		if id != "" && exact != "" {
			facts[id] = exact
		}
	}
	summary, _ := presentation["summary"].(map[string]any)
	segments := asAnySlice(summary["segments"])
	if len(segments) == 0 {
		return "", fmt.Errorf("governed analysis presentation summary is empty")
	}
	summaryText, err := renderGovernedSegments(segments, facts)
	if err != nil {
		return "", err
	}
	paragraphs := []string{summaryText}
	for _, item := range asAnySlice(presentation["insights"]) {
		insight, _ := item.(map[string]any)
		claim, _ := insight["claim"].(map[string]any)
		claimSegments := asAnySlice(claim["segments"])
		if len(claimSegments) == 0 {
			return "", fmt.Errorf("governed analysis insight claim is empty")
		}
		claimText, err := renderGovernedSegments(claimSegments, facts)
		if err != nil {
			return "", err
		}
		label := "分析"
		switch insight["kind"] {
		case "calculation_confirmation":
			label = "观察"
		case "business_interpretation":
			label = "解释"
		case "hypothesis":
			label = "假设"
		}
		switch insight["verificationStatus"] {
		case "unverified":
			label += "（未验证）"
		case "rejected":
			label += "（已拒绝）"
		}
		paragraphs = append(paragraphs, label+"："+claimText)
	}
	return strings.Join(paragraphs, "\n\n"), nil
}

// Render only Center-accepted text and fact references; drafting and calculation
// slots are never interpreted in this presentation path.
func renderGovernedSegments(segments []any, facts map[string]string) (string, error) {
	var out strings.Builder
	for _, item := range segments {
		segment, _ := item.(map[string]any)
		switch segment["type"] {
		case "text":
			text, ok := segment["text"].(string)
			if !ok {
				return "", fmt.Errorf("invalid governed analysis text segment")
			}
			out.WriteString(text)
		case "fact":
			ref, _ := segment["ref"].(map[string]any)
			factID, _ := ref["factId"].(string)
			exact, ok := facts[factID]
			if !ok {
				return "", fmt.Errorf("governed analysis fact reference is unresolved")
			}
			out.WriteString(exact)
		default:
			return "", fmt.Errorf("unsupported governed analysis presentation segment")
		}
	}
	return out.String(), nil
}

func asAnySlice(value any) []any {
	items, _ := value.([]any)
	return items
}
