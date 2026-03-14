import { describe, expect, test } from "bun:test";

import {
  DEFAULT_LOCAL_CLOUD_API_URL,
  DEFAULT_LOCALHOST_CLOUD_API_URL,
  DEFAULT_HOSTED_CLOUD_API_URL,
  formatUpstreamError,
  resolveCloudURLs,
} from "@/lib/server/cloudUrl";

describe("resolveCloudURLs", () => {
  test("uses configured cloud URL when provided", () => {
    const got = resolveCloudURLs({
      configuredCloudURL: "https://envsync.adityamer.dev/",
      hostname: "envsync.adityamer.dev",
      origin: "https://envsync.adityamer.dev",
    });

    expect(got).toEqual(["https://envsync.adityamer.dev"]);
  });

  test("uses local and hosted candidates for localhost", () => {
    const got = resolveCloudURLs({
      hostname: "localhost",
      origin: "http://localhost:3000",
    });

    expect(got).toEqual([DEFAULT_LOCAL_CLOUD_API_URL, DEFAULT_LOCALHOST_CLOUD_API_URL, DEFAULT_HOSTED_CLOUD_API_URL]);
  });

  test("uses configured local cloud URL when provided", () => {
    const got = resolveCloudURLs({
      configuredCloudURL: "http://127.0.0.1:8081/",
      hostname: "localhost",
      origin: "http://localhost:3000",
    });

    expect(got).toEqual(["http://127.0.0.1:8081"]);
  });

  test("uses origin for api subdomains", () => {
    const got = resolveCloudURLs({
      hostname: "api.envsync.adityamer.dev",
      origin: "https://api.envsync.adityamer.dev",
    });

    expect(got).toEqual(["https://api.envsync.adityamer.dev"]);
  });

  test("uses canonical hosted domain for non-local hosts", () => {
    const got = resolveCloudURLs({
      hostname: "envsync.adityamer.dev",
      origin: "https://envsync-preview.adityamer.dev",
    });

    expect(got).toEqual([DEFAULT_HOSTED_CLOUD_API_URL]);
  });

  test("uses canonical hosted domain even when origin matches", () => {
    const got = resolveCloudURLs({
      hostname: "envsync.adityamer.dev",
      origin: DEFAULT_HOSTED_CLOUD_API_URL,
    });

    expect(got).toEqual([DEFAULT_HOSTED_CLOUD_API_URL]);
  });
});

describe("formatUpstreamError", () => {
  test("returns empty string for empty payload", () => {
    expect(formatUpstreamError("   ")).toBe("");
  });

  test("normalizes html payloads", () => {
    expect(formatUpstreamError("<!DOCTYPE html><html><body>Not Found</body></html>")).toBe(
      "upstream_returned_html_404",
    );
  });

  test("truncates long payloads", () => {
    const source = "x".repeat(500);
    const got = formatUpstreamError(source);
    expect(got.length).toBe(283);
    expect(got.endsWith("...")).toBe(true);
  });
});
