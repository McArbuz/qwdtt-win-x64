package main

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/getlantern/systray"
)

var (
	lastWarn   string
	vkLoginMu  sync.Mutex
	vkLoggedIn bool
	uiGen      int
	uiClosing  bool
)

func setWarn(s string) {
	mu.Lock()
	lastWarn = s
	mu.Unlock()
	if s != "" {
		addLog("[PANEL] " + s)
	}
}

func vkLoginFile() string { return filepath.Join(baseDir, "vk_login.json") }
func loadVkLogin() {
	if b, e := os.ReadFile(vkLoginFile()); e == nil {
		var s struct {
			Logged bool `json:"logged"`
		}
		if json.Unmarshal(b, &s) == nil {
			vkLoggedIn = s.Logged
		}
	}
}
func saveVkLogin() {
	b, _ := json.Marshal(struct {
		Logged bool `json:"logged"`
	}{vkLoggedIn})
	os.WriteFile(vkLoginFile(), b, 0600)
}
func setVkLoggedIn(v bool) {
	vkLoginMu.Lock()
	changed := vkLoggedIn != v
	vkLoggedIn = v
	vkLoginMu.Unlock()
	if changed {
		saveVkLogin()
		if v {
			addLog("[VK] вход в аккаунт подтверждён ✓")
		}
	}
}
func vkIsLoggedIn() bool { vkLoginMu.Lock(); defer vkLoginMu.Unlock(); return vkLoggedIn }

func tryConnect(p Profile) {
	if p.Peer == "" {
		setWarn("Нет профиля: выбери или создай профиль.")
		return
	}
	if p.VkAuth == "account" && !vkIsLoggedIn() {
		setWarn("Сначала войди в аккаунт VK: нажми «🔑 Войти в VK», затем «Подключить».")
		go openVKWindow("login", "")
		return
	}
	setWarn("")
	connect(p)
}

func trayConnect() {
	mu.Lock()
	id := curID
	if id == "" && len(profiles) > 0 {
		id = profiles[0].ID
	}
	var p Profile
	for _, x := range profiles {
		if x.ID == id {
			p = x
		}
	}
	mu.Unlock()
	tryConnect(p)
}

func openBrowser() {
	go hidden("rundll32", "url.dll,FileProtocolHandler", "http://127.0.0.1:8765").Start()
}
func trayQuit() { systray.Quit() }

func makeIcon() []byte {
	const S = 32
	px := make([]byte, S*S*4)
	for y := 0; y < S; y++ {
		for x := 0; x < S; x++ {
			t := float64(x+y) / float64(2*S)
			r := uint8(79 + (124-79)*t)
			g := uint8(140 + (92-140)*t)
			b := uint8(255)
			dx := float64(x) - 15.5
			dy := float64(y) - 15.5
			if d := math.Sqrt(dx*dx + dy*dy); d >= 8 && d <= 11 {
				r, g, b = 255, 255, 255
			}
			i := (y*S + x) * 4
			px[i] = b
			px[i+1] = g
			px[i+2] = r
			px[i+3] = 255
		}
	}
	xor := make([]byte, 0, S*S*4)
	for y := S - 1; y >= 0; y-- {
		xor = append(xor, px[y*S*4:(y+1)*S*4]...)
	}
	and := make([]byte, S*4)
	hdr := make([]byte, 40)
	binary.LittleEndian.PutUint32(hdr[0:], 40)
	binary.LittleEndian.PutUint32(hdr[4:], S)
	binary.LittleEndian.PutUint32(hdr[8:], S*2)
	binary.LittleEndian.PutUint16(hdr[12:], 1)
	binary.LittleEndian.PutUint16(hdr[14:], 32)
	binary.LittleEndian.PutUint32(hdr[20:], uint32(len(xor)+len(and)))
	img := append(hdr, xor...)
	img = append(img, and...)
	out := make([]byte, 22)
	binary.LittleEndian.PutUint16(out[2:], 1)
	binary.LittleEndian.PutUint16(out[4:], 1)
	out[6] = S
	out[7] = S
	binary.LittleEndian.PutUint16(out[10:], 1)
	binary.LittleEndian.PutUint16(out[12:], 32)
	binary.LittleEndian.PutUint32(out[14:], uint32(len(img)))
	binary.LittleEndian.PutUint32(out[18:], 22)
	return append(out, img...)
}

func trayMain() { systray.Run(onTrayReady, func() {}) }

func onTrayReady() {
	systray.SetIcon(makeIcon())
	systray.SetTooltip("qWDTT • PC")
	mShow := systray.AddMenuItem("Открыть панель", "")
	mConn := systray.AddMenuItem("Подключить", "")
	mDisc := systray.AddMenuItem("Отключить", "")
	systray.AddSeparator()
	mQuit := systray.AddMenuItem("Выход", "")
	go func() {
		for {
			select {
			case <-mShow.ClickedCh:
				openBrowser()
			case <-mConn.ClickedCh:
				trayConnect()
			case <-mDisc.ClickedCh:
				disconnect()
			case <-mQuit.ClickedCh:
				bye()
			}
		}
	}()
	go func() {
		for {
			mu.Lock()
			st := status
			a := active
			mu.Unlock()
			txt := map[string]string{"off": "выключено", "starting": "запуск", "tunnel": "туннель", "on": "подключено", "restarting": "перезапуск", "error": "ошибка"}[st]
			systray.SetTooltip(fmt.Sprintf("qWDTT • %s • потоков %d", txt, a))
			time.Sleep(2 * time.Second)
		}
	}()
}

func pingExisting() bool {
	c := http.Client{Timeout: 700 * time.Millisecond}
	r, e := c.Get("http://127.0.0.1:8765/api/ping")
	if e != nil {
		return false
	}
	r.Body.Close()
	return r.StatusCode == 200
}
func requestReopen() {
	c := http.Client{Timeout: 1500 * time.Millisecond}
	r, e := c.Post("http://127.0.0.1:8765/api/reopen", "application/json", nil)
	if e == nil {
		r.Body.Close()
	}
}
func bumpUI()      { mu.Lock(); uiGen++; mu.Unlock(); openBrowser() }
func markClosing() { mu.Lock(); uiClosing = true; mu.Unlock() }
