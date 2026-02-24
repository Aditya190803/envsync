"use client";

import { useEffect, useRef, useState } from "react";

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
    <section className="hero relative px-6 pb-24 pt-14 sm:pt-20" id="home">
      {toastMessage ? (
        <div
          className="pointer-events-none fixed left-1/2 top-6 z-50 -translate-x-1/2 rounded-lg border border-[var(--fc-accent)]/40 bg-black/75 px-4 py-2 text-sm font-medium text-[var(--fc-text)] shadow-[0_10px_30px_rgba(0,0,0,0.35)] backdrop-blur-sm"
          role="status"
          aria-live="polite"
        >
          {toastMessage}
        </div>
      ) : null}
      <div className="glow-sphere pointer-events-none absolute left-1/2 top-[-2rem] h-96 w-96 -translate-x-1/2 rounded-full bg-[radial-gradient(circle,rgba(255,122,26,0.22)_0%,transparent_72%)] blur-3xl" />
      <div className="mx-auto w-full max-w-[1480px]">
        <div className="hero-content fade-in relative mx-auto max-w-[1120px] text-center">
          <span className="badge mb-7 inline-flex rounded-full border border-[var(--fc-accent)]/40 bg-[var(--fc-accent)]/10 px-4 py-1 text-xs font-semibold uppercase tracking-[0.18em] text-[var(--fc-accent)]">v1.0 is now live</span>
          <h1 className="m-0 text-balance font-serif text-[clamp(2rem,5.8vw,4.2rem)] leading-[1.06] tracking-[-0.012em] text-[var(--fc-text)]">
            Encrypted env vars.
            <br />
            <span className="accent-text inline-block md:whitespace-nowrap text-[var(--fc-accent)]">Explicit sync. Zero guesswork.</span>
          </h1>
          <p className="lead mx-auto mt-5 max-w-[860px] text-[clamp(1rem,1.45vw,1 rem)] leading-[1.52] text-[var(--fc-muted)]">
            Stop pasting .env files in Slack. Env-Sync provides a CLI-first workflow to securely share and version environment variables
            across your entire team.
          </p>

        <div className="install-block relative mx-auto mt-12 w-full max-w-[1280px] overflow-hidden rounded-2xl border border-white/14 bg-[linear-gradient(180deg,rgba(255,255,255,0.04),rgba(255,255,255,0.015))] shadow-[0_30px_90px_rgba(0,0,0,0.58)]">
          <div className="pointer-events-none absolute inset-0 rounded-2xl ring-1 ring-inset ring-[var(--fc-accent)]/12" />
          <div className="terminal-header flex items-center gap-2 border-b border-white/10 bg-[linear-gradient(90deg,rgba(255,255,255,0.06),rgba(255,255,255,0.025))] px-4 py-3">
              <span className="dot h-2.5 w-2.5 rounded-full bg-[#f26666]/55" />
              <span className="dot h-2.5 w-2.5 rounded-full bg-[#f5c14a]/55" />
              <span className="dot h-2.5 w-2.5 rounded-full bg-[#5acb7b]/55" />
              <span className="terminal-title ml-2 text-sm font-medium text-[var(--fc-muted)]">bash</span>
            </div>
            <div className="terminal-body flex items-center gap-4 px-5 py-5">
              <code className="min-w-0 flex-1 overflow-hidden text-ellipsis whitespace-nowrap text-left text-[0.9rem] leading-relaxed text-[var(--fc-text)]">
                <span className="prompt">$</span> {installCmd}
              </code>
              <button
                className="copy-btn inline-flex h-10 w-10 flex-shrink-0 items-center justify-center rounded-lg border border-white/18 bg-white/[0.02] text-[var(--fc-text)] transition hover:-translate-y-0.5 hover:border-[var(--fc-accent)]/65 hover:bg-[var(--fc-accent)]/12"
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
          <p className="sub-caption mt-6 text-sm text-[var(--fc-muted)]">Supports macOS, Linux, and Windows (WSL)</p>
        </div>
      </div>
    </section>
  );
}
