package main

import (
	"fmt"
	"html"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
)

// ==================== ARAMA SONUCU ====================

type SearchResult struct {
	HitPath   string // /url?q=https://...&sa=U&ved=...&usg=... (clean, &amp; → &)
	TargetURL string // https://www.gmailchecklive.com/ (q= parametresinden)
	Rank      int    // Genel sıralama
}

// Debug: ilk büyük HTML yanıtını kaydet
var debugOnce sync.Once

// ==================== ARA VE BUL ====================

// searchAndFind Google'da keyword'ü arar, hedef siteyi MaxPages sayfaya kadar tarar
// Döner: (sonuç, arama URL'si) — Ping-From header'ı için
func searchAndFind(keyword, targetSite, cookie string, uaInfo UAInfo) (*SearchResult, string) {
	encodedKeyword := strings.ReplaceAll(url.QueryEscape(keyword), "+", "%20")
	cleanTarget := normalizeURL(targetSite)

	offset := 0

	for page := 0; page < MaxPages; page++ {
		start := page * 10
		searchURL := buildSearchURL(encodedKeyword, uaInfo.Platform, start)

		htmlStr, err := doSearchRequest(searchURL, cookie, uaInfo)
		if err != nil {
			return nil, ""
		}

		// Debug: ilk büyük yanıtı dosyaya kaydet
		if len(htmlStr) > 5000 {
			debugOnce.Do(func() {
				os.WriteFile("debug_response.html", []byte(htmlStr), 0644)
				log.Printf("[DEBUG] Arama yanıtı debug_response.html dosyasına kaydedildi (%d byte)", len(htmlStr))
			})
		}

		// JS-only sayfa kontrolü (cookie geçersiz/zayıf)
		if strings.Contains(htmlStr, "/httpservice/retry/enablejs") {
			log.Printf("[ARAMA] Sayfa %d: JS-only yanıt (cookie zayıf)", page+1)
			return nil, ""
		}

		// Captcha / block kontrolü
		if strings.Contains(htmlStr, "unusual traffic") || strings.Contains(htmlStr, "/sorry/index") {
			log.Printf("[ARAMA] Sayfa %d: CAPTCHA/block algılandı!", page+1)
			return nil, ""
		}

		// Google consent sayfası kontrolü
		if strings.Contains(htmlStr, "consent.google") {
			log.Printf("[ARAMA] Sayfa %d: Consent sayfası", page+1)
			return nil, ""
		}

		// Regex ile sonuçları parse et
		results := parseResults(htmlStr)

		// Hedef siteyi ara
		for i, r := range results {
			cleanURL := normalizeURL(r.TargetURL)
			if strings.Contains(cleanURL, cleanTarget) {
				r.Rank = offset + i + 1
				return &r, searchURL
			}
		}

		offset += len(results)

		// Sonuç yoksa daha fazla sayfa tarama
		if len(results) == 0 {
			return nil, ""
		}

		// Sayfalar arası kısa bekleme
		sleepRandom(500, 1500)
	}

	return nil, ""
}

// ==================== URL OLUŞTURUCU ====================

// buildSearchURL platform'a göre Google arama URL'si oluşturur
func buildSearchURL(encodedKeyword, platform string, start int) string {
	sourceid := "safari"
	if platform == "Android" {
		sourceid = "chrome"
	}
	return fmt.Sprintf(
		"https://www.google.com/search?q=%s&oq=%s&sourceid=%s&ie=UTF-8&start=%d&hl=tr&gl=tr",
		encodedKeyword, encodedKeyword, sourceid, start,
	)
}

// ==================== ARAMA İSTEĞİ ====================

// doSearchRequest Google'a GET arama isteği gönderir (BAS header'ları ile birebir aynı)
func doSearchRequest(searchURL, cookie string, uaInfo UAInfo) (string, error) {
	req, err := http.NewRequest("GET", searchURL, nil)
	if err != nil {
		return "", err
	}

	// BAS header'ları — birebir aynısı
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "tr-TR,en;q=0.9")
	req.Header.Set("Cache-Control", "max-age=0")
	req.Header.Set("Cookie", cookie)
	req.Header.Set("Referer", "https://www.google.com/")
	req.Header.Set("Priority", "u=0, i")
	if uaInfo.SecChUa != "" {
		req.Header.Set("Sec-Ch-Ua", uaInfo.SecChUa)
		req.Header.Set("Sec-Ch-Prefers-Color-Scheme", "dark")
		req.Header.Set("Sec-Ch-Ua-Form-Factors", `"Mobile"`)
		req.Header.Set("Sec-Ch-Ua-Mobile", "?1")
		req.Header.Set("Sec-Ch-Ua-Platform", fmt.Sprintf(`"%s"`, uaInfo.Platform))
		req.Header.Set("Sec-Fetch-Dest", "document")
		req.Header.Set("Sec-Fetch-Mode", "navigate")
		req.Header.Set("Sec-Fetch-Site", "same-origin")
	}
	req.Header.Set("User-Agent", uaInfo.UserAgent)

	resp, err := clientFollow.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return string(body), nil
}

// ==================== REGEX PARSE ====================

// parseResults HTML'den arama sonuç linklerini çıkarır
// Birden fazla pattern destekler: /url?q=, data-href, data-url, href="https://..."
func parseResults(htmlStr string) []SearchResult {
	type rawMatch struct {
		hitPath   string // /url?q=... tam path (varsa)
		targetURL string // hedef URL
	}

	var rawMatches []rawMatch

	// 1. Ana pattern: /url?q=... (Google redirect URL — TAM path, &sa=U&ved=...&usg=... dahil)
	re1 := regexp.MustCompile(`/url\?q=https?://[^"]+`)
	for _, match := range re1.FindAllString(htmlStr, -1) {
		cleanPath := html.UnescapeString(match)
		parsed, err := url.Parse(cleanPath)
		if err != nil {
			continue
		}
		targetURL := parsed.Query().Get("q")
		if targetURL != "" && strings.HasPrefix(targetURL, "http") {
			rawMatches = append(rawMatches, rawMatch{hitPath: cleanPath, targetURL: targetURL})
		}
	}

	// 2. Fallback: data-href="https://..." (AI Overview ve yeni SERP formatları)
	re2 := regexp.MustCompile(`data-href="(https?://[^"]+)"`)
	for _, sm := range re2.FindAllStringSubmatch(htmlStr, -1) {
		if len(sm) > 1 {
			u := html.UnescapeString(sm[1])
			rawMatches = append(rawMatches, rawMatch{hitPath: "", targetURL: u})
		}
	}

	// 3. Fallback: data-url="https://..." (bazı yeni SERP blokları)
	re3 := regexp.MustCompile(`data-url="(https?://[^"]+)"`)
	for _, sm := range re3.FindAllStringSubmatch(htmlStr, -1) {
		if len(sm) > 1 {
			u := html.UnescapeString(sm[1])
			rawMatches = append(rawMatches, rawMatch{hitPath: "", targetURL: u})
		}
	}

	// 4. Fallback: <a class="..." href="https://..." (doğrudan linkler, organik sonuçlar)
	re4 := regexp.MustCompile(`<a[^>]+class="[^"]*"[^>]+href="(https?://[^"]+)"`)
	for _, sm := range re4.FindAllStringSubmatch(htmlStr, -1) {
		if len(sm) > 1 {
			u := html.UnescapeString(sm[1])
			rawMatches = append(rawMatches, rawMatch{hitPath: "", targetURL: u})
		}
	}

	var results []SearchResult
	seen := make(map[string]bool)

	for _, rm := range rawMatches {
		targetURL := rm.targetURL

		// Google/YouTube/cache linklerini filtrele
		lower := strings.ToLower(targetURL)
		if strings.Contains(lower, "google.") ||
			strings.Contains(lower, "youtube.com") ||
			strings.Contains(lower, "webcache.googleusercontent") ||
			strings.Contains(lower, "gstatic.com") ||
			strings.Contains(lower, "googleapis.com") ||
			strings.Contains(lower, "translate.google") {
			continue
		}

		// Tekrarları atla
		if seen[targetURL] {
			continue
		}
		seen[targetURL] = true

		hitPath := rm.hitPath
		if hitPath == "" {
			// /url?q= formatında değilse, doğrudan link kullan
			// Bu durumda Google redirect URL'si olmayacak, ama hedef siteyi bulduk
			hitPath = "/url?q=" + url.QueryEscape(targetURL)
		}

		results = append(results, SearchResult{
			HitPath:   hitPath,
			TargetURL: targetURL,
		})
	}

	return results
}

// ==================== YARDIMCI ====================

// normalizeURL URL'den http(s):// ve www. prefix'ini kaldırır, küçük harfe çevirir
func normalizeURL(u string) string {
	u = strings.ToLower(u)
	u = strings.TrimPrefix(u, "https://")
	u = strings.TrimPrefix(u, "http://")
	u = strings.TrimPrefix(u, "www.")
	u = strings.TrimSuffix(u, "/")
	return u
}
