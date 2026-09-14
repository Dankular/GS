package allocation

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	pb "agones.dev/agones/pkg/allocation/go"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

type GRPCAllocator struct {
	client    pb.AllocationServiceClient
	conn      *grpc.ClientConn
	namespace string
}

func NewGRPCAllocator(ctx context.Context, endpoint, namespace string, certPEM, keyPEM, caPEM []byte) (*GRPCAllocator, error) {
	return NewGRPCAllocatorWithServerName(ctx, endpoint, namespace, certPEM, keyPEM, caPEM, "")
}

// NewGRPCAllocatorWithServerName is identical to NewGRPCAllocator but permits
// an explicitly configured TLS server name when a Docker host reaches an
// allocator through a NodePort or private address. Cluster deployments should
// leave it empty and use the endpoint's DNS name.
func NewGRPCAllocatorWithServerName(ctx context.Context, endpoint, namespace string, certPEM, keyPEM, caPEM []byte, serverName string) (*GRPCAllocator, error) {
	if strings.TrimSpace(endpoint) == "" || strings.TrimSpace(namespace) == "" {
		return nil, errors.New("Agones gRPC allocator endpoint and namespace are required")
	}
	certificate, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, fmt.Errorf("load allocator client certificate: %w", err)
	}
	roots := x509.NewCertPool()
	if len(caPEM) == 0 || !roots.AppendCertsFromPEM(caPEM) {
		return nil, errors.New("allocator CA certificate is required and must be PEM")
	}
	config := &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{certificate}, RootCAs: roots, ServerName: strings.TrimSpace(serverName)}
	conn, err := grpc.NewClient(endpoint, grpc.WithTransportCredentials(credentials.NewTLS(config)))
	if err != nil {
		return nil, fmt.Errorf("connect to Agones allocator: %w", err)
	}
	return &GRPCAllocator{client: pb.NewAllocationServiceClient(conn), conn: conn, namespace: namespace}, nil
}

func NewGRPCAllocatorWithClient(client pb.AllocationServiceClient, namespace string) (*GRPCAllocator, error) {
	if client == nil || strings.TrimSpace(namespace) == "" {
		return nil, errors.New("Agones gRPC client and namespace are required")
	}
	return &GRPCAllocator{client: client, namespace: namespace}, nil
}

func (a *GRPCAllocator) Close() error {
	if a == nil || a.conn == nil {
		return nil
	}
	return a.conn.Close()
}

func (a *GRPCAllocator) Allocate(ctx context.Context, selector Selector) (Allocation, error) {
	if err := selector.Validate(); err != nil {
		return Allocation{}, err
	}
	if a == nil || a.client == nil {
		return Allocation{}, errors.New("Agones gRPC allocator is not configured")
	}
	labels := map[string]string{"platform.game/id": selector.GameID, "platform.game/mode": selector.ModeID, "platform.game/build-id": BuildLabel(selector.Build), "platform.game/region": selector.Region, "platform.game/protocol": selector.Protocol}
	if selector.CapacityClass != "" {
		labels["platform.game/capacity-class"] = selector.CapacityClass
	}
	request := &pb.AllocationRequest{Namespace: a.namespace, GameServerSelectors: []*pb.GameServerSelector{{MatchLabels: labels}}}
	if len(selector.Metadata) > 0 {
		request.Metadata = &pb.MetaPatch{Annotations: clone(selector.Metadata)}
	}
	response, err := a.client.Allocate(ctx, request)
	if err != nil {
		return Allocation{}, fmt.Errorf("Agones allocation: %w", err)
	}
	if response == nil || response.GetGameServerName() == "" || response.GetAddress() == "" {
		return Allocation{}, errors.New("Agones allocation response is missing server identity")
	}
	ports := map[string]int{}
	for _, port := range response.GetPorts() {
		if port.GetName() != "" && port.GetPort() > 0 {
			ports[port.GetName()] = int(port.GetPort())
		}
	}
	assignedID := selector.AllocationID
	if assignedID == "" {
		assignedID = allocationID()
	}
	return Allocation{AllocationID: assignedID, GameServer: response.GetGameServerName(), Address: response.GetAddress(), Ports: ports, Labels: labels}, nil
}

func allocationID() string {
	var value [12]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "agones-allocation-unknown"
	}
	return "agones-allocation-" + hex.EncodeToString(value[:])
}
