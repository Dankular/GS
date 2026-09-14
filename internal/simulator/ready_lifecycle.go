package simulator

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// HTTPReadyLifecycle reports the server's match-specific readiness to the
// control plane using the same server claim token used for result submission.
// MatchID is evaluated at Ready time because dynamic Agones assignment happens
// immediately before bootstrap.
type HTTPReadyLifecycle struct {
	ControlURL  string
	Token       string
	TokenSource func() string
	MatchID     func() string
	Client      *http.Client
}

func (s HTTPReadyLifecycle) Ready() error {
	token := s.Token
	if s.TokenSource != nil {
		token = s.TokenSource()
	}
	if strings.TrimSpace(s.ControlURL) == "" || strings.TrimSpace(token) == "" || s.MatchID == nil || strings.TrimSpace(s.MatchID()) == "" {
		return fmt.Errorf("control API ready lifecycle is not configured")
	}
	client := s.Client
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	request, err := http.NewRequestWithContext(context.Background(), http.MethodPost, strings.TrimRight(s.ControlURL, "/")+"/v1/server/matches/"+s.MatchID()+"/ready", nil)
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+token)
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("control API rejected ready lifecycle: %s", response.Status)
	}
	return nil
}

func (s HTTPReadyLifecycle) Health() error   { return nil }
func (s HTTPReadyLifecycle) Shutdown() error { return nil }
