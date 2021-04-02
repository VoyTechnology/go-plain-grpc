# go-plain-grpc

gRPC over HTTP and Websockets

> NOTICE: The project is still in early stages of development, and not all gRPC
> functionality has been verified to work. Use at your own risk and contribute
> back if you find something that is missing.

---

## Why

gRPC is an amazing framework. But sometimes it is impossible to use due to the
underlying transport layer. This project allows to use your already existing
codebase, and add interceptors and a proxy to make your gRPC work over normal
HTTP and Websockets - no generation necessary!

---

## Example

This example shows how to add go-plain-grpc to helloworld example from [gRPC-go]
repository.

### Client

```go
package main

import (
    "net/url"

    "google.golang.org/grpc"
    "voy.technology/plain-grpc"

    pb "google.golang.org/grpc/examples/helloworld/helloworld"
)

var endpointFlag = flag.String("endpoint", "", "Endpoint to the serve")

func main() {
    flag.Parse()

    // Error parsing removed for brevity
    endpoint, _ := url.Parse(*endpointFlag)

    opts := []grpc.DialOption{
        grpc.WithInsecure(),
    }

    // gRPC doesn't like Scheme in front of the connection url. We can use it to
    // our advantage to select when we want to use go-plain-grpc.
    if endpoint.Scheme == "http" {
        i := plaingrpc.NewHijacker(endpoint.String())
        opts = append(opts,
            grpc.WithUnaryInterceptor(i.UnaryInterceptor),
            grpc.WithStreamInterceptor(i.StreamInterceptor))
    }
    
    conn, _ := grpc.Dial(entrypoint.String(), opts...)
    defer conn.Close()

    c := pb.NewGreeterClient(conn)

    // Proceed as normal
}
```

### Server

```go
package main

// These can be combined into a single service using cmux
var httpPortFlag = flag.Int("http-port", 8080, "HTTP port")
var grpcPortFlag = flag.Int("grpc-port", 8888, "gRPC port")

func main() {
    flag.Parse()
    lis, _ := net.Listen("tcp", fmt.Sprintf(":%d", *grpcPortFlag))

    s := grpc.NewServer()
    pb.RegisterGreeterServer(s, &server{})
    
    go s.Serve(lis)

    // Proxy forwards all requests from HTTP server to the gRPC server
    proxy, _ := plaingrpc.NewProxy(s, lis.Addr())

    // Register the endpoints
    http.Handle("/", proxy)
    http.ListenAndServe(fmt.Sprintf(":%d", *httpPortFlag), nil)
}
```

---

## Special Thanks

Special Thanks to [Mike Cheng] for [grpc-over-http] on which this project is
roughly based on.

[gRPC-go]: https://github.com/grpc/grpc-go
[Mike Cheng]: https://github.com/mfycheng
[grpc-over-http]: https://github.com/mfycheng/grpc-over-http
