import { useConversations } from '../hooks/useMessages';
import { useChannels } from '../hooks/useChannels';
import type { Conversation } from '../services/api';

export default function ConversationList({ onSelect, activeId }: { onSelect: (c: Conversation) => void, activeId?: string }) {
  const { channels } = useChannels();
  const { conversations, loading, refresh } = useConversations();

  return (
    <section className="panel">
      <div className="header">
        <h3>Conversations</h3>
        <button onClick={refresh}>Refresh</button>
      </div>
      {!loading && conversations.length === 0 && <div className="placeholder">No conversations yet.</div>}
      {!loading && conversations.map(c => {
        const channel = channels.find(ch => ch.id === c.channel_id);
        return (
          <button key={c.id} className={`item ${c.id === activeId ? 'active' : ''}`} onClick={() => onSelect(c)}>
            <div className="meta">
              <strong>{c.display_name}</strong>
              <span className="badge">{channel?.platform || c.channel_id}</span>
            </div>
            <div className="muted small">{c.contact}</div>
          </button>
        );
      })}
      <style>{css}</style>
    </section>
  );
}

const css = `
  .panel {
    width: 360px;
    min-width: 360px;
    border-right: 1px solid rgba(255,255,255,.1);
    display: flex;
    flex-direction: column;
    background: linear-gradient(180deg, #0b1120 0%, #09101e 100%);
  }
  .header {
    padding: 14px 18px;
    display: flex;
    justify-content: space-between;
    align-items: center;
    border-bottom: 1px solid rgba(255,255,255,.1);
  }
  h3 { margin: 0; font-size: 18px; }
  .item {
    text-align: left;
    display: block;
    padding: 14px 18px;
    border-bottom: 1px solid rgba(255,255,255,.05);
    cursor: pointer;
    transition: background .2s;
    background: transparent;
    color: inherit;
    width: 100%;
  }
  .item:hover { background: rgba(255,255,255,.04); }
  .item.active { background: rgba(255,255,255,.08); }
  .meta { display: flex; justify-content: space-between; align-items: center; gap: 10px; }
  .badge { font-size: 11px; background: #1f2a38; padding: 4px 10px; border-radius: 999px; }
  .muted { color: #a1afc2; }
  .placeholder { padding: 18px; color: #a1afc2; }
`;
