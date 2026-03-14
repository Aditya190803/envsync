import "server-only";
import { StackServerApp } from "@stackframe/stack";
import { stackClientApp } from "./client";

// Server app has full read/write access to all users.
// Only use in secure server-side contexts (Server Components, Route Handlers, etc.)
// Requires STACK_SECRET_SERVER_KEY env variable.
export const stackServerApp = new StackServerApp({
  inheritsFrom: stackClientApp,
});
