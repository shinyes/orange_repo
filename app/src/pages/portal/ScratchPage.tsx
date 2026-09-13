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
import { BookOpenIcon, FolderOpenIcon, HardDriveDownloadIcon, Loader2Icon, SaveIcon } from 'lucide-react'

import { api } from '@/api'
import { Button } from '@/components/ui/button'
import { BackpackPickerDialog } from '@/components/portal/backpack'

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
  const [busy, setBusy] = useState<'save' | 'open' | null>(null)
  const [pickerOpen, setPickerOpen] = useState(false)

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
    setBusy('save')
    try {
      const bytes = await ask({ type: 'saveSb3' })
      if (!bytes || bytes.length === 0) throw new Error('导出内容为空')
      const res = await api.uploadScratchProject(name.trim() || '未命名作品', bytes.buffer as ArrayBuffer)
      toast.success(`已保存到书包（${(res.size / 1024).toFixed(0)} KB）`)
      void qc.invalidateQueries({ queryKey: ['scratch-projects'] })
    } catch (e) {
      toast.error(e instanceof Error ? e.message : '保存失败')
    } finally {
      setBusy(null)
    }
  }

  // ---- 从书包打开 ----
  const openProject = useCallback(async (projectId: number, projectName: string) => {
    setBusy('open')
    try {
      const bytes = await api.scratchProjectBytes(projectId)
      await ask({ type: 'loadSb3', bytes: new Uint8Array(bytes) }, [], 60_000)
      toast.success(`已载入《${projectName}》`)
      setParams((prev) => {
        const next = new URLSearchParams(prev)
        next.delete('openProject')
        return next
      }, { replace: true })
    } catch (e) {
      toast.error(e instanceof Error ? e.message : '打开失败')
    } finally {
      setBusy(null)
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
    <div className="flex h-full min-h-0 flex-col">
      {/* 顶部操作条：保存到书包 / 从书包打开 */}
      <div className="flex flex-wrap items-center gap-2 border-b bg-card px-3 py-2">
        <span className="text-xs text-muted-foreground">
          {ready ? '编辑器已就绪（简体中文）' : '编辑器加载中…'}
        </span>
        <div className="ml-auto flex items-center gap-2">
          <Button size="sm" variant="outline" className="h-8 text-xs" disabled={busy !== null} onClick={() => setPickerOpen(true)}>
            <BookOpenIcon className="size-3.5" /> 从书包打开
          </Button>
          <Button size="sm" className="h-8 text-xs" disabled={!ready || busy !== null} onClick={() => void saveToBackpack()}>
            {busy === 'save' ? <Loader2Icon className="size-3.5 animate-spin" /> : <SaveIcon className="size-3.5" />} 保存到书包
          </Button>
          <Button
            size="sm"
            variant="ghost"
            className="h-8 text-xs"
            disabled={!ready || busy !== null}
            title="把当前作品导出为 .sb3 下载到本机（编辑器自带的“保存到电脑”同样可用）"
            onClick={() => void downloadCurrent()}
          >
            <HardDriveDownloadIcon className="size-3.5" /> 导出 .sb3
          </Button>
        </div>
      </div>

      <div className="relative min-h-0 flex-1">
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
      </div>

      <BackpackPickerDialog
        open={pickerOpen}
        onOpenChange={setPickerOpen}
        onPick={(p) => { setPickerOpen(false); void openProject(p.id, p.name) }}
      />
    </div>
  )

  async function downloadCurrent() {
    try {
      const bytes = await ask({ type: 'saveSb3' })
      if (!bytes) return
      const buf = bytes.slice().buffer as ArrayBuffer
      const blob = new Blob([buf], { type: 'application/x.scratch.sb3' })
      const url = URL.createObjectURL(blob)
      const a = document.createElement('a')
      a.href = url
      a.download = `scratch-${Date.now()}.sb3`
      a.click()
      URL.revokeObjectURL(url)
    } catch (e) {
      toast.error(e instanceof Error ? e.message : '导出失败')
    }
  }
}

function Center({ children }: { children: React.ReactNode }) {
  return (
    <div className="flex h-full items-center justify-center text-xs text-muted-foreground">{children}</div>
  )
}
