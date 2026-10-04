// A Go program's main module imports this module before its dependencies,
// so it runs before any package's variable initializers and init functions
// (which run as the modules are evaluated): a panic there then ends the
// program as in Go, and so does an init that blocks forever (a deadlock;
// Node only: Bun keeps spinning on a top-level await that never settles).

import { crashOnUncaught, watchDeadlock } from "./chan.ts";

crashOnUncaught();
watchDeadlock();
