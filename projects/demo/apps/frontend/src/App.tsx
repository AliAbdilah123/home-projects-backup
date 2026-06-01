import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import './index.css';
import App from './App';
import TaskForm from './components/TaskForm';

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <div className="min-h-screen bg-slate-50 text-slate-900">
      <header className="border-b bg-white/70 backdrop-blur">
        <div className="mx-auto max-w-5xl px-4 py-4 flex items-center justify-between">
          <h1 className="text-xl font-bold tracking-tight">AIOPS LITE</h1>
          <nav className="text-sm text-slate-500">Demo Orchestration Board</nav>
        </div>
      </header>
      <main className="mx-auto max-w-5xl px-4 py-6">
        <section className="mb-8">
          <h2 className="text-lg font-semibold mb-2">Generate PRD + API Contract</h2>
          <p className="text-sm text-slate-600 mb-3">
            Kirimkan ide kerja, akan diterjemahkan menjadi PRD dan API contract otomatis (placeholder).
          </p>
        </section>
        <section className="mb-12">
          <TaskForm />
        </section>
        <section>
          <h2 className="text-lg font-semibold mb-2">Task Board</h2>
          <p className="text-sm text-slate-600">Task terakhir yang dikirim muncul di sini.</p>
          <div id="board" className="mt-3 rounded border bg-white" />
        </section>
      </main>
    </div>
  </StrictMode>,
);
