import React, { useEffect, useState } from 'react';
import { createRoot } from 'react-dom/client';
import './styles.css';

type Health = {
  status: string;
  database: string;
  time: string;
};

function App() {
  const [health, setHealth] = useState<Health | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const apiBase = import.meta.env.VITE_API_BASE_URL ?? '/api/v1';
    fetch(`${apiBase}/health`)
      .then((response) => {
        if (!response.ok) {
          throw new Error(`health returned ${response.status}`);
        }
        return response.json() as Promise<Health>;
      })
      .then(setHealth)
      .catch((err: unknown) => setError(err instanceof Error ? err.message : String(err)));
  }, []);

  return (
    <main className="shell">
      <section className="hero">
        <p className="eyebrow">Baileys-powered MVP scaffold</p>
        <h1>Omnichannel Chat Hub</h1>
        <p>
          A TypeScript/React + Go + SQLite foundation for WhatsApp-first shared inbox workflows.
        </p>
      </section>
      <section className="card">
        <h2>Backend health</h2>
        {health ? (
          <dl>
            <dt>Status</dt>
            <dd>{health.status}</dd>
            <dt>Database</dt>
            <dd>{health.database}</dd>
            <dt>Checked</dt>
            <dd>{health.time}</dd>
          </dl>
        ) : (
          <p>{error ? `Unavailable: ${error}` : 'Checking /api/v1/health…'}</p>
        )}
      </section>
    </main>
  );
}

createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>
);
