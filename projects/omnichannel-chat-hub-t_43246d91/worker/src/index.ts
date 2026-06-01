import { configFromEnv } from './config.js';
import { loadBaileys } from './baileys.js';

async function main() {
  const config = configFromEnv();
  const baileys = await loadBaileys();

  console.log(
    JSON.stringify({
      service: 'omnichannel-chat-hub-worker',
      status: 'ready',
      provider: 'baileys',
      authDir: config.authDir,
      logLevel: config.logLevel,
      baileysExports: Object.keys(baileys).length
    })
  );
}

main().catch((error: unknown) => {
  console.error(error);
  process.exit(1);
});
