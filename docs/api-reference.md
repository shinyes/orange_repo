# OrangeRepo API 参考（面向 AI / 程序化调用）

单机题库管理应用：Go + Fiber + SQLite。除登录接口外，全部请求需要会话 Cookie。
基础地址以 `https://host` 表示（本地默认 `http://localhost:8080`）。

## 1. 认证

所有非 `login`/`me` 接口返回 `401 {"error":"unauthorized"}` 时，会话已失效，需重新登录。

```http
POST /api/auth/login {password}             → 204 | 401
POST /api/auth/logout                        → 204
GET  /api/auth/me                            → 200 {"authenticated": bool}
PUT  /api/auth/password {oldPassword,newPassword} → 204
```

- 登录成功后响应头 `Set-Cookie: orange_session=...`；程序化调用请携带该 Cookie。
- 重新登录会使旧会话失效（单会话模型）。

## 2. 题目

### 列表与筛选

```
GET /api/problems?q=搜索词&tags=数学,算法&type=programming&ids=1,2,3
→ 200 {"problems": [ProblemSummary]}
```

- `tags`：完整路径，逗号分隔，多标签 **AND**；命中规则为前缀包含：题目拥有标签 `t` 或 `t/…`（子孙）即命中。
- `type`：`programming | single_choice | true_false`。
- `ids`（仅导出用）：逗号分隔的题目 id。

```json
ProblemSummary: {
  "id": 1, "type": "programming", "title": "A+B",
  "tags": ["入门"], "timeLimitMs": 1000, "memoryLimitMiB": 256,
  "createdAt": "2026-08-22T04:25:39Z"
}
```

### 创建 / 读取 / 更新 / 删除

```http
POST   /api/problems        (完整载荷) → 201 {"problem": Problem}
GET    /api/problems/:id                → 200 {"problem": Problem}
PUT    /api/problems/:id    (完整载荷) → 200 {"problem": Problem}
DELETE /api/problems/:id                → 204
PUT    /api/problems/:id/solutions {"solutions":[...]} → 200 {"solutions":[...]}
```

题目载荷（创建/更新用同一形状，服务端做归一化）：

```json
{
  "type": "single_choice",
  "title": "关于Python中的列表，下列描述错误的是?( )",
  "tags": ["CIE", "Python", "二级"],
  "statementMd": "题干 Markdown（支持 $KaTeX$ 公式与 ![图](/api/uploads/x.png)）",
  "bodyJson": {
    "options": ["A. …", "B. …"]
  },
  "answerJson": { "answerIndex": 0 },
  "solutions": [ { "language": "python", "code": "…", "markdown": "…" } ],
  "timeLimitMs": 1000,
  "memoryLimitMiB": 256
}
```

按题型约定：
- `programming`：`bodyJson={"inputFormat","outputFormat","samples":[{"input","output"}],"testCases":[{"input","output"}]}`；缺省时限 1000ms/256MiB；`answerJson=null/{}`。
- `single_choice`：`bodyJson={"options":[...]}`；`answerJson={"answerIndex":n}`。
- `true_false`：`bodyJson={}`；`answerJson={"answer":bool}`。

完整 `Problem` 比载荷多 `id`、`createdAt`；`solutions` 恒为数组。
删除题目会级联清理其在所有训练/练习中的条目（不删训练/练习本体）。

## 3. 标签

斜杠层级（`数学/几何/圆`），树节点 = 现存字面标签 ∪ 虚拟祖先前缀。

```http
GET    /api/tags?q=搜索词&tags=已选标签&type=类型
       → 200 {"tags":[{"tag":"数学/几何","count":2}], "total":5}
PATCH  /api/tags {"from":"数学/几何","to":"几何基础"} → 200 {"updated":2}
DELETE /api/tags?tag=数学 → 200 {"updated":3}
GET    /api/tag-order            → 200 {"order":{"":"[顶层顺序]","数学":["子级…"]}}
PUT    /api/tag-order {"order":{...}} → 204
```

- `count`：当前基底过滤（q/类型）下**命中该标签（前缀含子孙）的题目数**，与已选标签无关；`total`=满足完整已选集（AND）的题数。
- `__none__`：特殊哨兵标签，表示"无任何标签"的题目；不可重命名/删除。
- 重命名：精确匹配重写 + `from/…` 前缀子树整体搬家，与现存标签重名时去重合并。
- 删除：连同全部前缀子孙从所有题目移除。
- `tag-order` 手动排序：键为父路径（`""` 表示顶层），值为子标签顺序列表。

## 4. 上传图片与清理

```http
POST /api/images        multipart(form-data: file=图片) → 201 {"url":"/api/uploads/<hex>.<ext>"}
GET  /api/uploads/*     静态文件（需会话）
GET  /api/uploads/cleanup?dryRun=true → 200 {"orphaned":n,"total":m}
POST /api/uploads/cleanup             → 200 {"removed":n}
```

- 支持扩展名：png/jpg/jpeg/gif/webp/svg。
- 图片引用写入题目四个文本字段（statementMd / bodyJson / answerJson / solutions）中的 `/api/uploads/<file>` 即"被引用"；cleanup 只删除未被任何题目引用的图片。

## 5. 题册目录（可嵌套）

```http
GET    /api/booklet-directories
       → 200 {"directories":[{"id":1,"name":"竞赛","parentId":null,"orderNo":1}]}
POST   /api/booklet-directories {"name":"真题","parentId":1} → 201 {"id":2}
PUT    /api/booklet-directories/layout
       {"directories":[{"id":…,"parentId":…,"orderNo":…}]} → 204
PATCH  /api/booklet-directories/:id {"name":"…"} → 204
DELETE /api/booklet-directories/:id[?deleteBooklets=true] → 204
```

- 扁平列表；`parentId=null` 表示根。layout 为拖拽原子提交：须恰好覆盖全部目录、父链无环、parentId 不得指向自身，否则 400 全量回滚。
- 删除目录：`deleteBooklets=true` 时连同**直接归属**的训练/练习一并删除（并级联删除仅被它们引用的题目）；否则归属题册移到顶层；直接子目录总是上移一层。

## 6. 训练（章节结构）

```http
GET  /api/trainings
     → 200 {"trainings":[{"id":1,"title":"…","description":"…","tags":[…],"folderId":1,"problemCount":3,"createdAt":"…"}]}
POST /api/trainings {"title":"…","description":"…","tags":[…],"folderId":1} → 201 {"id":1}
GET  /api/trainings/:id → 200 {"training":{…},"chapters":[Chapter]}
PUT  /api/trainings/:id {"title","description","tags"} → 204
DELETE /api/trainings/:id → 204
PUT  /api/trainings/:id/folder {"folderId":1|null} → 204   （移入目录/移回根）
```

```json
Chapter: {"id":1,"trainingId":1,"title":"热身","orderNo":1,
          "items":[{"id":10,"chapterId":1,"problemId":5,"orderNo":1,"problemTitle":"A+B","problemType":"programming"}]}
```

章节与条目：

```http
POST /api/trainings/:id/chapters {"title":"…"} → 201 {"id":…}
PUT  /api/chapters/:id {"title":"…","orderNo":1} → 204
DELETE /api/chapters/:id → 204
POST /api/chapters/:id/items {"problemIds":[5,6]} → 201 {"itemIds":[10,11]}   （追加）
PUT  /api/chapters/:id/items {"itemIds":[11,10]} → 204                        （整表重排）
DELETE /api/items/:id → 204
PUT  /api/trainings/:id/layout
     {"chapterIds":[1,2],"chapters":[{"chapterId":1,"itemIds":[10,11]},{"chapterId":2,"itemIds":[]}]}
     → 200 {"chapters":[…]}
```

`layout` 为拖拽原子提交（章节全排列 + 条目并集须恰好覆盖全部条目，否则 400）。

**删除训练**：级联删除章节与其条目，并**一并删除仅被该训练/练习引用的题目**（被其他训练/练习引用的题目保留）；目录「连同题册删除」同规则。

## 7. 练习（平铺）

```http
GET  /api/practices
     → 200 {"practices":[{"id":1,"title":"…","folderId":1,"problemCount":3,…}]}
POST /api/practices {"title":"…","description":"…","tags":[…],"folderId":1} → 201 {"id":1}
GET  /api/practices/:id → 200 {"practice":{…},"items":[PracticeItem]}
PUT  /api/practices/:id {"title","description","tags"} → 204
DELETE /api/practices/:id → 204
PUT  /api/practices/:id/folder {"folderId":1|null} → 204
POST /api/practices/:id/items {"problemIds":[5,6]} → 201 {"itemIds":[…]}
PUT  /api/practices/:id/items {"itemIds":[…]} → 204     （整表重排，须覆盖全部条目）
DELETE /api/practice-items/:id → 204
```

```json
PracticeItem: {"id":10,"practiceId":1,"problemId":5,"orderNo":1,"problemTitle":"A+B","problemType":"programming"}
```

OrangeOJ 练习无分值语义；删除练习同样级联删除仅被其引用的题目。

## 8. 导入 / 导出（OrangeOJ ZIP）

```http
POST /api/import?mode=problems|training|practice|auto   multipart(form-data: zip=文件)
     → 201 {"imported":[{"id":…,"title":"…"}], "trainingId":…, "chapters":n, "practiceId":…, "title":"题册名称"}
GET  /api/export/problems?q=&tags=&type=&ids=1,2      → application/zip
GET  /api/export/trainings/:id                        → application/zip
GET  /api/export/practices/:id                        → application/zip
```

压缩包格式：`problems.json`（必填，2 空格缩进、不转义 HTML）+ `trainingPlan.json`（可选，章节结构）+ `images/`（可选，引用图片）。

- `mode=auto`（推荐）：包内含非空 `chapters` → 按训练导入，否则按练习导入。
- 训练/练习**名称**优先级：包内 `trainingPlan.json` 的 `title` → 上传文件名（去扩展名）→ 默认名（「导入的训练」/「导入的练习」）。
- `mode=training` 但包内无章节信息：自动创建「未分组」章节收纳全部题目。
- ZIP 上限 100MB；`mode` 非法值返回 400。
- 导入的题目始终入库；题面图片引用 `(images/…` 自动重写为 `(/api/uploads/…`。

## 9. 错误与通用约定

- 全部错误响应：`{"error":"消息"}`；状态码 400（参数/校验）、401（未认证）、404（不存在）、405（方法不允许）。
- JSON 字段一律 camelCase。
- 会话 Cookie 名：`orange_session`。
- 数据模型补充：`Problem` 的 `bodyJson`/`answerJson` 为原始 JSON 对象；`solutions` 恒为数组。
- 单用户部署：同时只有一个有效会话（重复登录会踢掉旧会话）。

## 10. 典型 AI 工作流示例

1. 登录：`POST /api/auth/login`（记录 Cookie）。
2. 建题：`POST /api/problems`（可在 `statementMd`/`bodyJson` 使用 Markdown 与 KaTeX）。
3. 整理：`POST /api/booklet-directories` 建目录 → `PUT /api/trainings/:id/folder` 归类。
4. 编制：`POST /api/trainings/:id/chapters` → `POST /api/chapters/:id/items {"problemIds":[…]}`。
5. 导出：`GET /api/export/trainings/:id`（ZIP 可直接导入 OrangeOJ）。