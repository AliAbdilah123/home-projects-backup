import { useChannels } from '../hooks/useChannels';
import type { Channel } from '../services/api';
import { useState } from 'react';

function PlatformIcon({ platform }: { platform: string }) {
  const map: Record<string, string> = {
    whatsapp: '📱',
    instagram: '📸',
    facebook: '💬',
    telegram: '✈️',
    email: '📧',
  };
  return <span aria-hidden="true">{map[platform] ?? '•'}</span>;
}

export default function ChannelSidebar() {
  const { channels, loading, add, connect, disconnect } = useChannels();
  const [form, setForm] = useState({ platform: 'whatsapp', identifier: '', name: '' });

  return (
    <aside className="sidebar">
      <h3>Channels</h3>
      <div className="card compact">
        <select value={form.platform} onChange={e => setForm({ ...form, platform: e.target.value })}>
          <option>whatsapp</option>
          <option>instagram</option>
          <option>facebook</option>
          <option>telegram</option>
          <option>email</option>
        </select>
        <input placeholder="Identifier / phone / username" value={form.identifier} onChange={e => setForm({ ...form, identifier: e.target.value })} />
        <input placeholder="Display name (optional)" value={form.name} onChange={e => setForm({ ...form, name: e.target.value })} />
        <button
          onClick={() => add({ platform: form.platform, identifier: form.identifier, name: form.name || `${form.platform} account` })}
        >Add channel</button>
      </div>
      {!loading && channels.length > 0 && (
        <ul className="list">
          {channels.map(c => <ChannelRow key={c.id} channel={c} onConnect={() => connect(c.id)} onDisconnect={() => disconnect(c.id)} />)}
        </ul>
      )}
      <style>{css}</style>
    </aside>
  );
}

function ChannelRow({ channel, onConnect, onDisconnect }: { channel: Channel, onConnect: () => void, onDisconnect: () => void }) {
  return (
    <li className="item">
      <div className="row">
        <PlatformIcon platform={channel.platform} />
        <div>
          <div className="info"><strong>{channel.name}</strong> <span className="badge">{channel.platform}</span></div>
          <div className="small">{channel.identifier}</div>
        </div>
      </div>
      <div className="actions">
        <button onClick={onConnect}>Connect</button>
        <button onClick={onDisconnect}>Disconnect</button>
      </div>
      <style>{css}</style>
    </li>
  );
}

const css = `
  .sidebar {
    width: 340px;
    min-width: 340px;
    border-right: 1px solid rgba(255,255,255,.1);
    padding: 16px;
    display: flex;
    flex-direction: column;
    gap: 12px;
  }
  h3 { margin: 0; font-size: 18px; }
  .list { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 10px; }
  .item { display: flex; flex-direction: column; gap: 8px; }
  .row { display: flex; gap: 10px; align-items: center; }
  .info { display: flex; gap: 6px; align-items: center; flex-wrap: wrap; justify-content: space-between;}
  .small { color: #a1afc2; font-size: 12px; }
  .badge {
    background: #1f2a38;
    padding: 4px 10px;
    font-size: 11px;
    border-radius: 999px;
    letter-spacing: .4px;
  }
  .actions { display: flex; gap: 8px; }
  select, input {
    width: 100%;
    background: #0b1220;
    color: inherit;
    border: 1px solid #2a3a52;
    padding: 10px;
    border-radius: 10px;
  }
  button { background: #1f2a38; color: inherit; border: 1px solid #2a3a52; padding: 8px; border-radius: 10px; }
`;
