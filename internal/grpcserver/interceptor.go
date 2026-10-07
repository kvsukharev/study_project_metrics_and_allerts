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
// whether the x-real-ip metadata value belongs to the given CIDR subnet.
// If cidr is empty the interceptor is a no-op pass-through.
// Returns codes.PermissionDenied when the IP is absent or outside the subnet.
func TrustedSubnetInterceptor(cidr string) grpc.UnaryServerInterceptor {
	if cidr == "" {
		return func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
			return handler(ctx, req)
		}
	}

	_, subnet, err := net.ParseCIDR(cidr)
	if err != nil {
		// Invalid CIDR — deny all requests to fail safely.
		return func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
			return nil, status.Errorf(codes.PermissionDenied, "permission denied")
		}
	}

	return func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
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
