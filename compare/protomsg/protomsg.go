// Package protomsg encodes and decodes protobuf messages with the
// generated types and google.golang.org/protobuf, as an app does that
// keeps or sends its data in the protobuf binary format without an RPC
// framework. js/impl/protomsg.mjs does the same with @bufbuild/protobuf.
package protomsg

import (
	"strconv"

	taskv1 "example.com/compare/gen/task/v1"
	"google.golang.org/protobuf/proto"
)

// Summary is what Decode returns about a list of tasks.
type Summary struct {
	Count int    `json:"count"`
	Done  int    `json:"done"`
	First string `json:"first"`
	Tags  int    `json:"tags"`
	Due   string `json:"due"`
}

// Decode decodes a ListTasksResponse and summarizes its tasks.
func Decode(b []byte) (Summary, error) {
	var res taskv1.ListTasksResponse
	if err := proto.Unmarshal(b, &res); err != nil {
		return Summary{}, err
	}
	var s Summary
	var due int64
	for _, t := range res.Tasks {
		if s.Count == 0 {
			s.First = t.Title
		}
		s.Count++
		if t.Done {
			s.Done++
		}
		s.Tags += len(t.Tags)
		due = max(due, t.DueUnix)
	}
	s.Due = strconv.FormatInt(due, 10)
	return s, nil
}

// Complete decodes a ListTasksResponse, marks every task done with the tag
// "done" and encodes it again.
func Complete(b []byte) ([]byte, error) {
	var res taskv1.ListTasksResponse
	if err := proto.Unmarshal(b, &res); err != nil {
		return nil, err
	}
	for _, t := range res.Tasks {
		t.Done = true
		t.Tags = append(t.Tags, "done")
	}
	return proto.Marshal(&res)
}
