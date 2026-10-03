import { useEffect, useState } from "react";
import { loadRuntimeConfig, type PublicRuntimeConfig } from "./config/runtimeConfig";

type Connectivity = "checking" | "connected" | "unavailable";

export function App() {
  const [config, setConfig] = useState<PublicRuntimeConfig | null>(null);
  const [connectivity, setConnectivity] = useState<Connectivity>("checking");

  useEffect(() => {
    let cancelled = false;
    void (async () => {
      try {
        const runtimeConfig = await loadRuntimeConfig();
        const response = await fetch(`${runtimeConfig.apiBaseURL}/health/ready`, { cache: "no-store" });
        if (cancelled) return;
        setConfig(runtimeConfig);
        setConnectivity(response.ok ? "connected" : "unavailable");
      } catch {
        if (!cancelled) setConnectivity("unavailable");
      }
    })();
    return () => { cancelled = true; };
  }, []);

  return (
    <main className="shell">
      <header>
        <p className="eyebrow">FleetForge</p>
        <h1>Firmware rollout control plane</h1>
        <p>Operational foundation — no campaign controls are available yet.</p>
      </header>
      <section aria-label="API connectivity" className="status-card">
        <h2>API connectivity</h2>
        <p data-status={connectivity}>{connectivity}</p>
        <dl>
          <div><dt>Build</dt><dd>{import.meta.env.VITE_BUILD_VERSION ?? "dev"}</dd></div>
          <div><dt>API</dt><dd>{config?.apiBaseURL ?? "Public runtime configuration unavailable"}</dd></div>
        </dl>
      </section>
    </main>
  );
}
