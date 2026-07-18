package hook

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunWritesBoundedPayloadFreeAtomicMarker(t *testing.T) {
	root := t.TempDir()
	t.Setenv("CODEX_INSPECTOR_HOME", root)
	payload := `{"session_id":"fake-session","turn_id":"fake-turn","transcript_path":"/fake/rollout.jsonl","hook_event_name":"UserPromptSubmit","prompt":"PAYLOAD-CANARY"}`
	if err := Run(strings.NewReader(payload)); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(filepath.Join(root, "queue"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || strings.HasPrefix(entries[0].Name(), ".") {
		t.Fatalf("unexpected queue entries: %v", entries)
	}
	b, err := os.ReadFile(filepath.Join(root, "queue", entries[0].Name()))
	if err != nil {
		t.Fatal(err)
	}
	if len(b) > 4096 || bytes.Contains(b, []byte("PAYLOAD-CANARY")) {
		t.Fatalf("marker leaked payload: %s", b)
	}
	var m Marker
	if json.Unmarshal(b, &m) != nil || m.SessionID != "fake-session" || m.EventKind != "user_prompt_submit" || m.ProtocolVersion != 1 {
		t.Fatalf("invalid marker: %s", b)
	}
	info, _ := os.Stat(filepath.Join(root, "queue", entries[0].Name()))
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("marker mode=%o", info.Mode().Perm())
	}
}
func TestRunRejectsOversizeAndInvalidWithoutMarker(t *testing.T) {
	root := t.TempDir()
	t.Setenv("CODEX_INSPECTOR_HOME", root)
	if Run(strings.NewReader(strings.Repeat("x", maxHookInput+1))) == nil {
		t.Fatal("oversize accepted")
	}
	if Run(strings.NewReader(`{"session_id":"fake","hook_event_name":"Future"}`)) == nil {
		t.Fatal("unknown event accepted")
	}
	entries, _ := os.ReadDir(filepath.Join(root, "queue"))
	if len(entries) != 0 {
		t.Fatal("invalid hook created marker")
	}
}

func TestFrozenMarkerFieldBoundaries(t *testing.T) {
	root := t.TempDir()
	t.Setenv("CODEX_INSPECTOR_HOME", root)
	valid128 := "a" + strings.Repeat("b", 127)
	validPath := "/fake/rollout.jsonl"
	if err := Run(strings.NewReader(`{"session_id":"` + valid128 + `","turn_id":"turn:1","transcript_path":"` + validPath + `","hook_event_name":"UserPromptSubmit"}`)); err != nil {
		t.Fatalf("valid boundary rejected: %v", err)
	}
	for name, payload := range map[string]string{
		"session too long": `{"session_id":"` + valid128 + `x","hook_event_name":"Stop"}`,
		"session slash":    `{"session_id":"bad/id","hook_event_name":"Stop"}`,
		"turn newline":     "{\"session_id\":\"ok\",\"turn_id\":\"bad\\nturn\",\"hook_event_name\":\"Stop\"}",
		"relative locator": `{"session_id":"ok","transcript_path":"relative.jsonl","hook_event_name":"Stop"}`,
		"control locator":  "{\"session_id\":\"ok\",\"transcript_path\":\"/bad\\tpath\",\"hook_event_name\":\"Stop\"}",
		"locator too long": `{"session_id":"ok","transcript_path":"/` + strings.Repeat("p", 4096) + `","hook_event_name":"Stop"}`,
		"missing agent":    `{"session_id":"ok","hook_event_name":"SubagentStart"}`,
	} {
		t.Run(name, func(t *testing.T) {
			if Run(strings.NewReader(payload)) == nil {
				t.Fatal("invalid boundary accepted")
			}
		})
	}
}

func TestValidateMarkerRejectsMalformedPersistedValues(t *testing.T) {
	base := Marker{SchemaVersion: "inspector.hook-marker/v1", ProtocolVersion: 1, EventKind: "turn_stop", SessionID: "session-1", ObservedAt: "2026-07-18T00:00:00Z"}
	if err := ValidateMarker(base); err != nil {
		t.Fatal(err)
	}
	bad := base
	bad.ObservedAt = "not-a-time"
	if ValidateMarker(bad) == nil {
		t.Fatal("invalid timestamp accepted")
	}
	bad = base
	bad.SessionID = "bad/id"
	if ValidateMarker(bad) == nil {
		t.Fatal("invalid opaque ID accepted")
	}
	bad = base
	bad.EventKind = "future"
	if ValidateMarker(bad) == nil {
		t.Fatal("invalid event accepted")
	}
	b, _ := json.Marshal(base)
	b = append(b[:len(b)-1], []byte(`,"payload":"PAYLOAD-CANARY"}`)...)
	if _, err := DecodeMarker(b); err == nil {
		t.Fatal("additional marker property accepted")
	}
	b, _ = json.Marshal(base)
	b = append(b, []byte(` {}`)...)
	if _, err := DecodeMarker(b); err == nil {
		t.Fatal("trailing marker value accepted")
	}
}
