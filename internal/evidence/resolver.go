package evidence

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/dylanjbarth/codex-inspector/internal/sources"
	"github.com/dylanjbarth/codex-inspector/internal/storage"
	"github.com/dylanjbarth/codex-inspector/internal/version"
)

const DefaultLimit int64 = 64 << 10
const MaxLimit int64 = 256 << 10

type Chunk struct {
	SchemaVersion          int            `json:"schemaVersion"`
	DatasetEpoch           string         `json:"datasetEpoch"`
	AppliedRevision        int64          `json:"appliedRevision"`
	Coverage               map[string]any `json:"coverage"`
	EvidenceID             string         `json:"evidenceId"`
	Availability           string         `json:"availability"`
	AvailabilityObservedAt string         `json:"availabilityObservedAt"`
	AvailabilityRevision   *int64         `json:"availabilityRevision"`
	Locator                Locator        `json:"locator"`
	SourcePrefixSHA256     string         `json:"sourcePrefixSha256"`
	EventFingerprint       string         `json:"eventFingerprint"`
	Offset                 int64          `json:"offset"`
	Bytes                  int64          `json:"bytes"`
	Text                   string         `json:"text,omitempty"`
	Encoding               string         `json:"encoding,omitempty"`
	Complete               bool           `json:"complete"`
}

type Locator struct {
	SourceID      string `json:"sourceId"`
	RecordOrdinal int    `json:"recordOrdinal"`
	ByteStart     int64  `json:"byteStart"`
	ByteEnd       int64  `json:"byteEnd"`
}

func Resolve(ctx context.Context, s *storage.Store, id string, revision, offset, limit int64) (Chunk, error) {
	codexHome, _ := sources.CodexHome()
	return ResolveAt(ctx, s, codexHome, id, revision, offset, limit)
}
func ResolveAt(ctx context.Context, s *storage.Store, codexHome, id string, revision, offset, limit int64) (Chunk, error) {
	return resolveAt(ctx, s, codexHome, id, revision, offset, limit, time.Now)
}

func resolveAt(ctx context.Context, s *storage.Store, codexHome, id string, revision, offset, limit int64, now func() time.Time) (Chunk, error) {
	if offset < 0 {
		return Chunk{}, errors.New("invalid evidence offset")
	}
	if limit == 0 {
		limit = DefaultLimit
	}
	if limit < 1 || limit > MaxLimit {
		return Chunk{}, errors.New("invalid evidence limit")
	}
	loc, err := s.Evidence(id, revision)
	if err != nil {
		return Chunk{}, err
	}
	var availabilityRevision *int64
	if loc.AvailabilityRevision.Valid {
		v := loc.AvailabilityRevision.Int64
		availabilityRevision = &v
	}
	base := Chunk{SchemaVersion: version.IndexSchema, DatasetEpoch: loc.Epoch, AppliedRevision: loc.Revision, Coverage: map[string]any{"fidelity": "exact", "observed": 1, "eligible": 1}, EvidenceID: id, Availability: loc.Availability, AvailabilityObservedAt: loc.AvailabilityObservedAt, AvailabilityRevision: availabilityRevision, Locator: Locator{SourceID: loc.SourceID, RecordOrdinal: loc.RecordOrdinal, ByteStart: loc.Start, ByteEnd: loc.End}, SourcePrefixSHA256: loc.SourcePrefix, EventFingerprint: loc.EventHash, Offset: offset}
	liveUnavailable := func(availability string) Chunk {
		base.Availability = availability
		base.AvailabilityObservedAt = now().UTC().Format(time.RFC3339Nano)
		base.AvailabilityRevision = nil
		base.Coverage = map[string]any{"fidelity": "unavailable", "observed": 0, "eligible": 1, "reason": availability}
		return base
	}
	f, err := os.Open(loc.Path)
	if err != nil {
		if rediscovered := rediscover(codexHome, filepath.Dir(s.Path()), loc); rediscovered != "" {
			f, err = os.Open(rediscovered)
		}
		if err != nil {
			return liveUnavailable("source_missing"), nil
		}
	}
	defer f.Close()
	length := loc.End - loc.Start
	if length < 0 {
		return Chunk{}, errors.New("invalid stored locator")
	}
	prefixHash := sha256.New()
	if _, err = io.Copy(prefixHash, &contextReader{ctx: ctx, r: io.NewSectionReader(f, 0, loc.End)}); err != nil {
		return liveUnavailable("unreadable"), nil
	}
	if hex.EncodeToString(prefixHash.Sum(nil)) != loc.SourcePrefix {
		return liveUnavailable("fingerprint_mismatch"), nil
	}
	eventHash := sha256.New()
	if _, err = io.Copy(eventHash, &contextReader{ctx: ctx, r: io.NewSectionReader(f, loc.Start, length)}); err != nil {
		return liveUnavailable("unreadable"), nil
	}
	if hex.EncodeToString(eventHash.Sum(nil)) != loc.EventHash {
		return liveUnavailable("fingerprint_mismatch"), nil
	}
	if offset > length {
		offset = length
	}
	end := offset + limit
	if end > length {
		end = length
	}
	part := make([]byte, end-offset)
	if _, err = io.ReadFull(&contextReader{ctx: ctx, r: io.NewSectionReader(f, loc.Start+offset, end-offset)}, part); err != nil {
		return Chunk{}, err
	}
	base.Offset = offset
	base.Bytes = int64(len(part))
	base.Complete = end == length
	base.Availability = "available"
	base.Text, base.Encoding = escape(part)
	return base, nil
}

type contextReader struct {
	ctx context.Context
	r   io.Reader
}

func (r *contextReader) Read(p []byte) (int, error) {
	select {
	case <-r.ctx.Done():
		return 0, r.ctx.Err()
	default:
		return r.r.Read(p)
	}
}
func rediscover(codexHome, inspectorHome string, loc storage.EvidenceLocator) string {
	if codexHome == "" {
		return ""
	}
	candidates, err := sources.Discover(codexHome, inspectorHome)
	if err != nil {
		return ""
	}
	for _, c := range candidates {
		if c.Kind == "session_index" {
			continue
		}
		b, e := sources.Parse(c, nil)
		if e == nil && b.Source.SessionID == loc.SourceSessionID && b.Source.SegmentFingerprint == loc.SegmentFingerprint {
			return c.Path
		}
	}
	return ""
}

func escape(b []byte) (string, string) {
	if utf8.Valid(b) && !hasUnsafeControl(b) {
		return string(b), "utf-8"
	}
	var out strings.Builder
	for len(b) > 0 {
		r, n := utf8.DecodeRune(b)
		if r == utf8.RuneError && n == 1 {
			fmt.Fprintf(&out, "\\x%02x", b[0])
			b = b[1:]
			continue
		}
		if r < ' ' && r != '\n' && r != '\r' && r != '\t' || r == 0x7f {
			for _, v := range b[:n] {
				fmt.Fprintf(&out, "\\x%02x", v)
			}
		} else {
			out.Write(b[:n])
		}
		b = b[n:]
	}
	return out.String(), "escaped-bytes"
}
func hasUnsafeControl(b []byte) bool {
	return bytes.IndexFunc(b, func(r rune) bool { return r < ' ' && r != '\n' && r != '\r' && r != '\t' || r == 0x7f }) >= 0
}
