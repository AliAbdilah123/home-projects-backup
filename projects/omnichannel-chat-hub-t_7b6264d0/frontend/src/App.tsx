import { FormEvent, useEffect, useMemo, useRef, useState } from 'react';
import { disconnectChannel, listChannels, pollBaileysSession, startBaileysSession } from './api';
import { AuthProvider, useAuth } from './auth';
import { navChannelsFixture } from './fixtures';
import type { BaileysSession, ChannelProvider } from './types';

const LAST_WA_SESSION_KEY = 'och.whatsapp.lastSession';

type ConnectionState = 'idle' | 'connecting' | 'qr_pending' | 'connected' | 'disconnected' | 'error';

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

function WhatsAppConnectionPanel() {
  const [displayName, setDisplayName] = useState('Support WhatsApp');
  const [channelID, setChannelID] = useState<string | null>(null);
  const [session, setSession] = useState<BaileysSession | null>(null);
  const [state, setState] = useState<ConnectionState>('idle');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [lastPollAt, setLastPollAt] = useState<string | null>(null);
  const pollTimerRef = useRef<number | null>(null);

  const clearPollTimer = () => {
    if (pollTimerRef.current !== null) {
      window.clearTimeout(pollTimerRef.current);
      pollTimerRef.current = null;
    }
  };

  const loadChannelSnapshot = async () => {
    const listed = await listChannels();
    const whatsapp = listed.channels.find((c) => c.provider === 'whatsapp_baileys');
    if (whatsapp) {
      setChannelID(whatsapp.id);
      setDisplayName(whatsapp.display_name || 'Support WhatsApp');
      setState(whatsapp.status === 'active' ? 'connected' : 'disconnected');
    } else {
      setChannelID(null);
      setSession(null);
      setState('idle');
      window.localStorage.removeItem(LAST_WA_SESSION_KEY);
    }
  };

  const schedulePoll = (sessionID: string, delayMs: number) => {
    clearPollTimer();
    pollTimerRef.current = window.setTimeout(async () => {
      try {
        const next = await pollBaileysSession(sessionID);
        setSession(next);
        setLastPollAt(new Date().toISOString());
        if (next.status === 'connected') {
          setState('connected');
        } else if (next.status === 'disconnected') {
          setState('disconnected');
        } else if (next.status === 'error') {
          setState('error');
          setError('Session entered error state. Start a new session to reconnect.');
        } else {
          setState('qr_pending');
        }
        const nextDelay = next.status === 'connected' ? 6000 : 3000;
        schedulePoll(sessionID, nextDelay);
      } catch (err: unknown) {
        setError(err instanceof Error ? err.message : 'Polling failed.');
        schedulePoll(sessionID, 5000);
      }
    }, delayMs);
  };

  useEffect(() => {
    loadChannelSnapshot().catch((err: unknown) => {
      setError(err instanceof Error ? err.message : 'Unable to load channels.');
      setState('error');
    });

    const existingSessionID = window.localStorage.getItem(LAST_WA_SESSION_KEY);
    if (existingSessionID) {
      schedulePoll(existingSessionID, 500);
    }

    return () => clearPollTimer();
  }, []);

  const start = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!displayName.trim()) {
      setError('Display name is required.');
      return;
    }

    setBusy(true);
    setError(null);
    setState('connecting');
    try {
      const started = await startBaileysSession(displayName.trim());
      setChannelID(started.channel.id);
      setSession(started.session);
      window.localStorage.setItem(LAST_WA_SESSION_KEY, started.session.id);
      setLastPollAt(new Date().toISOString());
      setState(started.session.status === 'connected' ? 'connected' : 'qr_pending');
      schedulePoll(started.session.id, 2000);
    } catch (err: unknown) {
      setState('error');
      setError(err instanceof Error ? err.message : 'Unable to start session.');
    } finally {
      setBusy(false);
    }
  };

  const disconnect = async () => {
    if (!channelID) {
      return;
    }
    setBusy(true);
    setError(null);
    try {
      await disconnectChannel(channelID);
      clearPollTimer();
      setState('disconnected');
      setSession((prev) => (prev ? { ...prev, status: 'disconnected' } : prev));
      await loadChannelSnapshot();
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : 'Unable to disconnect channel.');
    } finally {
      setBusy(false);
    }
  };

  const stateClass = `status-pill status-${state}`;

  return (
    <section className="panel" aria-live="polite">
      <h2>WhatsApp (Baileys) Connection</h2>
      <p className="muted">Start a session, scan the QR code in WhatsApp, and keep this tab open while status polling runs.</p>

      <div className="connection-meta">
        <span className={stateClass}>{state.replace('_', ' ')}</span>
        {channelID ? <span className="mono">channel: {channelID}</span> : null}
        {session?.id ? <span className="mono">session: {session.id}</span> : null}
      </div>

      {(state === 'idle' || state === 'disconnected' || state === 'error') && (
        <form className="inline-form" onSubmit={start}>
          <label htmlFor="wa-display-name">Display name</label>
          <input
            id="wa-display-name"
            value={displayName}
            onChange={(event) => setDisplayName(event.target.value)}
            placeholder="Support WhatsApp"
            disabled={busy}
            required
          />
          <button className="primary-button" type="submit" disabled={busy}>
            {busy ? 'Starting…' : 'Start session'}
          </button>
        </form>
      )}

      {session?.qr_code && state !== 'connected' && state !== 'disconnected' ? (
        <div className="qr-wrap">
          <p className="section-label">Scan this QR in WhatsApp</p>
          <pre className="qr-code">{session.qr_code}</pre>
        </div>
      ) : null}

      {lastPollAt ? <p className="muted compact">Last poll: {new Date(lastPollAt).toLocaleTimeString()}</p> : null}

      <div className="panel-actions">
        <button type="button" className="secondary-button" onClick={() => loadChannelSnapshot()} disabled={busy}>
          Refresh status
        </button>
        <button type="button" className="secondary-button danger" onClick={disconnect} disabled={busy || !channelID}>
          {busy ? 'Working…' : 'Disconnect'}
        </button>
      </div>

      {error ? <p className="form-error" role="alert">{error}</p> : null}
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
          <a href="#" className="nav-item">Inbox</a>
          <a href="#" className="nav-item">Contacts</a>
          <a href="#" className="nav-item active" aria-current="page">Settings</a>
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
          <WhatsAppConnectionPanel />
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
