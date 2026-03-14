import { StackHandler } from "@stackframe/stack";
import { getStackServerApp } from "@/lib/auth/stack";

export default function StackHandlerPage() {
  const stackServerApp = getStackServerApp();

  if (!stackServerApp) {
    return (
      <main className="mx-auto min-h-screen w-full max-w-3xl px-6 py-16 text-[var(--fc-text)]">
        <h1 className="text-3xl font-semibold">Stack Auth is not configured</h1>
        <p className="mt-4 text-[var(--fc-muted)]">
          Set Stack Auth environment variables before visiting handler routes.
        </p>
        <ul className="mt-4 list-disc space-y-1 pl-5 text-sm text-[var(--fc-muted)]">
          <li>NEXT_PUBLIC_STACK_PROJECT_ID</li>
          <li>NEXT_PUBLIC_STACK_PUBLISHABLE_CLIENT_KEY</li>
          <li>STACK_SECRET_SERVER_KEY</li>
        </ul>
      </main>
    );
  }

  return <StackHandler fullPage app={stackServerApp} />;
}
