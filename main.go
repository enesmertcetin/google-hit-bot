package main

import (
	"bufio"
	"crypto/rand"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"
)

// ==================== RENK KODLARI ====================

const (
	colorReset = "\033[0m"
	colorGreen = "\033[1;32m"
	colorCyan  = "\033[1;36m"
)

// enableANSI Windows konsolunda ANSI renk desteğini etkinleştirir
func enableANSI() {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	setConsoleMode := kernel32.NewProc("SetConsoleMode")
	getConsoleMode := kernel32.NewProc("GetConsoleMode")
	handle, _ := syscall.GetStdHandle(syscall.STD_OUTPUT_HANDLE)
	var mode uint32
	getConsoleMode.Call(uintptr(handle), uintptr(unsafe.Pointer(&mode)))
	mode |= 0x0004 // ENABLE_VIRTUAL_TERMINAL_PROCESSING
	setConsoleMode.Call(uintptr(handle), uintptr(mode))
}

// ==================== YAPILANDIRMA ====================

const (
	MaxPages      = 10  // Aranacak maksimum Google sayfa sayısı
	NumThreads    = 100  // Eşzamanlı goroutine sayısı
	MaxHitRetries = 9    // POST PING deneme sayısı (BAS: CYCLE_INDEX >= 9)
)

// Varsayılan değerler
const (
	DefaultCookieDir = `C:\Users\enesm\OneDrive\Desktop\yenicok1\yenicok1`
	DefaultProxy     = "PROXY_HOST:PORT:YOUR_PROXY_USERNAME:YOUR_PROXY_PASSWORD"
)	

// ==================== İSTATİSTİKLER ====================

var (
	statSearches int64 // Toplam arama denemesi
	statFound    int64 // Site bulundu
	statHits     int64 // Başarılı PING hit
	statSkipped  int64 // Atlanan (JS-only, consent, kötü cookie)
	statErrors   int64 // Gerçek hata sayısı (timeout, HTTP hata vb.)
)

// ==================== HTTP İSTEMCİLERİ ====================

var (
	clientFollow   *http.Client // Redirect takip eden (arama için)
	clientNoFollow *http.Client // Redirect takip etmeyen (POST PING için)
)

// ==================== MAIN ====================

func main() {
	enableANSI()
	reader := bufio.NewReader(os.Stdin)

	fmt.Println("╔══════════════════════════════════════╗")
	fmt.Println("║       GOOGLE HIT BOT (Go v2)         ║")
	fmt.Println("╚══════════════════════════════════════╝")
	fmt.Println()

	// Anahtar kelime
	fmt.Print("Anahtar Kelime: ")
	keyword, _ := reader.ReadString('\n')
	keyword = strings.TrimSpace(keyword)
	if keyword == "" {
		log.Fatal("[HATA] Anahtar kelime boş olamaz!")
	}

	// Hedef site
	fmt.Print("Hedef Site (örn: emcreklam.com): ")
	targetSite, _ := reader.ReadString('\n')
	targetSite = strings.TrimSpace(targetSite)
	if targetSite == "" {
		log.Fatal("[HATA] Hedef site boş olamaz!")
	}

	// Cookie dizini
	fmt.Printf("Cookie Klasörü [%s]: ", DefaultCookieDir)
	cookieDir, _ := reader.ReadString('\n')
	cookieDir = strings.TrimSpace(cookieDir)
	if cookieDir == "" {
		cookieDir = DefaultCookieDir
	}

	// Proxy
	fmt.Printf("Proxy [%s]: ", DefaultProxy)
	proxyStr, _ := reader.ReadString('\n')
	proxyStr = strings.TrimSpace(proxyStr)
	if proxyStr == "" {
		proxyStr = DefaultProxy
	}

	// Cookie dosya listesi (dosyaları OKUMAZ, sadece yol listesi)
	fmt.Print("[*] Cookie dosyaları taranıyor... ")
	cookieFiles, err := listCookieFiles(cookieDir)
	if err != nil {
		log.Fatalf("\n[HATA] Cookie dizini okunamadı: %v", err)
	}
	if len(cookieFiles) == 0 {
		log.Fatalf("\n[HATA] '%s' dizininde hiç .txt cookie dosyası bulunamadı!", cookieDir)
	}
	fmt.Printf("%d adet cookie dosyası bulundu.\n", len(cookieFiles))

	// Proxy ayarla
	fmt.Print("[*] Proxy ayarlanıyor... ")
	proxyURL := parseProxy(proxyStr)
	initClients(proxyURL)
	fmt.Println("OK")

	// Proxy test
	fmt.Print("[*] Proxy bağlantı testi (google.com)... ")
	if err := testProxy(); err != nil {
		fmt.Printf("BAŞARISIZ: %v\n", err)
		fmt.Println("[WARN] Proxy çalışmıyor olabilir, devam ediliyor...")
	} else {
		fmt.Println("BAŞARILI")
	}

	// İlk cookie dosyasını test et
	fmt.Print("[*] Cookie format testi... ")
	testCookie, testErr := readCookie(cookieFiles[0])
	if testErr != nil {
		fmt.Printf("HATA: %v\n", testErr)
	} else if testCookie == "" {
		fmt.Println("BOŞ (google.com cookie bulunamadı)")
	} else {
		cookieCount := len(strings.Split(testCookie, ";"))
		fmt.Printf("OK (%d cookie bulundu)\n", cookieCount)
	}

	fmt.Println()
	fmt.Println("========================================")
	fmt.Printf("[*] Anahtar Kelime : %s\n", keyword)
	fmt.Printf("[*] Hedef Site     : %s\n", targetSite)
	fmt.Printf("[*] Cookie Sayısı  : %d\n", len(cookieFiles))
	fmt.Printf("[*] Proxy          : %s\n", proxyStr)
	fmt.Printf("[*] Thread Sayısı  : %d\n", NumThreads)
	fmt.Printf("[*] Maks Sayfa     : %d\n", MaxPages)
	fmt.Printf("[*] Hit Tipi       : GET click + POST PING (retry: %d)\n", MaxHitRetries)
	fmt.Printf("[*] UA Tipi        : iOS GSA + Android Chrome (karışık)\n")
	fmt.Println("========================================")
	fmt.Println("[*] Bot başlatılıyor...")
	fmt.Printf("[*] %d thread başlatıldı, hit'ler bekleniyor...\n\n", NumThreads)

	// İstatistik goroutine'i
	go printStats()

	// Worker goroutine'leri
	var wg sync.WaitGroup
	for i := 0; i < NumThreads; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			worker(id, keyword, targetSite, cookieFiles)
		}(i + 1)
	}

	wg.Wait()
}

// ==================== COOKIE YÜKLEMEİ ====================

// Cookie JSON tipleri (BAS export format)
type CookieJSON struct {
	Domain         string  `json:"domain"`
	Name           string  `json:"name"`
	Value          string  `json:"value"`
	Path           string  `json:"path"`
	Expires        float64 `json:"expires"`
	ExpirationDate float64 `json:"expirationDate"`
}

type CookieWrapper struct {
	Cookies []CookieJSON `json:"cookies"`
}

// listCookieFiles dizindeki .txt dosya yollarını listeler (içerik OKUMAZ)
func listCookieFiles(dir string) ([]string, error) {
	files, err := filepath.Glob(filepath.Join(dir, "*.txt"))
	if err != nil {
		return nil, err
	}
	return files, nil
}

// readCookie cookie dosyasını okur, JSON parse eder veya raw text olarak döner
// BAS: Read File → HTTP-Client Restore Cookies
func readCookie(filePath string) (string, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", err
	}

	content := string(data)
	// BOM kaldır
	content = strings.TrimPrefix(content, "\xef\xbb\xbf")
	content = strings.TrimPrefix(content, "\ufeff")
	content = strings.TrimSpace(content)

	if content == "" {
		return "", nil
	}

	// JSON format dene (BAS export: {"cookies":[...]} veya [...])
	if strings.HasPrefix(content, "{") || strings.HasPrefix(content, "[") {
		cookieStr, err := parseCookieJSON(content)
		if err == nil && cookieStr != "" {
			return cookieStr, nil
		}
		// JSON parse başarısız → raw text olarak dene
	}

	// Fallback: raw cookie string (kontrol karakterlerini temizle)
	return sanitizeCookieString(content), nil
}

// parseCookieJSON JSON cookie dosyasını parse eder
func parseCookieJSON(jsonStr string) (string, error) {
	// Format 1: {"cookies": [...]}
	var wrapper CookieWrapper
	if err := json.Unmarshal([]byte(jsonStr), &wrapper); err == nil && len(wrapper.Cookies) > 0 {
		return buildCookieHeader(wrapper.Cookies), nil
	}

	// Format 2: [...]
	var cookies []CookieJSON
	if err := json.Unmarshal([]byte(jsonStr), &cookies); err == nil && len(cookies) > 0 {
		return buildCookieHeader(cookies), nil
	}

	return "", fmt.Errorf("geçersiz cookie JSON")
}

// buildCookieHeader cookie listesinden "name=value; name=value" header'ı oluşturur
// Sadece google.com domain'ine ait cookie'leri dahil eder
func buildCookieHeader(cookies []CookieJSON) string {
	var parts []string
	for _, c := range cookies {
		if shouldIncludeCookie(c.Domain) && c.Name != "" && c.Value != "" {
			parts = append(parts, c.Name+"="+c.Value)
		}
	}
	return strings.Join(parts, "; ")
}

// shouldIncludeCookie bu domain'in google.com için geçerli olup olmadığını kontrol eder
func shouldIncludeCookie(domain string) bool {
	d := strings.ToLower(strings.TrimSpace(domain))
	d = strings.TrimPrefix(d, ".")
	return d == "google.com" || strings.HasSuffix(d, ".google.com") ||
		d == "google.com.tr" || strings.HasSuffix(d, ".google.com.tr")
}

// sanitizeCookieString raw cookie string'den kontrol karakterlerini temizler
func sanitizeCookieString(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '\t' || (r >= 0x20 && r != 0x7F) {
			return r
		}
		return -1
	}, s)
}

// ==================== PROXY ====================

// parseProxy "host:port:username:password" → http://user:pass@host:port
func parseProxy(s string) string {
	parts := strings.SplitN(s, ":", 4)
	if len(parts) == 4 {
		proxyURL := &url.URL{
			Scheme: "http",
			User:   url.UserPassword(parts[2], parts[3]),
			Host:   parts[0] + ":" + parts[1],
		}
		return proxyURL.String()
	}
	if strings.HasPrefix(s, "http") {
		return s
	}
	return "http://" + s
}

// ==================== HTTP İSTEMCİ ====================

// initClients proxy kullanarak iki HTTP istemci oluşturur
func initClients(proxyURL string) {
	proxyParsed, err := url.Parse(proxyURL)
	if err != nil {
		log.Fatalf("[HATA] Proxy URL geçersiz: %v", err)
	}

	transport := &http.Transport{
		Proxy: http.ProxyURL(proxyParsed),
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
		MaxIdleConns:          500,
		MaxIdleConnsPerHost:   200,
		MaxConnsPerHost:       0,
		IdleConnTimeout:       120 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 15 * time.Second,
		ForceAttemptHTTP2:     false,
		DisableKeepAlives:     false,
	}

	clientFollow = &http.Client{
		Transport: transport,
		Timeout:   30 * time.Second,
	}

	clientNoFollow = &http.Client{
		Transport: transport,
		Timeout:   15 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

// ==================== PROXY TEST ====================

func testProxy() error {
	req, err := http.NewRequest("HEAD", "https://www.google.com", nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", generateUA().UserAgent)

	testClient := &http.Client{
		Transport: clientFollow.Transport,
		Timeout:   15 * time.Second,
	}
	resp, err := testClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return nil
}

// ==================== WORKER ====================

// worker sonsuz döngüde çalışan işçi goroutine
func worker(id int, keyword, targetSite string, cookieFiles []string) {
	for {
		// 1. Rastgele cookie dosyası seç ve oku (lazy loading)
		cookieFile := cookieFiles[randInt(len(cookieFiles))]
		cookie, err := readCookie(cookieFile)
		if err != nil || cookie == "" {
			atomic.AddInt64(&statSkipped, 1)
			continue
		}

		// 2. User-Agent üret
		uaInfo := generateUA()

		// 3. Google'da ara ve hedef siteyi bul
		result, searchURL := searchAndFind(keyword, targetSite, cookie, uaInfo)
		atomic.AddInt64(&statSearches, 1)

		if result == nil {
			atomic.AddInt64(&statSkipped, 1)
			continue
		}

		atomic.AddInt64(&statFound, 1)
		log.Printf("[Thread-%03d] Site bulundu! Sıra: %d | HitPath: %.80s", id, result.Rank, result.HitPath)

		// 4. GET click + POST PING hit gönder
		success := sendPingHit(result, searchURL, cookie, uaInfo)
		if success {
			atomic.AddInt64(&statHits, 1)
			log.Printf("%s[Thread-%03d] ✓ TIKLANDI → %s | Sıra: %d%s", colorGreen, id, targetSite, result.Rank, colorReset)
		} else {
			atomic.AddInt64(&statErrors, 1)
			log.Printf("[Thread-%03d] ✗ Hit başarısız → %s", id, targetSite)
		}

		sleepRandom(300, 800)
	}
}

// ==================== İSTATİSTİK ====================

func printStats() {
	for {
		time.Sleep(5 * time.Second)
		searches := atomic.LoadInt64(&statSearches)
		found := atomic.LoadInt64(&statFound)
		hits := atomic.LoadInt64(&statHits)
		skipped := atomic.LoadInt64(&statSkipped)
		errors := atomic.LoadInt64(&statErrors)
		rate := float64(0)
		if searches > 0 {
			rate = float64(found) / float64(searches) * 100
		}
		fmt.Printf("\n%s[STATS] Arama: %d | Bulundu: %d | Hit: %d | Atlanan: %d | Hata: %d | Oran: %.1f%%%s\n\n",
			colorCyan, searches, found, hits, skipped, errors, rate, colorReset,
		)
	}
}

// ==================== YARDIMCI ====================

// randInt kriptografik olarak güvenli rastgele sayı üretir [0, max)
func randInt(max int) int {
	if max <= 0 {
		return 0
	}
	n, err := rand.Int(rand.Reader, big.NewInt(int64(max)))
	if err != nil {
		return int(time.Now().UnixNano() % int64(max))
	}
	return int(n.Int64())
}

// sleepRandom minMs-maxMs arasında rastgele süre bekler
func sleepRandom(minMs, maxMs int) {
	ms := randInt(maxMs-minMs) + minMs
	time.Sleep(time.Duration(ms) * time.Millisecond)
}
