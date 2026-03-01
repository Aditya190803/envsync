import { NextResponse } from "next/server";

const INSTALL_SCRIPT_URL = "https://raw.githubusercontent.com/Aditya190803/envsync/main/install.sh";

export function GET() {
  return NextResponse.redirect(INSTALL_SCRIPT_URL, { status: 308 });
}

export function HEAD() {
  return NextResponse.redirect(INSTALL_SCRIPT_URL, { status: 308 });
}