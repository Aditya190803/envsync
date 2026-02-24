import type { Metadata } from "next";
import { IBM_Plex_Mono, Sora } from "next/font/google";
import "./globals.css";

const sora = Sora({
  variable: "--font-sora",
  subsets: ["latin"],
});

const plexMono = IBM_Plex_Mono({
  variable: "--font-plex-mono",
  subsets: ["latin"],
  weight: ["400", "500"],
});

export const metadata: Metadata = {
  metadataBase: new URL("https://envsync.dev"),
  title: "Env-Sync | Encrypted env vars with explicit sync",
  description:
    "Env-Sync is a terminal-first CLI for encrypted environment variables, explicit sync, restore, diagnostics, and auditable workflows.",
  openGraph: {
    title: "Env-Sync",
    description: "Encrypted env vars. Explicit sync. Zero guesswork.",
    url: "https://envsync.dev",
    siteName: "Env-Sync",
    type: "website",
  },
  twitter: {
    card: "summary_large_image",
    title: "Env-Sync",
    description: "Encrypted env vars. Explicit sync. Zero guesswork.",
  },
};

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <html lang="en">
      <body className={`${sora.variable} ${plexMono.variable} antialiased`}>{children}</body>
    </html>
  );
}
