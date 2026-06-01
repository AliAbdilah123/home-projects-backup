export type BaileysModule = typeof import("@whiskeysockets/baileys");

export async function loadBaileys(): Promise<BaileysModule> {
  return import("@whiskeysockets/baileys");
}
