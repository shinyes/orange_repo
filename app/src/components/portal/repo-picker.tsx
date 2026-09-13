// 仓库模板选择器：从当前域仓库的「训练/练习」里选一个作为拷贝来源。
// 空间管理页与门户（训练页/练习页）新建弹窗共用——Aegis: 复用既有能力，避免两处实现漂移。
import { useQuery } from '@tanstack/react-query'

import { api } from '@/api'
import { Button } from '@/components/ui/button'

export function RepoPicker(props: {
  repoKind: 'training' | 'practice'
  onRepoKind: (k: 'training' | 'practice') => void
  value: string
  onChange: (v: string) => void
  /** 目标域：门户页（训练/练习页）无管理端域上下文，按空间所属域取模板；管理端可不传（用当前所选域） */
  domainId?: number | null
}) {
  const isTraining = props.repoKind === 'training'
  const domainId = props.domainId ?? null
  // 指定域时用按域接口（门户页），否则沿用管理端当前所选域
  const trainingsQ = useQuery({
    queryKey: ['trainings', domainId],
    queryFn: () => (domainId != null ? api.trainingsInDomain(domainId) : api.trainings()),
    enabled: isTraining,
  })
  const practicesQ = useQuery({
    queryKey: ['practices', domainId],
    queryFn: () => (domainId != null ? api.practicesInDomain(domainId) : api.practices()),
    enabled: !isTraining,
  })
  const list = isTraining ? (trainingsQ.data?.trainings ?? []) : (practicesQ.data?.practices ?? [])

  return (
    <div className="space-y-2">
      <div className="flex gap-1.5">
        <Button size="xs" variant={isTraining ? 'default' : 'outline'} onClick={() => props.onRepoKind('training')}>
          仓库训练
        </Button>
        <Button size="xs" variant={!isTraining ? 'default' : 'outline'} onClick={() => props.onRepoKind('practice')}>
          仓库练习
        </Button>
      </div>
      <p className="text-xs text-muted-foreground">模板须属于当前域；拷贝时仅复制章节结构并引用同域题目。</p>
      {list.length === 0 ? (
        <p className="text-xs text-muted-foreground">当前域暂无{isTraining ? '训练' : '练习'}模板。</p>
      ) : (
        <div className="max-h-44 space-y-1 overflow-y-auto">
          {list.map((t) => (
            <label
              key={t.id}
              className={`flex cursor-pointer items-center gap-2 rounded-md px-2 py-1 text-sm hover:bg-muted ${
                props.value === String(t.id) ? 'bg-primary/5' : ''
              }`}
            >
              <input
                type="radio"
                name="repopicker"
                className="size-3.5 accent-[var(--primary)]"
                checked={props.value === String(t.id)}
                onChange={() => props.onChange(String(t.id))}
              />
              <span className="min-w-0 flex-1 truncate">{t.title}</span>
              <span className="text-[10px] text-muted-foreground">{t.problemCount} 题</span>
            </label>
          ))}
        </div>
      )}
    </div>
  )
}
