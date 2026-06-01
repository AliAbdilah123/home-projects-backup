import { listChannels, addChannel, connectChannel, disconnectChannel, type Channel } from '../services/api';
import { useCallback, useState } from 'react';

export function useChannels() {
  const [channels, setChannels] = useState<Channel[]>([]);
  const [loading, setLoading] = useState(false);

  const refresh = useCallback(async () => {
    setLoading(true);
    const data = await listChannels();
    setChannels(data);
    setLoading(false);
  }, []);

  const add = useCallback(async (payload: Partial<Channel>) => {
    const created = await addChannel(payload);
    setChannels(prev => [created, ...prev]);
    return created;
  }, []);

  const connect = useCallback(async (id: string) => {
    const updated = await connectChannel(id);
    setChannels(prev => prev.map(c => c.id === id ? updated : c));
  }, []);

  const disconnect = useCallback(async (id: string) => {
    const updated = await disconnectChannel(id);
    setChannels(prev => prev.map(c => c.id === id ? { ...c, status: updated.status } : c));
  }, []);

  return { channels, loading, refresh, add, connect, disconnect };
}
