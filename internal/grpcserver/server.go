// Package grpcserver implements the gRPC Metrics service.
package grpcserver

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/kvsukharev/go-musthave-metrics-tpl/internal/model"
	pb "github.com/kvsukharev/go-musthave-metrics-tpl/internal/proto"
	"github.com/kvsukharev/go-musthave-metrics-tpl/internal/storage"
)

// MetricsServer implements pb.MetricsServer.
type MetricsServer struct {
	pb.UnimplementedMetricsServer
	store storage.Storage
}

// New creates a MetricsServer backed by store.
func New(store storage.Storage) *MetricsServer {
	return &MetricsServer{store: store}
}

// UpdateMetrics receives a batch of metrics and persists them.
func (s *MetricsServer) UpdateMetrics(ctx context.Context, req *pb.UpdateMetricsRequest) (*pb.UpdateMetricsResponse, error) {
	metrics := make([]model.Metrics, 0, len(req.Metrics))
	for _, m := range req.Metrics {
		metrics = append(metrics, protoToModel(m))
	}
	if err := s.store.BatchUpdate(ctx, metrics); err != nil {
		return nil, status.Errorf(codes.Internal, "batch update: %v", err)
	}
	return &pb.UpdateMetricsResponse{}, nil
}

func protoToModel(m *pb.Metric) model.Metrics {
	out := model.Metrics{ID: m.Id}
	switch m.Type {
	case pb.Metric_COUNTER:
		out.MType = model.TypeCounter
		delta := m.Delta
		out.Delta = &delta
	default:
		out.MType = model.TypeGauge
		value := m.Value
		out.Value = &value
	}
	return out
}
