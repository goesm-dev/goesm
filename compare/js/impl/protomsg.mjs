// @bufbuild/protobuf's version of ../../protomsg/protomsg.go.
import { fromBinary, toBinary } from "@bufbuild/protobuf";
import { ListTasksResponseSchema } from "../gen/task/v1/task_pb.js";

export function Decode(b) {
  const res = fromBinary(ListTasksResponseSchema, b);
  const s = { count: 0, done: 0, first: "", tags: 0, due: "" };
  let due = 0n;
  for (const t of res.tasks) {
    if (s.count === 0) s.first = t.title;
    s.count++;
    if (t.done) s.done++;
    s.tags += t.tags.length;
    if (t.dueUnix > due) due = t.dueUnix;
  }
  s.due = due.toString();
  return s;
}

export function Complete(b) {
  const res = fromBinary(ListTasksResponseSchema, b);
  for (const t of res.tasks) {
    t.done = true;
    t.tags.push("done");
  }
  return toBinary(ListTasksResponseSchema, res);
}
