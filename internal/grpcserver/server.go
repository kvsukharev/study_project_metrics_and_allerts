// Package grpcserver implements the gRPC Metrics service.
package grpcserver

import (
	"context"
	"fmt"

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
		converted, err := protoToModel(m)
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "metric %q: %v", m.Id, err)
		}
		metrics = append(metrics, converted)
	}
	if err := s.store.BatchUpdate(ctx, metrics); err != nil {
		return nil, status.Errorf(codes.Internal, "batch update: %v", err)
	}
	return &pb.UpdateMetricsResponse{}, nil
}

func protoToModel(m *pb.Metric) (model.Metrics, error) {
	if m.Id == "" {
		return model.Metrics{}, fmt.Errorf("id must not be empty")
	}
	out := model.Metrics{ID: m.Id}
	switch m.Type {
	case pb.Metric_GAUGE:
		out.MType = model.TypeGauge
		value := m.Value
		out.Value = &value
	case pb.Metric_COUNTER:
		out.MType = model.TypeCounter
		delta := m.Delta
		out.Delta = &delta
	default:
		return model.Metrics{}, fmt.Errorf("unknown metric type %v", m.Type)
	}
	return out, nil
}
