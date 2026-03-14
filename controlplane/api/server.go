package api

import (
	"context"
	"fmt"
	"net"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/AnantSingh1510/agentd/controlplane/api/proto"
	"github.com/AnantSingh1510/agentd/divergence"
	"github.com/AnantSingh1510/agentd/kernel"
	"github.com/AnantSingh1510/agentd/negotiation"
)

type Server struct {
	proto.UnimplementedAgentDServer
	k          *kernel.Kernel
	negotiator *negotiation.Negotiator
	divergence *divergence.Store
	grpcServer *grpc.Server
}

func New(k *kernel.Kernel, neg *negotiation.Negotiator) *Server {
	div, err := divergence.New("localhost:6379")
	if err != nil {
		div = nil // divergence store is optional server still runs without it
	}
	return &Server{
		k:          k,
		negotiator: neg,
		divergence: div,
		grpcServer: grpc.NewServer(),
	}
}

func (s *Server) Listen(addr string) error {
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", addr, err)
	}
	proto.RegisterAgentDServer(s.grpcServer, s)
	return s.grpcServer.Serve(lis)
}

func (s *Server) Stop() {
	s.grpcServer.GracefulStop()
}

func (s *Server) SubmitTask(ctx context.Context, req *proto.SubmitTaskRequest) (*proto.SubmitTaskResponse, error) {
	if len(req.Payload) == 0 {
		return nil, status.Error(codes.InvalidArgument, "payload cannot be empty")
	}
	task, err := s.k.SubmitTask(req.Payload)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to submit task: %v", err)
	}
	return &proto.SubmitTaskResponse{
		TaskId:   task.ID,
		Status:   string(task.Status),
		WorkerId: task.WorkerID,
	}, nil
}

func (s *Server) GetTask(ctx context.Context, req *proto.GetTaskRequest) (*proto.GetTaskResponse, error) {
	if req.TaskId == "" {
		return nil, status.Error(codes.InvalidArgument, "task_id cannot be empty")
	}
	task, err := s.k.Scheduler.GetTask(req.TaskId)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "task not found: %v", err)
	}
	return &proto.GetTaskResponse{
		TaskId:   task.ID,
		Status:   string(task.Status),
		WorkerId: task.WorkerID,
	}, nil
}

func (s *Server) ListWorkers(ctx context.Context, req *proto.ListWorkersRequest) (*proto.ListWorkersResponse, error) {
	workers := s.k.Scheduler.ListWorkers()
	infos := make([]*proto.WorkerInfo, 0, len(workers))
	for _, w := range workers {
		infos = append(infos, &proto.WorkerInfo{
			Id:       w.ID,
			Capacity: int32(w.Capacity),
			Active:   int32(w.Active),
		})
	}
	return &proto.ListWorkersResponse{Workers: infos}, nil
}

func (s *Server) RegisterWorker(ctx context.Context, req *proto.RegisterWorkerRequest) (*proto.RegisterWorkerResponse, error) {
	if req.Id == "" {
		return nil, status.Error(codes.InvalidArgument, "worker id cannot be empty")
	}
	if req.Capacity <= 0 {
		return nil, status.Error(codes.InvalidArgument, "capacity must be greater than zero")
	}
	err := s.k.Scheduler.RegisterWorker(req.Id, int(req.Capacity))
	if err != nil {
		return nil, status.Errorf(codes.AlreadyExists, "worker registration failed: %v", err)
	}
	return &proto.RegisterWorkerResponse{Success: true}, nil
}

func (s *Server) RunNegotiation(ctx context.Context, req *proto.NegotiationRequest) (*proto.NegotiationResponse, error) {
	if s.negotiator == nil {
		return nil, status.Error(codes.Unavailable, "negotiator not configured")
	}
	if len(req.Prompt) == 0 {
		return nil, status.Error(codes.InvalidArgument, "prompt cannot be empty")
	}

	round, err := s.negotiator.Run(ctx, req.TaskId, req.Prompt)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "negotiation failed: %v", err)
	}

	if s.divergence != nil && round.Verdict != nil {
		if saveErr := s.divergence.SaveRound(ctx, round); saveErr != nil {
			_ = saveErr
		}
	}

	dissents := make([]*proto.DissentInfo, 0, len(round.Verdict.Dissents))
	for _, d := range round.Verdict.Dissents {
		dissents = append(dissents, &proto.DissentInfo{
			AgentId:   d.AgentID,
			ModelName: d.ModelName,
			Position:  d.Position,
		})
	}

	return &proto.NegotiationResponse{
		RoundId:     round.ID,
		FinalAnswer: round.Verdict.FinalAnswer,
		Reasoning:   round.Verdict.Reasoning,
		Dissents:    dissents,
	}, nil
}
