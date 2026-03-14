import { StackClientApp } from "@stackframe/stack";

// Next.js: project ID and publishable client key are auto-detected from
// NEXT_PUBLIC_STACK_PROJECT_ID and NEXT_PUBLIC_STACK_PUBLISHABLE_CLIENT_KEY
export const stackClientApp = new StackClientApp({
  tokenStore: "nextjs-cookie",
});
