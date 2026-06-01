import { listConversations, listMessages, sendMessage, type Conversation, type Message } from '../services/api';
import { useCallback, useEffect, useState } from 'react';

export function useConversations(channelId?: string) {
  const [conversations, setConversations] = useState<Conversation[]>([]);
  const [loading, setLoading] = useState(false);

  const refresh = useCallback(async () => {
    setLoading(true);
    const data = await listConversations(channelId);
    const sorted = data.sort((a, b) => b.last_message_at.localeCompare(a.last_message_at));
    setConversations(sorted);
    setLoading(false);
  }, [channelId]);

  useEffect(() => { refresh(); }, [refresh]);

  return { conversations, loading, refresh };
}

export function useMessages(convId?: string) {
  const [messages, setMessages] = useState<Message[]>([]);
  const [loading, setLoading] = useState(false);

  const refresh = useCallback(async () => {
    if (!convId) return;
    setLoading(true);
    const data = await listMessages(convId);
    setMessages(data);
    setLoading(false);
  }, [convId]);

  useEffect(() => { refresh(); }, [refresh]);

  const post = useCallback(async (content: string) => {
    if (!convId) return;
    const msg = await sendMessage(convId, { sender: 'agent', content });
    setMessages(prev => [...prev, msg]);
  }, [convId]);

  return { messages, loading, refresh, post };
}
