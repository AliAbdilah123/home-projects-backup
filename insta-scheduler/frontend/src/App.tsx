import { useState, useEffect } from 'react';
import { Calendar, dateFnsLocalizer } from 'react-big-calendar';
import { format, parse, startOfWeek, getDay } from 'date-fns';
import { ru } from 'date-fns/locale';
import 'react-big-calendar/lib/css/react-big-calendar.css';
import './styles.css';

type Event = { id: string; title: string; start: Date; end: Date; allDay?: boolean; resource?: Post };

type Post = { id: string; title: string; caption?: string; image_url?: string; status?: string; scheduled?: string; created_at: string };

const locales = { ru };

const localizer = dateFnsLocalizer({
  format,
  parse,
  startOfWeek: () => startOfWeek(new Date(), { weekStartsOn: 1 }),
  getDay,
  locales,
});

const API = '/projects/insta-scheduler/api/v1';

function statusBadgeClass(status?: string) {
  const s = (status || 'draft').toLowerCase();
  switch (s) {
    case 'scheduled':
      return 'badge scheduled';
    case 'publishing':
      return 'badge publishing';
    case 'published':
      return 'badge published';
    default:
      return 'badge draft';
  }
}

function StatusBadge({ status }: { status?: string }) {
  return <span className={statusBadgeClass(status)}>{(status || 'draft').toUpperCase()}</span>;
}

function PostDrawer({ post, onClose, onUpdated }: { post: Post; onClose: () => void; onUpdated: (post: Post) => void }) {
  const [form, setForm] = useState<Post>(post);
  const [status, setStatus] = useState(post.status || 'draft');
  const [scheduledText, setScheduledText] = useState(post.scheduled || '');

  const save = async () => {
    const body = new FormData();
    body.append('title', form.title);
    body.append('caption', form.caption || '');
    body.append('image_url', form.image_url || '');
    body.append('status', status);
    if (scheduledText) body.append('scheduled', scheduledText);
    const res = await fetch(`${API}/posts/update/${form.id}`, {
      method: 'POST',
      body,
    });
    if (res.ok) {
      onUpdated({ ...form, status, scheduled: scheduledText || undefined });
    }
  };

  const remove = async () => {
    await fetch(`${API}/posts/delete/${form.id}`, { method: 'DELETE' });
    onUpdated(form);
    onClose();
  };

  return (
    <div className="overlay" onClick={onClose}>
      <div className="drawer" onClick={(e) => e.stopPropagation()}>
        <div className="drawer-header">
          <h2>Post</h2>
          <button type="button" onClick={onClose}>×</button>
        </div>
        <div className="drawer-body">
          <div className="field">
            <label>Title</label>
            <input
              value={form.title}
              onChange={(e) => setForm({ ...form, title: e.target.value })}
            />
          </div>
          <div className="field">
            <label>Caption</label>
            <textarea
              rows={3}
              value={form.caption || ''}
              onChange={(e) => setForm({ ...form, caption: e.target.value })}
            />
          </div>
          <div className="field">
            <label>Image URL</label>
            <input
              value={form.image_url || ''}
              onChange={(e) => setForm({ ...form, image_url: e.target.value })}
            />
          </div>
          <div className="field">
            <label>Scheduled (ISO)</label>
            <input
              value={scheduledText}
              onChange={(e) => setScheduledText(e.target.value)}
              placeholder="2025-01-01T12:00:00Z"
            />
          </div>
          <div className="field">
            <label>Status</label>
            <select value={status} onChange={(e) => setStatus(e.target.value)}>
              <option value="draft">Draft</option>
              <option value="scheduled">Scheduled</option>
              <option value="publishing">Publishing</option>
              <option value="published">Published</option>
            </select>
          </div>
          <div className="drawer-actions">
            <button type="button" className="primary" onClick={save}>Save</button>
            <button type="button" className="danger" onClick={remove}>Delete</button>
          </div>
        </div>
      </div>
    </div>
  );
}

export default function App() {
  const [events, setEvents] = useState<Event[]>([]);
  const [loading, setLoading] = useState(false);
  const [selected, setSelected] = useState<Post | null>(null);
  const [view, setView] = useState<string>('month');
  const [slotView, setSlotView] = useState<string>('month');
  const [error, setError] = useState<string | null>(null);
  const [posts, setPosts] = useState<Post[]>([]);

  const load = async () => {
    setLoading(true);
    setError(null);
    try {
      const res = await fetch(`${API}/posts`);
      const text = await res.text();
      setPosts(text ? JSON.parse(text) : []);
      const data: Post[] = text ? JSON.parse(text) : [];
      setEvents(
        data
          .filter((p) => p.scheduled)
          .map((p) => {
            const start = new Date(p.scheduled!);
            const end = new Date(start.getTime() + 60 * 60 * 1000);
            return {
              id: p.id,
              title: p.title,
              start,
              end,
              allDay: false,
              resource: p,
            } as Event;
          })
      );
    } catch (e: any) {
      setError(e?.message || 'load failed');
    } finally {
      setLoading(false);
    }
  };

  const handleUpdated = (post: Post) => {
    load();
    setSelected(null);
  };

  const createFromSlot = async (slot: { start: Date }) => {
    const title = prompt('Title:');
    if (!title) return;
    const payload = new FormData();
    payload.append('title', title);
    payload.append('status', 'scheduled');
    payload.append('scheduled', slot.start.toISOString());
    await fetch(`${API}/posts/create`, { method: 'POST', body: payload });
    load();
  };

  useEffect(() => {
    load();
  }, []);

  return (
    <div className="app">
      <header className="topbar">
        <div>
          <h1>InstaScheduler</h1>
          <p className="muted">Calendar view for Instagram posts</p>
        </div>
        <div className="actions">
          <button type="button" onClick={load} disabled={loading}>
            {loading ? 'Loading...' : 'Refresh'}
          </button>
        </div>
      </header>

      <div className="layout">
        {error && <div className="error">Error: {error}</div>}
        {posts.length === 0 && !loading && <div className="muted">No posts yet. Click a slot to create one.</div>}
        <div className="calendar-tile">
          <Calendar<Event>
            selectable
            views={['month', 'week', 'day', 'agenda']}
            view={view as any}
            onView={(next: string) => setSlotView(next as string)}
            defaultDate={new Date()}
            localizer={localizer}
            culture="ru"
            events={events}
            onSelectSlot={(slot: { start: Date }) => createFromSlot(slot)}
            onSelectEvent={(event: { resource?: Post }) => setSelected((event.resource as Post) || null)}
            style={{ height: 720 }}
            components={{
              eventWrapper: ({ event }: { event: { title?: string; resource?: Post } }) => (
                <div className="event-chip">
                  {event.title}
                  {event.resource?.status && <span className={statusBadgeClass(event.resource.status)}>{event.resource.status}</span>}
                </div>
              ),
            }}
          />
        </div>
      </div>

      {selected && <PostDrawer post={selected} onClose={() => setSelected(null)} onUpdated={handleUpdated} />}
    </div>
  );
}
