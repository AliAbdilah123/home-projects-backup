import { FormEvent, useEffect, useMemo, useState } from 'react';
import {
  getConversationMessages,
  listChannels,
  listConversations,
  patchConversation,
  sendConversationMessage
} from './api';
import { AuthProvider, useAuth } from './auth';
import type { ChannelRecord, ConversationMessage, ConversationRecord } from './types';

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

function providerClass(provider: string): string {
  if (provider.includes('whatsapp')) return 'channel-badge channel-whatsapp';
  if (provider.includes('telegram')) return 'channel-badge channel-telegram';
  if (provider.includes('instagram')) return 'channel-badge channel-instagram';
  if (provider.includes('facebook')) return 'channel-badge channel-facebook';
  return 'channel-badge channel-webchat';
}

function InboxPanel() {
  const [channels, setChannels] = useState<ChannelRecord[]>([]);
  const [conversations, setConversations] = useState<ConversationRecord[]>([]);
  const [selectedConversationID, setSelectedConversationID] = useState<string | null>(null);
  const [messages, setMessages] = useState<ConversationMessage[]>([]);
  const [statusFilter, setStatusFilter] = useState('');
  const [channelFilter, setChannelFilter] = useState('');
  const [assignedToFilter, setAssignedToFilter] = useState('');
  const [unreadOnly, setUnreadOnly] = useState(false);
  const [loadingList, setLoadingList] = useState(true);
  const [loadingTimeline, setLoadingTimeline] = useState(false);
  const [listError, setListError] = useState<string | null>(null);
  const [timelineError, setTimelineError] = useState<string | null>(null);
  const [patchBusy, setPatchBusy] = useState(false);
  const [replyBusy, setReplyBusy] = useState(false);
  const [replyBody, setReplyBody] = useState('');

  const selectedConversation = useMemo(
    () => conversations.find((conversation) => conversation.id === selectedConversationID) ?? null,
    [conversations, selectedConversationID]
  );

  const channelMap = useMemo(() => {
    const map = new Map<string, ChannelRecord>();
    channels.forEach((channel) => map.set(channel.id, channel));
    return map;
  }, [channels]);

  const visibleConversations = useMemo(() => {
    if (!unreadOnly) {
      return conversations;
    }
    return conversations.filter((conversation) => conversation.unread_count > 0);
  }, [conversations, unreadOnly]);

  const loadConversations = async () => {
    setLoadingList(true);
    setListError(null);
    try {
      const [channelsRes, conversationsRes] = await Promise.all([
        listChannels(),
        listConversations({
          status: statusFilter || undefined,
          channel: channelFilter || undefined,
          assigned_to: assignedToFilter || undefined,
          page: 1,
          limit: 100
        })
      ]);
      setChannels(channelsRes.channels);
      setConversations(conversationsRes.conversations);
      if (conversationsRes.conversations.length === 0) {
        setSelectedConversationID(null);
        setMessages([]);
      } else if (!selectedConversationID || !conversationsRes.conversations.some((c) => c.id === selectedConversationID)) {
        setSelectedConversationID(conversationsRes.conversations[0].id);
      }
    } catch (err: unknown) {
      setListError(err instanceof Error ? err.message : 'Unable to load inbox conversations.');
    } finally {
      setLoadingList(false);
    }
  };

  const loadMessages = async (conversationID: string) => {
    setLoadingTimeline(true);
    setTimelineError(null);
    try {
      const res = await getConversationMessages(conversationID, 1, 100);
      setMessages(res.messages);
    } catch (err: unknown) {
      setTimelineError(err instanceof Error ? err.message : 'Unable to load timeline.');
    } finally {
      setLoadingTimeline(false);
    }
  };

  useEffect(() => {
    loadConversations();
  }, [statusFilter, channelFilter, assignedToFilter]);

  useEffect(() => {
    if (selectedConversationID) {
      loadMessages(selectedConversationID);
    }
  }, [selectedConversationID]);

  const applyPatch = async (payload: { status?: string; assigned_to?: string; mark_read?: boolean }) => {
    if (!selectedConversationID) {
      return;
    }
    setPatchBusy(true);
    try {
      await patchConversation(selectedConversationID, payload);
      await loadConversations();
      await loadMessages(selectedConversationID);
    } catch (err: unknown) {
      setTimelineError(err instanceof Error ? err.message : 'Unable to update conversation.');
    } finally {
      setPatchBusy(false);
    }
  };

  const sendReply = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!selectedConversationID || !replyBody.trim()) {
      return;
    }
    setReplyBusy(true);
    setTimelineError(null);
    try {
      await sendConversationMessage(selectedConversationID, replyBody.trim());
      setReplyBody('');
      await loadMessages(selectedConversationID);
      await loadConversations();
    } catch (err: unknown) {
      setTimelineError(err instanceof Error ? err.message : 'Unable to send message.');
    } finally {
      setReplyBusy(false);
    }
  };

  return (
    <section className="inbox-grid" aria-live="polite">
      <div className="inbox-list-panel">
        <div className="inbox-toolbar">
          <h2>Unified inbox</h2>
          <button type="button" className="secondary-button" onClick={() => loadConversations()} disabled={loadingList}>
            Refresh
          </button>
        </div>

        <div className="filters-grid">
          <select value={statusFilter} onChange={(e) => setStatusFilter(e.target.value)}>
            <option value="">All status</option>
            <option value="open">Open</option>
            <option value="pending">Pending</option>
            <option value="closed">Closed</option>
          </select>
          <select value={channelFilter} onChange={(e) => setChannelFilter(e.target.value)}>
            <option value="">All channels</option>
            {channels.map((channel) => (
              <option key={channel.id} value={channel.id}>{channel.display_name || channel.provider}</option>
            ))}
          </select>
          <input
            value={assignedToFilter}
            onChange={(e) => setAssignedToFilter(e.target.value)}
            placeholder="Assigned to"
            aria-label="Assigned to filter"
          />
          <label className="checkbox-row">
            <input type="checkbox" checked={unreadOnly} onChange={(e) => setUnreadOnly(e.target.checked)} />
            Unread only
          </label>
        </div>

        {loadingList ? <p className="muted">Loading conversations…</p> : null}
        {listError ? <p className="form-error" role="alert">{listError}</p> : null}
        {!loadingList && !listError && visibleConversations.length === 0 ? (
          <p className="empty-box">No conversations match the current filters.</p>
        ) : null}

        <ul className="conversation-list" role="listbox" aria-label="Conversations">
          {visibleConversations.map((conversation) => {
            const channel = channelMap.get(conversation.channel_id);
            const active = selectedConversationID === conversation.id;
            return (
              <li key={conversation.id}>
                <button
                  type="button"
                  className={`conversation-item ${active ? 'active' : ''}`}
                  onClick={() => setSelectedConversationID(conversation.id)}
                >
                  <div className="conversation-item-top">
                    <span className={providerClass(channel?.provider ?? 'webchat')}>{channel?.display_name || channel?.provider || 'Unknown'}</span>
                    <span className="status-pill">{conversation.status}</span>
                  </div>
                  <div className="conversation-item-meta">
                    <span className="mono">{conversation.id}</span>
                    <span>Unread: {conversation.unread_count}</span>
                  </div>
                  <p className="muted compact">Assigned: {conversation.assigned_to || 'Unassigned'}</p>
                </button>
              </li>
            );
          })}
        </ul>
      </div>

      <div className="timeline-panel">
        {!selectedConversation ? (
          <p className="empty-box">Select a conversation to view its timeline.</p>
        ) : (
          <>
            <div className="timeline-header">
              <div>
                <h3>{selectedConversation.id}</h3>
                <p className="muted compact">Channel: {channelMap.get(selectedConversation.channel_id)?.display_name || selectedConversation.channel_id}</p>
              </div>
              <div className="timeline-controls">
                <select
                  value={selectedConversation.status}
                  onChange={(e) => applyPatch({ status: e.target.value })}
                  disabled={patchBusy}
                >
                  <option value="open">Open</option>
                  <option value="pending">Pending</option>
                  <option value="closed">Closed</option>
                </select>
                <button type="button" className="secondary-button" onClick={() => applyPatch({ mark_read: true })} disabled={patchBusy}>
                  Mark read
                </button>
              </div>
            </div>

            <form
              className="assignment-row"
              onSubmit={async (event) => {
                event.preventDefault();
                const value = (new FormData(event.currentTarget).get('assigned_to') as string).trim();
                await applyPatch({ assigned_to: value });
              }}
            >
              <input name="assigned_to" defaultValue={selectedConversation.assigned_to} placeholder="Assign to user email" />
              <button type="submit" className="secondary-button" disabled={patchBusy}>Update assignment</button>
            </form>

            {loadingTimeline ? <p className="muted">Loading timeline…</p> : null}
            {timelineError ? <p className="form-error" role="alert">{timelineError}</p> : null}
            {!loadingTimeline && messages.length === 0 ? <p className="empty-box">No messages in this conversation yet.</p> : null}

            <ol className="timeline-list">
              {messages.map((message) => (
                <li key={message.id} className={`message-bubble ${message.direction === 'outbound' ? 'outbound' : 'inbound'}`}>
                  <p>{message.body}</p>
                  <div className="message-meta">
                    <span>{message.direction}</span>
                    <span>{new Date(message.sent_at).toLocaleString()}</span>
                  </div>
                </li>
              ))}
            </ol>

            <form className="reply-form" onSubmit={sendReply}>
              <label htmlFor="reply-body" className="section-label">Reply composer</label>
              <textarea
                id="reply-body"
                value={replyBody}
                onChange={(event) => setReplyBody(event.target.value)}
                placeholder="Type a reply message"
                rows={4}
                required
              />
              <button type="submit" className="primary-button" disabled={replyBusy || !replyBody.trim()}>
                {replyBusy ? 'Sending…' : 'Send message'}
              </button>
            </form>
          </>
        )}
      </div>
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

  return (
    <div className="app-layout">
      <aside className="sidebar" aria-label="Primary navigation">
        <h1>Omnichannel</h1>
        <nav>
          <a href="#" className="nav-item active" aria-current="page">Inbox</a>
          <a href="#" className="nav-item">Contacts</a>
          <a href="#" className="nav-item">Settings</a>
        </nav>
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
          <InboxPanel />
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
