package virtualtools

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestHandleCallSubAgentReturnsTypedFailedEnvelope(t *testing.T) {
	ctx := context.WithValue(context.Background(), ExecutePredefinedSubAgentKey, ExecutePredefinedSubAgentFunc(
		func(context.Context, string, string, string) (string, error) {
			return "partial child evidence", errors.New("failed to create execution-only agent")
		},
	))

	result, err := handleCallSubAgent(ctx, map[string]interface{}{
		"route_id":       "review",
		"todo_id":        "todo-1",
		"instructions":   "review the output",
		"preferred_tier": float64(1),
	})
	if err != nil {
		t.Fatalf("tool boundary should return a readable failure envelope, got error: %v", err)
	}
	var payload SubAgentResult
	if err := json.Unmarshal([]byte(result), &payload); err != nil {
		t.Fatalf("decode result: %v\n%s", err, result)
	}
	if payload.Success || payload.Error != "failed to create execution-only agent" || payload.Result != "partial child evidence" {
		t.Fatalf("failure envelope = %#v", payload)
	}
}

func TestHandleCallGenericAgentReturnsTypedFailedEnvelope(t *testing.T) {
	ctx := context.WithValue(context.Background(), ExecuteGenericAgentKey, ExecuteGenericAgentFunc(
		func(context.Context, string, string) (string, error) {
			return "partial generic evidence", errors.New("generic child failed")
		},
	))

	result, err := handleCallGenericAgent(ctx, map[string]interface{}{
		"todo_id":        "todo-1",
		"instructions":   "inspect",
		"preferred_tier": float64(1),
	})
	if err != nil {
		t.Fatalf("tool boundary should return a readable failure envelope, got error: %v", err)
	}
	var payload SubAgentResult
	if err := json.Unmarshal([]byte(result), &payload); err != nil {
		t.Fatalf("decode result: %v\n%s", err, result)
	}
	if payload.Success || payload.Error != "generic child failed" || payload.Result != "partial generic evidence" {
		t.Fatalf("failure envelope = %#v", payload)
	}
}

func TestHandleCallGenericAgentPropagatesMessageSequence(t *testing.T) {
	var captured []GenericAgentMessage
	ctx := context.WithValue(context.Background(), ExecuteGenericAgentKey, ExecuteGenericAgentFunc(
		func(ctx context.Context, todoID, instructions string) (string, error) {
			captured = GenericAgentMessageSequenceFromContext(ctx)
			if todoID != "review" || instructions != "collect shared evidence" {
				t.Fatalf("unexpected opening args: todo=%q instructions=%q", todoID, instructions)
			}
			return "complete", nil
		},
	))

	_, err := handleCallGenericAgent(ctx, map[string]interface{}{
		"todo_id":        "review",
		"instructions":   "collect shared evidence",
		"preferred_tier": float64(1),
		"message_sequence": []interface{}{
			map[string]interface{}{"id": "lens", "title": "Lens", "message": "inspect"},
			map[string]interface{}{"id": "final", "message": "consolidate"},
		},
	})
	if err != nil {
		t.Fatalf("handleCallGenericAgent: %v", err)
	}
	want := []GenericAgentMessage{
		{ID: "lens", Title: "Lens", Message: "inspect"},
		{ID: "final", Message: "consolidate"},
	}
	if !reflect.DeepEqual(captured, want) {
		t.Fatalf("captured sequence = %#v, want %#v", captured, want)
	}
}

func TestCallGenericAgentSchemaPublishesMessageSequence(t *testing.T) {
	tools := CreateSubAgentTools()
	for _, tool := range tools {
		if tool.Function == nil || tool.Function.Name != "call_generic_agent" {
			continue
		}
		encoded, err := json.Marshal(tool.Function.Parameters)
		if err != nil {
			t.Fatal(err)
		}
		text := string(encoded)
		for _, want := range []string{`"message_sequence"`, `"maxItems":12`, `"id"`, `"message"`} {
			if !strings.Contains(text, want) {
				t.Fatalf("call_generic_agent schema missing %s: %s", want, text)
			}
		}
		return
	}
	t.Fatal("call_generic_agent tool not found")
}

func TestHandleCallSubAgentPropagatesMessageSequenceRestart(t *testing.T) {
	called := false
	ctx := context.WithValue(context.Background(), ExecutePredefinedSubAgentKey, ExecutePredefinedSubAgentFunc(
		func(ctx context.Context, routeID, todoID, instructions string) (string, error) {
			called = true
			if restart, _ := ctx.Value(SubAgentMessageSequenceRestartKey).(bool); !restart {
				t.Fatalf("expected message sequence restart flag to be propagated")
			}
			if routeID != "seq-route" || todoID != "todo-1" || instructions != "run again" {
				t.Fatalf("unexpected args: route=%q todo=%q instructions=%q", routeID, todoID, instructions)
			}
			return "ok", nil
		},
	))

	result, err := handleCallSubAgent(ctx, map[string]interface{}{
		"route_id":                 "seq-route",
		"todo_id":                  "todo-1",
		"instructions":             "run again",
		"preferred_tier":           float64(1),
		"message_sequence_restart": true,
	})
	if err != nil {
		t.Fatalf("handleCallSubAgent returned error: %v", err)
	}
	if !called {
		t.Fatalf("expected execute function to be called")
	}
	if !strings.Contains(result, `"success": true`) {
		t.Fatalf("expected successful result JSON, got %s", result)
	}
}

func TestHandleCallSubAgentPassesThroughAsyncStart(t *testing.T) {
	const asyncResult = `{"async":true,"execution_id":"child-123","status":"running"}`
	ctx := context.WithValue(context.Background(), ExecutePredefinedSubAgentKey, ExecutePredefinedSubAgentFunc(
		func(context.Context, string, string, string) (string, error) {
			return asyncResult, nil
		},
	))

	result, err := handleCallSubAgent(ctx, map[string]interface{}{
		"route_id":       "review",
		"todo_id":        "todo-1",
		"instructions":   "review the output",
		"preferred_tier": float64(1),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result != asyncResult {
		t.Fatalf("result=%s, want the authoritative async start unchanged", result)
	}
	if strings.Contains(result, `"success"`) || strings.Contains(result, `"completed_at"`) {
		t.Fatalf("async start was falsely wrapped as terminal: %s", result)
	}
}

func TestQueryAndStopSubAgentHandlersDispatchByExecutionID(t *testing.T) {
	ctx := context.WithValue(context.Background(), QuerySubAgentKey, QuerySubAgentFunc(
		func(_ context.Context, executionID string) (string, error) {
			return "query:" + executionID, nil
		},
	))
	ctx = context.WithValue(ctx, StopSubAgentKey, StopSubAgentFunc(
		func(_ context.Context, executionID string) (string, error) {
			return "stop:" + executionID, nil
		},
	))

	queryResult, err := handleQuerySubAgent(ctx, map[string]interface{}{"execution_id": "child-1"})
	if err != nil || queryResult != "query:child-1" {
		t.Fatalf("query result=(%q, %v)", queryResult, err)
	}
	stopResult, err := handleStopSubAgent(ctx, map[string]interface{}{"execution_id": "child-1"})
	if err != nil || stopResult != "stop:child-1" {
		t.Fatalf("stop result=(%q, %v)", stopResult, err)
	}
	if _, err := handleQuerySubAgent(context.Background(), map[string]interface{}{"execution_id": "child-1"}); err == nil {
		t.Fatal("query handler succeeded without an orchestrator-owned function")
	}
}
