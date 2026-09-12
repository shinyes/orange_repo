// 代码高亮（Shiki）：测评记录、题解等场景的提交代码渲染。
//
// 打包与性能策略：
// - 细粒度打包：`shiki/core` + 显式注册的语言/主题，只打进 cpp/c/python/go + github-light。
//   旧实现 `import('shiki')` 会带出全量语言（产物中约 300 个语言 chunk）与内联的 oniguruma wasm（约 600KB chunk）。
// - JavaScript 正则引擎：`createJavaScriptRegexEngine` 纯本地计算，不下载/编译 wasm，也不请求任何外部资源。
// - highlighter 为模块级单例（并发调用只初始化一次），并在首屏渲染后的空闲时段预热，
//   首次打开代码块时即可同步渲染，不再出现“加载许久”。
// - 兜底：引擎未就绪 / 加载失败 / 未知语言 / 渲染异常，一律回退纯文本，不白屏、不报错。
import { useEffect, useState } from 'react'
import type { HighlighterCore } from 'shiki/core'
import { cn } from '@/lib/utils'

// 语言 → Shiki 语言映射。
const LANG_ALIAS: Record<string, string> = {
  cpp: 'cpp',
  'c++': 'cpp',
  c: 'c',
  python: 'python',
  python3: 'python',
  py: 'python',
  go: 'go',
  golang: 'go',
  turtle: 'python',
}

export function resolveHighlightLang(language?: string): string {
  if (!language) return 'text'
  return LANG_ALIAS[language.trim().toLowerCase()] ?? 'text'
}

// 实际打进产物的语言与主题：cpp/c/python/go + github-light，其余语言由 resolveHighlightLang 归到 'text'。
const HIGHLIGHT_THEME = 'github-light'

// 预热样本：只为提前触发对应语法的正则编译，内容本身不重要（真实提交代码用到的上下文越全越好）。
// cpp 语法最重（上千条正则），所以 cpp 放最前、样本也最完整；其余语言成本很低。
const WARM_SAMPLES: readonly [string, string][] = [
  [
    'cpp',
    `#include <bits/stdc++.h>
#define MAXN 100005
using namespace std;
typedef long long ll;
template <typename T> T sqr(T x) { return x * x; }
struct Node { int l, r; ll sum; Node() : l(0), r(0), sum(0) {} };
class Solver {
public:
  explicit Solver(int n) : n_(n) {}
  ll solve(const vector<ll>& a) const {
    ll res = 0; const char* s = "hello\\n"; char c = 'x'; double d = 1.5e-3;
    for (int i = 0; i < n_; ++i) { if (a[i] > res) res = a[i]; else continue; }
    auto f = [](ll v) -> ll { return v << 1; };
    return res ^ f(0x1fULL) % 1000000007; // comment
  }
private:
  int n_;
};
int main() { Solver s(10); cout << s.solve({1, 2, 3}) << endl; return 0; }`,
  ],
  ['python', 'def main(x: int = 0) -> int:\n    """doc"""\n    return x  # comment'],
  ['go', 'package main\n\nimport "fmt"\n\nfunc main() { fmt.Println("hi") }'],
  ['c', 'int main(void) { char *s = "x"; return 0; }'],
]

// ---- 高亮引擎单例 ----

let highlighter: HighlighterCore | null = null
let highlighterPromise: Promise<HighlighterCore> | null = null

async function initHighlighter(): Promise<HighlighterCore> {
  const [
    { createHighlighterCore },
    { createJavaScriptRegexEngine },
    { default: githubLight },
    { default: cpp },
    { default: c },
    { default: python },
    { default: go },
  ] = await Promise.all([
    import('shiki/core'),
    import('shiki/engine/javascript'),
    import('shiki/themes/github-light.mjs'),
    import('shiki/langs/cpp.mjs'),
    import('shiki/langs/c.mjs'),
    import('shiki/langs/python.mjs'),
    import('shiki/langs/go.mjs'),
  ])
  const created = await createHighlighterCore({
    themes: [githubLight],
    langs: [cpp, c, python, go],
    // JS 正则引擎：纯本地计算，不加载 oniguruma wasm，也就没有对应的网络请求。
    engine: createJavaScriptRegexEngine(),
  })
  highlighter = created
  return created
}

// 并发调用共用同一个 Promise；失败后清空，使后续调用可重试（预热失败不影响首次使用）。
function loadHighlighter(): Promise<HighlighterCore> {
  if (!highlighterPromise) {
    highlighterPromise = initHighlighter().catch((err: unknown) => {
      highlighterPromise = null
      throw err
    })
  }
  return highlighterPromise
}

// ---- 渲染与缓存 ----

// 渲染结果缓存：同一份代码反复出现（开合题解、切换 tab、列表与详情复用）时无需重新分词。
// 只缓存成功结果；有数量与长度上限，避免超长代码把内存撑大。
const RENDER_CACHE_MAX_ENTRIES = 24
const RENDER_CACHE_MAX_CODE_LENGTH = 20000
const renderCache = new Map<string, string>()

function renderHighlight(h: HighlighterCore, code: string, lang: string): string {
  const cacheKey = `${lang}\u0000${code}`
  const cached = renderCache.get(cacheKey)
  if (cached !== undefined) return cached
  const html = h.codeToHtml(code, { lang, theme: HIGHLIGHT_THEME })
  if (code.length <= RENDER_CACHE_MAX_CODE_LENGTH) {
    if (renderCache.size >= RENDER_CACHE_MAX_ENTRIES) {
      const oldest = renderCache.keys().next().value
      if (oldest !== undefined) renderCache.delete(oldest)
    }
    renderCache.set(cacheKey, html)
  }
  return html
}

// 引擎已就绪时的同步渲染；未就绪或渲染失败返回 null，由调用方回退纯文本。
function peekHighlight(code: string, lang: string): string | null {
  if (!highlighter) return null
  try {
    return renderHighlight(highlighter, code, lang)
  } catch {
    return null
  }
}

// ---- 空闲预热 ----

// 首屏渲染之后的空闲时段执行；无 requestIdleCallback（老浏览器）时退化为 setTimeout。
function onIdle(task: () => void): void {
  if (typeof requestIdleCallback === 'function') {
    requestIdleCallback(task, { timeout: 2000 })
  } else {
    setTimeout(task, 200)
  }
}

let prewarmScheduled = false

// 预热：先加载引擎与语法文件，再逐语言触发一次渲染（每次占用一个空闲回调，
// 避免一次性长时间占用主线程）。预热只是提前付掉首次高亮的编译开销，失败可忽略。
function prewarm(): void {
  if (prewarmScheduled || typeof window === 'undefined') return
  prewarmScheduled = true
  onIdle(() => {
    void loadHighlighter()
      .then((h) => {
        const step = (i: number) => {
          const sample = WARM_SAMPLES[i]
          if (!sample) return
          onIdle(() => {
            try {
              h.codeToHtml(sample[1], { lang: sample[0], theme: HIGHLIGHT_THEME })
            } catch {
              // 预热失败忽略：真正渲染时仍走纯文本兜底或重试。
            }
            step(i + 1)
          })
        }
        step(0)
      })
      .catch(() => {
        // 预热失败忽略：首次使用时 loadHighlighter 会重试。
      })
  })
}

// 本模块只在题目相关页面（懒加载路由）被引入，因此这里的预热正好发生在用户可能打开代码块之前。
prewarm()

// CodeBlock 用 Shiki 高亮代码；引擎未就绪/失败时显示纯文本并标注状态。
export function CodeBlock({ code, language, className }: { code: string; language?: string; className?: string }) {
  const lang = resolveHighlightLang(language)
  const source = code ?? ''
  // 引擎已预热时首帧就能同步给出高亮结果，避免先闪一帧纯文本。
  const [html, setHtml] = useState<string | null>(() => peekHighlight(source, lang))
  const [failed, setFailed] = useState(false)

  useEffect(() => {
    let alive = true
    setFailed(false)
    const ready = peekHighlight(source, lang)
    if (ready !== null) {
      setHtml(ready)
      return () => {
        alive = false
      }
    }
    setHtml(null)
    void loadHighlighter()
      .then((h) => renderHighlight(h, source, lang))
      .then((rendered) => {
        if (alive) setHtml(rendered)
      })
      .catch(() => {
        if (alive) setFailed(true) // 引擎加载/渲染失败：保持纯文本
      })
    return () => {
      alive = false
    }
  }, [source, lang])

  if (html) {
    return (
      <div
        className={cn('overflow-x-auto rounded-lg text-xs leading-relaxed [&_pre]:m-0 [&_pre]:bg-transparent [&_pre]:p-3', className)}
        dangerouslySetInnerHTML={{ __html: html }}
      />
    )
  }
  return (
    <div className={cn('relative', className)}>
      <pre className="overflow-x-auto rounded-lg bg-muted p-3 text-xs leading-relaxed">
        <code>{source}</code>
      </pre>
      {!failed && (
        <span className="pointer-events-none absolute right-2 top-2 rounded bg-background/80 px-1.5 py-0.5 text-[10px] text-muted-foreground">
          高亮加载中…
        </span>
      )}
    </div>
  )
}
