// OrangeOJ Scratch 宿主页（跑在独立 Scratch 容器里，主站以跨源 iframe 嵌入）。
//
// 与主站的约定
//   · iframe URL：<SCRATCH_URL>/?locale=zh-cn&parent=<主站 origin>
//   · 所有跨窗口消息都做**双向 origin 校验**：
//       - 只接受来自 parent 参数指定 origin 的消息
//       - 一律只向该 origin 回消息（绝不使用 '*'）
//   · 协议：source 字段区分方向；protocol 版本不匹配时主站会提示升级
//   · 工程字节（.sb3）经 postMessage 传输；**书包的存取全部由主站完成**（iframe 不调用主站 API，
//     因此跨源也不需要 cookie/CORS）
//
// 挂载方式来自官方 src/index-standalone.tsx：
//   new GUI.EditorState(params, configFactory) + GUI.createStandaloneRoot(state, node).render(props)
// 取 VM 用官方容器提供的 onVmInit 接缝（containers/gui.jsx: this.props.onVmInit(this.props.vm)），
// 由此可用 vm.saveProjectSb3()（真 .sb3，含图片/音频素材）与 vm.loadProject()——**无需修改 Scratch 源码**。
;(function () {
  'use strict'

  var PROTOCOL = 1
  var GUI = window.GUI
  var container = document.getElementById('app')
  var params = new URLSearchParams(window.location.search)
  var locale = params.get('locale') || 'zh-cn'
  // 主站 origin（跨源通信的唯一可信目标）；缺省时退化为"只接受同源消息"，避免误配后敞开
  var parentOrigin = params.get('parent') || window.location.origin

  function post(type, payload, transfer) {
    try {
      var msg = Object.assign({ source: 'orangeoj-scratch', protocol: PROTOCOL, type: type }, payload || {})
      parent.postMessage(msg, parentOrigin, transfer || [])
    } catch (e) { /* 忽略 */ }
  }

  // ---- 工具栏右侧按钮（书包 / 保存到书包）：模仿 scratch.zhike.in 的布局——
  //      书包放在编辑器顶栏最右侧。按钮由**本站宿主页**注入（不改 scratch-gui 源码），
  //      点击后用 postMessage 交给主站处理（书包数据与登录态都在主站）。
  function injectToolbar() {
    var css = document.createElement('style')
    css.textContent = [
      '#orangeoj-scratch-toolbar{position:fixed;top:7px;right:10px;z-index:2147483000;display:flex;gap:8px;align-items:center}',
      '#orangeoj-scratch-toolbar button{display:inline-flex;align-items:center;gap:6px;height:32px;padding:0 12px;',
      'border-radius:8px;border:1px solid rgba(0,0,0,.12);background:#fff;color:#575e75;font-size:13px;font-weight:600;',
      'cursor:pointer;box-shadow:0 1px 2px rgba(0,0,0,.06);transition:background .15s,border-color .15s;font-family:inherit}',
      '#orangeoj-scratch-toolbar button:hover{background:#f2f6ff;border-color:rgba(76,151,255,.55)}',
      '#orangeoj-scratch-toolbar button.primary{background:#4c97ff;border-color:#4c97ff;color:#fff}',
      '#orangeoj-scratch-toolbar button.primary:hover{background:#4280d7;border-color:#4280d7}',
      '#orangeoj-scratch-toolbar button[disabled]{opacity:.55;cursor:default}',
      '#orangeoj-scratch-toolbar svg{width:15px;height:15px;fill:currentColor}',
    ].join('')
    document.head.appendChild(css)

    var bar = document.createElement('div')
    bar.id = 'orangeoj-scratch-toolbar'

    function makeButton(label, title, primary, svgPath, onClick) {
      var b = document.createElement('button')
      b.type = 'button'
      b.title = title
      b.disabled = true // 编辑器就绪后再启用
      if (primary) b.className = 'primary'
      b.innerHTML = '<svg viewBox="0 0 24 24" aria-hidden="true"><path d="' + svgPath + '"/></svg><span>' + label + '</span>'
      b.addEventListener('click', onClick)
      return b
    }

    var ICON_BAG = 'M10 4h4a2 2 0 0 1 2 2v1h1a3 3 0 0 1 3 3v7a3 3 0 0 1-3 3H7a3 3 0 0 1-3-3v-7a3 3 0 0 1 3-3h1V6a2 2 0 0 1 2-2zm0 3h4V6h-4v1z'
    var ICON_SAVE = 'M5 3h11l3 3v15H5V3zm3 2v5h7V5H8zm-1 9h10v5H7v-5z'

    var bagBtn = makeButton('书包', '打开我的书包（已保存的作品）', false, ICON_BAG, function () {
      post('ui', { action: 'openBackpack' })
    })
    var saveBtn = makeButton('保存到书包', '把当前作品保存到书包（服务端，含图片与声音素材）', true, ICON_SAVE, function () {
      post('ui', { action: 'saveToBackpack' })
    })
    bar.appendChild(bagBtn)
    bar.appendChild(saveBtn)
    document.body.appendChild(bar)
    return function (enabled) {
      bagBtn.disabled = !enabled
      saveBtn.disabled = !enabled
    }
  }

  if (!GUI) {
    post('error', { message: 'window.GUI 未加载（scratch-gui-standalone.js 缺失）' })
    return
  }

  var setToolbarEnabled = injectToolbar()

  // ---- 素材存储：指向本容器内的本地镜像（/static/scratch-assets/<md5ext>），完全离线 ----
  // 说明：官方 LegacyStorage 会把素材指向 assets.scratch.mit.edu；这里用公开的 addWebStore
  // 换成同容器相对路径，因此编辑器运行期不访问外网。
  var storage = new GUI.ScratchStorage()
  var ASSET_BASE = 'static/scratch-assets/'
  function assetUrl(asset) {
    return ASSET_BASE + asset.assetId + '.' + asset.dataFormat
  }
  storage.addWebStore(
    [GUI.AssetType.ImageVector, GUI.AssetType.ImageBitmap, GUI.AssetType.Sound],
    assetUrl
  )

  var editorConfig = {
    storage: {
      scratchStorage: storage,
      // 主站不用编辑器的"保存到服务器"：保存走 vm.saveProjectSb3() + 主站书包接口
      saveProject: function () {
        return Promise.reject(new Error('项目保存由 OrangeOJ 书包处理'))
      }
    }
  }

  var state = new GUI.EditorState({ locale: locale }, function () { return editorConfig })
  var root = GUI.createStandaloneRoot(state, container)

  root.render({
    locale: locale,
    onVmInit: function (vm) {
      window.__ORANGEOJ_VM__ = vm
      try {
        setToolbarEnabled(true)
      } catch (e) { /* 忽略 */ }
      post('vm-ready', { locale: locale })
    },
    canSave: false,       // 隐藏"保存到服务器"（书包由主站顶栏负责）
    showComingSoon: false,
    enableCommunity: false,
    backpackVisible: false // 编辑器自带书包隐藏：本站书包在主站顶栏
  })

  // ---- 主站命令通道 ----
  window.addEventListener('message', function (ev) {
    if (ev.origin !== parentOrigin) return
    var msg = ev.data || {}
    if (msg.source !== 'orangeoj-host') return
    var vm = window.__ORANGEOJ_VM__
    if (msg.type === 'ping') {
      post('reply', { id: msg.id, ok: true, protocol: PROTOCOL })
      return
    }
    if (!vm) {
      post('reply', { id: msg.id, ok: false, error: '编辑器尚未就绪' })
      return
    }
    if (msg.type === 'saveSb3') {
      Promise.resolve(vm.saveProjectSb3())
        .then(function (blob) { return blob.arrayBuffer() })
        .then(function (buf) {
          post('reply', { id: msg.id, ok: true, bytes: new Uint8Array(buf) }, [buf])
        })
        .catch(function (e) {
          post('reply', { id: msg.id, ok: false, error: String((e && e.message) || e) })
        })
      return
    }
    if (msg.type === 'loadSb3') {
      try {
        var src = msg.bytes instanceof Uint8Array ? msg.bytes : new Uint8Array(msg.bytes || [])
        // 复制到独立 buffer：结构化克隆过来的视图可能带偏移
        var copy = new Uint8Array(src.length)
        copy.set(src)
        Promise.resolve(vm.loadProject(copy.buffer))
          .then(function () { post('reply', { id: msg.id, ok: true }) })
          .catch(function (e) { post('reply', { id: msg.id, ok: false, error: String((e && e.message) || e) }) })
      } catch (e) {
        post('reply', { id: msg.id, ok: false, error: String((e && e.message) || e) })
      }
      return
    }
    post('reply', { id: msg.id, ok: false, error: '未知指令：' + msg.type })
  })

  post('host-ready', { locale: locale, protocol: PROTOCOL })
})()
