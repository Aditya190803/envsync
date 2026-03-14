"use client";

import Link from "next/link";
import { useCallback, useEffect, useMemo, useState } from "react";

import { formatRelativeTime } from "@/lib/security/device";

type DeviceStatus = "pending" | "approved" | "revoked";

type DeviceRecord = {
  id: string;
  deviceId: string;
  displayName: string;
  status: DeviceStatus;
  lastSeenAt: number | null;
  updatedAt: number;
};

type PendingEnrollment = {
  id: string;
  targetDeviceId: string;
  targetDeviceName: string;
  createdAt: number;
};

type DevicesApiResponse = {
  devices: DeviceRecord[];
  pendingEnrollments: PendingEnrollment[];
};

type SnapshotApiResponse = {
  project: string;
  revision: number;
  payload: unknown;
  saltB64: string | null;
  keyCheckB64: string | null;
  updatedByDeviceId?: string | null;
  updatedAt?: number;
};

async function readError(response: Response) {
  try {
    const body = (await response.json()) as { error?: string };
    return body.error || "request_failed";
  } catch {
    return "request_failed";
  }
}

function getTrackedProjectsCount(payload: unknown) {
  if (!payload || typeof payload !== "object") {
    return 0;
  }
  const maybeProjects = (payload as { projects?: unknown }).projects;
  if (!maybeProjects || typeof maybeProjects !== "object" || Array.isArray(maybeProjects)) {
    return 0;
  }
  return Object.keys(maybeProjects as Record<string, unknown>).length;
}

export default function DashboardPage() {
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [devices, setDevices] = useState<DeviceRecord[]>([]);
  const [pendingEnrollments, setPendingEnrollments] = useState<PendingEnrollment[]>([]);
  const [snapshot, setSnapshot] = useState<SnapshotApiResponse | null>(null);

  const refresh = useCallback(async () => {
    setLoading(true);
    setError(null);

    try {
      const [devicesResponse, snapshotResponse] = await Promise.all([
        fetch("/api/devices", { method: "GET", cache: "no-store" }),
        fetch("/api/vault/snapshot?project=default", { method: "GET", cache: "no-store" }),
      ]);

      if (!devicesResponse.ok) {
        throw new Error(await readError(devicesResponse));
      }
      if (!snapshotResponse.ok) {
        throw new Error(await readError(snapshotResponse));
      }

      const devicesData = (await devicesResponse.json()) as DevicesApiResponse;
      const snapshotData = (await snapshotResponse.json()) as SnapshotApiResponse;

      setDevices(devicesData.devices ?? []);
      setPendingEnrollments(devicesData.pendingEnrollments ?? []);
      setSnapshot(snapshotData);
    } catch (err) {
      setError(err instanceof Error ? err.message : "dashboard_load_failed");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  const approvedCount = useMemo(() => devices.filter((device) => device.status === "approved").length, [devices]);
  const revokedCount = useMemo(() => devices.filter((device) => device.status === "revoked").length, [devices]);
  const hasRealSnapshot = typeof snapshot?.updatedAt === "number";
  const trackedProjects = useMemo(
    () => (hasRealSnapshot ? getTrackedProjectsCount(snapshot?.payload) : null),
    [hasRealSnapshot, snapshot],
  );

  const mostRecentSeenDevice = useMemo(() => {
    const seenDevices = devices.filter((device) => typeof device.lastSeenAt === "number");
    if (seenDevices.length === 0) {
      return null;
    }
    return seenDevices.sort((a, b) => (b.lastSeenAt ?? 0) - (a.lastSeenAt ?? 0))[0] ?? null;
  }, [devices]);

  const latestUpdates = useMemo(
    () =>
      devices
        .slice()
        .sort((a, b) => b.updatedAt - a.updatedAt)
        .slice(0, 5),
    [devices],
  );

  return (
    <div className="space-y-6">
      <section className="rounded-2xl border border-white/10 bg-white/[0.02] p-6">
        <div className="flex flex-col gap-4 md:flex-row md:items-start md:justify-between">
          <div>
            <h1 className="text-3xl font-semibold">Overview</h1>
            <p className="mt-2 max-w-3xl text-sm text-[var(--fc-muted)]">
              Real-time status from your authenticated device and encrypted vault state.
            </p>
          </div>
          <button
            type="button"
            onClick={() => void refresh()}
            disabled={loading}
            className="inline-flex h-10 items-center justify-center rounded-full border border-white/20 px-4 text-sm transition hover:border-[var(--fc-accent)]/60 disabled:cursor-not-allowed disabled:opacity-50"
          >
            {loading ? "Refreshing..." : "Refresh"}
          </button>
        </div>

        {error ? <p className="mt-4 text-sm text-red-300">Error: {error}</p> : null}
      </section>

      <section className="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
        <article className="rounded-xl border border-white/10 bg-black/20 p-5">
          <p className="text-xs uppercase tracking-[0.16em] text-[var(--fc-muted)]">Approved devices</p>
          <p className="mt-2 text-3xl font-semibold">{approvedCount}</p>
        </article>
        <article className="rounded-xl border border-white/10 bg-black/20 p-5">
          <p className="text-xs uppercase tracking-[0.16em] text-[var(--fc-muted)]">Pending approvals</p>
          <p className="mt-2 text-3xl font-semibold">{pendingEnrollments.length}</p>
        </article>
        <article className="rounded-xl border border-white/10 bg-black/20 p-5">
          <p className="text-xs uppercase tracking-[0.16em] text-[var(--fc-muted)]">Vault revision</p>
          <p className="mt-2 text-3xl font-semibold">{hasRealSnapshot ? snapshot?.revision : "-"}</p>
          <p className="mt-2 text-xs text-[var(--fc-muted)]">
            Last updated {hasRealSnapshot ? formatRelativeTime(snapshot?.updatedAt ?? null) : "-"}
          </p>
        </article>
        <article className="rounded-xl border border-white/10 bg-black/20 p-5">
          <p className="text-xs uppercase tracking-[0.16em] text-[var(--fc-muted)]">Tracked projects</p>
          <p className="mt-2 text-3xl font-semibold">{trackedProjects ?? "-"}</p>
          <p className="mt-2 text-xs text-[var(--fc-muted)]">
            {!hasRealSnapshot ? "No snapshot yet" : snapshot?.saltB64 ? "Recovery salt present" : "Recovery salt missing"}
          </p>
        </article>
      </section>

      <section className="grid gap-4 xl:grid-cols-2">
        <article className="rounded-2xl border border-white/10 bg-black/20 p-5">
          <div className="flex items-center justify-between">
            <h2 className="text-lg font-semibold">Device health</h2>
            <Link href="/dashboard/devices" className="text-sm text-[var(--fc-accent)] transition hover:brightness-110">
              Manage devices
            </Link>
          </div>

          <div className="mt-4 grid gap-2 text-sm text-[var(--fc-muted)]">
            <p>Total devices: {devices.length}</p>
            <p>Revoked devices: {revokedCount}</p>
            <p>
              Most recently seen: {mostRecentSeenDevice ? mostRecentSeenDevice.displayName : "No activity yet"}
            </p>
            <p>
              Seen at: {mostRecentSeenDevice ? formatRelativeTime(mostRecentSeenDevice.lastSeenAt) : "-"}
            </p>
          </div>

          <div className="mt-4 divide-y divide-white/10 rounded-lg border border-white/10">
            {latestUpdates.length === 0 ? (
              <p className="px-3 py-3 text-sm text-[var(--fc-muted)]">No registered devices yet.</p>
            ) : (
              latestUpdates.map((device) => (
                <div key={device.id} className="flex items-center justify-between px-3 py-3 text-sm">
                  <p className="truncate pr-3">{device.displayName}</p>
                  <div className="flex items-center gap-3">
                    <span className="text-[var(--fc-muted)]">{formatRelativeTime(device.updatedAt)}</span>
                    <span
                      className={
                        device.status === "approved"
                          ? "text-emerald-300"
                          : device.status === "pending"
                            ? "text-amber-300"
                            : "text-rose-300"
                      }
                    >
                      {device.status}
                    </span>
                  </div>
                </div>
              ))
            )}
          </div>
        </article>

        <article className="rounded-2xl border border-white/10 bg-black/20 p-5">
          <h2 className="text-lg font-semibold">Vault state</h2>

          <div className="mt-4 grid gap-2 text-sm text-[var(--fc-muted)]">
            <p>Project: {hasRealSnapshot ? snapshot?.project : "-"}</p>
            <p>Current revision: {hasRealSnapshot ? snapshot?.revision : "-"}</p>
            <p>Updated by device: {hasRealSnapshot ? (snapshot?.updatedByDeviceId ?? "-") : "-"}</p>
            <p>Key check: {hasRealSnapshot ? (snapshot?.keyCheckB64 ? "present" : "missing") : "-"}</p>
            <p>Snapshot age: {hasRealSnapshot ? formatRelativeTime(snapshot?.updatedAt ?? null) : "-"}</p>
          </div>

          {!hasRealSnapshot ? (
            <p className="mt-4 text-sm text-[var(--fc-muted)]">
              No encrypted snapshot has been written by an approved device yet.
            </p>
          ) : null}

          <div className="mt-4 rounded-lg border border-white/10 bg-black/30 p-3">
            <p className="text-xs uppercase tracking-[0.14em] text-[var(--fc-muted)]">Pending enrollment queue</p>
            {pendingEnrollments.length === 0 ? (
              <p className="mt-2 text-sm text-[var(--fc-muted)]">No pending requests.</p>
            ) : (
              <ul className="mt-2 space-y-2 text-sm">
                {pendingEnrollments.slice(0, 5).map((request) => (
                  <li key={request.id} className="flex items-center justify-between gap-3">
                    <span className="truncate">{request.targetDeviceName}</span>
                    <span className="text-[var(--fc-muted)]">{formatRelativeTime(request.createdAt)}</span>
                  </li>
                ))}
              </ul>
            )}
          </div>
        </article>
      </section>
    </div>
  );
}
