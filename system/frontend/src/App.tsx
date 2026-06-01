import { useState, useEffect } from 'react'

interface ProcessInfo {
  pid: number
  name: string
  memPercent: number
  rssMB?: number
  cpuPercent?: number
}

interface FileInfo {
  path: string
  sizeBytes: number
}

interface DeviceStats {
  totalMemoryMB: number
  usedMemoryMB: number
  totalDiskBytes: number
  usedDiskBytes: number
  cpuPercent: number
}

type Tab = 'processes' | 'files'

function formatBytes(bytes: number): string {
  if (bytes === 0) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return `${parseFloat((bytes / Math.pow(k, i)).toFixed(2))} ${sizes[i]}`
}

function ProgressBar({ label, value, total, color }: { label: string; value: number; total: number; color: string }) {
  const percentage = total === 0 ? 0 : Math.round((value / total) * 100)
  return (
    <div style={{ marginBottom: '1.5rem' }}>
      <div className="flex justify-between mb-1">
        <span className="text-sm font-medium text-gray-900">{label}</span>
        <span className="text-sm text-gray-700">
          {formatBytes(value)} / {formatBytes(total)} ({percentage}%)
        </span>
      </div>
      <div className="w-full bg-gray-200 rounded-full h-6 overflow-hidden">
        <div
          className="h-full rounded-full transition-all duration-500"
          style={{ width: `${percentage}%`, backgroundColor: color }}
        />
      </div>
    </div>
  )
}

function Card({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <div className="bg-white border border-gray-200 rounded-xl shadow-sm p-5">
      <h3 className="text-lg font-semibold text-gray-900 mb-3">{title}</h3>
      {children}
    </div>
  )
}

function Table({ headers, rows }: { headers: string[]; rows: (string | number)[][] }) {
  return (
    <div className="overflow-x-auto">
      <table className="min-w-full text-sm">
        <thead>
          <tr>
            {headers.map((header) => (
              <th
                key={header}
                className="text-left py-2 px-3 border-b border-gray-200 font-medium text-gray-700"
              >
                {header}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {rows.map((row, idx) => (
            <tr key={idx}>
              {row.map((cell, cellIdx) => (
                <td key={cellIdx} className="py-2 px-3 border-b border-gray-100 text-gray-900">
                  {cell}
                </td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}

function App() {
  const [stats, setStats] = useState<DeviceStats | null>(null)
  const [processes, setProcesses] = useState<ProcessInfo[]>([])
  const [files, setFiles] = useState<FileInfo[]>([])
  const [activeTab, setActiveTab] = useState<Tab>('processes')
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    let alive = true
    async function load() {
      try {
        const base = '/projects/system'
        const [d, p, f] = await Promise.all([
          fetch(`${base}/api/v1/device`, { cache: 'no-store' }),
          fetch(`${base}/api/v1/top/processes?limit=20`, { cache: 'no-store' }),
          fetch(`${base}/api/v1/top/files?limit=20`, { cache: 'no-store' }),
        ])
        if (!d.ok || !p.ok || !f.ok) throw new Error('api error')
        const device: DeviceStats = await d.json()
        const procs: ProcessInfo[] = await p.json()
        const fileList: FileInfo[] = await f.json()
        if (!alive) return
        setStats(device)
        setProcesses(procs)
        setFiles(fileList)
        setError(null)
      } catch (err) {
        if (alive) setError(err instanceof Error ? err.message : String(err))
      }
    }
    load()
    const id = setInterval(load, 3000)
    return () => {
      alive = false
      clearInterval(id)
    }
  }, [])

  if (error && !stats) {
    return (
      <div className="min-h-screen bg-gray-50 flex items-center justify-center">
        <p className="text-red-600">Failed to load: {error}</p>
      </div>
    )
  }

  if (!stats) {
    return (
      <div className="min-h-screen bg-gray-50 flex items-center justify-center text-gray-900">
        Loading...
      </div>
    )
  }

  return (
    <div className="min-h-screen bg-gray-50 p-6">
      <div className="max-w-5xl mx-auto">
        <header className="mb-6 text-center">
          <h1 className="text-2xl font-bold text-gray-900">Server Monitor</h1>
          <p className="text-gray-700">CPU, Memory, Disk and top usage</p>
        </header>

        <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
          <Card title="Memory">
            <ProgressBar
              label="RAM"
              value={stats.usedMemoryMB * 1024 * 1024}
              total={stats.totalMemoryMB * 1024 * 1024}
              color="#3b82f6"
            />
            <p className="text-xs text-gray-600">
              Used {stats.usedMemoryMB} MB of {stats.totalMemoryMB} MB
            </p>
          </Card>

          <Card title="Disk">
            <ProgressBar
              label="Root"
              value={stats.usedDiskBytes}
              total={stats.totalDiskBytes}
              color="#10b981"
            />
            <p className="text-xs text-gray-600">
              Free {formatBytes(stats.totalDiskBytes - stats.usedDiskBytes)}
            </p>
          </Card>
        </div>

        <div className="mt-8">
          <Card title="Top Contributors">
            <div className="flex gap-3 mb-4">
              <button
                className={`px-3 py-1.5 rounded-lg border ${
                  activeTab === 'processes'
                    ? 'bg-blue-500 text-white border-blue-500'
                    : 'bg-white text-gray-900 border-gray-300'
                }`}
                onClick={() => setActiveTab('processes')}
              >
                Processes (Memory)
              </button>
              <button
                className={`px-3 py-1.5 rounded-lg border ${
                  activeTab === 'files'
                    ? 'bg-blue-500 text-white border-blue-500'
                    : 'bg-white text-gray-900 border-gray-300'
                }`}
                onClick={() => setActiveTab('files')}
              >
                Files
              </button>
            </div>
            {activeTab === 'processes' ? (
              <Table
                headers={['PID', 'Name', 'Memory %', 'RSS (MB)']}
                rows={processes.map((p) => [
                  p.pid,
                  p.name,
                  `${p.memPercent.toFixed(2)}%`,
                  p.rssMB != null ? `${p.rssMB}` : '-',
                ])}
              />
            ) : (
              <Table
                headers={['Path', 'Size']}
                rows={files.map((f) => [f.path, formatBytes(f.sizeBytes)])}
              />
            )}
          </Card>
        </div>

        <footer className="mt-6 text-center text-xs text-gray-500">Auto-refresh every 3s</footer>
      </div>
    </div>
  )
}

export default App
