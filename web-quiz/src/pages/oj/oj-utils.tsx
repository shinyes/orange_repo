import type { Verdict } from '@/lib/types'

// 题型标签文案。
export function typeLabel(type: string): string {
  switch (type) {
    case 'programming':
      return '编程'
    case 'single_choice':
      return '单选'
    case 'true_false':
      return '判断'
    default:
      return type
  }
}

// verdict 中文与样式。
export const VERDICT_META: Record<string, { text: string; cls: string; solid: string }> = {
  AC: { text: '通过', cls: 'text-emerald-600 bg-emerald-50 border-emerald-200', solid: 'bg-emerald-600' },
  OK: { text: '运行成功', cls: 'text-emerald-600 bg-emerald-50 border-emerald-200', solid: 'bg-emerald-600' },
  WA: { text: '答案错误', cls: 'text-red-600 bg-red-50 border-red-200', solid: 'bg-red-600' },
  CE: { text: '编译错误', cls: 'text-amber-600 bg-amber-50 border-amber-200', solid: 'bg-amber-600' },
  RE: { text: '运行错误', cls: 'text-red-600 bg-red-50 border-red-200', solid: 'bg-red-600' },
  TLE: { text: '超出时间限制', cls: 'text-amber-600 bg-amber-50 border-amber-200', solid: 'bg-amber-600' },
  MLE: { text: '超出内存限制', cls: 'text-amber-600 bg-amber-50 border-amber-200', solid: 'bg-amber-600' },
  PENDING: { text: '判题中', cls: 'text-muted-foreground bg-muted border-border', solid: 'bg-muted-foreground' },
}

export function verdictText(v: Verdict | string): string {
  return VERDICT_META[v]?.text ?? v
}

export function verdictCls(v: Verdict | string): string {
  return VERDICT_META[v]?.cls ?? 'text-muted-foreground bg-muted border-border'
}

export function isAcceptedVerdict(v: string): boolean {
  return v === 'AC' || v === 'OK'
}

// 语言标签。
export function langLabel(lang: string): string {
  if (lang === 'cpp') return 'C++'
  if (lang === 'python') return 'Python 3'
  return lang
}

// 题目状态徽标（编程/客观共用）。
export function problemStatusBadge(completed: boolean): { text: string; cls: string } {
  return completed
    ? { text: '已完成', cls: 'text-emerald-700 bg-emerald-50 border-emerald-200' }
    : { text: '未完成', cls: 'text-muted-foreground bg-muted border-border' }
}

// 生成编程题起始模板。
export function starterCode(lang: string): string {
  if (lang === 'cpp') {
    return '#include <iostream>\nusing namespace std;\n\nint main() {\n    // TODO\n    return 0;\n}\n'
  }
  return '# TODO\n'
}
