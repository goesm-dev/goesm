// Package greet is a Connect service (connectrpc.com/connect) and a client
// that calls it with the Connect, Connect JSON and gRPC-Web protocols.
package greet

import (
	"context"
	"errors"
	"fmt"
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
	res := connect.NewResponse(&greetv1.GreetResponse{Greeting: fmt.Sprintf("Hello, %s! tags=%d hdr=%s", req.Msg.Name, len(req.Msg.Tags), req.Header().Get("X-Test")), Sum: sum})
	res.Header().Set("X-Reply", "yes")
	return res, nil
}

func (server) Count(ctx context.Context, req *connect.Request[greetv1.GreetRequest], stream *connect.ServerStream[greetv1.GreetResponse]) error {
	for i, n := range req.Msg.Nums {
		if err := stream.Send(&greetv1.GreetResponse{Greeting: req.Msg.Name, Sum: int64(i) * int64(n)}); err != nil {
			return err
		}
	}
	return nil
}

// Handler serves the service under /greet.v1.GreetService/.
func Handler() http.Handler {
	_, h := greetv1connect.NewGreetServiceHandler(server{})
	return h
}

// Run calls the service at baseURL through client with each protocol.
func Run(client *http.Client, baseURL string) {
	ctx := context.Background()
	for _, opt := range []struct {
		name string
		opts []connect.ClientOption
	}{{"connect", nil}, {"connect+json", []connect.ClientOption{connect.WithProtoJSON()}}, {"grpcweb", []connect.ClientOption{connect.WithGRPCWeb()}}} {
		c := greetv1connect.NewGreetServiceClient(client, baseURL, opt.opts...)
		req := connect.NewRequest(&greetv1.GreetRequest{Name: "goesm", Nums: []int32{1, 2, 3}, Tags: map[string]string{"a": "b"}})
		req.Header().Set("X-Test", "t1")
		res, err := c.Greet(ctx, req)
		if err != nil {
			fmt.Println(opt.name, "greet error:", err)
		} else {
			fmt.Println(opt.name, "greet:", res.Msg.Greeting, res.Msg.Sum, res.Header().Get("X-Reply"))
		}
		_, err = c.Greet(ctx, connect.NewRequest(&greetv1.GreetRequest{}))
		fmt.Println(opt.name, "error:", connect.CodeOf(err), err)
		st, err := c.Count(ctx, connect.NewRequest(&greetv1.GreetRequest{Name: "s", Nums: []int32{5, 6, 7}}))
		if err != nil {
			fmt.Println(opt.name, "count error:", err)
			continue
		}
		for st.Receive() {
			fmt.Println(opt.name, "count:", st.Msg().Greeting, st.Msg().Sum)
		}
		fmt.Println(opt.name, "count end:", st.Err())
	}
}
