"use client";

import { useCallback, useEffect, useMemo, useState } from "react";

import {
  formatRelativeTime,
  getOrCreateLocalDeviceIdentity,
  getStoredVaultKeyB64,
  type LocalDeviceIdentity,
  unwrapVaultKeyForLocalDevice,
  wrapVaultKeyForDevicePublicKey,
} from "@/lib/security/device";

type DeviceStatus = "pending" | "approved" | "revoked";

type DeviceRecord = {
  id: string;
  deviceId: string;
  displayName: string;
  status: DeviceStatus;
  keyAlgorithm: string;
  approvedAt: number | null;
  revokedAt: number | null;
  lastSeenAt: number | null;
  updatedAt: number;
};

type PendingEnrollment = {
  id: string;
  targetDeviceId: string;
  targetDeviceName: string;
  targetPublicKey: string;
  targetKeyAlgorithm: string;
  requesterDeviceId: string | null;
  createdAt: number;
};

type DevicesApiResponse = {
  devices: DeviceRecord[];
  pendingEnrollments: PendingEnrollment[];
};

async function readError(response: Response) {
  try {
    const body = (await response.json()) as { error?: string };
    return body.error || "request_failed";
  } catch {
    return "request_failed";
  }
}

export default function DevicesPage() {
  const [identity, setIdentity] = useState<LocalDeviceIdentity | null>(null);
  const [devices, setDevices] = useState<DeviceRecord[]>([]);
  const [pendingEnrollments, setPendingEnrollments] = useState<PendingEnrollment[]>([]);
  const [loading, setLoading] = useState(true);
  const [isRegistering, setIsRegistering] = useState(false);
  const [busyAction, setBusyAction] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);

  const localDevice = useMemo(() => {
    if (!identity) return null;
    return devices.find((device) => device.deviceId === identity.deviceId) ?? null;
  }, [devices, identity]);

  const localPendingRequest = useMemo(() => {
    if (!identity) return null;
    return pendingEnrollments.find((request) => request.targetDeviceId === identity.deviceId) ?? null;
  }, [pendingEnrollments, identity]);

  const approvedDevicesCount = useMemo(() => devices.filter((device) => device.status === "approved").length, [devices]);

  const fetchDevices = useCallback(async () => {
    const response = await fetch("/api/devices", { method: "GET", cache: "no-store" });
    if (!response.ok) {
      throw new Error(await readError(response));
    }
    const data = (await response.json()) as DevicesApiResponse;
    setDevices(data.devices ?? []);
    setPendingEnrollments(data.pendingEnrollments ?? []);
  }, []);

  const ensureVaultKeyAvailable = useCallback(async (currentIdentity: LocalDeviceIdentity) => {
    if (getStoredVaultKeyB64()) {
      return;
    }
    const response = await fetch(`/api/vault/wrapped-key?deviceId=${encodeURIComponent(currentIdentity.deviceId)}`, {
      method: "GET",
      cache: "no-store",
    });
    if (!response.ok) {
      return;
    }

    const body = (await response.json()) as { wrappedVaultKeyB64?: string };
    if (!body.wrappedVaultKeyB64) {
      return;
    }
    await unwrapVaultKeyForLocalDevice(currentIdentity, body.wrappedVaultKeyB64);
  }, []);

  const bootstrap = useCallback(async () => {
    setLoading(true);
    setError(null);

    try {
      const currentIdentity = await getOrCreateLocalDeviceIdentity();
      setIdentity(currentIdentity);

      await fetchDevices();
      await ensureVaultKeyAvailable(currentIdentity);
    } catch (err) {
      setError(err instanceof Error ? err.message : "unable_to_initialize_devices");
    } finally {
      setLoading(false);
    }
  }, [ensureVaultKeyAvailable, fetchDevices]);

  useEffect(() => {
    void bootstrap();
  }, [bootstrap]);

  async function handleRegisterCurrentDevice() {
    if (!identity) return;
    setIsRegistering(true);
    setError(null);
    setNotice(null);

    try {
      const enrollResponse = await fetch("/api/devices", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          deviceId: identity.deviceId,
          displayName: identity.displayName,
          publicKey: identity.publicKeySpkiB64,
          keyAlgorithm: identity.keyAlgorithm,
          requesterDeviceId: identity.deviceId,
        }),
      });
      if (!enrollResponse.ok) {
        throw new Error(await readError(enrollResponse));
      }

      await fetchDevices();
      await ensureVaultKeyAvailable(identity);
      setNotice("Device enrollment request submitted.");
    } catch (err) {
      setError(err instanceof Error ? err.message : "enrollment_failed");
    } finally {
      setIsRegistering(false);
    }
  }

  async function handleApproveWithRecovery(requestId: string) {
    if (!identity) return;
    setBusyAction(`approve-recovery-${requestId}`);
    setError(null);
    setNotice(null);
    try {
      const wrapped = await wrapVaultKeyForDevicePublicKey(identity.publicKeySpkiB64, { allowGenerate: true });
      const response = await fetch("/api/devices/approve", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          requestId,
          recoveryUsed: true,
          wrappedVaultKeyB64: wrapped.wrappedVaultKeyB64,
          wrapperAlgorithm: wrapped.wrapperAlgorithm,
          keyVersion: wrapped.keyVersion,
        }),
      });
      if (!response.ok) {
        throw new Error(await readError(response));
      }
      await fetchDevices();
      await ensureVaultKeyAvailable(identity);
      setNotice("Device approved using recovery fallback.");
    } catch (err) {
      setError(err instanceof Error ? err.message : "approval_failed");
    } finally {
      setBusyAction(null);
    }
  }

  async function handleApproveFromCurrentDevice(request: PendingEnrollment) {
    if (!identity) return;
    setBusyAction(`approve-${request.id}`);
    setError(null);
    setNotice(null);

    try {
      await ensureVaultKeyAvailable(identity);
      const wrapped = await wrapVaultKeyForDevicePublicKey(request.targetPublicKey, { allowGenerate: false });

      const response = await fetch("/api/devices/approve", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          requestId: request.id,
          approverDeviceId: identity.deviceId,
          recoveryUsed: false,
          wrappedVaultKeyB64: wrapped.wrappedVaultKeyB64,
          wrapperAlgorithm: wrapped.wrapperAlgorithm,
          keyVersion: wrapped.keyVersion,
        }),
      });
      if (!response.ok) {
        throw new Error(await readError(response));
      }

      await fetchDevices();
      setNotice(`Approved ${request.targetDeviceName}.`);
    } catch (err) {
      setError(err instanceof Error ? err.message : "approval_failed");
    } finally {
      setBusyAction(null);
    }
  }

  async function handleRevoke(targetDeviceId: string) {
    setBusyAction(`revoke-${targetDeviceId}`);
    setError(null);
    setNotice(null);
    try {
      const response = await fetch("/api/devices/revoke", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          targetDeviceId,
          actorDeviceId: localDevice?.status === "approved" ? localDevice.deviceId : undefined,
        }),
      });

      if (!response.ok) {
        throw new Error(await readError(response));
      }

      await fetchDevices();
      setNotice("Device revoked.");
    } catch (err) {
      setError(err instanceof Error ? err.message : "revoke_failed");
    } finally {
      setBusyAction(null);
    }
  }

  return (
    <div className="space-y-6">
      <section className="rounded-2xl border border-white/10 bg-white/[0.02] p-6">
        <h1 className="text-2xl font-semibold">Devices</h1>
        <p className="mt-3 max-w-3xl text-sm text-[var(--fc-muted)]">
          Login alone never approves decrypt access. New devices stay pending until approved by an existing approved
          device, or by recovery fallback.
        </p>
        <div className="mt-4 grid gap-3 text-sm md:grid-cols-3">
          <div className="rounded-xl border border-white/10 bg-black/20 px-4 py-3">
            <p className="text-xs uppercase tracking-[0.14em] text-[var(--fc-muted)]">Approved devices</p>
            <p className="mt-1 text-xl font-semibold">{approvedDevicesCount}</p>
          </div>
          <div className="rounded-xl border border-white/10 bg-black/20 px-4 py-3">
            <p className="text-xs uppercase tracking-[0.14em] text-[var(--fc-muted)]">Pending requests</p>
            <p className="mt-1 text-xl font-semibold">{pendingEnrollments.length}</p>
          </div>
          <div className="rounded-xl border border-white/10 bg-black/20 px-4 py-3">
            <p className="text-xs uppercase tracking-[0.14em] text-[var(--fc-muted)]">This browser</p>
            <p className="mt-1 truncate text-sm font-semibold">{identity?.displayName ?? "Initializing..."}</p>
          </div>
        </div>
        <div className="mt-4 flex flex-wrap items-center gap-3">
          <button
            type="button"
            onClick={() => void handleRegisterCurrentDevice()}
            disabled={!identity || isRegistering}
            className="inline-flex rounded-full bg-[var(--fc-accent)] px-4 py-2 text-xs font-semibold text-[#2b1708] transition hover:brightness-110 disabled:cursor-not-allowed disabled:opacity-50"
          >
            {isRegistering ? "Submitting..." : "Register this browser"}
          </button>
          <p className="text-xs text-[var(--fc-muted)]">
            Device records are created only when you explicitly register from this page.
          </p>
        </div>
        {notice ? <p className="mt-4 text-sm text-emerald-300">{notice}</p> : null}
        {error ? <p className="mt-4 text-sm text-red-300">Error: {error}</p> : null}
      </section>

      <section className="rounded-2xl border border-white/10 bg-black/20 p-4">
        <div className="grid grid-cols-[1.2fr_0.7fr_0.8fr_0.8fr] gap-3 border-b border-white/10 px-3 pb-3 text-xs uppercase tracking-[0.14em] text-[var(--fc-muted)]">
          <p>Device</p>
          <p>Status</p>
          <p>Last seen</p>
          <p>Action</p>
        </div>
        <div className="divide-y divide-white/10">
          {loading ? (
            <p className="px-3 py-4 text-sm text-[var(--fc-muted)]">Loading devices...</p>
          ) : devices.length === 0 ? (
            <p className="px-3 py-4 text-sm text-[var(--fc-muted)]">No devices registered yet.</p>
          ) : (
            devices.map((device) => {
              const statusColor =
                device.status === "approved"
                  ? "text-emerald-300"
                  : device.status === "pending"
                    ? "text-amber-300"
                    : "text-rose-300";
              return (
                <div key={device.id} className="grid grid-cols-[1.2fr_0.7fr_0.8fr_0.8fr] gap-3 px-3 py-3 text-sm">
                  <p>
                    {device.displayName}
                    {identity?.deviceId === device.deviceId ? (
                      <span className="ml-2 text-xs text-[var(--fc-muted)]">(this browser)</span>
                    ) : null}
                  </p>
                  <p className={statusColor}>{device.status}</p>
                  <p className="text-[var(--fc-muted)]">{formatRelativeTime(device.lastSeenAt)}</p>
                  <button
                    type="button"
                    onClick={() => handleRevoke(device.deviceId)}
                    disabled={busyAction === `revoke-${device.deviceId}` || device.status === "revoked"}
                    className="justify-self-start rounded-full border border-white/20 px-3 py-1 text-xs transition hover:border-[var(--fc-accent)]/60 disabled:cursor-not-allowed disabled:opacity-50"
                  >
                    {busyAction === `revoke-${device.deviceId}` ? "Revoking..." : "Revoke"}
                  </button>
                </div>
              );
            })
          )}
        </div>
      </section>

      <section className="rounded-2xl border border-white/10 bg-black/20 p-4">
        <div className="flex items-center justify-between border-b border-white/10 px-3 pb-3">
          <h2 className="text-sm font-semibold uppercase tracking-[0.14em] text-[var(--fc-muted)]">Pending approvals</h2>
        </div>
        <div className="divide-y divide-white/10">
          {pendingEnrollments.length === 0 ? (
            <p className="px-3 py-4 text-sm text-[var(--fc-muted)]">No pending enrollment requests.</p>
          ) : (
            pendingEnrollments.map((request) => {
              const isLocalRequest = request.targetDeviceId === identity?.deviceId;
              const canApproveFromDevice = localDevice?.status === "approved" && !isLocalRequest;
              return (
                <div key={request.id} className="flex flex-col gap-3 px-3 py-3 text-sm md:flex-row md:items-center md:justify-between">
                  <div>
                    <p className="font-medium">{request.targetDeviceName}</p>
                    <p className="text-xs text-[var(--fc-muted)]">Requested {formatRelativeTime(request.createdAt)}</p>
                  </div>
                  <div className="flex flex-wrap gap-2">
                    {isLocalRequest && localPendingRequest ? (
                      <button
                        type="button"
                        onClick={() => handleApproveWithRecovery(request.id)}
                        disabled={busyAction === `approve-recovery-${request.id}`}
                        className="rounded-full bg-[var(--fc-accent)] px-3 py-1 text-xs font-semibold text-[#2b1708] transition hover:brightness-110 disabled:cursor-not-allowed disabled:opacity-50"
                      >
                        {busyAction === `approve-recovery-${request.id}`
                          ? "Approving..."
                          : "Approve with recovery fallback"}
                      </button>
                    ) : null}
                    <button
                      type="button"
                      onClick={() => handleApproveFromCurrentDevice(request)}
                      disabled={!canApproveFromDevice || busyAction === `approve-${request.id}`}
                      className="rounded-full border border-white/20 px-3 py-1 text-xs transition hover:border-[var(--fc-accent)]/60 disabled:cursor-not-allowed disabled:opacity-50"
                    >
                      {busyAction === `approve-${request.id}` ? "Approving..." : "Approve from this device"}
                    </button>
                  </div>
                </div>
              );
            })
          )}
        </div>
      </section>
    </div>
  );
}
