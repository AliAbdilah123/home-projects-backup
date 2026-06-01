import axios from 'axios';

export const api = axios.create({
  baseURL: '/api/v1'
});

export type Channel = {
  id: string;
  platform: string;
  identifier: string;
  name: string;
  status: string;
  connected_at: string | null;
  created_at: string;
  creds_raw: string | null;
};

export type Conversation = {
  id: string;
  channel_id: string;
  external_id: string | null;
  contact: string;
  display_name: string;
  unread_count: number;
  last_message_at: string;
  created_at: string;
};

export type Message = {
  id: string;
  conv_id: string;
  direction: string;
  sender: string;
  content: string;
  media_url?: string;
  status: string;
  created_at: string;
};

export const listChannels = () => api.get<Channel[]>('/channels').then(r => r.data);
export const addChannel = (payload: Partial<Channel>) => api.post<Channel>('/channels', payload).then(r => r.data);
export const connectChannel = (id: string) => api.post(`/channels/${id}/connect`).then(r => r.data);
export const disconnectChannel = (id: string) => api.post(`/channels/${id}/disconnect`).then(r => r.data);
export const listConversations = (channel_id?: string) =>
  api.get<Conversation[]>('/conversations', { params: { channel_id } }).then(r => r.data);
export const upsertConversation = (payload: { channel_id: string; external_id?: string; contact: string; display_name?: string }) =>
  api.post<{ id: string }>('/conversations', payload).then(r => r.data);
export const listMessages = (convId: string) =>
  api.get<Message[]>(`/conversations/${convId}/messages`).then(r => r.data);
export const sendMessage = (convId: string, payload: { sender?: string; content: string; media_url?: string }) =>
  api.post<Message>(`/conversations/${convId}/messages`, payload).then(r => r.data);
export const getStats = () => api.get<{ channels: number; conversations: number; messages: number }>('/stats').then(r => r.data);
