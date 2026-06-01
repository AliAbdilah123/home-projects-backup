export type Role = 'owner' | 'admin' | 'agent';

export type AuthUser = {
  id: string;
  email: string;
  name: string;
  role: Role;
};

export type LoginRequest = {
  email: string;
  password: string;
};

export type LoginResponse = {
  token: string;
  user: AuthUser;
};

export type ChannelProvider = 'whatsapp' | 'telegram' | 'instagram' | 'facebook' | 'webchat';

export type NavChannel = {
  id: string;
  provider: ChannelProvider;
  label: string;
  status: 'active' | 'inactive';
};

export type ChannelRecord = {
  id: string;
  provider: string;
  display_name: string;
  status: string;
};

export type ChannelsListResponse = {
  channels: ChannelRecord[];
};

export type BaileysSession = {
  id: string;
  status: 'qr_pending' | 'connected' | 'disconnected' | 'error' | string;
  qr_code: string;
  poll_url: string;
};

export type StartBaileysSessionResponse = {
  channel: ChannelRecord;
  session: BaileysSession;
};
