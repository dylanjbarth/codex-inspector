package hook

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/dylanjbarth/codex-inspector/internal/home"
	"github.com/dylanjbarth/codex-inspector/internal/version"
)

const (
	maxHookInput = 1 << 20
	maxMarker    = 4096
)

type input struct {
	SessionID      string  `json:"session_id"`
	TurnID         string  `json:"turn_id"`
	TranscriptPath *string `json:"transcript_path"`
	Event          string  `json:"hook_event_name"`
	AgentID        string  `json:"agent_id"`
}
type Marker struct {
	SchemaVersion   string  `json:"schemaVersion"`
	ProtocolVersion int     `json:"protocolVersion"`
	EventKind       string  `json:"eventKind"`
	SessionID       string  `json:"sessionId"`
	TurnID          string  `json:"turnId,omitempty"`
	SubagentID      string  `json:"subagentId,omitempty"`
	TranscriptPath  *string `json:"transcriptPath,omitempty"`
	ObservedAt      string  `json:"observedAt"`
}

var kinds = map[string]string{"SessionStart": "session_start", "UserPromptSubmit": "user_prompt_submit", "PreCompact": "pre_compact", "PostCompact": "post_compact", "SubagentStart": "subagent_start", "SubagentStop": "subagent_stop", "Stop": "turn_stop"}
var opaqueID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)

func validLocator(p *string) bool {
	if p == nil {
		return true
	}
	if len(*p) == 0 || len(*p) > 4096 || !utf8.ValidString(*p) || !filepath.IsAbs(*p) {
		return false
	}
	return strings.IndexFunc(*p, func(r rune) bool { return r < 0x20 || r == 0x7f }) == -1
}

func ValidateMarker(m Marker) error {
	if m.SchemaVersion != "inspector.hook-marker/v1" || m.ProtocolVersion != version.Protocol {
		return errors.New("invalid marker contract version")
	}
	known := false
	for _, k := range kinds {
		if m.EventKind == k {
			known = true
			break
		}
	}
	if !known || !opaqueID.MatchString(m.SessionID) {
		return errors.New("invalid marker event or session")
	}
	if m.TurnID != "" && !opaqueID.MatchString(m.TurnID) {
		return errors.New("invalid marker turn")
	}
	if m.SubagentID != "" && !opaqueID.MatchString(m.SubagentID) {
		return errors.New("invalid marker subagent")
	}
	if (m.EventKind == "subagent_start" || m.EventKind == "subagent_stop") && m.SubagentID == "" {
		return errors.New("subagent marker requires subagent ID")
	}
	if !validLocator(m.TranscriptPath) {
		return errors.New("invalid marker transcript locator")
	}
	if _, err := time.Parse(time.RFC3339Nano, m.ObservedAt); err != nil {
		return errors.New("invalid marker timestamp")
	}
	return nil
}

func DecodeMarker(b []byte) (Marker, error) {
	var m Marker
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if err := d.Decode(&m); err != nil {
		return Marker{}, errors.New("invalid marker JSON")
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return Marker{}, errors.New("invalid trailing marker JSON")
	}
	if err := ValidateMarker(m); err != nil {
		return Marker{}, err
	}
	return m, nil
}

func Run(r io.Reader) error {
	lr := io.LimitReader(r, maxHookInput+1)
	b, err := io.ReadAll(lr)
	if err != nil {
		return err
	}
	if len(b) > maxHookInput {
		return errors.New("hook input exceeds safe parsing limit")
	}
	var in input
	if err = json.Unmarshal(b, &in); err != nil {
		return errors.New("invalid hook JSON")
	}
	kind, ok := kinds[in.Event]
	if !ok || !opaqueID.MatchString(in.SessionID) || (in.TurnID != "" && !opaqueID.MatchString(in.TurnID)) || (in.AgentID != "" && !opaqueID.MatchString(in.AgentID)) || !validLocator(in.TranscriptPath) {
		return errors.New("unsupported hook event")
	}
	l, err := home.Resolve()
	if err != nil {
		return err
	}
	if err = home.Ensure(l); err != nil {
		return err
	}
	m := Marker{"inspector.hook-marker/v1", version.Protocol, kind, in.SessionID, in.TurnID, in.AgentID, in.TranscriptPath, time.Now().UTC().Format(time.RFC3339Nano)}
	if err = ValidateMarker(m); err != nil {
		return err
	}
	out, err := json.Marshal(m)
	if err != nil {
		return err
	}
	if len(out) > maxMarker {
		return errors.New("marker exceeds 4 KiB")
	}
	var nonce [12]byte
	if _, err = rand.Read(nonce[:]); err != nil {
		return err
	}
	base := fmt.Sprintf("%d-%s.json", time.Now().UnixNano(), hex.EncodeToString(nonce[:]))
	tmp := filepath.Join(l.Queue, "."+base+".tmp")
	final := filepath.Join(l.Queue, base)
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	if _, err = f.Write(out); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		os.Remove(tmp)
		return err
	}
	if err = os.Rename(tmp, final); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

func Compatible(protocol string) bool {
	return strings.TrimSpace(protocol) == fmt.Sprint(version.Protocol)
}
