"use client";

import { type ReactNode, useEffect, useMemo } from "react";
import { StackProvider, StackTheme } from "@stackframe/stack";
import { ConvexProvider } from "convex/react";

import { getStackClientApp } from "@/lib/auth/stack";
import { getConvexReactClient } from "@/lib/convex/client";

const stackTheme = {
  radius: "0.95rem",
  light: {
    background: "#f5f7fb",
    foreground: "#0f1623",
    card: "#ffffff",
    cardForeground: "#0f1623",
    popover: "#ffffff",
    popoverForeground: "#0f1623",
    primary: "#ff7a1a",
    primaryForeground: "#271506",
    secondary: "#e9eef7",
    secondaryForeground: "#223049",
    muted: "#eef3fa",
    mutedForeground: "#5b667b",
    accent: "#ffb173",
    accentForeground: "#2d1a08",
    destructive: "#e8505a",
    destructiveForeground: "#ffffff",
    border: "#d8e0ec",
    input: "#e4ebf5",
    ring: "#ff9a43",
  },
  dark: {
    background: "#0b1018",
    foreground: "#f5f7fb",
    card: "#121a27",
    cardForeground: "#f5f7fb",
    popover: "#121a27",
    popoverForeground: "#f5f7fb",
    primary: "#ff7a1a",
    primaryForeground: "#2b1708",
    secondary: "#1c2737",
    secondaryForeground: "#d8dfec",
    muted: "#172131",
    mutedForeground: "#9aa8bf",
    accent: "#ff9a43",
    accentForeground: "#2a1607",
    destructive: "#ff5a64",
    destructiveForeground: "#2a0b0d",
    border: "#2a3649",
    input: "#1a2534",
    ring: "#ff9a43",
  },
} as const;

export function AppProviders({ children }: { children: ReactNode }) {
  const stackApp = useMemo(() => getStackClientApp(), []);
  const convexClient = useMemo(() => getConvexReactClient(), []);

  useEffect(() => {
    if (!stackApp || !convexClient) {
      return;
    }

    convexClient.setAuth(stackApp.getConvexClientAuth({}));
    return () => {
      convexClient.clearAuth();
    };
  }, [stackApp, convexClient]);

  let tree = children;

  if (convexClient) {
    tree = <ConvexProvider client={convexClient}>{tree}</ConvexProvider>;
  }
  if (stackApp) {
    tree = <StackProvider app={stackApp}>{tree}</StackProvider>;
  }

  return <StackTheme theme={stackTheme}>{tree}</StackTheme>;
}
