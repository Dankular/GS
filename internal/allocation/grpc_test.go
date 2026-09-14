package allocation

import (
	"context"
	"testing"

	pb "agones.dev/agones/pkg/allocation/go"
	"google.golang.org/grpc"
)

type fakeAgonesClient struct {
	request  *pb.AllocationRequest
	response *pb.AllocationResponse
	err      error
}

func (f *fakeAgonesClient) Allocate(_ context.Context, request *pb.AllocationRequest, _ ...grpc.CallOption) (*pb.AllocationResponse, error) {
	f.request = request
	return f.response, f.err
}

func TestGRPCAllocatorBuildsRequiredSelectorAndMapsResponse(t *testing.T) {
	fake := &fakeAgonesClient{response: &pb.AllocationResponse{GameServerName: "gs-1", Address: "10.0.0.1", Ports: []*pb.AllocationResponse_GameServerStatusPort{{Name: "game", Port: 7000}}}}
	allocator, err := NewGRPCAllocatorWithClient(fake, "platform-gameservers-eu-west")
	if err != nil {
		t.Fatal(err)
	}
	result, err := allocator.Allocate(context.Background(), Selector{GameID: "arena", ModeID: "dm", Build: "sha256:build", Region: "eu-west", Protocol: "3", AllocationID: "allocation-1", Metadata: map[string]string{"gameservice.io/match-id": "match-1"}})
	if err != nil {
		t.Fatal(err)
	}
	labels := fake.request.GetGameServerSelectors()[0].GetMatchLabels()
	if fake.request.GetNamespace() != "platform-gameservers-eu-west" || labels["platform.game/build"] != "sha256:build" {
		t.Fatalf("unexpected request: %#v", fake.request)
	}
	if fake.request.GetMetadata().GetAnnotations()["gameservice.io/match-id"] != "match-1" {
		t.Fatalf("allocation metadata was not sent: %#v", fake.request.GetMetadata())
	}
	if result.GameServer != "gs-1" || result.Ports["game"] != 7000 || result.AllocationID != "allocation-1" {
		t.Fatalf("unexpected result: %#v", result)
	}
}
