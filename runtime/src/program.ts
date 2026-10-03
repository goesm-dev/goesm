// A Go program's main module imports this module before its dependencies,
// so it runs before any package's variable initializers and init functions
// (which run as the modules are evaluated): a panic there then ends the
// program as in Go.

import { crashOnUncaught } from "./chan.ts";

crashOnUncaught();
