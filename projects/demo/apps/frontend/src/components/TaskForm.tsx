import { useState, useEffect } from 'react';

export default function TaskForm() {
  const [title, setTitle] = useState('');
  const [description, setDescription] = useState('');
  const [complexity, setComplexity] = useState('medium');
  const [model, setModel] = useState('glm-5.1');

  const submit = async () => {
    const res = await fetch('/api/v1/tasks', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ title, description, complexity, model, assignee: model }),
    });
    if (res.ok) {
      setTitle('');
      setDescription('');
      window.dispatchEvent(new Event('tasks-reload'));
    }
  };

  return (
    <form
      onSubmit={(e) => {
        e.preventDefault();
        void submit();
      }}
      className="grid gap-3 rounded-lg border bg-white p-4 shadow-sm sm:grid-cols-2"
    >
      <div className="sm:col-span-2">
        <label className="text-xs font-medium text-slate-600">Judul Task</label>
        <input
          value={title}
          onChange={(e) => setTitle(e.target.value)}
          placeholder="Contoh: Buat PRD fitur onboarding"
          className="mt-1 w-full rounded-md border px-3 py-2 text-sm shadow-sm"
          required
        />
      </div>
      <div className="sm:col-span-2">
        <label className="text-xs font-medium text-slate-600">Deskripsi</label>
        <textarea
          value={description}
          onChange={(e) => setDescription(e.target.value)}
          placeholder="Ringkasan input untuk PRD + contract"
          className="mt-1 w-full rounded-md border px-3 py-2 text-sm shadow-sm"
        />
      </div>
      <div>
        <label className="text-xs font-medium text-slate-600">Complexity</label>
        <select
          value={complexity}
          onChange={(e) => setComplexity(e.target.value)}
          className="mt-1 w-full rounded-md border px-3 py-2 text-sm shadow-sm"
        >
          <option value="low">Low</option>
          <option value="medium">Medium</option>
          <option value="high">High</option>
        </select>
      </div>
      <div>
        <label className="text-xs font-medium text-slate-600">Model / Worker</label>
        <select
          value={model}
          onChange={(e) => setModel(e.target.value)}
          className="mt-1 w-full rounded-md border px-3 py-2 text-sm shadow-sm"
        >
          <option value="deepseek-v4-pro">Deepseek V4 Pro</option>
          <option value="glm-5.1">GLM 5.1</option>
          <option value="gpt-5.5">GPT 5.5</option>
          <option value="opus">Opus</option>
        </select>
      </div>
      <div className="sm:col-span-2">
        <button
          type="submit"
          className="rounded-md bg-slate-900 px-4 py-2 text-sm font-medium text-white hover:bg-slate-800"
        >
          Submit Task
        </button>
      </div>
    </form>
  );
}
