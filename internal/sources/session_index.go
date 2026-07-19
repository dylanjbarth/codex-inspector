package sources

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"time"

	"github.com/dylanjbarth/codex-inspector/internal/facts"
)

func ReadSessionLabels(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	labels := map[string]string{}
	s := bufio.NewScanner(f)
	s.Buffer(make([]byte, 64*1024), 1024*1024)
	for s.Scan() {
		var row struct{ ID, ThreadName, UpdatedAt string }
		var wire struct {
			ID         string `json:"id"`
			ThreadName string `json:"thread_name"`
			UpdatedAt  string `json:"updated_at"`
		}
		if json.Unmarshal(s.Bytes(), &wire) != nil || wire.ID == "" || wire.ThreadName == "" {
			continue
		}
		if _, err := time.Parse(time.RFC3339, wire.UpdatedAt); err != nil {
			continue
		}
		row.ID, row.ThreadName, row.UpdatedAt = wire.ID, wire.ThreadName, wire.UpdatedAt
		labels[row.ID] = row.ThreadName
	}
	return labels, s.Err()
}

func ParseSessionIndex(candidate Candidate) (facts.Batch, error) {
	f, err := os.Open(candidate.Path)
	if err != nil {
		return facts.Batch{}, err
	}
	defer f.Close()
	reader := bufio.NewReaderSize(f, 64*1024)
	h := sha256.New()
	labels := map[string]string{}
	var offset int64
	ordinal, pending := 0, 0
	for {
		line, e := reader.ReadBytes('\n')
		if len(line) > 0 {
			if e == io.EOF && line[len(line)-1] != '\n' {
				pending = len(line)
				break
			}
			h.Write(line)
			offset += int64(len(line))
			ordinal++
			var wire struct {
				ID         string `json:"id"`
				ThreadName string `json:"thread_name"`
				UpdatedAt  string `json:"updated_at"`
			}
			if json.Unmarshal(line, &wire) == nil && wire.ID != "" && wire.ThreadName != "" {
				if _, te := time.Parse(time.RFC3339, wire.UpdatedAt); te == nil {
					labels[wire.ID] = wire.ThreadName
				}
			}
		}
		if e == io.EOF {
			break
		}
		if e != nil {
			return facts.Batch{}, e
		}
	}
	prefix := hex.EncodeToString(h.Sum(nil))
	identity := sha256.Sum256([]byte("session-index/v1"))
	identityHex := hex.EncodeToString(identity[:])
	return facts.Batch{Source: facts.Source{ID: "source:" + identityHex, SessionID: "session-index", SegmentFingerprint: identityHex, Kind: "session_index", Path: candidate.Path, Size: candidate.Size, MTimeNS: candidate.MTimeNS, CompleteOffset: offset, CompleteOrdinal: ordinal, PendingTail: pending, PrefixSHA256: prefix, AdapterVersion: AdapterVersion, State: map[bool]string{true: "indexing", false: "current"}[pending > 0]}, SessionLabels: labels}, nil
}
