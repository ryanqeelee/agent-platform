package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Tencent/WeKnora/internal/application/repository"
	"github.com/Tencent/WeKnora/internal/event"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// transcriptStreams records what the installer projects onto the replayable
// stream, in order, so a test can state the shape the console will read.
type transcriptStreams struct {
	events []interfaces.StreamEvent
	err    error
}

func (s *transcriptStreams) AppendEvent(
	_ context.Context, _, _ string, evt interfaces.StreamEvent,
) error {
	s.events = append(s.events, evt)
	return s.err
}

func (s *transcriptStreams) GetEvents(
	_ context.Context, _, _ string, from int,
) ([]interfaces.StreamEvent, int, error) {
	return s.events[from:], len(s.events), nil
}

func (s *transcriptStreams) types() []types.ResponseType {
	out := make([]types.ResponseType, 0, len(s.events))
	for _, evt := range s.events {
		out = append(out, evt.Type)
	}
	return out
}

// transcriptMessages is the durable installation-row half. The embedded
// interface keeps this fake restricted to the one conditional write owned by
// the transcript.
type transcriptMessages struct {
	repository.TenantSkillRepository
	currentRunID string
	transcript   types.JSON
	updated      []types.JSON
	err          error
}

func (m *transcriptMessages) UpdateInstallTranscript(
	_ context.Context, _, _, runID string, transcript types.JSON,
) (bool, error) {
	if runID != m.currentRunID {
		return false, nil
	}
	m.transcript = append(types.JSON(nil), transcript...)
	m.updated = append(m.updated, append(types.JSON(nil), transcript...))
	return true, m.err
}

func newTranscriptForTest(t *testing.T) (
	*installTranscript, *event.EventBus, *transcriptStreams, *transcriptMessages,
) {
	t.Helper()
	bus := event.NewEventBus()
	streams := &transcriptStreams{}
	messages := &transcriptMessages{currentRunID: "run-1"}
	tr := newInstallTranscript(
		context.Background(), bus, streams, messages, "cfg-1", "sk-1", "run-1", nil,
	)
	tr.Subscribe()
	return tr, bus, streams, messages
}

func TestInstallTranscriptProjectsTheAgentsWork(t *testing.T) {
	_, bus, streams, _ := newTranscriptForTest(t)
	ctx := context.Background()

	require.NoError(t, bus.Emit(ctx, event.Event{
		ID: "t-1", Type: event.EventAgentThought,
		Data: event.AgentThoughtData{Content: "check for uv", Done: false},
	}))
	require.NoError(t, bus.Emit(ctx, event.Event{
		ID: "c-1", Type: event.EventAgentToolCall,
		Data: event.AgentToolCallData{
			ToolName: "shell_exec", ToolCallID: "call-1",
			Arguments: map[string]any{"command": "uv --version"},
		},
	}))
	require.NoError(t, bus.Emit(ctx, event.Event{
		ID: "r-1", Type: event.EventAgentToolResult,
		Data: event.AgentToolResultData{
			ToolName: "shell_exec", ToolCallID: "call-1", Success: true, Output: "uv 0.4.0",
		},
	}))

	require.Equal(t, []types.ResponseType{
		types.ResponseTypeThinking,
		types.ResponseTypeToolCall,
		types.ResponseTypeToolResult,
	}, streams.types())

	call := streams.events[1]
	require.Equal(t, "shell_exec", call.Data["tool_name"])
	require.Equal(t, "call-1", call.Data["tool_call_id"])
}

// A failed command is the single most useful thing in an install transcript,
// so it must reach the stream as an error the console already knows how to
// render, not be dropped for having Success=false.
func TestInstallTranscriptReportsAFailedCommandAsAnError(t *testing.T) {
	_, bus, streams, _ := newTranscriptForTest(t)

	require.NoError(t, bus.Emit(context.Background(), event.Event{
		ID: "r-1", Type: event.EventAgentToolResult,
		Data: event.AgentToolResultData{
			ToolName: "shell_exec", ToolCallID: "call-1",
			Success: false, Error: "exit status 1",
		},
	}))

	require.Equal(t, []types.ResponseType{types.ResponseTypeError}, streams.types())
	require.Equal(t, "exit status 1", streams.events[0].Content)
	require.Equal(t, false, streams.events[0].Data["success"])
}

func TestInstallTranscriptPersistsTheAnswerAndStepsOnComplete(t *testing.T) {
	tr, bus, streams, messages := newTranscriptForTest(t)
	ctx := context.Background()
	require.NoError(t, tr.Create(ctx, "install pdf-tools"))
	messages.updated = nil

	require.NoError(t, bus.Emit(ctx, event.Event{
		ID: "a-1", Type: event.EventAgentFinalAnswer,
		Data: event.AgentFinalAnswerData{Content: "installed ", Done: false},
	}))
	require.NoError(t, bus.Emit(ctx, event.Event{
		ID: "a-1", Type: event.EventAgentFinalAnswer,
		Data: event.AgentFinalAnswerData{Content: "pdf-tools", Done: true},
	}))
	require.NoError(t, bus.Emit(ctx, event.Event{
		ID: "done", Type: event.EventAgentComplete,
		Data: event.AgentCompleteData{
			MessageID: "sk-1", TotalDurationMs: 4200, TotalSteps: 3,
		},
	}))
	tr.Finish(ctx, nil)

	require.Equal(t, types.ResponseTypeComplete, streams.types()[len(streams.types())-1])
	require.Len(t, messages.updated, 1)
	require.Contains(t, string(messages.transcript), "installed pdf-tools")
	var persisted []*types.Message
	require.NoError(t, json.Unmarshal(messages.transcript, &persisted))
	require.Len(t, persisted, 2)
	require.Equal(t, int64(4200), persisted[1].AgentDurationMs)
}

// An install that dies before the engine emits complete is exactly the run
// someone will come looking for, so Finish must close the record itself.
func TestInstallTranscriptRecordsAFailedRun(t *testing.T) {
	tr, _, streams, messages := newTranscriptForTest(t)
	require.NoError(t, tr.Create(context.Background(), "install pdf-tools"))
	streams.events = nil
	messages.updated = nil

	tr.Finish(context.Background(), errors.New("installer agent failed: boom"))

	require.Equal(t, []types.ResponseType{
		types.ResponseTypeError,
		types.ResponseTypeComplete,
	}, streams.types())
	require.Len(t, messages.updated, 1)
	require.Contains(t, string(messages.transcript), "boom")
}

// The transcript is a troubleshooting aid. A broken Redis or a failing write
// must not be able to fail an otherwise good install.
func TestInstallTranscriptSwallowsItsOwnFailures(t *testing.T) {
	bus := event.NewEventBus()
	streams := &transcriptStreams{err: errors.New("redis down")}
	messages := &transcriptMessages{
		currentRunID: "run-1", err: errors.New("db down"),
	}
	tr := newInstallTranscript(
		context.Background(), bus, streams, messages, "cfg-1", "sk-1", "run-1", nil,
	)
	tr.Subscribe()

	require.NoError(t, bus.Emit(context.Background(), event.Event{
		ID: "t-1", Type: event.EventAgentThought,
		Data: event.AgentThoughtData{Content: "hi", Done: true},
	}))
	require.NotPanics(t, func() { tr.Finish(context.Background(), nil) })
}

func TestInstallTranscriptCreatePersistsTheOpeningPrompt(t *testing.T) {
	streams := &transcriptStreams{}
	messages := &transcriptMessages{currentRunID: "run-1"}
	tr := newInstallTranscript(
		context.Background(), event.NewEventBus(), streams, messages,
		"cfg-1", "sk-1", "run-1", nil,
	)

	require.NoError(t, tr.Create(context.Background(), "install pdf-tools"))
	require.Len(t, messages.updated, 1)
	require.Contains(t, string(messages.transcript), "install pdf-tools")
}

func TestInstallTranscriptCannotOverwriteANewerRun(t *testing.T) {
	current := types.JSON(`[{"content":"newer"}]`)
	messages := &transcriptMessages{
		currentRunID: "run-new", transcript: append(types.JSON(nil), current...),
	}
	tr := newInstallTranscript(
		context.Background(), event.NewEventBus(), &transcriptStreams{}, messages,
		"cfg-1", "sk-1", "run-old", nil,
	)

	require.NoError(t, tr.Create(context.Background(), "stale prompt"))
	tr.Finish(context.Background(), errors.New("stale failure"))

	require.JSONEq(t, string(current), string(messages.transcript))
	require.Empty(t, messages.updated)
}

// A console that attaches to a running install reads the event log and nothing
// else, so the prompt has to be in the log or the run shows up as an agent
// answering a question nobody can see.
func TestInstallTranscriptCreateOpensTheLogWithThePrompt(t *testing.T) {
	streams := &transcriptStreams{}
	messages := &transcriptMessages{currentRunID: "run-1"}
	tr := newInstallTranscript(
		context.Background(), event.NewEventBus(), streams, messages,
		"cfg-1", "sk-1", "run-1", nil,
	)

	require.NoError(t, tr.Create(context.Background(), "install pdf-tools"))

	require.Equal(t, []types.ResponseType{types.ResponseTypeInstallPrompt}, streams.types())
	require.Equal(t, "install pdf-tools", streams.events[0].Content)
}

// An install may run a second installer round, because verification happens
// after the agent stops and a dependency it names is something the agent can
// still fix. The engine reports "complete" at the end of every turn, so the
// transcript must not treat the first one as the end of the install — a console
// that saw it would stop following before the repair round began.
func TestInstallTranscriptStaysOpenAcrossInstallerRounds(t *testing.T) {
	tr, bus, streams, messages := newTranscriptForTest(t)
	ctx := context.Background()
	require.NoError(t, tr.Create(ctx, "install pdf-tools"))
	streams.events = nil
	messages.updated = nil

	finishRound := func(steps int, ms int64) {
		require.NoError(t, bus.Emit(ctx, event.Event{
			ID: "done", Type: event.EventAgentComplete,
			Data: event.AgentCompleteData{
				MessageID: "sk-1", TotalSteps: steps, TotalDurationMs: ms,
			},
		}))
	}

	finishRound(3, 1000)
	tr.RecordPrompt("Verification failed. Install defusedxml into the venv.")
	finishRound(2, 500)
	tr.Finish(ctx, nil)

	kinds := streams.types()
	require.Equal(t, []types.ResponseType{
		types.ResponseTypeInstallPrompt,
		types.ResponseTypeComplete,
	}, kinds, "the repair instruction has to be in the log, and the install ends once")
	require.Contains(t, streams.events[0].Content, "Install defusedxml")

	terminal := streams.events[len(streams.events)-1]
	require.EqualValues(t, 5, terminal.Data["total_steps"],
		"an install that needed a repair round cost both turns")
	require.EqualValues(t, 1500, terminal.Data["total_duration_ms"])
	require.Len(t, messages.updated, 1)
}

// Finish is deferred by the install path and also called on the error path that
// gives up before that defer is armed. Saying "this install is over" twice would
// duplicate the verdict in the record people read.
func TestInstallTranscriptFinishIsIdempotent(t *testing.T) {
	tr, _, streams, messages := newTranscriptForTest(t)
	require.NoError(t, tr.Create(context.Background(), "install pdf-tools"))
	streams.events = nil
	messages.updated = nil

	tr.Finish(context.Background(), errors.New("python verification failed: boom"))
	tr.Finish(context.Background(), errors.New("python verification failed: boom"))

	require.Equal(t, []types.ResponseType{
		types.ResponseTypeError,
		types.ResponseTypeComplete,
	}, streams.types())
	require.Len(t, messages.updated, 1)
}

func TestAsymptoticInstallPercent(t *testing.T) {
	require.Equal(t, 35, asymptoticInstallPercent(0))
	require.Equal(t, 35, asymptoticInstallPercent(-3), "no commands yet means the seeded anchor")

	// The first command lifts the bar visibly; the exact value pins the curve.
	require.Equal(t, 39, asymptoticInstallPercent(1))
	require.Equal(t, 42, asymptoticInstallPercent(2))

	// Monotonic for any round count, always below the agent_done anchor at 80,
	// and converged to the ceiling once a run exhausts max_iterations.
	prev := 0
	for k := 1; k <= 200; k++ {
		p := asymptoticInstallPercent(k)
		require.GreaterOrEqual(t, p, prev)
		require.LessOrEqual(t, p, 79)
		prev = p
	}
	require.Equal(t, 79, asymptoticInstallPercent(200))
	// A run that exhausts max_iterations=30 tops out around 75 — the ceiling
	// belongs to the stage anchor, not to any finite command count.
	require.Equal(t, 75, asymptoticInstallPercent(30))
}

// The progress card watches the same bus the transcript does, so every command
// the installer runs is one step forward — with a one-line summary of what it
// was — and muting after the first round stops the flow entirely.
func TestInstallTranscriptPublishesActivityProgressOnToolCalls(t *testing.T) {
	bus := event.NewEventBus()
	streams := &transcriptStreams{}
	messages := &transcriptMessages{currentRunID: "run-1"}
	type activity struct {
		steps   int
		lastCmd string
	}
	var got []activity
	tr := newInstallTranscript(context.Background(), bus, streams, messages,
		"cfg-1", "sk-1", "run-1",
		func(steps int, lastCmd string) { got = append(got, activity{steps, lastCmd}) })
	tr.Subscribe()
	ctx := context.Background()

	emitCall := func(id string, data event.AgentToolCallData) {
		data.ToolCallID = id
		require.NoError(t, bus.Emit(ctx, event.Event{
			ID: id, Type: event.EventAgentToolCall, Data: data,
		}))
	}
	emitCall("c-1", event.AgentToolCallData{
		ToolName: "shell_exec", Arguments: map[string]any{"command": "uv venv .venv"},
	})
	emitCall("c-2", event.AgentToolCallData{
		ToolName: "shell_exec", Hint: `shell_exec: apt-get install -y libgl1`,
	})
	emitCall("c-3", event.AgentToolCallData{
		ToolName: "shell_exec",
		Arguments: map[string]any{"command": "uv pip install --python .venv/bin/python " +
			strings.Repeat("very-long-package-name-", 12)},
	})

	require.Len(t, got, 3)
	require.Equal(t, 1, got[0].steps)
	require.Equal(t, "shell_exec: uv venv .venv", got[0].lastCmd)
	require.Equal(t, `shell_exec: apt-get install -y libgl1`, got[1].lastCmd,
		"the engine's hint is preferred when it has one")
	require.Equal(t, 3, got[2].steps)
	require.Less(t, len([]rune(got[2].lastCmd)), 90, "a long command is truncated to one line")

	// A repair round must not drag the bar back below the stage anchors, so
	// muting after the first round ends the activity publishes for good.
	tr.muteActivityProgress()
	emitCall("c-4", event.AgentToolCallData{
		ToolName: "shell_exec", Arguments: map[string]any{"command": "uv pip install defusedxml"},
	})
	require.Len(t, got, 3)
}

// A transcript without a publisher (remove path, most tests) counts nothing
// and calls nothing.
func TestInstallTranscriptWithoutActivityPublisherStaysSilent(t *testing.T) {
	tr, bus, streams, _ := newTranscriptForTest(t)

	require.NoError(t, bus.Emit(context.Background(), event.Event{
		ID: "c-1", Type: event.EventAgentToolCall,
		Data: event.AgentToolCallData{
			ToolName: "shell_exec", ToolCallID: "call-1",
			Arguments: map[string]any{"command": "uv venv .venv"},
		},
	}))
	tr.muteActivityProgress()

	require.NotPanics(t, func() { tr.Finish(context.Background(), nil) })
	require.Equal(t, []types.ResponseType{types.ResponseTypeToolCall, types.ResponseTypeComplete},
		streams.types(), "the transcript itself is unchanged by progress being unwired")
}
