// connect-es's version of ../../rpc/rpc.go.
import { createClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import { TaskService } from "../gen/task/v1/task_pb.js";

const clients = new Map();

export async function List(baseUrl, owner, limit) {
  let c = clients.get(baseUrl);
  if (!c) {
    c = createClient(TaskService, createConnectTransport({ baseUrl, useBinaryFormat: true }));
    clients.set(baseUrl, c);
  }
  const res = await c.listTasks({ owner, limit });
  const s = { count: 0, done: 0, first: "", tags: 0 };
  for (const t of res.tasks) {
    if (s.count === 0) s.first = t.title;
    s.count++;
    if (t.done) s.done++;
    s.tags += t.tags.length;
  }
  return s;
}
