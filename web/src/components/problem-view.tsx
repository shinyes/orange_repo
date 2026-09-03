import { useState } from 'react'
import { Badge } from '@/components/ui/badge'
import { Separator } from '@/components/ui/separator'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Markdown, preserveLineBreaks } from '@/lib/markdown'
import { CodeBlock } from '@/lib/code-highlight'
import type {
  Problem,
  ProgrammingBody,
  Solution,
} from '@/lib/types'

export const TYPE_LABEL: Record<string, string> = {
  programming: '编程题',
  single_choice: '单选题',
  true_false: '判断题',
}

// 题目查看视图（题面 / 答案 / 题解）：题目详情页与训练/练习条目浮窗共用。
// 注：标题由外层容器渲染（详情页头部 / 浮窗标题），本视图不含题目标题。
export function ProblemView({ problem }: { problem: Problem }) {
  const [tab, setTab] = useState('problem')
  return (
    <div>
      {/* 类型 / 时限 / 标签 */}
      <div className="mb-4 flex flex-wrap items-center gap-1.5">
        <Badge>{TYPE_LABEL[problem.type] ?? problem.type}</Badge>
        {problem.type === 'programming' && (
          <Badge variant="outline" className="text-xs text-muted-foreground">
            {problem.timeLimitMs}ms · {problem.memoryLimitMiB}MiB
          </Badge>
        )}
        {(problem.tags ?? []).map((t) => (
          <Badge key={t} variant="secondary" className="text-xs">
            {t}
          </Badge>
        ))}
      </div>

      <Tabs value={tab} onValueChange={(v) => setTab(v as string)}>
        <TabsList>
          <TabsTrigger value="problem">题目</TabsTrigger>
          <TabsTrigger value="solutions">题解 {((problem.solutions ?? []).length) > 0 && `(${(problem.solutions ?? []).length})`}</TabsTrigger>
        </TabsList>
        <TabsContent value="problem" className="space-y-5 text-[17px] leading-relaxed">
          <section>
            <h2 className="mb-2 text-xs font-medium tracking-wide text-muted-foreground">题面</h2>
            <StatementView problem={problem} />
          </section>
          <Separator />
          <section>
            <h2 className="mb-2 text-xs font-medium tracking-wide text-muted-foreground">答案</h2>
            <AnswerView problem={problem} />
          </section>
        </TabsContent>
        <TabsContent value="solutions">
          <SolutionsView solutions={problem.solutions} />
        </TabsContent>
      </Tabs>
    </div>
  )
}

// ---------- 题面视图 ----------

function StatementView({ problem }: { problem: Problem }) {
  const body = (problem.bodyJson ?? {}) as ProgrammingBody
  return (
    <div>
      <Markdown text={preserveLineBreaks(problem.statementMd || '（暂无题面）')} className="markdown-body" />
      {problem.type === 'programming' && (
        <div className="mt-4 space-y-3">
          {(body.inputFormat || body.outputFormat) && (
            <div className="grid gap-3 sm:grid-cols-2">
              <div className="rounded-lg border p-3">
                <div className="mb-1 text-xs font-medium text-muted-foreground">输入格式</div>
                <Markdown text={body.inputFormat || '—'} className="markdown-body text-sm" />
              </div>
              <div className="rounded-lg border p-3">
                <div className="mb-1 text-xs font-medium text-muted-foreground">输出格式</div>
                <Markdown text={body.outputFormat || '—'} className="markdown-body text-sm" />
              </div>
            </div>
          )}
          {(body.samples?.length ?? 0) > 0 && (
            <div>
              <div className="mb-2 text-sm font-medium">样例</div>
              <div className="space-y-2">
                {body.samples!.map((s, i) => (
                  <div key={i} className="grid gap-2 sm:grid-cols-2">
                    <CaseBlock label={`输入 #${i + 1}`} text={s.input} />
                    <CaseBlock label={`输出 #${i + 1}`} text={s.output} />
                  </div>
                ))}
              </div>
            </div>
          )}
        </div>
      )}
    </div>
  )
}

function CaseBlock({ label, text }: { label: string; text: string }) {
  return (
    <div className="overflow-hidden rounded-lg border">
      <div className="border-b bg-muted px-3 py-1 text-xs text-muted-foreground">{label}</div>
      <pre className="overflow-x-auto p-3 text-sm">{text}</pre>
    </div>
  )
}

// ---------- 答案视图 ----------

function AnswerView({ problem }: { problem: Problem }) {
  const body = (problem.bodyJson ?? {}) as ProgrammingBody
  const answer = (problem.answerJson ?? {}) as Record<string, unknown>
  if (problem.type === 'programming') {
    const cases = body.testCases ?? []
    return (
      <div>
        <div className="mb-2 text-sm font-medium">测试点（{cases.length}）</div>
        {cases.length === 0 ? (
          <Empty>未配置测试点</Empty>
        ) : (
          <div className="space-y-2">
            {cases.map((c, i) => (
              <div key={i} className="grid gap-2 sm:grid-cols-2">
                <CaseBlock label={`输入 #${i + 1}`} text={c.input} />
                <CaseBlock label={`期望输出 #${i + 1}`} text={c.output} />
              </div>
            ))}
          </div>
        )}
      </div>
    )
  }
  if (problem.type === 'single_choice') {
    const options = ((problem.bodyJson as { options?: string[] })?.options) ?? []
    const idx = typeof answer.answerIndex === 'number' ? (answer.answerIndex as number) : -1
    return (
      <div className="space-y-2">
        {options.map((opt, i) => (
          <div
            key={i}
            className={`flex items-start gap-2.5 rounded-lg border p-3 text-sm ${
              i === idx ? 'border-primary bg-primary/5 font-medium' : ''
            }`}
          >
            <span className={`mt-0.5 flex size-6 shrink-0 items-center justify-center rounded-full text-xs ${i === idx ? 'bg-primary text-primary-foreground' : 'bg-muted text-muted-foreground'}`}>
              {String.fromCharCode(65 + i)}
            </span>
            <span className="min-w-0 flex-1">
              {/* 选项支持 Markdown / KaTeX（与题面同一渲染管线）；孤立换行保真显示 */}
              <Markdown text={preserveLineBreaks(opt)} className="markdown-body text-sm" />
            </span>
            {i === idx && <Badge className=" shrink-0">正确答案</Badge>}
          </div>
        ))}
        {options.length === 0 && <Empty>未配置选项</Empty>}
      </div>
    )
  }
  const isTrue = answer.answer === true
  return (
    <div className="flex items-center gap-3">
      <div className={`rounded-lg border px-5 py-3 text-lg font-semibold ${isTrue ? 'border-primary text-primary' : 'text-muted-foreground'}`}>
        ✓ 正确
      </div>
      <div className={`rounded-lg border px-5 py-3 text-lg font-semibold ${!isTrue ? 'border-primary text-primary' : 'text-muted-foreground'}`}>
        ✗ 错误
      </div>
    </div>
  )
}

export function Empty({ children }: { children: React.ReactNode }) {
  return <div className="rounded-lg border border-dashed p-6 text-center text-sm text-muted-foreground">{children}</div>
}

// ---------- 题解视图 ----------

function SolutionsView({ solutions }: { solutions: Solution[] }) {
  const list = solutions ?? []
  if (list.length === 0) return <Empty>暂无题解，可在「编辑」页添加</Empty>
  return (
    <div className="space-y-4">
      {list.map((s, i) => (
        <div key={i} className="overflow-hidden rounded-xl border">
          <div className="flex items-center gap-2 border-b bg-muted/50 px-4 py-2">
            <Badge variant="outline">{s.language}</Badge>
            <span className="text-xs text-muted-foreground">题解 {i + 1}</span>
          </div>
          <div className="space-y-3 p-4">
            {s.markdown && <Markdown text={s.markdown} className="markdown-body text-sm" />}
            {s.code && <CodeBlock code={s.code} language={s.language} />}
          </div>
        </div>
      ))}
    </div>
  )
}