// Scratch 空间页：内嵌 Scratch 编辑器（独立容器，主站反代同源访问），并提供书包的存/取。
//
// 分工：
//   · iframe（Scratch 容器）= 编辑器本体，只负责"导出/载入 .sb3 字节"
//   · 本页（主站）= 所有书包 API 调用（登录态在父页面，跨源也不需要 cookie/CORS）
// 通信：postMessage，双向校验 origin（同源模式即主站自身 origin；子域模式为 SCRATCH_URL 的 origin）。
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { toast } from 'sonner'
import { FolderOpenIcon, Loader2Icon } from 'lucide-react'

import { api } from '@/api'
import { Button } from '@/components/ui/button'
import { BackpackDialog } from '@/components/portal/backpack'

const PROTOCOL = 1
const SCRATCH_LOCALE = 'zh-cn'

interface Pending {
  resolve: (bytes: Uint8Array | null) => void
  reject: (e: Error) => void
  timer: number
}

export function ScratchPage() {
  const { spaceId } = useParams()
  const [params, setParams] = useSearchParams()
  const navigate = useNavigate()
  const qc = useQueryClient()
  const iframeRef = useRef<HTMLIFrameElement | null>(null)
  const pending = useRef<Map<number, Pending>>(new Map())
  const nextId = useRef(1)
  const [ready, setReady] = useState(false)
  const [pickerOpen, setPickerOpen] = useState(false)
  // 实时暂存状态
  const [savedAt, setSavedAt] = useState<Date | null>(null)
  const [saving, setSaving] = useState(false)
  const readyRef = useRef(false)
  const savingRef = useRef(false)
  const currentProjectRef = useRef<{ id: number; name: string } | null>(null)
  const lastHashRef = useRef(0)
  const lastBytesRef = useRef<Uint8Array | null>(null)
  const dirtyTimerRef = useRef<number | null>(null)
  readyRef.current = ready

  const cfgQ = useQuery({ queryKey: ['app-config'], queryFn: api.appConfig, staleTime: 5 * 60_000 })
  const baseUrl = (cfgQ.data?.scratchUrl ?? '').replace(/\/$/, '')
  const deployed = baseUrl !== ''
  const iframeSrc = useMemo(() => {
    if (!deployed) return ''
    return `${baseUrl}/?locale=${SCRATCH_LOCALE}&parent=${encodeURIComponent(window.location.origin)}`
  }, [baseUrl, deployed])
  const targetOrigin = useMemo(() => {
    try {
      return new URL(baseUrl || window.location.href, window.location.href).origin
    } catch {
      return window.location.origin
    }
  }, [baseUrl])

  // ---- iframe 消息通道 ----
  useEffect(() => {
    function onMessage(ev: MessageEvent) {
      if (ev.origin !== targetOrigin) return
      const msg = ev.data ?? {}
      if (msg.source !== 'orangeoj-scratch') return
      if (msg.type === 'vm-ready' || msg.type === 'host-ready') {
        if (msg.protocol && msg.protocol !== PROTOCOL) {
          toast.error(`Scratch 容器协议版本不匹配（容器 ${msg.protocol} / 主站 ${PROTOCOL}），请同步升级镜像`)
        }
        setReady(true)
        return
      }
      if (msg.type === 'error') {
        toast.error(String(msg.message || 'Scratch 编辑器出错'))
        return
      }
      // 编辑器工具栏右侧的按钮（书包 / 保存 / 退出）→ 统一由主站处理
      // （书包数据与登录态都在主站；iframe 不直接调 API）
      if (msg.type === 'ui' && typeof msg.action === 'string') {
        if (msg.action === 'openBackpack') setPickerOpen(true)
        else if (msg.action === 'saveToBackpack') void saveRef.current()
        else if (msg.action === 'exit') exitRef.current()
        else if (msg.action === 'dirty') {
          // 编辑器有改动 → 5 秒防抖后保存一次（避免频繁序列化，也保证"改完立刻关页面"不丢）
          if (dirtyTimerRef.current) window.clearTimeout(dirtyTimerRef.current)
          dirtyTimerRef.current = window.setTimeout(() => {
            void saveNowRef.current(false)
          }, 5_000)
        }
        return
      }
      if (msg.type === 'reply' && typeof msg.id === 'number') {
        const p = pending.current.get(msg.id)
        if (!p) return
        pending.current.delete(msg.id)
        window.clearTimeout(p.timer)
        if (msg.ok) p.resolve(msg.bytes ? new Uint8Array(msg.bytes) : null)
        else p.reject(new Error(String(msg.error || '编辑器操作失败')))
      }
    }
    window.addEventListener('message', onMessage)
    return () => window.removeEventListener('message', onMessage)
  }, [targetOrigin])

  const ask = useCallback((payload: Record<string, unknown>, transfer: Transferable[] = [], timeoutMs = 30_000) => {
    return new Promise<Uint8Array | null>((resolve, reject) => {
      const frame = iframeRef.current
      if (!frame || !frame.contentWindow) {
        reject(new Error('编辑器尚未加载'))
        return
      }
      const id = nextId.current++
      const timer = window.setTimeout(() => {
        pending.current.delete(id)
        reject(new Error('编辑器响应超时'))
      }, timeoutMs)
      pending.current.set(id, { resolve, reject, timer })
      frame.contentWindow.postMessage(
        { source: 'orangeoj-host', protocol: PROTOCOL, id, ...payload },
        targetOrigin,
        transfer,
      )
    })
  }, [targetOrigin])

  // ---- 保存当前作品到书包 ----
  async function saveToBackpack() {
    if (!ready) {
      toast.error('编辑器还在加载，请稍候')
      return
    }
    const name = window.prompt('保存到书包：给作品起个名字', `我的作品 ${new Date().toLocaleString('zh-CN', { hour12: false })}`)
    if (name === null) return
    try {
      const bytes = await ask({ type: 'saveSb3' })
      if (!bytes || bytes.length === 0) throw new Error('导出内容为空')
      const res = await api.uploadScratchProject(name.trim() || '未命名作品', bytes.buffer as ArrayBuffer)
      currentProjectRef.current = { id: res.id, name: name.trim() || '未命名作品' }
      lastHashRef.current = 0
      setSavedAt(new Date())
      toast.success(`已保存到书包（${(res.size / 1024).toFixed(0)} KB）`)
      void qc.invalidateQueries({ queryKey: ['scratch-projects'] })
    } catch (e) {
      toast.error(e instanceof Error ? e.message : '保存失败')
    } finally {
      /* 无按钮态需要复位（保存按钮在编辑器工具栏里） */
    }
  }
  // 消息回调里要用到最新的 saveToBackpack（避免闭包捕获旧版本）
  const saveRef = useRef<() => Promise<void>>(saveToBackpack)
  saveRef.current = saveToBackpack

  // ---- 实时暂存 / 保存 ----
  // 做法：每 25 秒向编辑器要一次当前工程（sb3），用轻量哈希判断是否有变化；
  // 有变化就覆盖写回（首次会创建「（自动暂存）」作品，之后一直覆盖同一个作品，
  // 从书包打开的作品则直接覆盖原作品）。状态显示在编辑器工具栏左侧。
  // saveCurrent(force)：force=true 时无论哈希是否变化都写一次（退出/关闭页面用）。
  const saveCurrent = useCallback(
    async (force: boolean) => {
      if (!readyRef.current || savingRef.current) return false
      savingRef.current = true
      setSaving(true)
      try {
        const bytes = await ask({ type: 'saveSb3' })
        if (!bytes || bytes.length === 0) return false
        const h = hashBytes(bytes)
        lastBytesRef.current = bytes
        if (!force && h === lastHashRef.current) return false // 没有改动，跳过
        const buf = bytes.slice().buffer as ArrayBuffer
        if (currentProjectRef.current) {
          await api.updateScratchProjectContent(currentProjectRef.current.id, buf)
        } else {
          const res = await api.uploadScratchProject('（自动暂存）', buf)
          currentProjectRef.current = { id: res.id, name: '（自动暂存）' }
        }
        lastHashRef.current = h
        setSavedAt(new Date())
        void qc.invalidateQueries({ queryKey: ['scratch-projects'] })
        return true
      } catch (e) {
        if (force) toast.error(e instanceof Error ? e.message : '保存失败')
        return false
      } finally {
        savingRef.current = false
        setSaving(false)
      }
    },
    [ask, qc],
  )
  const saveNowRef = useRef(saveCurrent)
  saveNowRef.current = saveCurrent

  // ---- 退出编辑器（编辑器工具栏最右侧的「退出」按钮）----
  // 点击退出 = 保存 + 返回：等待保存完成（失败会提示），避免"点了退出结果改动丢了"
  function exitEditor() {
    void (async () => {
      await saveNowRef.current(true)
      navigate(`/s/${spaceId}/training`)
    })()
  }
  const exitRef = useRef(exitEditor)
  exitRef.current = exitEditor

  const autosaveRef = useRef<() => Promise<void>>(async () => {})
  autosaveRef.current = async () => {
    await saveNowRef.current(false)
  }

  // ---- 关闭页面 / 切到后台时兜底保存 ----
  // 说明：页面卸载时来不及做 postMessage 往返（取不到编辑器里的工程），
  // 所以用最近一次拿到的字节（lastBytesRef）作为内容，通过 navigator.sendBeacon 发出——
  // 它是浏览器专门为"离开页面时仍要送达"设计的，且会带上 Cookie（同源）。
  // 后端为此额外提供 POST /content（sendBeacon 只能发 POST）。
  useEffect(() => {
    function flush() {
      const bytes = lastBytesRef.current
      if (!bytes || bytes.length === 0) return
      if (hashBytes(bytes) === lastHashRef.current) return // 无改动
      const blob = new Blob([bytes.slice().buffer as ArrayBuffer], { type: 'application/octet-stream' })
      const pid = currentProjectRef.current?.id
      try {
        if (pid) {
          navigator.sendBeacon(`/api/portal/scratch/projects/${pid}/content`, blob)
        } else {
          // 还没有对应作品：用 keepalive 请求创建一个暂存作品
          void fetch(`/api/portal/scratch/projects?name=${encodeURIComponent('（自动暂存）')}`, {
            method: 'POST',
            credentials: 'include',
            keepalive: true,
            headers: { 'Content-Type': 'application/octet-stream' },
            body: blob,
          })
        }
      } catch {
        /* 忽略：已经尽力 */
      }
    }
    const onVisibility = () => {
      if (document.visibilityState !== 'hidden') return
      // 先尝试正常保存一次（切后台时异步请求通常来得及），失败/来不及再由 beacon 兜底
      void saveNowRef.current(false).finally(() => flush())
    }
    window.addEventListener('pagehide', flush)
    document.addEventListener('visibilitychange', onVisibility)
    return () => {
      window.removeEventListener('pagehide', flush)
      document.removeEventListener('visibilitychange', onVisibility)
    }
  }, [])

  // 定时暂存（编辑器就绪后开始）
  useEffect(() => {
    if (!ready) return
    const timer = window.setInterval(() => {
      void autosaveRef.current()
    }, 25_000)
    // 打开作品后也先记一次基线，避免刚打开就立刻"暂存"
    const t = window.setTimeout(() => void autosaveRef.current(), 8_000)
    return () => {
      window.clearInterval(timer)
      window.clearTimeout(t)
    }
  }, [ready])

  // 把暂存状态显示到编辑器工具栏（跨源：通过 postMessage 通知宿主页）
  useEffect(() => {
    const frame = iframeRef.current
    if (!frame || !frame.contentWindow) return
    const text = saving ? '自动暂存中…' : savedAt ? `已自动暂存 ${savedAt.toLocaleTimeString('zh-CN', { hour12: false }).slice(0, 5)}` : ''
    frame.contentWindow.postMessage({ source: 'orangeoj-host', protocol: PROTOCOL, type: 'status', text }, targetOrigin)
  }, [saving, savedAt, targetOrigin])

  // ---- 从书包打开 ----
  const openProject = useCallback(async (projectId: number, projectName: string) => {
    try {
      const bytes = await api.scratchProjectBytes(projectId)
      await ask({ type: 'loadSb3', bytes: new Uint8Array(bytes) }, [], 60_000)
      currentProjectRef.current = { id: projectId, name: projectName }
      lastHashRef.current = 0 // 刚载入：下一轮暂存会建立新基线
      toast.success(`已载入《${projectName}》`)
      setParams((prev) => {
        const next = new URLSearchParams(prev)
        next.delete('openProject')
        return next
      }, { replace: true })
    } catch (e) {
      toast.error(e instanceof Error ? e.message : '打开失败')
    } finally {
      /* 无按钮态需要复位（保存按钮在编辑器工具栏里） */
    }
  }, [ask, setParams])

  // 书包里点"在 Scratch 中打开" → 跳到这里并带 ?openProject=id
  const openId = Number(params.get('openProject') || 0)
  useEffect(() => {
    if (!ready || !openId) return
    void (async () => {
      try {
        const { projects } = await api.scratchProjects()
        const target = projects.find((p) => p.id === openId)
        await openProject(openId, target?.name ?? `#${openId}`)
      } catch (e) {
        toast.error(e instanceof Error ? e.message : '打开失败')
      }
    })()
  }, [ready, openId, openProject])

  if (cfgQ.isLoading) {
    return <Center><Loader2Icon className="mr-2 size-4 animate-spin" /> 正在检查 Scratch 服务…</Center>
  }

  // 未部署：给出明确提示与配置方法（不让人以为坏了）
  if (!deployed) {
    return (
      <div className="mx-auto w-full max-w-3xl px-4 py-16 lg:px-8">
        <div className="rounded-2xl border bg-card p-8 text-center">
          <FolderOpenIcon className="mx-auto mb-3 size-8 text-muted-foreground" />
          <p className="text-sm font-medium">Scratch 编辑器未部署</p>
          <p className="mt-2 text-xs leading-relaxed text-muted-foreground">
            本站的 Scratch 创作页由独立容器提供（含离线素材库）。管理员启动 Scratch 容器后，
            在主服务上配置其内部地址即可启用：
          </p>
          <pre className="mt-3 overflow-x-auto rounded-lg bg-muted p-3 text-left font-mono text-[11px] leading-relaxed">
{`# docker compose：主服务环境变量（两条一起配 = 主站在该子域根路径反代容器）
ORANGEOJ_SCRATCH_URL=https://scratch.example.com
ORANGEOJ_SCRATCH_INTERNAL_URL=http://orangescratch:80
# 只配 ORANGEOJ_SCRATCH_URL = 子域直连容器（自行暴露端口与证书）`}
          </pre>
          <p className="mt-3 text-xs text-muted-foreground">
            当前空间仍可正常使用训练 / 练习 / 刷题等其他功能。
          </p>
          <Button variant="outline" className="mt-4" onClick={() => navigate(`/s/${spaceId}/training`)}>
            返回训练列表
          </Button>
        </div>
      </div>
    )
  }

  return (
    <div className="relative h-full min-h-0">
      {/* 编辑器占满整页（模仿 scratch.zhike.in 的独立编辑器页）；
          书包 / 保存 按钮由宿主页注入在编辑器顶栏最右侧（见 app/scratch/host/host.js），
          点击后经 postMessage 回到本页处理。 */}
      {!ready && (
        <div className="absolute inset-0 z-10 flex items-center justify-center gap-2 bg-background/70 text-xs text-muted-foreground">
          <Loader2Icon className="size-4 animate-spin" /> Scratch 编辑器加载中…
        </div>
      )}
      <iframe
        ref={iframeRef}
        src={iframeSrc}
        title="Scratch 编辑器"
        className="h-full w-full border-0"
        allow="microphone; camera; clipboard-read; clipboard-write; fullscreen"
      />

      {/* 书包面板（工具栏「书包」按钮触发）：可直接把作品载入当前编辑器 */}
      <BackpackDialog
        open={pickerOpen}
        onOpenChange={setPickerOpen}
        onOpenInScratch={(p) => { setPickerOpen(false); void openProject(p.id, p.name) }}
      />
    </div>
  )

}

function Center({ children }: { children: React.ReactNode }) {
  return (
    <div className="flex h-full items-center justify-center text-xs text-muted-foreground">{children}</div>
  )
}
// hashBytes：FNV-1a 抽样哈希（只用来判断"工程是否变化"，不做安全用途）
function hashBytes(bytes: Uint8Array): number {
  let h = 2166136261
  for (let i = 0; i < bytes.length; i += 97) {
    h ^= bytes[i]
    h = Math.imul(h, 16777619)
  }
  return h
}
