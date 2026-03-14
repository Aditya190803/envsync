import { ConvexReactClient } from "convex/react";

const convexUrl = process.env.NEXT_PUBLIC_CONVEX_URL;

export const convexConfigured = Boolean(convexUrl);

let convexClientSingleton: ConvexReactClient | null = null;

export function getConvexReactClient(): ConvexReactClient | null {
  if (!convexConfigured) {
    return null;
  }
  if (!convexClientSingleton) {
    convexClientSingleton = new ConvexReactClient(convexUrl!);
  }
  return convexClientSingleton;
}
