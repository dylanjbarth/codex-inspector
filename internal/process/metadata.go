package process

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"
)

type Metadata struct {
	InstanceID      string    `json:"instanceId"`
	PID             int       `json:"pid"`
	Port            int       `json:"port"`
	ProtocolVersion int       `json:"protocolVersion"`
	StartupStage    string    `json:"startupStage,omitempty"`
	CodexHome       string    `json:"codexHome"`
	CodexHomeSource string    `json:"codexHomeSource"`
	StartedAt       time.Time `json:"startedAt"`
}

func Path(run string) string { return filepath.Join(run, "server.json") }

func Read(run string) (Metadata, error) {
	b, err := os.ReadFile(Path(run))
	if err != nil {
		return Metadata{}, err
	}
	var m Metadata
	if json.Unmarshal(b, &m) != nil || m.InstanceID == "" || m.PID < 1 || m.Port < 1 || m.Port > 65535 || m.ProtocolVersion < 1 || m.CodexHome == "" || (m.CodexHomeSource != "environment" && m.CodexHomeSource != "default") {
		return Metadata{}, errors.New("invalid process metadata")
	}
	return m, nil
}

func Write(run string, m Metadata) error {
	b, err := json.Marshal(m)
	if err != nil {
		return err
	}
	tmp := Path(run) + ".tmp"
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err = f.Write(b); err == nil {
		err = f.Sync()
	}
	cerr := f.Close()
	if err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(tmp)
		return err
	}
	if err = os.Chmod(tmp, 0o600); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, Path(run))
}

func RemoveIfInstance(run, id string) error {
	m, err := Read(run)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if m.InstanceID != id {
		return nil
	}
	return os.Remove(Path(run))
}
