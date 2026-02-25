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
  title: "ENV Sync | Sync env across devices & teams",
  description:
    "ENV Sync is a CLI for syncing environment variables across devices and teams on a project basis.",
  openGraph: {
    title: "ENV Sync",
    description: "Sync env across devices & teams.",
    url: "https://envsync.dev",
    siteName: "ENV Sync",
    type: "website",
  },
  twitter: {
    card: "summary_large_image",
    title: "ENV Sync",
    description: "Sync env across devices & teams.",
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
