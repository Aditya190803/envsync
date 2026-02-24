import type { Metadata } from "next";
import Link from "next/link";

export const metadata: Metadata = {
  title: "Env-Sync Docs",
  description: "Complete command reference for envsync CLI.",
};

function CodeBlock({ code }: { code: string }) {
  return (
    <pre className="overflow-x-auto rounded-xl border border-white/10 bg-black/35 p-4 text-sm text-[var(--fc-text)]">
      <code>{code}</code>
    </pre>
  );
}

const usageText = `envsync - encrypted env var sync

Usage:
  envsync init
  envsync project create <name>
  envsync project list
  envsync project use <name>
  envsync team create <name>
  envsync team list
  envsync team use <name>
  envsync team add-member <team> <actor> <role>
  envsync team list-members [team]
  envsync env create <name>
  envsync env use <name>
  envsync set <KEY> <value>
  envsync rotate <KEY> <value>
  envsync get <KEY>
  envsync delete <KEY>
  envsync list [--show]
  envsync load
  envsync history <KEY>
  envsync rollback <KEY> --version <n>
  envsync push [--force]
  envsync pull [--force-remote]
  envsync phrase save
  envsync phrase clear
  envsync doctor
  envsync restore`;

const commands = [
  { command: "envsync init", description: "Initialize envsync on this machine and create local encrypted state." },
  { command: "envsync project create <name>", description: "Create a new project namespace." },
  { command: "envsync project list", description: "List all available projects." },
  { command: "envsync project use <name>", description: "Set the active project for subsequent commands." },
  { command: "envsync team create <name>", description: "Create a new team." },
  { command: "envsync team list", description: "List all teams." },
  { command: "envsync team use <name>", description: "Set the active team." },
  { command: "envsync team add-member <team> <actor> <role>", description: "Add a member (actor) to a team with a role." },
  { command: "envsync team list-members [team]", description: "List members of a team. Uses active team if omitted." },
  { command: "envsync env create <name>", description: "Create an environment (for example: dev, staging, prod)." },
  { command: "envsync env use <name>", description: "Set the active environment." },
  { command: "envsync set <KEY> <value>", description: "Create or update a secret key with a value." },
  { command: "envsync rotate <KEY> <value>", description: "Rotate a secret by writing a new version for an existing key." },
  { command: "envsync get <KEY>", description: "Read the current value for a key." },
  { command: "envsync delete <KEY>", description: "Delete a key from the active project/environment." },
  { command: "envsync list [--show]", description: "List keys. Use --show to include values in output." },
  { command: "envsync load", description: "Load active env vars into shell-compatible output/workflow." },
  { command: "envsync history <KEY>", description: "Show version history for a key." },
  { command: "envsync rollback <KEY> --version <n>", description: "Rollback a key to a previous version number." },
  { command: "envsync push [--force]", description: "Push local encrypted state to remote. --force overrides conflict checks." },
  { command: "envsync pull [--force-remote]", description: "Pull remote encrypted state to local. --force-remote prefers remote on conflicts." },
  { command: "envsync phrase save", description: "Save recovery phrase to local keychain/secure storage." },
  { command: "envsync phrase clear", description: "Clear stored recovery phrase from local keychain/secure storage." },
  { command: "envsync doctor", description: "Run diagnostics for local config, state, and remote connectivity." },
  { command: "envsync restore", description: "Restore local state from remote using your recovery phrase." },
] as const;

export default function DocsPage() {
  return (
    <div className="min-h-screen bg-[var(--fc-bg)] text-[var(--fc-text)]">
      <header className="sticky top-0 z-40 border-b border-white/10 bg-[color-mix(in_oklab,var(--fc-bg)_90%,black)]/85 backdrop-blur">
        <div className="mx-auto flex h-16 w-full max-w-5xl items-center justify-between px-6">
          <Link href="/" className="text-sm font-semibold tracking-tight text-[var(--fc-text)]">
            Env-Sync
          </Link>
          <Link href="/" className="text-sm text-[var(--fc-muted)] transition hover:text-[var(--fc-text)]">
            Home
          </Link>
        </div>
      </header>

      <main className="mx-auto w-full max-w-5xl px-6 py-10">
        <section className="rounded-2xl border border-white/10 bg-[linear-gradient(180deg,rgba(255,255,255,0.03),rgba(255,255,255,0.015))] p-6">
          <p className="text-xs font-semibold uppercase tracking-[0.2em] text-[var(--fc-accent)]">Documentation</p>
          <h1 className="mt-3 text-4xl font-semibold leading-tight sm:text-5xl">CLI Arguments</h1>
          <p className="mt-4 max-w-3xl text-[var(--fc-muted)]">
            Complete `envsync` command list and what each command does.
          </p>
        </section>

        <section className="mt-8 space-y-4">
          <h2 className="text-2xl font-semibold">Usage</h2>
          <CodeBlock code={usageText} />
        </section>

        <section className="mt-8 space-y-4">
          <h2 className="text-2xl font-semibold">What Each Command Does</h2>
          <div className="space-y-3">
            {commands.map((item) => (
              <article key={item.command} className="rounded-xl border border-white/10 bg-black/20 p-4">
                <p className="font-mono text-sm text-[var(--fc-text)]">{item.command}</p>
                <p className="mt-2 text-sm text-[var(--fc-muted)]">{item.description}</p>
              </article>
            ))}
          </div>
        </section>
      </main>
    </div>
  );
}
