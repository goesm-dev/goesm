// Package worker is a Connect backend for Cloudflare Workers as small as
// such a Worker gets: one Connect service (Connect, gRPC and gRPC-Web, binary
// and JSON) behind net/http, served through the runtime's fetchHandler.
// TestUseCaseWorker calls it and bounds its bundle.
package worker

import (
	"context"
	"errors"
	"net/http"

	"connectrpc.com/connect"
	greetv1 "usecases/gen/greet/v1"
	"usecases/gen/greet/v1/greetv1connect"
)

type server struct{}

func (server) Greet(ctx context.Context, req *connect.Request[greetv1.GreetRequest]) (*connect.Response[greetv1.GreetResponse], error) {
	if req.Msg.Name == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("name is required"))
	}
	var sum int64
	for _, n := range req.Msg.Nums {
		sum += int64(n)
	}
	return connect.NewResponse(&greetv1.GreetResponse{Greeting: "Hello, " + req.Msg.Name + "!", Sum: sum}), nil
}

func (server) Count(ctx context.Context, req *connect.Request[greetv1.GreetRequest], stream *connect.ServerStream[greetv1.GreetResponse]) error {
	for i, n := range req.Msg.Nums {
		if err := stream.Send(&greetv1.GreetResponse{Greeting: req.Msg.Name, Sum: int64(i) * int64(n)}); err != nil {
			return err
		}
	}
	return nil
}

func Handler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle(greetv1connect.NewGreetServiceHandler(server{}))
	return mux
}
