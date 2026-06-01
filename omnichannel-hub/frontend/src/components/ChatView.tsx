import { useEffect, useRef, useState } from 'react';
import { useMessages } from '../hooks/useMessages';

export default function ChatView({ conversation }: { conversation?: any }) {
  const { messages, loading, post } = useMessages(conversation?.id);
  const [value, setValue] = useState('');
  const endRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    endRef.current?.scrollIntoView({ behavior: 'smooth' });
  }, [messages]);

  if (!conversation) return <div className="empty">Select a conversation to chat.</div>;

  return (
    <section className="chat">
      <header className="title">
        <div>
          <div className="big">{conversation.display_name}</div>
          <div className="muted small">{conversation.contact}</div>
        </div>
      </header>

      <div className="messages">
        {!loading && messages.length === 0 && <div className="placeholder">No messages yet.</div>}
        {messages.map(m => (
          <div key={m.id} className={`msg ${m.direction}`}>
            <div className="bubble">{m.content}</div>
            <div className="meta muted small">
              <span>{m.direction}</span>
              <span>{m.sender}</span>
              <span>{new Date(m.created_at).toLocaleTimeString()}</span>
            </div>
          </div>
        ))}
        <div ref={endRef} />
      </div>

      <form className="composer" onSubmit={e => { e.preventDefault(); if (value.trim()) { post(value.trim()); setValue(''); } }}>
        <input value={value} onChange={e => setValue(e.target.value)} placeholder="Type a message…" />
        <button type="submit" disabled={!value.trim()}>Send</button>
      </form>
      <style>{css}</style>
    </section>
  );
}

const css = `
  .chat { flex: 1; display: flex; flex-direction: column; min-width: 0; }
  .title { padding: 18px; border-bottom: 1px solid rgba(255,255,255,.1); }
  .big { font-size: 20px; }
  .muted { color: #a1afc2; }
  .messages {
    flex: 1;
    min-height: 300px;
    overflow-y: auto;
    padding: 18px;
    display: flex;
    flex-direction: column;
    gap: 12px;
  }
  .msg { display: flex; flex-direction: column; max-width: 80%; }
  .msg.inbound { align-self: flex-start; }
  .msg.outbound { align-self: flex-end; }
  .bubble {
    background: #111b2e;
    border: 1px solid #223044;
    padding: 12px 16px;
    border-radius: 18px;
  }
  .msg.outbound .bubble { background: #0f2a38; border-color: #0f3a4e; }
  .msg .meta { font-size: 11px; display: flex; gap: 10px; margin-top: 4px; }
  .empty, .placeholder { color: #a1afc2; padding: 18px; }
  .composer { display: flex; gap: 8px; padding: 16px; border-top: 1px solid rgba(255,255,255,.1); }
  input {
    flex: 1;
    background: #0b1220;
    color: inherit;
    border: 1px solid #2a3a52;
    padding: 12px;
    border-radius: 12px;
  }
  button {
    background: #0f3a4e;
    color: inherit;
    border: 1px solid #0f4a64;
    padding: 10px 16px;
    border-radius: 12px;
  }
`;
