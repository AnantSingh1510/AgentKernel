package api

import (
	"context"
	"net"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/AnantSingh1510/agentd/controlplane/api/proto"
	"github.com/AnantSingh1510/agentd/kernel"
	"github.com/AnantSingh1510/agentd/negotiation"
)

func fakeHandler(answer string) negotiation.ModelHandler {
	return func(ctx context.Context, prompt []byte) ([]byte, string, error) {
		return []byte(answer), "because " + answer, nil
	}
}

func setupServer(t *testing.T) (proto.AgentDClient, func()) {
	t.Helper()

	k, err := kernel.New(kernel.DefaultConfig())
	if err != nil {
		t.Fatalf("kernel init failed: %v", err)
	}

	participants := []*negotiation.Participant{
		{AgentID: "a1", ModelName: "m1", Role: negotiation.RoleProposer,
			Handler: fakeHandler("42")},
		{AgentID: "a2", ModelName: "m2", Role: negotiation.RoleChallenger,
			Handler: fakeHandler("42")},
		{AgentID: "a3", ModelName: "m3", Role: negotiation.RoleArbitrator,
			Handler: fakeHandler("42")},
	}

	neg, err := negotiation.New(k, participants)
	if err != nil {
		t.Fatalf("negotiator init failed: %v", err)
	}

	srv := New(k, neg)

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}

	proto.RegisterAgentDServer(srv.grpcServer, srv)
	go srv.grpcServer.Serve(lis)

	conn, err := grpc.NewClient(lis.Addr().String(),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}

	client := proto.NewAgentDClient(conn)

	cleanup := func() {
		conn.Close()
		srv.Stop()
		k.Shutdown()
	}

	return client, cleanup
}

func TestSubmitTask(t *testing.T) {
	client, cleanup := setupServer(t)
	defer cleanup()

	resp, err := client.SubmitTask(context.Background(), &proto.SubmitTaskRequest{
		Payload: []byte("test task"),
	})
	if err != nil {
		t.Fatalf("SubmitTask failed: %v", err)
	}
	if resp.TaskId == "" {
		t.Fatal("expected non-empty task ID")
	}
}

func TestSubmitTaskEmptyPayload(t *testing.T) {
	client, cleanup := setupServer(t)
	defer cleanup()

	_, err := client.SubmitTask(context.Background(), &proto.SubmitTaskRequest{
		Payload: []byte{},
	})
	if err == nil {
		t.Fatal("expected error for empty payload")
	}
}

func TestRegisterAndListWorkers(t *testing.T) {
	client, cleanup := setupServer(t)
	defer cleanup()

	_, err := client.RegisterWorker(context.Background(), &proto.RegisterWorkerRequest{
		Id:       "w-test",
		Capacity: 3,
	})
	if err != nil {
		t.Fatalf("RegisterWorker failed: %v", err)
	}

	resp, err := client.ListWorkers(context.Background(), &proto.ListWorkersRequest{})
	if err != nil {
		t.Fatalf("ListWorkers failed: %v", err)
	}

	found := false
	for _, w := range resp.Workers {
		if w.Id == "w-test" && w.Capacity == 3 {
			found = true
		}
	}
	if !found {
		t.Fatal("registered worker not found in list")
	}
}

func TestGetTask(t *testing.T) {
	client, cleanup := setupServer(t)
	defer cleanup()

	submit, _ := client.SubmitTask(context.Background(), &proto.SubmitTaskRequest{
		Payload: []byte("get me"),
	})

	resp, err := client.GetTask(context.Background(), &proto.GetTaskRequest{
		TaskId: submit.TaskId,
	})
	if err != nil {
		t.Fatalf("GetTask failed: %v", err)
	}
	if resp.TaskId != submit.TaskId {
		t.Fatalf("expected %s, got %s", submit.TaskId, resp.TaskId)
	}
}

func TestGetTaskNotFound(t *testing.T) {
	client, cleanup := setupServer(t)
	defer cleanup()

	_, err := client.GetTask(context.Background(), &proto.GetTaskRequest{
		TaskId: "nonexistent",
	})
	if err == nil {
		t.Fatal("expected error for nonexistent task")
	}
}

func TestRunNegotiation(t *testing.T) {
	client, cleanup := setupServer(t)
	defer cleanup()

	resp, err := client.RunNegotiation(context.Background(), &proto.NegotiationRequest{
		TaskId: "task-neg-1",
		Prompt: []byte("What is 6x7?"),
	})
	if err != nil {
		t.Fatalf("RunNegotiation failed: %v", err)
	}
	if resp.RoundId == "" {
		t.Fatal("expected non-empty round ID")
	}
	if string(resp.FinalAnswer) != "42" {
		t.Fatalf("expected '42', got '%s'", resp.FinalAnswer)
	}
}