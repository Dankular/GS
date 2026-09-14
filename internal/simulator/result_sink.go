package simulator

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Dankular/GameService/internal/matches"
)

type HTTPResultSink struct {
	ControlURL string
	Token      string
	Client     *http.Client
}

func (s HTTPResultSink) Submit(ctx context.Context, submission matches.ResultSubmission) error {
	if strings.TrimSpace(s.ControlURL) == "" || strings.TrimSpace(s.Token) == "" {
		return fmt.Errorf("control API result sink is not configured")
	}
	client := s.Client
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	body, err := json.Marshal(struct {
		Sequence      int64           `json:"sequence"`
		Payload       json.RawMessage `json:"payload"`
		PayloadDigest string          `json:"payloadDigest"`
	}{submission.Sequence, submission.Payload, submission.PayloadDigest})
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(s.ControlURL, "/")+"/v1/server/matches/"+submission.MatchID+"/results", bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+s.Token)
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("control API rejected result: %s", response.Status)
	}
	return nil
}
