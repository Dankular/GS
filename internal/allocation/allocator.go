package allocation

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
)

var ErrInvalidSelector = errors.New("invalid allocation selector")
var ErrNoCompatibleServer = errors.New("no compatible game server available")

type Selector struct {
	GameID        string
	ModeID        string
	Build         string
	Region        string
	Protocol      string
	CapacityClass string
	AllocationID  string
	Metadata      map[string]string
}

func (s Selector) Validate() error {
	for name, value := range map[string]string{"game": s.GameID, "mode": s.ModeID, "build": s.Build, "region": s.Region, "protocol": s.Protocol} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%w: %s is required", ErrInvalidSelector, name)
		}
	}
	return nil
}

type Allocation struct {
	AllocationID string
	GameServer   string
	Address      string
	Ports        map[string]int
	Labels       map[string]string
}

type Allocator interface {
	Allocate(context.Context, Selector) (Allocation, error)
}

// Server is the compatibility information returned by a discovery adapter.
// Production adapters populate it from Agones Allocator responses.
type Server struct {
	Name    string
	Address string
	Ports   map[string]int
	Labels  map[string]string
	Ready   bool
}

type FakeAllocator struct {
	mu      sync.Mutex
	servers []Server
	next    uint64
}

func NewFakeAllocator(servers []Server) *FakeAllocator {
	return &FakeAllocator{servers: append([]Server(nil), servers...)}
}

func (a *FakeAllocator) Allocate(ctx context.Context, selector Selector) (Allocation, error) {
	if err := selector.Validate(); err != nil {
		return Allocation{}, err
	}
	select {
	case <-ctx.Done():
		return Allocation{}, ctx.Err()
	default:
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	for i := range a.servers {
		server := a.servers[i]
		if !server.Ready || !compatible(server.Labels, selector) {
			continue
		}
		a.servers[i].Ready = false
		a.next++
		ports := map[string]int{}
		for key, port := range server.Ports {
			ports[key] = port
		}
		assignedID := selector.AllocationID
		if assignedID == "" {
			assignedID = fmt.Sprintf("fake-allocation-%d", a.next)
		}
		return Allocation{AllocationID: assignedID, GameServer: server.Name, Address: server.Address, Ports: ports, Labels: clone(server.Labels)}, nil
	}
	return Allocation{}, ErrNoCompatibleServer
}

func compatible(labels map[string]string, selector Selector) bool {
	for key, expected := range map[string]string{"platform.game/id": selector.GameID, "platform.game/mode": selector.ModeID, "platform.game/build": selector.Build, "platform.game/region": selector.Region, "platform.game/protocol": selector.Protocol} {
		if labels[key] != expected {
			return false
		}
	}
	if selector.CapacityClass != "" && labels["platform.game/capacity-class"] != selector.CapacityClass {
		return false
	}
	return true
}

func clone(input map[string]string) map[string]string {
	output := map[string]string{}
	for key, value := range input {
		output[key] = value
	}
	return output
}

func RequiredLabels(selector Selector) []string {
	labels := []string{"platform.game/id=" + selector.GameID, "platform.game/mode=" + selector.ModeID, "platform.game/build=" + selector.Build, "platform.game/region=" + selector.Region, "platform.game/protocol=" + selector.Protocol}
	if selector.CapacityClass != "" {
		labels = append(labels, "platform.game/capacity-class="+selector.CapacityClass)
	}
	sort.Strings(labels)
	return labels
}
