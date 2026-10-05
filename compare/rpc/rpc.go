// Package rpc calls a Connect service with connect-go and the generated
// protobuf types, as a page does with connect-es: a unary call in the
// Connect protocol with the binary protobuf encoding, over fetch.
// js/impl/rpc.mjs is the same with connect-es.
package rpc

import (
	"context"
	"net/http"

	"connectrpc.com/connect"
	taskv1 "example.com/compare/gen/task/v1"
	"example.com/compare/gen/task/v1/taskv1connect"
)

// Summary is what List returns about the tasks the server sent.
type Summary struct {
	Count int    `json:"count"`
	Done  int    `json:"done"`
	First string `json:"first"`
	Tags  int    `json:"tags"`
}

var clients = map[string]taskv1connect.TaskServiceClient{}

// List calls TaskService.ListTasks at baseURL.
func List(baseURL, owner string, limit int) (Summary, error) {
	c, ok := clients[baseURL]
	if !ok {
		c = taskv1connect.NewTaskServiceClient(http.DefaultClient, baseURL)
		clients[baseURL] = c
	}
	res, err := c.ListTasks(context.Background(), connect.NewRequest(&taskv1.ListTasksRequest{Owner: owner, Limit: int32(limit)}))
	if err != nil {
		return Summary{}, err
	}
	var s Summary
	for _, t := range res.Msg.Tasks {
		if s.Count == 0 {
			s.First = t.Title
		}
		s.Count++
		if t.Done {
			s.Done++
		}
		s.Tags += len(t.Tags)
	}
	return s, nil
}
