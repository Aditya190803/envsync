"use client";

import Link from "next/link";
import { useState, type ReactNode } from "react";
import { useUser } from "@stackframe/stack";

import { stackClientConfigured } from "@/lib/auth/stack";

function MissingConfig() {
  return (
    <main className="mx-auto min-h-screen w-full max-w-4xl px-6 py-16 text-[var(--fc-text)]">
      <h1 className="text-3xl font-semibold">Dashboard setup required</h1>
      <p className="mt-4 text-[var(--fc-muted)]">
        Set Stack Auth environment variables in your site runtime before using dashboard routes.
      </p>
      <ul className="mt-4 list-disc space-y-1 pl-5 text-sm text-[var(--fc-muted)]">
        <li>NEXT_PUBLIC_STACK_PROJECT_ID</li>
        <li>NEXT_PUBLIC_STACK_PUBLISHABLE_CLIENT_KEY</li>
        <li>STACK_SECRET_SERVER_KEY</li>
      </ul>
    </main>
  );
}

function SignedOut() {
  return (
    <main className="mx-auto min-h-screen w-full max-w-4xl px-6 py-16 text-[var(--fc-text)]">
      <h1 className="text-3xl font-semibold">Sign in to continue</h1>
      <p className="mt-4 text-[var(--fc-muted)]">
        Identity login starts enrollment. Device approval (or recovery fallback) completes decrypt access.
      </p>
      <div className="mt-8 flex items-center gap-3">
        <Link
          href="/handler/sign-in?after_auth_return_to=%2Fdashboard"
          className="inline-flex rounded-full bg-[var(--fc-accent)] px-5 py-2.5 text-sm font-semibold text-[#2b1708] transition hover:brightness-110"
        >
          Sign in with Google or GitHub
        </Link>
        <Link href="/" className="text-sm text-[var(--fc-muted)] transition hover:text-[var(--fc-text)]">
          Back to home
        </Link>
      </div>
    </main>
  );
}

function SignedInLayout({ children }: { children: ReactNode }) {
  const user = useUser();
  const [isSigningOut, setIsSigningOut] = useState(false);

  if (!user) {
    return <SignedOut />;
  }

  async function onSignOut() {
    if (!user) {
      return;
    }
    try {
      setIsSigningOut(true);
      await user.signOut({ redirectUrl: "/" });
    } finally {
      setIsSigningOut(false);
    }
  }

  return (
    <div className="min-h-screen bg-[var(--fc-bg)] text-[var(--fc-text)]">
      <header className="border-b border-white/10 bg-[color-mix(in_oklab,var(--fc-bg)_88%,black)]/85 backdrop-blur">
        <div className="mx-auto flex h-16 w-full max-w-6xl items-center justify-between px-6">
          <div>
            <p className="text-xs uppercase tracking-[0.18em] text-[var(--fc-muted)]">EnvSync Dashboard</p>
            <p className="text-sm font-semibold">{user.displayName ?? user.primaryEmail ?? "Account"}</p>
          </div>
          <nav className="flex items-center gap-4 text-sm text-[var(--fc-muted)]">
            <Link href="/dashboard" className="transition hover:text-[var(--fc-text)]">
              Overview
            </Link>
            <Link href="/dashboard/devices" className="transition hover:text-[var(--fc-text)]">
              Devices
            </Link>
            <button
              type="button"
              onClick={onSignOut}
              disabled={isSigningOut}
              className="rounded-full border border-white/20 px-3 py-1.5 transition hover:border-[var(--fc-accent)]/60 hover:text-[var(--fc-text)] disabled:cursor-not-allowed disabled:opacity-60"
            >
              {isSigningOut ? "Signing out..." : "Sign out"}
            </button>
          </nav>
        </div>
      </header>
      <main className="mx-auto w-full max-w-6xl px-6 py-10">{children}</main>
    </div>
  );
}

export default function DashboardLayout({ children }: { children: ReactNode }) {
  if (!stackClientConfigured) {
    return <MissingConfig />;
  }
  return <SignedInLayout>{children}</SignedInLayout>;
}
