"use client";

import { useEffect, useRef, useState } from "react";
import { TerminalBlock } from "./TerminalBlock";

export function Hero() {
  const installCmd = "curl -fsSL https://raw.githubusercontent.com/Aditya190803/envsync/main/install.sh | bash";
  const [toastMessage, setToastMessage] = useState<string | null>(null);
  const dismissTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  function showToast(message: string) {
    if (dismissTimerRef.current) {
      clearTimeout(dismissTimerRef.current);
    }

    setToastMessage(message);
    dismissTimerRef.current = setTimeout(() => {
      setToastMessage(null);
      dismissTimerRef.current = null;
    }, 2200);
  }

  useEffect(() => {
    return () => {
      if (dismissTimerRef.current) {
        clearTimeout(dismissTimerRef.current);
      }
    };
  }, []);

  async function onCopy() {
    try {
      await navigator.clipboard.writeText(installCmd);
      showToast("Copied install command");
    } catch {
      showToast("Could not copy command");
    }
  }

  return (
    <section className="relative isolate overflow-hidden px-6 pb-20 pt-16 sm:pt-24" id="home">
      {toastMessage ? (
        <div
          className="pointer-events-none fixed left-1/2 top-6 z-50 -translate-x-1/2 rounded-lg border border-[var(--fc-accent)]/45 bg-black/80 px-4 py-2 text-sm font-medium text-[var(--fc-text)] shadow-[0_10px_30px_rgba(0,0,0,0.35)] backdrop-blur-sm"
          role="status"
          aria-live="polite"
        >
          {toastMessage}
        </div>
      ) : null}
      <div className="pointer-events-none absolute left-1/2 top-[-9rem] h-[30rem] w-[30rem] -translate-x-1/2 rounded-full bg-[radial-gradient(circle,rgba(255,122,26,0.2)_0%,rgba(255,122,26,0)_68%)] blur-3xl" />
      <div className="pointer-events-none absolute inset-y-0 left-[-7%] w-1/2 bg-[radial-gradient(circle_at_30%_35%,rgba(255,122,26,0.08),transparent_58%)]" />
      <div className="mx-auto w-full max-w-[1460px]">
        <div className="grid items-start gap-8 lg:grid-cols-[0.88fr_1.12fr] lg:gap-6 xl:gap-8">
          <div className="relative pt-2 text-center lg:pt-5 lg:text-left">
            <p className="inline-flex rounded-full border border-[var(--fc-accent)]/35 bg-[linear-gradient(90deg,rgba(255,122,26,0.2),rgba(255,122,26,0.07))] px-4 py-1.5 text-xs font-semibold uppercase tracking-[0.17em] text-[var(--fc-accent)]">
              CLI-first secrets workflow
            </p>
            <h1 className="hero-serif mt-6 text-balance text-[clamp(2.35rem,5.2vw,4.9rem)] leading-[1.02] tracking-[-0.02em] text-[var(--fc-text)]">
              Encrypted env vars.
              <br />
              <span className="inline-block text-[var(--fc-accent)] md:whitespace-nowrap">Explicit sync. Zero guesswork.</span>
            </h1>
            <p className="mx-auto mt-6 max-w-xl text-balance text-[clamp(1rem,1.5vw,1.35rem)] leading-[1.62] text-[var(--fc-muted)] lg:mx-0">
              Stop pasting .env files in Slack. Env-Sync gives your team an opinionated CLI flow to encrypt, share, and version secrets
              safely.
            </p>
            <div className="mt-6 flex flex-wrap items-center justify-center gap-x-5 gap-y-2 text-sm text-[var(--fc-muted)] lg:justify-start">
              <span className="rounded-full border border-white/12 bg-white/[0.02] px-3 py-1">AES-256-GCM</span>
              <span className="rounded-full border border-white/12 bg-white/[0.02] px-3 py-1">Argon2id key derivation</span>
              <span className="rounded-full border border-white/12 bg-white/[0.02] px-3 py-1">No background sync</span>
            </div>
          </div>

          <div className="relative mx-auto w-full max-w-none lg:mx-0 lg:pt-3">
            <div className="pointer-events-none absolute -inset-6 rounded-[2rem] bg-[radial-gradient(circle_at_40%_35%,rgba(255,122,26,0.18),transparent_65%)] blur-2xl" />
            <div className="relative rounded-[1.35rem] border border-white/12 bg-[#0b1017] p-2 shadow-[0_24px_80px_rgba(0,0,0,0.5)]">
              <TerminalBlock className="relative" />
            </div>
          </div>
        </div>
        <div className="mx-auto mt-12 w-full max-w-[1000px]">
          <div className="relative overflow-hidden rounded-2xl border border-white/14 bg-[#0b1017] shadow-[0_28px_75px_rgba(0,0,0,0.55)]">
            <div className="pointer-events-none absolute inset-0 rounded-2xl ring-1 ring-inset ring-[var(--fc-accent)]/10" />
            <div className="flex items-center justify-center gap-2 border-b border-white/10 bg-white/[0.03] px-4 py-3 text-center">
              <span className="ml-2 text-sm font-medium text-[var(--fc-muted)]">Install Script</span>
            </div>
            <div className="flex items-center gap-3 px-4 py-4 sm:px-5 sm:py-5">
              <code className="min-w-0 flex-1 overflow-x-auto text-left font-mono text-[0.9rem] leading-relaxed text-[var(--fc-text)] [scrollbar-width:none] [&::-webkit-scrollbar]:hidden">
                <span className="prompt">$</span> {installCmd}
              </code>
              <button
                className="inline-flex h-10 w-10 flex-shrink-0 items-center justify-center rounded-lg border border-white/18 bg-white/[0.02] text-[var(--fc-text)] transition hover:-translate-y-0.5 hover:border-[var(--fc-accent)]/65 hover:bg-[var(--fc-accent)]/12"
                data-copy={installCmd}
                aria-label="Copy install command"
                type="button"
                onClick={onCopy}
              >
                <svg
                  xmlns="http://www.w3.org/2000/svg"
                  width="20"
                  height="20"
                  viewBox="0 0 24 24"
                  fill="none"
                  stroke="currentColor"
                  strokeWidth="2"
                  strokeLinecap="round"
                  strokeLinejoin="round"
                >
                  <rect width="14" height="14" x="8" y="8" rx="2" ry="2" />
                  <path d="M4 16c-1.1 0-2-.9-2-2V4c0-1.1.9-2 2-2h10c1.1 0 2 .9 2 2" />
                </svg>
              </button>
            </div>
          </div>
          <p className="mt-4 text-center text-sm text-[var(--fc-muted)] lg:text-left">Supports macOS, Linux, and Windows (WSL)</p>
        </div>
      </div>
    </section>
  );
}
