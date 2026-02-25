import { FeatureCard } from "@/components/FeatureCard";
import { Footer } from "@/components/Footer";
import { Hero } from "@/components/Hero";
import { Navbar } from "@/components/Navbar";
import { SectionHeader } from "@/components/SectionHeader";

const features = [
  {
    title: "End-to-end encryption",
    description: "Your secrets are encrypted before they leave your machine. Only you can read them.",
  },
  {
    title: "Control when sync happens",
    description: "Explicit push and pull. No background processes, no surprises — you decide when to sync.",
  },
  {
    title: "Restore from any device",
    description: "Lost your laptop? Onboard a new machine in seconds with your recovery phrase.",
  },
  {
    title: "Never lose changes",
    description: "Built-in conflict detection so you never accidentally overwrite your team's secrets.",
  },
  {
    title: "Quick health checks",
    description: "Run diagnostics to verify your setup, connectivity, and config in seconds.",
  },
  {
    title: "Know who did what",
    description: "Structured audit logs tell you exactly what changed, when, and by whom.",
  },
];

const steps = [
  {
    title: "Initialize and scope",
    detail: "Create your encrypted local vault, select project, and choose environment.",
  },
  {
    title: "Manage secrets safely",
    detail: "Set, get, list, rollback, and review history without exposing plaintext by default.",
  },
  {
    title: "Sync with control",
    detail: "Push and pull to file, HTTP, or Convex backends with optimistic concurrency checks.",
  },
];

function BoltIcon() {
  return (
    <svg viewBox="0 0 24 24" className="h-5 w-5" fill="none" stroke="currentColor" strokeWidth="1.7" aria-hidden="true">
      <path d="M13 2 4 14h7l-1 8 10-14h-7l0-6Z" />
    </svg>
  );
}

export default function Home() {
  return (
    <div className="min-h-screen bg-[var(--fc-bg)] text-[var(--fc-text)]">
      <Navbar />
      <main>
        <Hero />

        <section className="mx-auto w-full max-w-6xl px-6 pb-6">
          <div className="grid gap-4 rounded-2xl border border-white/10 bg-white/[0.02] p-4 sm:grid-cols-3">
            <div className="rounded-xl border border-white/10 bg-black/20 p-4">
              <p className="text-xs uppercase tracking-[0.18em] text-[var(--fc-muted)]">Open source</p>
              <p className="mt-2 text-2xl font-semibold">Free forever</p>
            </div>
            <div className="rounded-xl border border-white/10 bg-black/20 p-4">
              <p className="text-xs uppercase tracking-[0.18em] text-[var(--fc-muted)]">Setup time</p>
              <p className="mt-2 text-2xl font-semibold">Under 5 min</p>
            </div>
            <div className="rounded-xl border border-white/10 bg-black/20 p-4">
              <p className="text-xs uppercase tracking-[0.18em] text-[var(--fc-muted)]">Team size</p>
              <p className="mt-2 text-2xl font-semibold">Any size</p>
            </div>
          </div>
        </section>

        <section id="features" className="mx-auto w-full max-w-6xl px-6 py-20">
          <SectionHeader
            eyebrow="Capabilities"
            title="Sync env across devices & teams"
            description="Built for teams that want project-based sync, strong encryption, and explicit operational control."
          />
          <div className="mt-12 grid gap-5 sm:grid-cols-2 lg:grid-cols-3">
            {features.map((feature) => (
              <FeatureCard key={feature.title} icon={<BoltIcon />} title={feature.title} description={feature.description} />
            ))}
          </div>
        </section>

        <section id="how" className="border-y border-white/10 bg-white/[0.02] py-20">
          <div className="mx-auto w-full max-w-6xl px-6">
            <SectionHeader
              eyebrow="How it works"
              title="Project-based sync. Team-ready."
              description="Three clear steps from local setup to secure team sync."
            />
            <div className="mt-12 grid gap-6 md:grid-cols-3">
              {steps.map((step, index) => (
                <article key={step.title} className="rounded-2xl border border-white/10 bg-black/20 p-6">
                  <p className="text-sm font-semibold text-[var(--fc-accent)]">0{index + 1}</p>
                  <h3 className="mt-3 text-xl font-semibold">{step.title}</h3>
                  <p className="mt-3 text-[var(--fc-muted)]">{step.detail}</p>
                </article>
              ))}
            </div>
          </div>
        </section>

        <section id="security" className="mx-auto w-full max-w-6xl px-6 py-20">
          <SectionHeader
            eyebrow="Security"
            title="No hidden plaintext paths"
            description="Phrase-derived keys, encrypted values, version history, and audit trails all designed for high-signal operations."
          />
          <div className="mt-10 grid gap-6 md:grid-cols-2">
            <article className="rounded-2xl border border-white/10 bg-black/25 p-6">
              <h3 className="text-lg font-semibold">Security model</h3>
              <ul className="mt-4 space-y-2 text-sm text-[var(--fc-muted)]">
                <li>Argon2id key derivation from recovery phrase</li>
                <li>AES-256-GCM encryption for secret values</li>
                <li>Remote writes validated by optimistic `revision` checks</li>
              </ul>
            </article>
            <article className="rounded-2xl border border-white/10 bg-black/25 p-6">
              <h3 className="text-lg font-semibold">Remote integrations</h3>
              <ul className="mt-4 space-y-2 text-sm text-[var(--fc-muted)]">
                <li>HTTP remote backend with bearer token support</li>
                <li>Convex backup transport with deploy key + API key modes</li>
                <li>Restore command for second-machine recovery</li>
              </ul>
            </article>
          </div>
        </section>

        <section className="mx-auto w-full max-w-6xl px-6 py-20">
          <div className="rounded-3xl border border-[var(--fc-accent)]/35 bg-[linear-gradient(120deg,rgba(255,122,26,0.2),rgba(255,122,26,0.05))] p-10 text-center shadow-[0_24px_80px_rgba(255,122,26,0.15)]">
            <h2 className="text-3xl font-semibold sm:text-4xl">Sync env across devices & teams.</h2>
            <p className="mx-auto mt-4 max-w-2xl text-[var(--fc-muted)]">
              Start with one install command and sync environment variables across your team on a project basis.
            </p>
            <a
              href="#home"
              className="mt-8 inline-flex rounded-full bg-[var(--fc-accent)] px-8 py-3 text-sm font-semibold text-[#2b1708] transition hover:brightness-110"
            >
              Install ENV Sync
            </a>
          </div>
        </section>
      </main>
      <Footer />
    </div>
  );
}
