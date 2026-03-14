import { ConvexHttpClient } from "convex/browser";
import type { NextRequest } from "next/server";

import { getStackServerApp } from "@/lib/auth/stack";

const convexUrl = process.env.NEXT_PUBLIC_CONVEX_URL;

export async function getAuthedConvexClient(req: NextRequest) {
  if (!convexUrl) {
    throw new Error("NEXT_PUBLIC_CONVEX_URL is not configured");
  }

  const stackApp = getStackServerApp();
  if (!stackApp) {
    throw new Error("Stack Auth server app is not configured");
  }

  const user = await stackApp.getUser({ tokenStore: req, or: "return-null" });
  if (!user) {
    return null;
  }

  const convexToken = await stackApp.getConvexHttpClientAuth({ tokenStore: req });
  const client = new ConvexHttpClient(convexUrl);
  client.setAuth(convexToken);
  return client;
}
