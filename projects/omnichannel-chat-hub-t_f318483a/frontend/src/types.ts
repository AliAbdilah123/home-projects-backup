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
