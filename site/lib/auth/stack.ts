import { StackClientApp, StackServerApp } from "@stackframe/stack";

const stackProjectId = process.env.NEXT_PUBLIC_STACK_PROJECT_ID;
const stackPublishableClientKey = process.env.NEXT_PUBLIC_STACK_PUBLISHABLE_CLIENT_KEY;
const stackSecretServerKey = process.env.STACK_SECRET_SERVER_KEY;
const stackBaseUrl = process.env.NEXT_PUBLIC_STACK_BASE_URL;
const stackAfterAuthPath = "/dashboard";

export const stackClientConfigured = Boolean(stackProjectId && stackPublishableClientKey);
export const stackServerConfigured = Boolean(stackClientConfigured && stackSecretServerKey);

let stackClientAppSingleton: StackClientApp<true> | null = null;
let stackServerAppSingleton: StackServerApp<true> | null = null;

export function getStackClientApp(): StackClientApp<true> | null {
  if (!stackClientConfigured) {
    return null;
  }
  if (!stackClientAppSingleton) {
    stackClientAppSingleton = new StackClientApp({
      projectId: stackProjectId!,
      publishableClientKey: stackPublishableClientKey!,
      baseUrl: stackBaseUrl,
      urls: {
        afterSignIn: stackAfterAuthPath,
        afterSignUp: stackAfterAuthPath,
      },
      tokenStore: "nextjs-cookie",
    });
  }
  return stackClientAppSingleton;
}

export function getStackServerApp(): StackServerApp<true> | null {
  if (!stackServerConfigured) {
    return null;
  }
  if (!stackServerAppSingleton) {
    stackServerAppSingleton = new StackServerApp({
      projectId: stackProjectId!,
      publishableClientKey: stackPublishableClientKey!,
      secretServerKey: stackSecretServerKey!,
      baseUrl: stackBaseUrl,
      urls: {
        afterSignIn: stackAfterAuthPath,
        afterSignUp: stackAfterAuthPath,
      },
      tokenStore: "nextjs-cookie",
    });
  }
  return stackServerAppSingleton;
}
