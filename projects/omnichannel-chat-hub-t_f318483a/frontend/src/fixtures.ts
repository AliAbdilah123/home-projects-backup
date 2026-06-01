import type { NavChannel } from './types';

export const navChannelsFixture: NavChannel[] = [
  { id: 'wa-1', provider: 'whatsapp', label: 'WhatsApp', status: 'active' },
  { id: 'tg-1', provider: 'telegram', label: 'Telegram', status: 'inactive' },
  { id: 'ig-1', provider: 'instagram', label: 'Instagram', status: 'inactive' }
];
