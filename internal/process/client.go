package process

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type health struct {
	Healthy         bool   `json:"healthy"`
	InstanceID      string `json:"instanceId"`
	ProtocolVersion int    `json:"protocolVersion"`
}

func Healthy(m Metadata) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	return HealthyContext(ctx, m)
}

func HealthyContext(ctx context.Context, m Metadata) bool {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("http://127.0.0.1:%d/v1/health", m.Port), nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return false
	}
	var h health
	if json.NewDecoder(resp.Body).Decode(&h) != nil {
		return false
	}
	return h.Healthy && h.ProtocolVersion == m.ProtocolVersion && subtle.ConstantTimeCompare([]byte(h.InstanceID), []byte(m.InstanceID)) == 1
}
