import { FormEvent, useMemo, useState } from 'react';
import { AuthProvider, useAuth } from './auth';
import { navChannelsFixture } from './fixtures';
import type { ChannelProvider } from './types';

function providerClass(provider: ChannelProvider): string {
  return `channel-badge channel-${provider}`;
}

function LoadingState() {
  return (
    <main className="state-screen" aria-live="polite">
      <p className="state-title">Loading workspace…</p>
      <p className="state-copy">Checking your current session.</p>
    </main>
  );
}

function ErrorState({ message, onRetry }: { message: string; onRetry: () => void }) {
  return (
    <main className="state-screen" role="alert" aria-live="assertive">
      <p className="state-title">Unable to continue</p>
      <p className="state-copy">{message}</p>
      <button type="button" className="primary-button" onClick={onRetry}>Retry</button>
    </main>
  );
}

function LoginScreen() {
  const { login, error } = useAuth();
  const [email, setEmail] = useState('owner@example.com');
  const [password, setPassword] = useState('owner123');
  const [pending, setPending] = useState(false);

  const canSubmit = email.trim().length > 0 && password.length > 0 && !pending;

  const onSubmit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    setPending(true);
    try {
      await login({ email, password });
    } catch {
      // surfaced in auth.error
    } finally {
      setPending(false);
    }
  };

  return (
    <main className="auth-screen">
      <section className="auth-card" aria-labelledby="login-heading">
        <h1 id="login-heading">Omnichannel Chat Hub</h1>
        <p className="muted">Sign in to access conversations and channels.</p>
        <form onSubmit={onSubmit} className="auth-form">
          <label htmlFor="email">Email</label>
          <input id="email" name="email" type="email" autoComplete="email" value={email} onChange={(e) => setEmail(e.target.value)} required />

          <label htmlFor="password">Password</label>
          <input id="password" name="password" type="password" autoComplete="current-password" value={password} onChange={(e) => setPassword(e.target.value)} required />

          {error ? <p className="form-error" role="alert">{error}</p> : null}
          <button type="submit" className="primary-button" disabled={!canSubmit}>
            {pending ? 'Signing in…' : 'Sign in'}
          </button>
        </form>
      </section>
    </main>
  );
}

function EmptyState() {
  return (
    <section className="panel">
      <h2>Inbox</h2>
      <p className="muted">No conversations yet. Connect a channel and wait for inbound messages.</p>
    </section>
  );
}

function DevTestingPanel() {
  const sampleCurl = `curl -X POST http://127.0.0.1:8080/api/v1/webhooks/dev/inbound \\
  -H 'Authorization: Bearer <owner-token>' \\
  -H 'Content-Type: application/json' \\
  -d '{
    "channel_external_id":"dev-wa-1",
    "contact_external_id":"15551234567",
    "message_external_id":"msg-1",
    "body":"hello from local dev"
  }'`;

  return (
    <section className="panel">
      <h2>Dev inbound test fixture</h2>
      <p className="muted">
        Enabled only when backend config <code>ENABLE_DEV_WEBHOOKS=true</code> is set. This endpoint simulates inbound
        channel traffic without scanning a real WhatsApp QR.
      </p>
      <pre className="dev-snippet" aria-label="Dev inbound webhook example">{sampleCurl}</pre>
    </section>
  );
}

function AppShell() {
  const { user, logout } = useAuth();
  const [isLoggingOut, setIsLoggingOut] = useState(false);

  const channelSummary = useMemo(() => {
    const active = navChannelsFixture.filter((channel) => channel.status === 'active').length;
    return `${active}/${navChannelsFixture.length} active`;
  }, []);

  return (
    <div className="app-layout">
      <aside className="sidebar" aria-label="Primary navigation">
        <h1>Omnichannel</h1>
        <nav>
          <a href="#" className="nav-item active" aria-current="page">Inbox</a>
          <a href="#" className="nav-item">Contacts</a>
          <a href="#" className="nav-item">Settings</a>
        </nav>
        <div className="channel-list" aria-label="Channel status">
          <p className="section-label">Channels · {channelSummary}</p>
          <ul>
            {navChannelsFixture.map((channel) => (
              <li key={channel.id}>
                <span className={providerClass(channel.provider)}>{channel.label}</span>
              </li>
            ))}
          </ul>
        </div>
      </aside>

      <div className="main-column">
        <header className="topbar">
          <div>
            <p className="section-label">Signed in</p>
            <strong>{user?.name}</strong>
            <p className="muted compact">{user?.email} · {user?.role}</p>
          </div>
          <button
            type="button"
            className="secondary-button"
            disabled={isLoggingOut}
            onClick={async () => {
              setIsLoggingOut(true);
              await logout();
              setIsLoggingOut(false);
            }}
          >
            {isLoggingOut ? 'Signing out…' : 'Sign out'}
          </button>
        </header>

        <main className="content" aria-live="polite">
          <EmptyState />
          <DevTestingPanel />
        </main>
      </div>
    </div>
  );
}

function AuthRouter() {
  const { status } = useAuth();

  if (status === 'loading') {
    return <LoadingState />;
  }
  if (status === 'unauthenticated') {
    return <LoginScreen />;
  }

  return <AppShell />;
}

function AuthenticatedBoundary() {
  const [retryTick, setRetryTick] = useState(0);

  if (retryTick < 0) {
    return null;
  }

  try {
    return <AuthRouter key={retryTick} />;
  } catch (err: unknown) {
    const message = err instanceof Error ? err.message : 'Unexpected rendering error.';
    return <ErrorState message={message} onRetry={() => setRetryTick((x) => x + 1)} />;
  }
}

export default function App() {
  return (
    <AuthProvider>
      <AuthenticatedBoundary />
    </AuthProvider>
  );
}
