package agent

import (
	"context"
	"fmt"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"

	"github.com/kvsukharev/go-musthave-metrics-tpl/internal/model"
	pb "github.com/kvsukharev/go-musthave-metrics-tpl/internal/proto"
)

// GRPCSender sends metrics to the server over gRPC.
type GRPCSender struct {
	client pb.MetricsClient
	conn   *grpc.ClientConn
}

// NewGRPCSender dials addr (e.g. "localhost:3200") and returns a GRPCSender.
// The caller must call Close() when done.
func NewGRPCSender(addr string) (*GRPCSender, error) {
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("dial grpc %s: %w", addr, err)
	}
	return &GRPCSender{
		client: pb.NewMetricsClient(conn),
		conn:   conn,
	}, nil
}

// Close releases the underlying connection.
func (g *GRPCSender) Close() error {
	return g.conn.Close()
}

// SendBatch sends metrics as a single UpdateMetrics RPC call.
// It attaches the agent's outbound IP as x-real-ip metadata.
func (g *GRPCSender) SendBatch(ctx context.Context, metrics []model.Metrics) error {
	if len(metrics) == 0 {
		return nil
	}
	pbMetrics := make([]*pb.Metric, 0, len(metrics))
	for _, m := range metrics {
		pbMetrics = append(pbMetrics, modelToProto(m))
	}

	md := metadata.Pairs()
	if ip := outboundIP(); ip != "" {
		md.Set("x-real-ip", ip)
	}
	ctx = metadata.NewOutgoingContext(ctx, md)

	_, err := g.client.UpdateMetrics(ctx, &pb.UpdateMetricsRequest{Metrics: pbMetrics})
	return err
}

func modelToProto(m model.Metrics) *pb.Metric {
	out := &pb.Metric{Id: m.ID}
	switch m.MType {
	case model.TypeCounter:
		out.Type = pb.Metric_COUNTER
		if m.Delta != nil {
			out.Delta = *m.Delta
		}
	default:
		out.Type = pb.Metric_GAUGE
		if m.Value != nil {
			out.Value = *m.Value
		}
	}
	return out
}
