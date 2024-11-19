FROM golang:1.20 as build

RUN apt-get update -y && apt-get install -y build-essential wget unzip curl git libtspi-dev


RUN curl -OL https://github.com/google/protobuf/releases/download/v3.19.0/protoc-3.19.0-linux-x86_64.zip && \
    unzip protoc-3.19.0-linux-x86_64.zip -d protoc3 && \
    mv protoc3/bin/* /usr/local/bin/ && \
    mv protoc3/include/* /usr/local/include/

WORKDIR /app

ADD . /app

RUN go mod download
RUN GO111MODULE=on 
RUN go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
RUN go install github.com/golang/protobuf/protoc-gen-go@latest

RUN protoc --go_out=. --go_opt=paths=source_relative --go-grpc_opt=require_unimplemented_servers=false --go-grpc_out=. --go-grpc_opt=paths=source_relative helloworld/helloworld.proto

RUN export GOBIN=/app/bin && go build -o grpc_server greeter_server/main.go

FROM gcr.io/distroless/base
COPY --from=build /app/grpc_server /

EXPOSE 50051
ENTRYPOINT ["/grpc_server"]
