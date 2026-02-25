import Link from "next/link";

const quickLinks = [
  { label: "Features", href: "#features" },
  { label: "How it works", href: "#how" },
  { label: "Security", href: "#security" },
  { label: "Docs", href: "/docs" },
];

const projectLinks = [
  { label: "GitHub", href: "https://github.com/Aditya190803/envsync" },
  { label: "Issues", href: "https://github.com/Aditya190803/envsync/issues" },
];

export function Footer() {
  const year = new Date().getFullYear();

  return (
    <footer className="relative border-t border-white/10 bg-[linear-gradient(180deg,rgba(255,255,255,0.03),rgba(255,255,255,0.01))]">
      <div className="pointer-events-none absolute inset-x-0 top-0 h-px bg-[linear-gradient(90deg,transparent,rgba(255,122,26,0.75),transparent)]" />

      <div className="mx-auto grid w-full max-w-6xl gap-10 px-6 py-12 md:grid-cols-[1.2fr_1fr_1fr] md:gap-6">
        <div>
          <p className="text-lg font-semibold tracking-tight text-[var(--fc-text)]">ENV Sync</p>
          <p className="mt-3 max-w-sm text-sm leading-relaxed text-[var(--fc-muted)]">
            Sync env across devices & teams with secure encryption.
          </p>
          <p className="mt-5 inline-flex rounded-full border border-[var(--fc-accent)]/35 bg-[var(--fc-accent-soft)] px-3 py-1 text-xs font-medium uppercase tracking-[0.14em] text-[var(--fc-accent)]">
            Encrypted by default
          </p>
        </div>

        <div>
          <h3 className="text-xs font-semibold uppercase tracking-[0.18em] text-[var(--fc-muted)]">Navigate</h3>
          <ul className="mt-4 space-y-3 text-sm">
            {quickLinks.map((link) => (
              <li key={link.label}>
                {link.href.startsWith("/") ? (
                  <Link href={link.href} className="text-[var(--fc-text)]/90 transition hover:text-[var(--fc-accent)]">
                    {link.label}
                  </Link>
                ) : (
                  <a href={link.href} className="text-[var(--fc-text)]/90 transition hover:text-[var(--fc-accent)]">
                    {link.label}
                  </a>
                )}
              </li>
            ))}
          </ul>
        </div>

        <div>
          <h3 className="text-xs font-semibold uppercase tracking-[0.18em] text-[var(--fc-muted)]">Project</h3>
          <ul className="mt-4 space-y-3 text-sm">
            {projectLinks.map((link) => (
              <li key={link.label}>
                <a
                  href={link.href}
                  target="_blank"
                  rel="noreferrer"
                  className="inline-flex items-center gap-2 text-[var(--fc-text)]/90 transition hover:text-[var(--fc-accent)]"
                >
                  {link.label}
                  <span aria-hidden="true" className="text-xs text-[var(--fc-muted)]">
                    ↗
                  </span>
                </a>
              </li>
            ))}
          </ul>
        </div>
      </div>

      <div className="border-t border-white/10">
        <div className="mx-auto flex w-full max-w-6xl flex-col gap-2 px-6 py-4 text-xs text-[var(--fc-muted)] sm:flex-row sm:items-center sm:justify-between">
          <p>© {year} ENV Sync. All rights reserved.</p>
          <p>Sync env across devices & teams.</p>
        </div>
      </div>
    </footer>
  );
}
