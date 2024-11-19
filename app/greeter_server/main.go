package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net"
	"net/http"
	"time"

	pb "github.com/salrashid123/grpc_stats_envoy_istio/app/helloworld"

	"github.com/google/uuid"

	// *********** Start gRPC built in
	grpc_prometheus "github.com/grpc-ecosystem/go-grpc-prometheus"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	// *********** End gRPC built in

	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"
	// *********** Start OpenCensus
	// full sample at https://gist.github.com/salrashid123/bf0dbf3a979273f9baa475644d5aea01
	// "contrib.go.opencensus.io/exporter/prometheus"
	// "go.opencensus.io/plugin/ocgrpc"
	// "go.opencensus.io/stats/view"
	// *********** End Opencensus
)

var (
	grpcport     = flag.String("grpcport", ":50051", "grpcport")
	randomJitter = flag.Int("randomJitter", 100, "host:port of gRPC server")

	tlsCert = flag.String("tlsCert", "grpc.crt", "TLS Server Certificate")
	tlsKey  = flag.String("tlsKey", "grpc.key", "TLS Server Key")
	usetls  = flag.Bool("usetls", false, "startup with TLS")

	hs *health.Server

	// *********** Start gRPC built in
	reg                     = prometheus.NewRegistry()
	grpcMetrics             = grpc_prometheus.NewServerMetrics()
	customizedCounterMetric = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "demo_server_say_hello_method_handle_count",
		Help: "Total number of RPCs handled on the server.",
	}, []string{"name"})
	// *********** End gRPC built in
)

// server is used to implement helloworld.GreeterServer.
type server struct{}
type healthServer struct{}

func init() {
	// *********** Start gRPC built in
	reg.MustRegister(grpcMetrics, customizedCounterMetric)
	customizedCounterMetric.WithLabelValues("Test")
	// *********** End gRPC built in
}

func (s *healthServer) Check(ctx context.Context, in *healthpb.HealthCheckRequest) (*healthpb.HealthCheckResponse, error) {
	log.Printf("Handling grpc Check request: " + in.Service)
	if in.Service == "helloworld.Greeter" {
		return &healthpb.HealthCheckResponse{Status: healthpb.HealthCheckResponse_SERVING}, nil
	}
	return &healthpb.HealthCheckResponse{Status: healthpb.HealthCheckResponse_UNKNOWN}, nil
}

func (s *healthServer) Watch(in *healthpb.HealthCheckRequest, srv healthpb.Health_WatchServer) error {
	return status.Error(codes.Unimplemented, "Watch is not implemented")
}

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

	var s *grpc.Server
	sopts := []grpc.ServerOption{grpc.MaxConcurrentStreams(10)}

	if *usetls {
		ce, err := credentials.NewServerTLSFromFile(*tlsCert, *tlsKey)
		if err != nil {
			log.Fatalf("Failed to generate credentials %v", err)
		}
		sopts = append(sopts, grpc.Creds(ce))

	}

	// *********** Start OpenCensus
	// pe, err := prometheus.NewExporter(prometheus.Options{
	// 	Namespace: "oc",
	// })
	// if err != nil {
	// 	log.Fatalf("Failed to create Prometheus exporter: %v", err)
	// }

	// go func() {
	// 	mux := http.NewServeMux()
	// 	mux.Handle("/metrics", pe)
	// 	if err := http.ListenAndServe(":9092", mux); err != nil {
	// 		log.Fatalf("Failed to run Prometheus /metrics endpoint: %v", err)
	// 	}
	// }()

	// if err := view.Register(ocgrpc.DefaultServerViews...); err != nil {
	// 	log.Fatal(err)
	// }
	// sopts = append(sopts, grpc.StatsHandler(&ocgrpc.ServerHandler{}))
	// s = grpc.NewServer(sopts...)
	// // *********** End Opencensus

	// *********** Start Direct
	// Use gRPC-go internal prom exporter
	httpServer := &http.Server{Handler: promhttp.HandlerFor(reg, promhttp.HandlerOpts{}), Addr: fmt.Sprintf("0.0.0.0:%d", 9092)}
	sopts = append(sopts, grpc.StreamInterceptor(grpcMetrics.StreamServerInterceptor()), grpc.UnaryInterceptor(grpcMetrics.UnaryServerInterceptor()))

	s = grpc.NewServer(sopts...)
	grpcMetrics.InitializeMetrics(s)
	go func() {
		if err := httpServer.ListenAndServe(); err != nil {
			log.Fatal("Unable to start a http server.")
		}
	}()
	// *********** End Direct

	pb.RegisterGreeterServer(s, &server{})

	healthpb.RegisterHealthServer(s, &healthServer{})

	log.Printf("Starting server...")
	if err := s.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
