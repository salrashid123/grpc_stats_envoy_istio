package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net"
	"time"

	pb "helloworld"

	"github.com/google/uuid"
	//"github.com/prometheus/client_golang/prometheus"
	//"github.com/prometheus/client_golang/prometheus/promhttp"
	"golang.org/x/net/context"
	"google.golang.org/grpc"

	//grpc_prometheus "github.com/grpc-ecosystem/go-grpc-prometheus"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"
)

var (
	grpcport     = flag.String("grpcport", ":50051", "grpcport")
	randomJitter = flag.Int("randomJitter", 100, "host:port of gRPC server")
	hs           *health.Server

	// uncomment for direct metrics
	// reg = prometheus.NewRegistry()
	// grpcMetrics             = grpc_prometheus.NewServerMetrics()
	// customizedCounterMetric = prometheus.NewCounterVec(prometheus.CounterOpts{
	// 	Name: "demo_server_say_hello_method_handle_count",
	// 	Help: "Total number of RPCs handled on the server.",
	// }, []string{"name"})
)

// server is used to implement helloworld.GreeterServer.
type server struct{}
type healthServer struct{}

func init() {
	// uncomment for direct metrics
	// reg.MustRegister(grpcMetrics, customizedCounterMetric)
	// customizedCounterMetric.WithLabelValues("Test")
}

func (s *healthServer) Check(ctx context.Context, in *healthpb.HealthCheckRequest) (*healthpb.HealthCheckResponse, error) {
	log.Printf("Handling grpc Check request: " + in.Service)
	return &healthpb.HealthCheckResponse{Status: healthpb.HealthCheckResponse_SERVING}, nil
}

func (s *healthServer) Watch(in *healthpb.HealthCheckRequest, srv healthpb.Health_WatchServer) error {
	return status.Error(codes.Unimplemented, "Watch is not implemented")
}

// SayHello implements helloworld.GreeterServer
func (s *server) SayHello(ctx context.Context, in *pb.HelloRequest) (*pb.HelloReply, error) {
	uid, _ := uuid.NewUUID()
	msg := fmt.Sprintf("Hello %s  --> %s ", in.Name, uid.String())

	time.Sleep(time.Duration(rand.Intn(*randomJitter)) * time.Millisecond)
	return &pb.HelloReply{Message: msg}, nil
}

func (s *server) SayHelloServerStream(in *pb.HelloRequest, stream pb.Greeter_SayHelloServerStreamServer) error {
	log.Println("Got SayHelloServerStream: Request ")
	for i := 0; i < 5; i++ {
		time.Sleep(time.Duration(rand.Intn(*randomJitter)) * time.Millisecond)
		stream.Send(&pb.HelloReply{Message: "SayHelloServerStream Response"})
	}
	return nil
}

func (s server) SayHelloBiDiStream(srv pb.Greeter_SayHelloBiDiStreamServer) error {
	ctx := srv.Context()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		req, err := srv.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			log.Printf("receive error %v", err)
			continue
		}
		log.Printf("Got SayHelloBiDiStream %s", req.Name)
		resp := &pb.HelloReply{Message: "SayHelloBiDiStream Server Response"}
		time.Sleep(time.Duration(rand.Intn(*randomJitter)) * time.Millisecond)
		if err := srv.Send(resp); err != nil {
			log.Printf("send error %v", err)
		}
	}
}

func (s server) SayHelloClientStream(stream pb.Greeter_SayHelloClientStreamServer) error {
	for {
		req, err := stream.Recv()
		if err == io.EOF {
			time.Sleep(time.Duration(rand.Intn(*randomJitter)) * time.Millisecond)
			return stream.SendAndClose(&pb.HelloReply{Message: "SayHelloClientStream  Response"})
		}
		if err != nil {
			return err
		}
		log.Printf("Got SayHelloClientStream Request: %s", req.Name)
	}
}

func main() {
	flag.Parse()
	lis, err := net.Listen("tcp", *grpcport)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	// uncomment for direct metrics
	// httpServer := &http.Server{Handler: promhttp.HandlerFor(reg, promhttp.HandlerOpts{}), Addr: fmt.Sprintf("0.0.0.0:%d", 9092)}
	// s := grpc.NewServer(
	// 	grpc.StreamInterceptor(grpcMetrics.StreamServerInterceptor()),
	// 	grpc.UnaryInterceptor(grpcMetrics.UnaryServerInterceptor()),
	// )

	s := grpc.NewServer()
	pb.RegisterGreeterServer(s, &server{})

	healthpb.RegisterHealthServer(s, &healthServer{})

	// uncomment for direct metrics
	// grpcMetrics.InitializeMetrics(s)
	// go func() {
	// 	if err := httpServer.ListenAndServe(); err != nil {
	// 		log.Fatal("Unable to start a http server.")
	// 	}
	// }()

	log.Printf("Starting server...")
	if err := s.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
