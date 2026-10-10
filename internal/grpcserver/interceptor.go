package grpcserver

import (
	"context"
	"net"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// TrustedSubnetInterceptor returns a gRPC UnaryServerInterceptor that checks
// whether the x-real-ip metadata value belongs to subnet.
// If subnet is nil the interceptor is a no-op pass-through.
// Returns codes.PermissionDenied when the IP is absent or outside the subnet.
func TrustedSubnetInterceptor(subnet *net.IPNet) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if subnet == nil {
			return handler(ctx, req)
		}
		md, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			return nil, status.Errorf(codes.PermissionDenied, "permission denied")
		}
		vals := md.Get("x-real-ip")
		if len(vals) == 0 {
			return nil, status.Errorf(codes.PermissionDenied, "permission denied")
		}
		ip := net.ParseIP(vals[0])
		if ip == nil || !subnet.Contains(ip) {
			return nil, status.Errorf(codes.PermissionDenied, "permission denied")
		}
		return handler(ctx, req)
	}
}
