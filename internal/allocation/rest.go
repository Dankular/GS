package allocation

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type RESTAllocator struct {
	Endpoint  string
	Namespace string
	Client    *http.Client
}

type allocationRequest struct {
	Namespace           string     `json:"namespace"`
	GameServerSelectors []selector `json:"gameServerSelectors"`
	Metadata            *metaPatch `json:"metadata,omitempty"`
}
type metaPatch struct {
	Annotations map[string]string `json:"annotations,omitempty"`
}
type selector struct {
	MatchLabels map[string]string `json:"matchLabels"`
}
type allocationResponse struct {
	GameServerName string `json:"gameServerName"`
	Address        string `json:"address"`
	Ports          []struct {
		Name string `json:"name"`
		Port int    `json:"port"`
	} `json:"ports"`
}

func (a RESTAllocator) Allocate(ctx context.Context, selectorValue Selector) (Allocation, error) {
	if err := selectorValue.Validate(); err != nil {
		return Allocation{}, err
	}
	if strings.TrimSpace(a.Endpoint) == "" || strings.TrimSpace(a.Namespace) == "" {
		return Allocation{}, errors.New("Agones REST allocator endpoint and namespace are required")
	}
	client := a.Client
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	body, err := json.Marshal(allocationRequest{Namespace: a.Namespace, GameServerSelectors: []selector{{MatchLabels: map[string]string{"platform.game/id": selectorValue.GameID, "platform.game/mode": selectorValue.ModeID, "platform.game/build": selectorValue.Build, "platform.game/region": selectorValue.Region, "platform.game/protocol": selectorValue.Protocol}}}, Metadata: metadata(selectorValue.Metadata)})
	if err != nil {
		return Allocation{}, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(a.Endpoint, "/")+"/gameserverallocation", strings.NewReader(string(body)))
	if err != nil {
		return Allocation{}, err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(request)
	if err != nil {
		return Allocation{}, fmt.Errorf("Agones allocation request: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		detail, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return Allocation{}, fmt.Errorf("Agones allocation returned %s: %s", response.Status, strings.TrimSpace(string(detail)))
	}
	var decoded allocationResponse
	if err := json.NewDecoder(response.Body).Decode(&decoded); err != nil {
		return Allocation{}, fmt.Errorf("decode Agones allocation: %w", err)
	}
	if decoded.GameServerName == "" || decoded.Address == "" {
		return Allocation{}, errors.New("Agones allocation response is missing server identity")
	}
	ports := map[string]int{}
	for _, port := range decoded.Ports {
		if port.Name != "" && port.Port > 0 {
			ports[port.Name] = port.Port
		}
	}
	assignedID := selectorValue.AllocationID
	if assignedID == "" {
		assignedID = decoded.GameServerName + "-allocation"
	}
	return Allocation{AllocationID: assignedID, GameServer: decoded.GameServerName, Address: decoded.Address, Ports: ports, Labels: map[string]string{"platform.game/id": selectorValue.GameID, "platform.game/mode": selectorValue.ModeID, "platform.game/build": selectorValue.Build, "platform.game/region": selectorValue.Region, "platform.game/protocol": selectorValue.Protocol}}, nil
}

func metadata(values map[string]string) *metaPatch {
	if len(values) == 0 {
		return nil
	}
	return &metaPatch{Annotations: values}
}

func MTLSHTTPClient(certPEM, keyPEM, caPEM []byte) (*http.Client, error) {
	certificate, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, fmt.Errorf("load allocator client certificate: %w", err)
	}
	pool := x509.NewCertPool()
	if len(caPEM) == 0 || !pool.AppendCertsFromPEM(caPEM) {
		return nil, errors.New("allocator CA certificate is required and must be PEM")
	}
	return &http.Client{Timeout: 10 * time.Second, Transport: &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{certificate}, RootCAs: pool}}}, nil
}
