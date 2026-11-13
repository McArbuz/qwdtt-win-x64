package main

import (
	"encoding/base64"
	"encoding/json"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/jchv/go-webview2"
)

var (
	vkMu         sync.Mutex
	vkOpen       bool
	vkW          webview2.WebView
	vkPendingURL string
	vkLastURL    string
	vkHash       string
	vkCandIdx    int
)

func stripHash(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.Index(s, "/call/join/"); i >= 0 {
		s = s[i+len("/call/join/"):]
	}
	if i := strings.IndexAny(s, "?#/"); i >= 0 {
		s = s[:i]
	}
	return strings.Trim(s, "/")
}
func joinCandidates(h string) []string {
	return []string{
		"https://m.vk.ru/call/join/" + h,
		"https://vk.ru/call/join/" + h,
		"https://m.vk.com/call/join/" + h,
		"https://vk.com/call/join/" + h,
	}
}

const vkJS = `(function(){
  try{Object.defineProperty(window,'onbeforeunload',{configurable:true,get:function(){return null;},set:function(){}});}catch(e){}
  try{var _ae=window.addEventListener.bind(window);window.addEventListener=function(t,l,o){if(String(t).toLowerCase()==='beforeunload')return;return _ae(t,l,o);};_ae('beforeunload',function(e){e.stopImmediatePropagation();},true);}catch(e){}
  function stopJoin(){try{clearInterval(window.__wdtt_join_timer);}catch(e){}try{window.__wdtt_obs&&window.__wdtt_obs.disconnect();}catch(e){}}
  function emitTurn(t){try{var d=JSON.parse(t);var ts=d.turn_server||(d.response&&d.response.turn_server);if(ts&&ts.username&&ts.credential&&ts.urls){window.wdttTurn(JSON.stringify(ts));window.__wdtt_turn_done=true;stopJoin();}}catch(e){}}
  function emitCap(t){try{var d=JSON.parse(t);var s=(d.response&&d.response.success_token)||d.success_token;if(s){window.wdttCaptcha(s);}}catch(e){}}
  if(!window.__wdtt_hook){
    window.__wdtt_hook=true;
    var of=window.fetch;
    window.fetch=function(){
      var a=arguments;var u=(typeof a[0]==='string')?a[0]:((a[0]&&a[0].url)||'');
      return of.apply(this,a).then(function(r){
        try{r.clone().text().then(function(t){
          if(t&&t.indexOf('turn_server')!==-1)emitTurn(t);
          if(u.indexOf('captchaNotRobot.check')!==-1||(t&&t.indexOf('success_token')!==-1))emitCap(t);
        });}catch(e){}
        return r;
      });
    };
    var oo=XMLHttpRequest.prototype.open,os=XMLHttpRequest.prototype.send;
    XMLHttpRequest.prototype.open=function(m,u){this._wu=u;return oo.apply(this,arguments);};
    XMLHttpRequest.prototype.send=function(){
      var x=this;
      x.addEventListener('load',function(){
        try{var t=x.responseText;
          if(t&&t.indexOf('turn_server')!==-1)emitTurn(t);
          if((x._wu&&x._wu.indexOf('captchaNotRobot.check')!==-1)||(t&&t.indexOf('success_token')!==-1))emitCap(t);
        }catch(e){}
      });
      return os.apply(this,arguments);
    };
  }
  var reject=['открыть в приложении','open in app','open in the','vk звонки','скачать приложение'];
  function norm(s){return (s||'').replace(/\s+/g,' ').trim().toLowerCase();}
  function etext(el){return norm(el.innerText||el.textContent||el.value||el.getAttribute('aria-label')||'');}
  function score(el){
    var t=etext(el);if(!t||t.length>120)return -1;
    for(var i=0;i<reject.length;i++){if(t.indexOf(reject[i])!==-1)return -1;}
    if(t.indexOf('продолжить в браузере')!==-1&&t.indexOf('открыть')!==-1&&t.length>30)return -1;
    if(t==='продолжить в браузере'||t==='continue in browser')return 100;
    if(t.indexOf('продолжить в браузере')!==-1)return 90-Math.min(t.length,80);
    if(t.indexOf('continue in browser')!==-1)return 85-Math.min(t.length,80);
    if(t==='присоединиться'||t==='join')return 95;
    if(t.indexOf('присоединиться к звонку через браузер')!==-1)return 70;
    if(t.indexOf('присоединиться через браузер')!==-1)return 65;
    if(t.indexOf('присоединиться к звонку')!==-1)return 60;
    if(t.indexOf('присоединиться')!==-1&&t.length<=40)return 55;
    if(t.indexOf('войти в звонок')!==-1)return 50;
    if(t==='продолжить'||t==='continue')return 40;
    if(t.indexOf('продолжить')!==-1&&t.length<=25)return 35;
    return -1;
  }
  function betterChild(el,ps){var k=el.querySelectorAll('button, a, [role="button"], input[type="button"], input[type="submit"]');for(var i=0;i<k.length;i++){if(k[i]===el)continue;if(score(k[i])>=ps)return true;}return false;}
  function pick(min){var ns=document.querySelectorAll('button, a, [role="button"], input[type="button"], input[type="submit"], .vkuiButton, [class*="Button"]');var b=null,bs=-1,bl=9999;for(var i=0;i<ns.length;i++){var el=ns[i];var sc=score(el);if(sc<min)continue;if(betterChild(el,sc))continue;var tl=etext(el).length;if(sc>bs||(sc===bs&&tl<bl)){b=el;bs=sc;bl=tl;}}return b?{el:b,score:bs,text:etext(b)}:null;}
  function fire(el){try{el.scrollIntoView({block:'center'});}catch(e){}try{el.click();}catch(e){}try{el.dispatchEvent(new MouseEvent('click',{bubbles:true,cancelable:true,view:window}));}catch(e){}}
  var done=false;
  function tryJoin(){
    if(done||window.__wdtt_turn_done)return;
    var p=pick(50)||pick(35);if(!p)return;
    fire(p.el);done=true;setTimeout(function(){done=false;},2200);
    try{window.wdttLog('автоклик: '+p.text.substring(0,50));}catch(e){}
  }
  if(!window.__wdtt_join_timer){
    window.__wdtt_join_timer=setInterval(tryJoin,1000);
    try{window.__wdtt_obs=new MutationObserver(function(){tryJoin();});window.__wdtt_obs.observe(document.documentElement||document.body,{childList:true,subtree:true});}catch(e){}
    setTimeout(function(){stopJoin();},60000);
  }
  function chk404(){var t=document.body?document.body.innerText:'';if(t.indexOf('Такой страницы нет')!==-1||t.indexOf('Страница не найдена')!==-1){try{window.wdtt404();}catch(e){}}}
  setTimeout(chk404,1500);setTimeout(chk404,4500);
  function chkLogin(){try{var h=location.host;if(h.indexOf('vk.ru')===-1&&h.indexOf('vk.com')===-1)return;fetch(location.origin+'/settings',{credentials:'include',redirect:'follow'}).then(function(r){var u=r.url||'';var ok=u.indexOf('login')===-1&&u.indexOf('auth')===-1;try{window.wdttLogin(ok?1:0);}catch(e){}}).catch(function(){});}catch(e){}}
  if(!window.__wdtt_login_timer){window.__wdtt_login_timer=setInterval(chkLogin,3000);setTimeout(chkLogin,1200);}
})();`

func openVKWindow(mode, arg string) {
	var url string
	switch mode {
	case "login":
		url = "https://m.vk.ru/login"
	case "captcha":
		url = arg
	case "join":
		h := stripHash(arg)
		vkMu.Lock()
		vkHash = h
		vkCandIdx = 0
		vkMu.Unlock()
		url = joinCandidates(h)[0]
	default:
		url = "https://m.vk.ru/"
	}
	vkMu.Lock()
	vkPendingURL = url
	go focusVK()
	if vkOpen {
		vkMu.Unlock()
		return
	}
	vkOpen = true
	vkMu.Unlock()
	go runVKWindow()
}

func vkHWND() uintptr {
	t, _ := syscall.UTF16PtrFromString("qWDTT — VK")
	h, _, _ := syscall.NewLazyDLL("user32.dll").NewProc("FindWindowW").Call(0, uintptr(unsafe.Pointer(t)))
	return h
}

func closeVKWindow() {
	vkMu.Lock()
	w := vkW
	vkMu.Unlock()
	if w == nil {
		return
	}
	w.Dispatch(func() { w.Navigate("about:blank") })
	time.Sleep(350 * time.Millisecond)
	w.Dispatch(func() {
		if h := vkHWND(); h != 0 {
			syscall.NewLazyDLL("user32.dll").NewProc("DestroyWindow").Call(h)
		}
		w.Terminate()
	})
	time.Sleep(350 * time.Millisecond)
	if h := vkHWND(); h != 0 {
		syscall.NewLazyDLL("user32.dll").NewProc("PostMessageW").Call(h, 0x0010, 0, 0)
	}
}

func focusVK() {
	u := syscall.NewLazyDLL("user32.dll")
	find := u.NewProc("FindWindowW")
	show := u.NewProc("ShowWindow")
	fg := u.NewProc("SetForegroundWindow")
	pos := u.NewProc("SetWindowPos")
	t, _ := syscall.UTF16PtrFromString("qWDTT — VK")
	for i := 0; i < 30; i++ {
		h, _, _ := find.Call(0, uintptr(unsafe.Pointer(t)))
		if h != 0 {
			show.Call(h, 9)
			pos.Call(h, ^uintptr(0), 0, 0, 0, 0, 3)
			pos.Call(h, ^uintptr(1), 0, 0, 0, 0, 3)
			fg.Call(h)
			return
		}
		time.Sleep(150 * time.Millisecond)
	}
}

func runVKWindow() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	w := webview2.NewWithOptions(webview2.WebViewOptions{Debug: false, AutoFocus: true, DataPath: filepath.Join(baseDir, "vkprofile")})
	if w == nil {
		addLog("[VK] WebView2 недоступен")
		vkMu.Lock()
		vkOpen = false
		vkMu.Unlock()
		return
	}
	vkMu.Lock()
	vkW = w
	vkMu.Unlock()
	defer w.Destroy()
	w.SetTitle("qWDTT — VK")
	w.SetSize(1024, 780, webview2.HintNone)

	w.Bind("wdttTurn", func(js string) {
		var ts struct {
			Username   string   `json:"username"`
			Credential string   `json:"credential"`
			URLs       []string `json:"urls"`
		}
		if json.Unmarshal([]byte(js), &ts) != nil || ts.Username == "" || ts.Credential == "" || len(ts.URLs) == 0 {
			return
		}
		b, _ := json.Marshal(map[string]interface{}{"u": ts.Username, "p": ts.Credential, "urls": ts.URLs})
		vkMu.Lock()
		h := vkHash
		vkMu.Unlock()
		sendCore("TURN_CREDS|" + h + "|" + base64.StdEncoding.EncodeToString(b))
		addLog("[VK] TURN-креды аккаунта переданы ядру ✓")
	})
	w.Bind("wdttCaptcha", func(token string) {
		if token != "" {
			sendCore("CAPTCHA_RESULT|" + token)
			addLog("[VK] токен капчи передан ядру ✓")
		}
	})
	w.Bind("wdtt404", func() {
		vkMu.Lock()
		h := vkHash
		vkCandIdx++
		idx := vkCandIdx
		vkMu.Unlock()
		c := joinCandidates(h)
		if idx < len(c) {
			addLog("[VK] 404 — пробую другой адрес звонка…")
			vkMu.Lock()
			vkPendingURL = c[idx]
			vkMu.Unlock()
		} else {
			addLog("[VK] хеш звонка недействителен или устарел — обнови хеш в профиле")
		}
	})
	w.Bind("wdttLog", func(msg string) { addLog("[VK] " + msg) })
	w.Bind("wdttLogin", func(v int) { setVkLoggedIn(v != 0) })

	w.Init(vkJS)
	vkMu.Lock()
	init := vkPendingURL
	vkLastURL = init
	vkMu.Unlock()
	w.Navigate(init)

	go func() {
		for {
			vkMu.Lock()
			open := vkOpen
			pend := vkPendingURL
			last := vkLastURL
			vkMu.Unlock()
			if !open {
				return
			}
			if pend != last && pend != "" {
				vkMu.Lock()
				vkLastURL = pend
				vkMu.Unlock()
				u := pend
				w.Dispatch(func() { w.Navigate(u) })
			}
			time.Sleep(300 * time.Millisecond)
		}
	}()

	w.Run()
	vkMu.Lock()
	vkOpen = false
	vkW = nil
	vkMu.Unlock()
}
