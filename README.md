
## gRPC per method observability with envoy, Istio, OpenCensus and GKE


This repo is a simple tutorial on how to setup envoy and istio such that _per method_ statistics are emitted to prometheus+grafana.

There are several techniques to emitting and collecting gRPC stats shown here:

* `client`-> `envoy` -> `server`:  Envoy will surface prometheus stats endpoint
* `client`-> `istio(envoy)` -> `server`:  Istio (envoy) will again surface a prometheus endpoint
* `client` (native prometheus gRPC):  Use gRPC built in prometheus stats endpoint `"github.com/grpc-ecosystem/go-grpc-prometheus"`
* `client` (opencensus prometheus): Use Opencensus's Prometheus exporter for gRPC `"contrib.go.opencensus.io/exporter/prometheus"`
* `client`-> `kubernetes ingress/service` -> `server`:  Deploy a GKE gRPC service and allow a prometheus server to scrape each gRPC server eposing `"github.com/grpc-ecosystem/go-grpc-prometheus"`

Both istio and envoy have robust metric monitoring capability but the default granularity for these are at the [service level](https://istio.io/latest/docs/concepts/observability/#service-level-metrics).  If you needed resolution at the method you would need to some further configuration settings if using envoy or istio:

- [Envoy gRPC Statistics filter](https://www.envoyproxy.io/docs/envoy/latest/configuration/http/http_filters/grpc_stats_filter) (`envoy.filters.http.grpc_stats`)
- [Istio wasm telemetry](https://istio.io/latest/docs/reference/config/proxy_extensions/wasm_telemetry/)


This repo walks you through a sample setup for both standalone envoy and istio where the count of a specific method call (`/helloworld.Greeter/SayHello`) is emitted and collected via prometheus and subsequently rendered in grafana.

You can either test this with envoy or istio but the mechanism is a bit different

## Envoy

With envoy, we will use the built in `envoy.filters.http.grpc_stats` filter.

### Start Envoy 

First get envoy.  We are using a pretty recent version (as of end of 2020).

```bash
docker cp `docker create envoyproxy/envoy-dev:latest`:/usr/local/bin/envoy .
```

start envoy

```bash
cd envoy/
./envoy -c server.yaml
```

Notice we configured the filter to emit everything:

```yaml
          http_filters:
          - name: envoy.filters.http.grpc_stats
            typed_config:
              "@type": type.googleapis.com/envoy.extensions.filters.http.grpc_stats.v3.FilterConfig
              stats_for_all_methods: true
              enable_upstream_stats: true                            
          - name: envoy.filters.http.router
            typed_config:
              "@type": type.googleapis.com/envoy.extensions.filters.http.router.v3.Router                  
```

### Start GRPC Server

Now start the gRPC server

```bash
cd app/
go run greeter_server/main.go
```

The server will by default listen in on `:50051`


## Start Prometheus

Start a local instance of prometheus that interrogates envoy:

```bash
cd envoy/
docker run \
    --net=host \
    -p 9090:9090 \
    -v `pwd`/prometheus/prometheus.yml:/etc/prometheus/prometheus.yml \
    prom/prometheus
```

Ensure Prometheus Target (i.e, envoy) are up:  [http://localhost:9090/targets](http://localhost:9090/targets)

![images/prom_targets.png](images/prom_targets.png)

then verify envoy is emitting stats:

![images/prom_hc.png](images/prom_hc.png)

### Start Grafana

Start grafana 

```bash
docker run --net=host -p 3000:3000 grafana/grafana 
```

Configure grafana for a [prometheus datasource](https://prometheus.io/docs/visualization/grafana/#creating-a-prometheus-data-source)

```
 http://localhost:3000  admin/admin
 configuration=>datasources
   Select `Prometheus`
    Name: `envoy`
    URL: `http://localhost:9090/`
```

![images/grafana_ds.png](images/grafana_ds.png)


### Grpc Client

Run gRPC client. 

```bash
go run greeter_client/main.go  --host localhost:8080
```

The gRPC client will make several different types of API calls:

-  `rpc SayHello (HelloRequest) returns (HelloReply) {}`   Unary
-  `rpc SayHelloClientStream(stream HelloRequest) returns (HelloReply) {}` Client Stream
-  `rpc SayHelloServerStream(HelloRequest) returns (stream HelloReply) {}` Server Stream   
-  `rpc SayHelloBiDiStream(stream HelloRequest) returns (stream HelloReply) {}` Bidirectional Stream


(you can put this command in a loop,eg 6 times)

### Observe

Prometheus will begin collecting all the metrics from envoy at this point.  The specific one we are interested in is just `SayHello` API method call.

#### Envoy

You can view the prometheus endpoint directly on envoy and see the stats:
Envoy:  [http://localhost:9000/stats/prometheus](http://localhost:9000/stats/prometheus)

`envoy_cluster_grpc_Greeter_0{envoy_grpc_bridge_method="SayHello",envoy_grpc_bridge_service="helloworld",envoy_cluster_name="service_grpc"}`


#### Prometheus

Similarly, on the Prometheus dashboard, see the gRPC Statistics (eg, number `SayHello` invocations)

In our case, if you invoked the client 6 times, you'll see that count:

```log
# TYPE envoy_cluster_grpc_Greeter_0 counter
envoy_cluster_grpc_Greeter_0{envoy_grpc_bridge_method="SayHello",envoy_grpc_bridge_service="helloworld",envoy_cluster_name="service_grpc"} 6
envoy_cluster_grpc_Greeter_0{envoy_grpc_bridge_method="SayHelloBiDiStream",envoy_grpc_bridge_service="helloworld",envoy_cluster_name="service_grpc"} 6
envoy_cluster_grpc_Greeter_0{envoy_grpc_bridge_method="SayHelloClientStream",envoy_grpc_bridge_service="helloworld",envoy_cluster_name="service_grpc"} 6
envoy_cluster_grpc_Greeter_0{envoy_grpc_bridge_method="SayHelloServerStream",envoy_grpc_bridge_service="helloworld",envoy_cluster_name="service_grpc"} 6
```

#### Grafana

Finally, create a dashboard for the invocation counts

Use the same metric as above (i.,e invocation count)

![images/grafana_dash_1.png](images/grafana_dash_1.png)


### Istio

The istio configuration is a bit different.  It uses a specific wasm stats filter as described here

- [Classifying Metrics Based on Request or Response](https://istio.io/latest/docs/tasks/observability/metrics/classify-metrics/#classify-metrics-by-request)


#### Create cluster and install istio

Create an istio cluster (we are using minikube)

```bash
cd istio/
minikube start --driver=kvm2  --cpus=4 --kubernetes-version=v1.28 --host-only-cidr 192.168.39.1/24
## setup a loadbalancer metallb, enter the ip range shown below
minikube addons configure metallb
# -- Enter Load Balancer Start IP: 192.168.39.104
# -- Enter Load Balancer End IP: 192.168.39.110

### in a new window
minikube dashboard

kubectl create ns istio-system

export ISTIO_VERSION=1.24.0
export ISTIO_VERSION_MINOR=1.24

wget -P /tmp/ https://github.com/istio/istio/releases/download/$ISTIO_VERSION/istio-$ISTIO_VERSION-linux-amd64.tar.gz
tar xvf /tmp/istio-$ISTIO_VERSION-linux-amd64.tar.gz -C /tmp/
rm /tmp/istio-$ISTIO_VERSION-linux-amd64.tar.gz

export PATH=/tmp/istio-$ISTIO_VERSION/bin:$PATH

### install istio with EnvoyFilter  https://istio.io/v1.21/docs/tasks/observability/metrics/telemetry-api/
#### TODO: see how TelemetryAPI works...

istioctl install --set profile=demo \
 --set values.gateways.istio-ingressgateway.runAsRoot=true  \
 -f istio-operator.yaml


# --set meshConfig.defaultProviders.metrics=EnvoyFilter

# install prometheus, grafana, kaili and jaeger for good measure
kubectl apply -f https://raw.githubusercontent.com/istio/istio/release-$ISTIO_VERSION_MINOR/samples/addons/prometheus.yaml
sleep 20
kubectl apply -f https://raw.githubusercontent.com/istio/istio/release-$ISTIO_VERSION_MINOR/samples/addons/kiali.yaml
kubectl apply -f https://raw.githubusercontent.com/istio/istio/release-$ISTIO_VERSION_MINOR/samples/addons/grafana.yaml
kubectl apply -f https://raw.githubusercontent.com/istio/istio/release-$ISTIO_VERSION_MINOR/samples/addons/jaeger.yaml

kubectl label namespace default istio-injection=enabled

kubectl get no,po,rc,svc,ing,deployment -n istio-system 
```

(wait a couple of mins)

- Open up several new shell windows and type in one line into each:

```bash
kubectl -n istio-system port-forward $(kubectl -n istio-system get pod -l app.kubernetes.io/name=grafana -o jsonpath='{.items[0].metadata.name}') 3000:3000

kubectl port-forward -n istio-system $(kubectl get pod -n istio-system -l app.kubernetes.io/name=prometheus -o jsonpath='{.items[0].metadata.name}') 9090:9090

kubectl -n istio-system port-forward $(kubectl -n istio-system get pod -l app.kubernetes.io/name=kiali -o jsonpath='{.items[0].metadata.name}') 20001:20001
```

Open up browser windows to each

Open up a browsers tabs and go to:

- `Grafana`: [http://localhost:3000/dashboard/db/istio-mesh-dashboard](http://localhost:3000/dashboard/db/istio-mesh-dashboard)
- `Prometheus`: [http://localhost:9090/targets](http://localhost:9090/targets)


- Find the gatewayIP

```bash
$ kubectl get svc istio-ingressgateway -n istio-system

export GATEWAY_IP=$(kubectl -n istio-system get service istio-ingressgateway -o jsonpath='{.status.loadBalancer.ingress[0].ip}')
echo $GATEWAY_IP
```

#### Deploy gRPC Server 

```bash
kubectl apply -f istio-deployment.yaml -f istio-svc1.yaml -f istio-ingress-gateway.yaml

## regen the gateway to pick up any certs (though this deployment doesn't include ingresscerts)
INGRESS_POD_NAME=$(kubectl get po -n istio-system | grep ingressgateway\- | awk '{print$1}'); echo ${INGRESS_POD_NAME};
kubectl delete po/$INGRESS_POD_NAME -n istio-system
```

(optional) If you want, enable wasm proxy debugging:

```bash
kubectl port-forward -n default deployment/svc1 15000

curl -XPOST "localhost:15000/logging?wasm=trace"
```

and look at the logs on the service itself

```bash
$ kubectl get po
NAME                    READY   STATUS    RESTARTS   AGE
svc1-7d64d798fb-j9xh7   2/2     Running   0          57s

$ kubectl logs svc1-7d64d798fb-j9xh7 -c istio-proxy
```

#### Test gRPC Client

Now test that the basic istio/grpc app is accessible before we enable stats:

```bash
cd app/

$ go run greeter_client/main.go --host $GATEWAY_IP:80
$ go run greeter_client/main.go --host $GATEWAY_IP:80
```


#### View Prometheus

You should now see the prometheus stats now under `istio_requests_total` that emits the label `request_operation` with value `SayHello` 

```log
istio_requests_total{app="svc1",connection_security_policy="mutual_tls",destination_app="svc1",destination_canonical_revision="latest",destination_canonical_service="svc1",destination_principal="spiffe://cluster.local/ns/default/sa/svc1-sa",destination_service="svc1.default.svc.cluster.local",destination_service_name="svc1",destination_service_namespace="default",destination_version="unknown",destination_workload="svc1",destination_workload_namespace="default",grpc_response_status="0",instance="10.0.1.6:15020",istio_io_rev="default",job="kubernetes-pods",kubernetes_namespace="default",kubernetes_pod_name="svc1-7d64d798fb-j9xh7",pod_template_hash="7d64d798fb",reporter="destination",request_operation="SayHello",request_protocol="grpc",response_code="200",response_flags="-",security_istio_io_tlsMode="istio",service_istio_io_canonical_name="svc1",service_istio_io_canonical_revision="latest",source_app="istio-ingressgateway",source_canonical_revision="latest",source_canonical_service="istio-ingressgateway",source_principal="spiffe://cluster.local/ns/istio-system/sa/istio-ingressgateway-service-account",source_version="unknown",source_workload="istio-ingressgateway",source_workload_namespace="istio-system"}
```

which you can see in prometheus console (note the count is 6 per the number of invocations)

![images/istio_prom.png](images/istio_prom.png)

#### View Grafana

The same stat on grafana too

![images/istio_grafana.png](images/istio_grafana.png)

---

## Metrics from gRPC Prometheus endpoint

You can also start a gRPC server which also serves as an endpoint surfacing prometheus metrics.  In this mode, the individual gRPC server starts up an http server which only serves to provide prometheus metrics.  This is described in the link here:

- [Go gRPC Interceptors for Prometheus monitoring](https://github.com/grpc-ecosystem/go-grpc-prometheus)

You can optionally enable this setting by uncommenting the lines in `greeter_server/main.go` 

```golang
import (
	grpc_prometheus "github.com/grpc-ecosystem/go-grpc-prometheus"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	// *********** Start gRPC built in
	reg                     = prometheus.NewRegistry()
	grpcMetrics             = grpc_prometheus.NewServerMetrics()
	customizedCounterMetric = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "demo_server_say_hello_method_handle_count",
		Help: "Total number of RPCs handled on the server.",
	}, []string{"name"})
	// *********** End gRPC built in
)

func init() {
	// *********** Start gRPC built in
	reg.MustRegister(grpcMetrics, customizedCounterMetric)
	customizedCounterMetric.WithLabelValues("Test")
	// *********** End gRPC built in
}

func main() {
  ...
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

```

and in `prometheus/prometheus.yml`:

```yaml
scrape_configs:
  - job_name: 'grpcserver'
    scrape_interval: 1s
    metrics_path: /metrics
    static_configs:
      - targets: ['localhost:9092']
```

```bash
cd app/
go run greeter_server/main.go
go run greeter_client/main.go  --host localhost:50051
```


The metrics would look like the following

```log
grpc_server_msg_received_total{grpc_method="Check",grpc_service="grpc.health.v1.Health",grpc_type="unary"} 8
grpc_server_msg_received_total{grpc_method="SayHello",grpc_service="helloworld.Greeter",grpc_type="unary"} 4
grpc_server_msg_received_total{grpc_method="SayHelloBiDiStream",grpc_service="helloworld.Greeter",grpc_type="bidi_stream"} 40
grpc_server_msg_received_total{grpc_method="SayHelloClientStream",grpc_service="helloworld.Greeter",grpc_type="client_stream"} 16
grpc_server_msg_received_total{grpc_method="SayHelloServerStream",grpc_service="helloworld.Greeter",grpc_type="server_stream"} 4
grpc_server_msg_received_total{grpc_method="Watch",grpc_service="grpc.health.v1.Health",grpc_type="server_stream"} 0
```
---

## Metrics using OpenCensus

Also included in this repo is sample code in how to emit gRPC stats using [OpenCensus](https://opencensus.io/).  Specifically, we will enable default gRPC stats and use OpenCensus's Prometheus Exporter.

You can optionally enable this setting by uncommenting the lines in `greeter_server/main.go` (alternatively replace entire `main.go` with [https://gist.github.com/salrashid123/bf0dbf3a979273f9baa475644d5aea01](https://gist.github.com/salrashid123/bf0dbf3a979273f9baa475644d5aea01)) as `docker.io/salrashid123/grpc_stats_envoy_istio:oc`

```golang
import (
	// *********** Start OpenCensus
	"contrib.go.opencensus.io/exporter/prometheus"
	"go.opencensus.io/plugin/ocgrpc"
	"go.opencensus.io/stats/view"
	// *********** End Opencensus
)

func main() {
  ...
	// *********** Start OpenCensus
	pe, err := prometheus.NewExporter(prometheus.Options{
		Namespace: "oc",
	})
	if err != nil {
		log.Fatalf("Failed to create Prometheus exporter: %v", err)
	}

	go func() {
		mux := http.NewServeMux()
		mux.Handle("/metrics", pe)
		if err := http.ListenAndServe(":9092", mux); err != nil {
			log.Fatalf("Failed to run Prometheus /metrics endpoint: %v", err)
		}
	}()

	if err := view.Register(ocgrpc.DefaultServerViews...); err != nil {
		log.Fatal(err)
	}
	sopts = append(sopts, grpc.StatsHandler(&ocgrpc.ServerHandler{}))
	s = grpc.NewServer(sopts...)
	// *********** End Opencensus
```

and in `prometheus/prometheus.yml`:

```yaml
scrape_configs:
  - job_name: 'grpcserver'
    scrape_interval: 1s
    metrics_path: /metrics
    static_configs:
      - targets: ['localhost:9092']
```

Then run the server and client. 

```bash
cd app/
go run greeter_server/main.go
go run greeter_client/main.go  --host localhost:50051
```

The prometheus stats should show up as the following:

```log
# HELP oc_grpc_io_server_completed_rpcs Count of RPCs by method and status.
# TYPE oc_grpc_io_server_completed_rpcs counter
oc_grpc_io_server_completed_rpcs{grpc_server_method="grpc.health.v1.Health/Check",grpc_server_status="OK"} 6
oc_grpc_io_server_completed_rpcs{grpc_server_method="helloworld.Greeter/SayHello",grpc_server_status="OK"} 6
oc_grpc_io_server_completed_rpcs{grpc_server_method="helloworld.Greeter/SayHelloBiDiStream",grpc_server_status="OK"} 6
oc_grpc_io_server_completed_rpcs{grpc_server_method="helloworld.Greeter/SayHelloClientStream",grpc_server_status="OK"} 6
oc_grpc_io_server_completed_rpcs{grpc_server_method="helloworld.Greeter/SayHelloServerStream",grpc_server_status="OK"} 6
```

This repo is just a tutorial on fine-grain observability with istio and envoy


## Metrics Endpoint

In this variation, we will create a  cluster into which we will deploy a gRPC server that exposes the prometheus scraping endpoint at `/metrics`.

A  prometheus server will use [kubernetes_sd_configs](https://prometheus.io/docs/prometheus/latest/configuration/configuration/#kubernetes_sd_config) to discover each gRPC service (deployed with `ClusterIP`) as targets to accumulate metrics from each service

The sample  gRPC ingress code is taken from [GKE gRPC Ingress LoadBalancing](https://github.com/salrashid123/gcegrpc/tree/master/gke_ingress_lb)

```bash
cd minikube/

minikube start --driver=kvm2  --cpus=4 --kubernetes-version=v1.28 --host-only-cidr 192.168.39.1/24
## setup a ingress and loadbalancer metallb, enter the ip range shown below
minikube addons enable ingress
minikube addons configure metallb
# -- Enter Load Balancer Start IP: 192.168.39.104
# -- Enter Load Balancer End IP: 192.168.39.110

### in a new window
minikube dashboard

kubectl apply -f fe-secret.yaml -f fe-server-secret.yaml
sleep 4

kubectl apply \
 -f fe-configmap.yaml \
 -f fe-deployment.yaml \
 -f fe-ingress.yaml \
 -f fe-srv-ingress.yaml \
 -f fe-srv-lb.yaml


$ kubectl get po,svc,ing,cm
NAME                                 READY   STATUS    RESTARTS   AGE
pod/fe-deployment-7bb47bddbb-wqp5t   2/2     Running   0          96s
pod/fe-deployment-7bb47bddbb-xpr7x   2/2     Running   0          96s

NAME                      TYPE           CLUSTER-IP       EXTERNAL-IP      PORT(S)           AGE
service/fe-srv-ingress    ClusterIP      10.105.56.161    <none>           50051/TCP         36s
service/fe-srv-lb         LoadBalancer   10.107.164.192   192.168.39.104   50051:30181/TCP   96s
service/kubernetes        ClusterIP      10.96.0.1        <none>           443/TCP           8m34s
service/rpc-app-service   ClusterIP      10.108.3.141     <none>           9092/TCP          96s

NAME                                   CLASS   HOSTS               ADDRESS          PORTS     AGE
ingress.networking.k8s.io/fe-ingress   nginx   server.domain.com   192.168.39.112   80, 443   96s

NAME                         DATA   AGE
configmap/kube-root-ca.crt   1      8m21s
configmap/settings           2      96s

export INGRESS_IP=`kubectl -n default get ingress.networking.k8s.io/fe-ingress -o jsonpath='{.status.loadBalancer.ingress[0].ip}'`
echo $INGRESS_IP

export SVC_IP=`kubectl -n default get svc/fe-srv-lb -o jsonpath='{.status.loadBalancer.ingress[0].ip}'`
echo $SVC_IP


# Test gRPC client server
## via Service
$ go run greeter_client/main.go --host $SVC_IP:50051 --usetls  --cacert certs/root-ca.crt --servername server.domain.com

## via Ingress
$ go run greeter_client/main.go --host $INGRESS_IP:443 --usetls -skipHealhCheck --cacert certs/root-ca.crt --servername server.domain.com

# now deploy prometheus 
kubectl apply -f prom-deployment.yaml
```

View the prometheus service endpoint

```bash
kubectl  port-forward service/prometheus-service  9090
```

Open a browser and goto: `http://localhost:9090/targets`

You should see the two gRPC application pods as targets (which means prometheus is scraping from each gRPC server)

![images/gke_prom_target.png](images/gke_prom_target.png)

as well as the stats:

![images/gke_grpc_msg.png](images/gke_grpc_msg.png)

```log
grpc_server_handled_total{grpc_code="OK", grpc_method="SayHello", grpc_service="helloworld.Greeter", grpc_type="unary", instance="10.244.0.8:9092", job="grpcserver"}	3
grpc_server_handled_total{grpc_code="OK", grpc_method="SayHelloServerStream", grpc_service="helloworld.Greeter", grpc_type="server_stream", instance="10.244.0.8:9092", job="grpcserver"}	1
grpc_server_handled_total{grpc_code="OK", grpc_method="Check", grpc_service="grpc.health.v1.Health", grpc_type="unary", instance="10.244.0.7:9092", job="grpcserver"}	1
grpc_server_handled_total{grpc_code="OK", grpc_method="SayHello", grpc_service="helloworld.Greeter", grpc_type="unary", instance="10.244.0.7:9092", job="grpcserver"}	7
grpc_server_handled_total{grpc_code="OK", grpc_method="SayHelloBiDiStream", grpc_service="helloworld.Greeter", grpc_type="bidi_stream", instance="10.244.0.7:9092", job="grpcserver"}	2
grpc_server_handled_total{grpc_code="OK", grpc_method="SayHelloClientStream", grpc_service="helloworld.Greeter", grpc_type="client_stream", instance="10.244.0.7:9092", job="grpcserver"}	2
grpc_server_handled_total{grpc_code="OK", grpc_method="SayHelloServerStream", grpc_service="helloworld.Greeter", grpc_type="server_stream", instance="10.244.0.7:9092", job="grpcserver"}
```