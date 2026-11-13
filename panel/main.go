package main

import (
	"bufio"
	_ "embed"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"math/bits"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

//go:embed index.html
var indexHTML string

type Profile struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Peer     string `json:"peer"`
	Pass     string `json:"pass"`
	Hashes   string `json:"hashes"`
	Workers  int    `json:"workers"`
	Port     int    `json:"port"`
	Obfs     string `json:"obfs"`
	TurnTCP  bool   `json:"turnTcp"`
	NoDTLS   bool   `json:"noDtls"`
	DNS      string `json:"dns"`
	DeviceID string `json:"deviceId"`
	VkAuth   string `json:"vkAuth"`
}
type Settings struct {
	Theme    string `json:"theme"`
	CorePath string `json:"corePath"`
	WGPath   string `json:"wgPath"`
	Bypass   string `json:"bypass"`
}

const tunnelName = "qwdtt-panel"

var (
	mu             sync.Mutex
	profiles       []Profile
	settings       Settings
	status         = "off"
	active         int
	traffic        string
	rx, tx         float64
	speed          float64
	lastTotal      float64
	lastStat       time.Time
	stage          int
	failedAt       = -1
	turnIPs        = map[string]bool{}
	curID          string
	lastErr        string
	isAdmin        bool
	stopped        = true
	curCmd         *exec.Cmd
	curIn          io.WriteCloser
	baseDir        string
	lastBeat       time.Time
	captchaPending bool
	captchaURL     string
	targetWorkers  int

	logMu   sync.Mutex
	logBuf  []string
	clients = map[chan string]bool{}
)

func hidden(name string, args ...string) *exec.Cmd {
	c := exec.Command(name, args...)
	c.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	return c
}
func msgBox(title, text string) {
	p := syscall.NewLazyDLL("user32.dll").NewProc("MessageBoxW")
	t, _ := syscall.UTF16PtrFromString(text)
	c, _ := syscall.UTF16PtrFromString(title)
	p.Call(0, uintptr(unsafe.Pointer(t)), uintptr(unsafe.Pointer(c)), 0x10)
}
func ensureAdmin() {
	if hidden("net", "session").Run() == nil {
		return
	}
	msgBox("qWDTT • PC — нужны права администратора",
		"Панель не может работать без прав администратора.\n\nЗакрой это окно и запусти qwdtt-panel.exe заново:\nправая кнопка мыши → «Запуск от имени администратора».")
	os.Exit(1)
}
func defaults() {
	settings = Settings{Theme: "dark", CorePath: "qwdtt.exe",
		WGPath: `C:\Program Files\WireGuard\wireguard.exe`,
		Bypass: "87.240.128.0/18, 93.186.224.0/20, 95.213.0.0/17, 90.156.232.0/21, 91.231.132.0/22, 77.88.8.0/24"}
}
func loadAll() {
	// ---------- НАСТРОЙКИ ----------
	// Стартуем с дефолтов из defaults() и накладываем поверх них файл.
	// Файла нет / JSON битый / поля пустые — панель всё равно запустится.
	s := settings
	if b, e := os.ReadFile(filepath.Join(baseDir, "settings.json")); e == nil {
		if je := json.Unmarshal(b, &s); je != nil {
			addLog("[PANEL] settings.json повреждён: " + je.Error() + " — беру настройки по умолчанию")
			s = settings
		}
	} else if !os.IsNotExist(e) {
		addLog("[PANEL] не могу прочитать settings.json: " + e.Error())
	}

	s.Theme = strings.TrimSpace(s.Theme)
	if s.Theme != "light" {
		s.Theme = "dark"
	}
	s.CorePath = strings.TrimSpace(s.CorePath)
	if s.CorePath == "" {
		s.CorePath = "qwdtt.exe"
	}
	s.WGPath = strings.TrimSpace(s.WGPath)
	if s.WGPath == "" {
		s.WGPath = `C:\Program Files\WireGuard\wireguard.exe`
	}
	if strings.TrimSpace(s.Bypass) == "" {
		s.Bypass = "87.240.128.0/18, 93.186.224.0/20, 95.213.0.0/17, 90.156.232.0/21, 91.231.132.0/22, 77.88.8.0/24"
	}
	settings = s

	// ---------- ПРОФИЛИ ----------
	// Удалённый или битый profiles.json = пустой список, а не паника.
	ps := []Profile{}
	if b, e := os.ReadFile(filepath.Join(baseDir, "profiles.json")); e == nil {
		var tmp []Profile
		if je := json.Unmarshal(b, &tmp); je != nil {
			addLog("[PANEL] profiles.json повреждён: " + je.Error() + " — начинаю с пустого списка")
		} else {
			ps = tmp
		}
	} else if !os.IsNotExist(e) {
		addLog("[PANEL] не могу прочитать profiles.json: " + e.Error())
	}
	if ps == nil { // в файле был null
		ps = []Profile{}
	}

	// Подчищаем записи, чтобы кривой конфиг не ронял панель и ядро.
	seen := map[string]bool{}
	for i := range ps {
		if strings.TrimSpace(ps[i].ID) == "" || seen[ps[i].ID] {
			ps[i].ID = fmt.Sprintf("%d", time.Now().UnixNano()+int64(i))
		}
		seen[ps[i].ID] = true
		if ps[i].Workers <= 0 {
			ps[i].Workers = 9
		}
		if ps[i].Port <= 0 {
			ps[i].Port = 9000
		}
		if ps[i].VkAuth == "" {
			ps[i].VkAuth = "anonymous"
		}
	}
	profiles = ps

	// ---------- САМОВОССТАНОВЛЕНИЕ ----------
	// Если файлов не было (ты их удалил) — создаём заново с рабочими значениями.
	if _, e := os.Stat(filepath.Join(baseDir, "settings.json")); e != nil {
		saveSettings()
	}
	if _, e := os.Stat(filepath.Join(baseDir, "profiles.json")); e != nil {
		saveProfiles()
	}

	addLog(fmt.Sprintf("[PANEL] конфиг загружен: профилей %d", len(profiles)))
}
func saveProfiles() {
	b, _ := json.MarshalIndent(profiles, "", "  ")
	os.WriteFile(filepath.Join(baseDir, "profiles.json"), b, 0600)
}
func saveSettings() {
	b, _ := json.MarshalIndent(settings, "", "  ")
	os.WriteFile(filepath.Join(baseDir, "settings.json"), b, 0600)
}

func addLog(s string) {
	logMu.Lock()
	logBuf = append(logBuf, s)
	if len(logBuf) > 600 {
		logBuf = logBuf[len(logBuf)-600:]
	}
	for c := range clients {
		select {
		case c <- s:
		default:
		}
	}
	logMu.Unlock()
}
func sendCore(s string) {
	mu.Lock()
	in := curIn
	mu.Unlock()
	if in != nil {
		io.WriteString(in, s+"\n")
	}
}
func scan(r io.Reader) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		addLog(line)
		parseLine(line)
	}
}
func adv(n int) {
	if n > stage {
		stage = n
	}
}
func parseLine(l string) {
	mu.Lock()
	if i := strings.Index(l, "Активных: "); i >= 0 {
		rest := l[i+len("Активных: "):]
		if j := strings.IndexAny(rest, " |"); j > 0 {
			active, _ = strconv.Atoi(strings.TrimSpace(rest[:j]))
		}
	}
	if i := strings.Index(l, "зарегистрирован (всего: "); i >= 0 {
		rest := l[i+len("зарегистрирован (всего: "):]
		if j := strings.Index(rest, ")"); j > 0 {
			active, _ = strconv.Atoi(strings.TrimSpace(rest[:j]))
		}
	}
	if i := strings.Index(l, "отключён (осталось: "); i >= 0 {
		rest := l[i+len("отключён (осталось: "):]
		if j := strings.Index(rest, ")"); j > 0 {
			active, _ = strconv.Atoi(strings.TrimSpace(rest[:j]))
		}
	}
	if strings.Contains(l, "[СТАТИСТИКА]") {
		if i := strings.Index(l, "Трафик: "); i >= 0 {
			rest := l[i+len("Трафик: "):]
			if j := strings.Index(rest, " МБ"); j > 0 {
				if v, e := strconv.ParseFloat(strings.TrimSpace(rest[:j]), 64); e == nil {
					now := time.Now()
					if !lastStat.IsZero() {
						if dt := now.Sub(lastStat).Seconds(); dt > 0 {
							speed = (v - lastTotal) / dt
							if speed < 0 {
								speed = 0
							}
						}
					}
					lastTotal = v
					lastStat = now
					traffic = fmt.Sprintf("%.2f", v)
				}
			}
		}
		if i := strings.Index(l, "↓"); i >= 0 {
			rest := l[i+len("↓"):]
			if j := strings.Index(rest, " МБ"); j > 0 {
				rx, _ = strconv.ParseFloat(strings.TrimSpace(rest[:j]), 64)
			}
		}
		if i := strings.Index(l, "↑"); i >= 0 {
			rest := l[i+len("↑"):]
			if j := strings.Index(rest, " МБ"); j > 0 {
				tx, _ = strconv.ParseFloat(strings.TrimSpace(rest[:j]), 64)
			}
		}
	}
	if strings.Contains(l, "Креды OK") || strings.Contains(l, "[VKCalls] SUCCESS") {
		adv(1)
	}
	if strings.Contains(l, "Relay:") {
		adv(2)
	}
	if strings.Contains(l, "[DTLS] Соединение установлено") {
		adv(3)
	}
	if strings.Contains(l, "Конфиг получен") || strings.Contains(l, "Сохранён в wg-turn.conf") {
		adv(4)
	}
	if strings.Contains(l, "TURN TCP (") || strings.Contains(l, "TURN UDP (") {
		if i := strings.Index(l, "("); i >= 0 {
			rest := l[i+1:]
			if j := strings.Index(rest, ":"); j > 0 {
				turnIPs[rest[:j]] = true
			}
		}
	}
	if strings.HasPrefix(l, "VK_AUTH_REQUIRED|") {
		link := strings.TrimPrefix(l, "VK_AUTH_REQUIRED|")
		go openVKWindow("join", link)
	}
	if strings.HasPrefix(l, "CAPTCHA_SOLVE|manual|") {
		p := strings.SplitN(l, "|", 4)
		if len(p) >= 3 {
			captchaURL = p[2]
			captchaPending = true
			go openVKWindow("captcha", p[2])
		}
	}
	mu.Unlock()
}

type rng struct{ a, b uint32 }

func ipU32(ip net.IP) uint32 { return binary.BigEndian.Uint32(ip.To4()) }
func u32ip(v uint32) string {
	var b [4]byte
	binary.BigEndian.PutUint32(b[:], v)
	return net.IP(b[:]).String()
}
func parseNet(s string) (rng, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return rng{}, false
	}
	if !strings.Contains(s, "/") {
		ip := net.ParseIP(s)
		if ip == nil || ip.To4() == nil {
			return rng{}, false
		}
		v := ipU32(ip)
		return rng{v, v}, true
	}
	_, n, err := net.ParseCIDR(s)
	if err != nil || n.IP.To4() == nil {
		return rng{}, false
	}
	ones, _ := n.Mask.Size()
	a := ipU32(n.IP)
	size := uint64(1) << (32 - ones)
	return rng{a, uint32(uint64(a) + size - 1)}, true
}
func subtract(rs []rng, e rng) []rng {
	var out []rng
	for _, r := range rs {
		if e.b < r.a || e.a > r.b {
			out = append(out, r)
			continue
		}
		if e.a > r.a {
			out = append(out, rng{r.a, e.a - 1})
		}
		if e.b < r.b {
			out = append(out, rng{e.b + 1, r.b})
		}
	}
	return out
}
func rangesToCIDRs(rs []rng) []string {
	var out []string
	for _, r := range rs {
		a, b := r.a, r.b
		for a <= b {
			var size uint64
			if a == 0 {
				size = 1 << 32
			} else {
				size = uint64(a & (^a + 1))
			}
			rem := uint64(b) - uint64(a) + 1
			for size > rem {
				size >>= 1
			}
			out = append(out, fmt.Sprintf("%s/%d", u32ip(a), 32-bits.Len64(size-1)))
			next := uint64(a) + size
			if next > 0xFFFFFFFF {
				break
			}
			a = uint32(next)
		}
	}
	return out
}
func computeAllowed(bypass string, ips []string) string {
	rs := []rng{{0, 0xFFFFFFFF}}
	for _, s := range strings.FieldsFunc(bypass, func(r rune) bool { return r == ',' || r == '\n' || r == ' ' || r == ';' }) {
		if e, ok := parseNet(s); ok {
			rs = subtract(rs, e)
		}
	}
	for _, ip := range ips {
		if e, ok := parseNet(ip); ok {
			rs = subtract(rs, e)
		}
	}
	return strings.Join(rangesToCIDRs(rs), ", ")
}
func turnKeys() []string {
	var k []string
	for ip := range turnIPs {
		k = append(k, ip)
	}
	return k
}
func setStatus(s string) {
	mu.Lock()
	status = s
	if s == "off" {
		active = 0
		speed = 0
		targetWorkers = 0
		lastStat = time.Time{}
	}
	mu.Unlock()
}
func setErr(s string) {
	mu.Lock()
	lastErr = s
	status = "error"
	failedAt = stage
	mu.Unlock()
	addLog("[PANEL] " + s)
}
func corePath() string {
	mu.Lock()
	p := settings.CorePath
	mu.Unlock()
	if p == "" {
		p = "qwdtt.exe"
	}
	if !filepath.IsAbs(p) {
		p = filepath.Join(baseDir, p)
	}
	return p
}
func startCore(p Profile) (chan struct{}, error) {
	cp := corePath()
	args := []string{"-peer", p.Peer, "-password", p.Pass, "-vk", p.Hashes, "-obfs", p.Obfs, "-mode", "vpn",
		"-listen", fmt.Sprintf("127.0.0.1:%d", p.Port), "-n", strconv.Itoa(p.Workers), "-go-dns", p.DNS, "-device-id", p.DeviceID}
	if p.TurnTCP {
		args = append(args, "-turn-tcp")
	}
	if p.NoDTLS {
		args = append(args, "-notls")
	}
	if p.VkAuth == "account" {
		args = append(args, "-vk-auth", "account")
	}
	cmd := exec.Command(cp, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	cmd.Dir = filepath.Dir(cp)
	in, e := cmd.StdinPipe()
	if e != nil {
		return nil, e
	}
	out, e := cmd.StdoutPipe()
	if e != nil {
		return nil, e
	}
	er, e := cmd.StderrPipe()
	if e != nil {
		return nil, e
	}
	if e := cmd.Start(); e != nil {
		return nil, e
	}
	mu.Lock()
	curCmd = cmd
	curIn = in
	mu.Unlock()
	go scan(out)
	go scan(er)
	done := make(chan struct{})
	go func() { cmd.Wait(); close(done) }()
	return done, nil
}
func installTunnel() error {
	coreDir := filepath.Dir(corePath())
	raw, err := os.ReadFile(filepath.Join(coreDir, "wg-turn.conf"))
	if err != nil {
		return err
	}
	mu.Lock()
	byp, wg := settings.Bypass, settings.WGPath
	ips := turnKeys()
	mu.Unlock()
	allowed := computeAllowed(byp, ips)
	var out []string
	for _, line := range strings.Split(string(raw), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "AllowedIPs") {
			out = append(out, "AllowedIPs = "+allowed)
		} else {
			out = append(out, strings.TrimRight(line, "\r"))
		}
	}
	pd := os.Getenv("ProgramData")
	if pd == "" {
		pd = `C:\ProgramData`
	}
	dir := filepath.Join(pd, "qWDTT")
	os.MkdirAll(dir, 0755)
	panelPath := filepath.Join(dir, tunnelName+".conf")
	if e := os.WriteFile(panelPath, []byte(strings.Join(out, "\n")), 0600); e != nil {
		return e
	}
	if b, e := hidden("icacls", panelPath, "/inheritance:r", "/grant:r", "*S-1-5-18:(F)", "*S-1-5-32-544:(F)").CombinedOutput(); e != nil {
		addLog("[PANEL] icacls: " + strings.TrimSpace(string(b)))
	}
	hidden(wg, "/uninstalltunnelservice", tunnelName).Run()
	time.Sleep(400 * time.Millisecond)
	b, e := hidden(wg, "/installtunnelservice", panelPath).CombinedOutput()
	if e != nil {
		return fmt.Errorf("%v %s", e, strings.TrimSpace(string(b)))
	}
	svc := "WireGuardTunnel$" + tunnelName
	for i := 0; i < 20; i++ {
		o, _ := hidden("sc", "query", svc).CombinedOutput()
		if strings.Contains(string(o), "RUNNING") {
			addLog("[PANEL] служба туннеля запущена ✓")
			mu.Lock()
			stage = 5
			mu.Unlock()
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("служба %s не перешла в RUNNING", svc)
}
func waitConf(path string, done chan struct{}, d time.Duration) bool {
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		select {
		case <-done:
			return false
		default:
		}
		if _, e := os.Stat(path); e == nil {
			return true
		}
		time.Sleep(500 * time.Millisecond)
	}
	return false
}
func backoff(n int) int {
	v := 5 + n*2
	if v > 30 {
		v = 30
	}
	return v
}
func supervise(p Profile) {
	conf := filepath.Join(filepath.Dir(corePath()), "wg-turn.conf")
	for attempt := 0; ; attempt++ {
		mu.Lock()
		if stopped {
			mu.Unlock()
			return
		}
		mu.Unlock()
		setStatus("starting")
		os.Remove(conf)
		done, err := startCore(p)
		if err != nil {
			setErr("не удалось запустить ядро: " + err.Error())
			return
		}
		if waitConf(conf, done, 90*time.Second) {
			setStatus("tunnel")
			if e := installTunnel(); e != nil {
				setErr("WireGuard: " + e.Error())
			} else {
				setStatus("on")
				addLog("[PANEL] туннель WireGuard поднят автоматически ✓")
			}
		}
		<-done
		mu.Lock()
		s := stopped
		mu.Unlock()
		if s {
			break
		}
		setStatus("restarting")
		addLog(fmt.Sprintf("[PANEL] мотор завершился, перезапуск через %d с", backoff(attempt)))
		time.Sleep(time.Duration(backoff(attempt)) * time.Second)
	}
	setStatus("off")
}
func connect(p Profile) {
	mu.Lock()
	if !stopped {
		mu.Unlock()
		return
	}
	stopped = false
	curID = p.ID
	turnIPs = map[string]bool{}
	active = 0
	traffic = ""
	rx, tx, speed = 0, 0, 0
	lastTotal = 0
	lastStat = time.Time{}
	targetWorkers = p.Workers
	stage = 0
	failedAt = -1
	lastErr = ""
	captchaPending = false
	mu.Unlock()
	go supervise(p)
}
func disconnect() {
	mu.Lock()
	stopped = true
	cmd, in, wg := curCmd, curIn, settings.WGPath
	mu.Unlock()
	closeVKWindow()
	hidden(wg, "/uninstalltunnelservice", tunnelName).Run()
	if in != nil {
		io.WriteString(in, "STOP\n")
	}
	if cmd != nil && cmd.Process != nil {
		time.AfterFunc(4*time.Second, func() { cmd.Process.Kill() })
	}
	setStatus("off")
}
func bye() {
	go func() { markClosing(); disconnect(); trayQuit(); time.Sleep(2500 * time.Millisecond); os.Exit(0) }()
}
func importLink(link string) (Profile, error) {
	u, err := url.Parse(strings.TrimSpace(link))
	if err != nil {
		return Profile{}, err
	}
	q := u.Query()
	p := Profile{ID: fmt.Sprintf("%d", time.Now().UnixNano()), Name: q.Get("name"), Peer: q.Get("peer"), Pass: q.Get("pass"),
		Hashes: q.Get("hashes"), Obfs: "audio", TurnTCP: true, DNS: "yandex", DeviceID: "kot-pc", Port: 9000, Workers: 9, VkAuth: "anonymous"}
	if p.Name == "" {
		p.Name = "Профиль"
	}
	if v := q.Get("workers"); v != "" {
		p.Workers, _ = strconv.Atoi(v)
	}
	if v := q.Get("port"); v != "" {
		p.Port, _ = strconv.Atoi(v)
	}
	if p.Workers < 9 {
		p.Workers = 9
	}
	if p.Port == 0 {
		p.Port = 9000
	}
	return p, nil
}
func exportLink(p Profile) string {
	q := url.Values{}
	q.Set("name", p.Name)
	q.Set("peer", p.Peer)
	q.Set("hashes", p.Hashes)
	q.Set("workers", strconv.Itoa(p.Workers))
	q.Set("port", strconv.Itoa(p.Port))
	q.Set("pass", p.Pass)
	return "qwdtt://config?" + q.Encode()
}
func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}
func snapshot() map[string]interface{} {
	mu.Lock()
	defer mu.Unlock()
	return map[string]interface{}{"status": status, "active": active, "traffic": traffic, "rx": rx, "tx": tx, "speed": speed,
		"stage": stage, "failedAt": failedAt, "target": targetWorkers, "turn": turnKeys(), "profiles": profiles, "settings": settings,
		"current": curID, "error": lastErr, "admin": isAdmin, "warn": lastWarn, "closing": uiClosing, "uiGen": uiGen, "captchaPending": captchaPending, "captchaURL": captchaURL}
}
func logsSSE(w http.ResponseWriter, r *http.Request) {
	f, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "no stream", 500)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	ch := make(chan string, 1024)
	logMu.Lock()
	for _, l := range logBuf {
		ch <- l
	}
	clients[ch] = true
	logMu.Unlock()
	defer func() { logMu.Lock(); delete(clients, ch); logMu.Unlock() }()
	for {
		select {
		case <-r.Context().Done():
			return
		case line := <-ch:
			fmt.Fprintf(w, "data: %s\n\n", strings.ReplaceAll(line, "\n", " "))
			f.Flush()
		}
	}
}
func main() {
	exe, _ := os.Executable(); baseDir = filepath.Dir(exe)
	installCrashLog()
	defer crashGuard()
	if pingExisting() { requestReopen(); os.Exit(0) }
	ensureAdmin(); defaults(); loadAll()
	isAdmin = hidden("net", "session").Run() == nil
	lastBeat = time.Now()
	loadVkLogin()

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		io.WriteString(w, indexHTML)
	})
	http.HandleFunc("/api/state", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, snapshot()) })
	http.HandleFunc("/api/logs", logsSSE)
	http.HandleFunc("/api/beat", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		lastBeat = time.Now()
		mu.Unlock()
		writeJSON(w, map[string]bool{"ok": true})
	})
	http.HandleFunc("/api/bye", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, map[string]bool{"ok": true}); bye() })
	http.HandleFunc("/api/vklogin", func(w http.ResponseWriter, r *http.Request) {
		go openVKWindow("login", "")
		writeJSON(w, map[string]bool{"ok": true})
	})
	http.HandleFunc("/api/captcha", func(w http.ResponseWriter, r *http.Request) {
		var b struct {
			Result string `json:"result"`
		}
		json.NewDecoder(r.Body).Decode(&b)
		mu.Lock()
		captchaPending = false
		mu.Unlock()
		if b.Result != "" {
			sendCore("CAPTCHA_RESULT|" + strings.TrimSpace(b.Result))
			addLog("[PANEL] результат капчи отправлен ядру")
		}
		writeJSON(w, map[string]bool{"ok": true})
	})
	http.HandleFunc("/api/profiles", func(w http.ResponseWriter, r *http.Request) {
		var p Profile
		json.NewDecoder(r.Body).Decode(&p)
		if p.ID == "" {
			p.ID = fmt.Sprintf("%d", time.Now().UnixNano())
		}
		if p.VkAuth == "" {
			p.VkAuth = "anonymous"
		}
		mu.Lock()
		found := false
		for i := range profiles {
			if profiles[i].ID == p.ID {
				profiles[i] = p
				found = true
			}
		}
		if !found {
			profiles = append(profiles, p)
		}
		saveProfiles()
		mu.Unlock()
		writeJSON(w, p)
	})
	http.HandleFunc("/api/delete", func(w http.ResponseWriter, r *http.Request) {
		var b struct {
			ID string `json:"id"`
		}
		json.NewDecoder(r.Body).Decode(&b)
		mu.Lock()
		var np []Profile
		for _, p := range profiles {
			if p.ID != b.ID {
				np = append(np, p)
			}
		}
		profiles = np
		saveProfiles()
		mu.Unlock()
		writeJSON(w, map[string]bool{"ok": true})
	})
	http.HandleFunc("/api/import", func(w http.ResponseWriter, r *http.Request) {
		var b struct {
			Link string `json:"link"`
		}
		json.NewDecoder(r.Body).Decode(&b)
		p, e := importLink(b.Link)
		if e != nil {
			writeJSON(w, map[string]string{"error": e.Error()})
			return
		}
		mu.Lock()
		profiles = append(profiles, p)
		saveProfiles()
		mu.Unlock()
		writeJSON(w, p)
	})
	http.HandleFunc("/api/export", func(w http.ResponseWriter, r *http.Request) {
		id := r.URL.Query().Get("id")
		mu.Lock()
		var p Profile
		for _, x := range profiles {
			if x.ID == id {
				p = x
			}
		}
		mu.Unlock()
		writeJSON(w, map[string]string{"link": exportLink(p)})
	})
	http.HandleFunc("/api/connect", func(w http.ResponseWriter, r *http.Request) {
		var b struct {
			ID string `json:"id"`
		}
		json.NewDecoder(r.Body).Decode(&b)
		mu.Lock()
		var p Profile
		for _, x := range profiles {
			if x.ID == b.ID {
				p = x
			}
		}
		mu.Unlock()
		tryConnect(p)
		writeJSON(w, map[string]bool{"ok": true})
	})
	http.HandleFunc("/api/disconnect", func(w http.ResponseWriter, r *http.Request) { disconnect(); writeJSON(w, map[string]bool{"ok": true}) })
	http.HandleFunc("/api/settings", func(w http.ResponseWriter, r *http.Request) {
		var s Settings
		json.NewDecoder(r.Body).Decode(&s)
		mu.Lock()
		settings = s
		saveSettings()
		mu.Unlock()
		writeJSON(w, s)
	})
	http.HandleFunc("/api/quit", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, map[string]bool{"ok": true}); bye() })
	http.HandleFunc("/api/ping", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, map[string]bool{"ok": true}) })
	http.HandleFunc("/api/reopen", func(w http.ResponseWriter, r *http.Request) { bumpUI(); writeJSON(w, map[string]bool{"ok": true}) })

	addr := "127.0.0.1:8765"
	ln, e := net.Listen("tcp", addr)
	if e != nil {
		msgBox("qWDTT • PC — порт занят",
			"Не могу занять "+addr+".\n\nСкорее всего, висит старая копия панели.\nДиспетчер задач → Подробности → сними qwdtt-panel.exe и запусти снова.")
		os.Exit(1)
	}
	addLog("[PANEL] запущен: http://" + addr)
	go http.Serve(ln, nil)
	openBrowser()
	trayMain() // блокирующий вызов — держим последним
}
