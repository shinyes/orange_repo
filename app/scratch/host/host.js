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

  if (!GUI) {
    post('error', { message: 'window.GUI 未加载（scratch-gui-standalone.js 缺失）' })
    return
  }

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
