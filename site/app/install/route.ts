import { NextResponse } from "next/server";

const INSTALL_SCRIPT_URL = "https://raw.githubusercontent.com/Aditya190803/envsync/main/install.sh";

const SCRIPT_HEADERS = {
  "content-type": "text/x-shellscript; charset=utf-8",
  "cache-control": "public, s-maxage=300, stale-while-revalidate=86400",
};

async function fetchInstallScript() {
  const response = await fetch(INSTALL_SCRIPT_URL, {
    cache: "no-store",
    headers: {
      accept: "text/plain",
      "user-agent": "envsync-site-install-endpoint",
    },
  });

  if (!response.ok) {
    return new NextResponse("Failed to fetch install script", { status: 502 });
  }

  const script = await response.text();
  return new NextResponse(script, { status: 200, headers: SCRIPT_HEADERS });
}

export async function GET() {
  return fetchInstallScript();
}

export async function HEAD() {
  const response = await fetch(INSTALL_SCRIPT_URL, {
    method: "HEAD",
    cache: "no-store",
    headers: {
      accept: "text/plain",
      "user-agent": "envsync-site-install-endpoint",
    },
  });

  if (!response.ok) {
    return new NextResponse(null, { status: 502 });
  }

  return new NextResponse(null, { status: 200, headers: SCRIPT_HEADERS });
}