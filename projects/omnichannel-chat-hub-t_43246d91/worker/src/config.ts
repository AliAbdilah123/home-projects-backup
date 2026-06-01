export type WorkerConfig = {
  logLevel: string;
  authDir: string;
};

export function configFromEnv(env: NodeJS.ProcessEnv = process.env): WorkerConfig {
  return {
    logLevel: env.WORKER_LOG_LEVEL ?? 'info',
    authDir: env.BAILEYS_AUTH_DIR ?? './data/baileys-auth'
  };
}
