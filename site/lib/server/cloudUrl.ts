export const DEFAULT_HOSTED_CLOUD_API_URL = "https://envsync.adityamer.dev";
export const DEFAULT_LOCAL_CLOUD_API_URL = "http://127.0.0.1:8081";
export const DEFAULT_LOCALHOST_CLOUD_API_URL = "http://localhost:8081";

type ResolveCloudURLsOptions = {
  configuredCloudURL?: string;
  hostname: string;
  origin: string;
};

function isLocalHostname(hostname: string) {
  return hostname === "localhost" || hostname === "127.0.0.1" || hostname === "::1";
}

export function formatUpstreamError(rawBody: string) {
  const trimmed = rawBody.trim();
  if (!trimmed) {
    return "";
  }

  const normalized = trimmed.toLowerCase();
  if (normalized.startsWith("<!doctype html") || normalized.startsWith("<html")) {
    return "upstream_returned_html_404";
  }

  const maxLen = 280;
  if (trimmed.length <= maxLen) {
    return trimmed;
  }
  return `${trimmed.slice(0, maxLen)}...`;
}

export function resolveCloudURLs(options: ResolveCloudURLsOptions) {
  const configured = options.configuredCloudURL?.trim();
  if (configured) {
    return [configured.replace(/\/$/, "")];
  }

  if (options.hostname.startsWith("api.")) {
    return [options.origin];
  }

  // In local Next dev, /v1/* does not exist on the site app.
  // Try local cloud first, then hosted cloud.
  if (isLocalHostname(options.hostname)) {
    const localhostCandidates: string[] = [
      DEFAULT_LOCAL_CLOUD_API_URL,
      DEFAULT_LOCALHOST_CLOUD_API_URL,
      DEFAULT_HOSTED_CLOUD_API_URL,
    ];
    return localhostCandidates.filter((value, index) => localhostCandidates.indexOf(value) === index);
  }

  // Production and preview hosts should use the canonical cloud domain unless explicitly overridden.
  return [DEFAULT_HOSTED_CLOUD_API_URL];
}
