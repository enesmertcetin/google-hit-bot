package main

import (
	"fmt"
	"io"
	"net/http"
	"strings"
)

// ==================== HIT: GET CLICK + POST PING ====================
// Gerçek tarayıcı tıklaması 2 aşamadan oluşur:
// 1. GET /url?q=...&ved=...&usg=... → Google tıklamayı BURADA kaydeder (302 redirect)
// 2. POST PING → ek beacon sinyali
// İkisi birlikte gönderilmeli!

// sendHit önce GET click, sonra POST PING gönderir
func sendPingHit(result *SearchResult, searchURL, cookie string, uaInfo UAInfo) bool {
	hitURL := "https://www.google.com" + result.HitPath

	// Adım 1: GET click — Google tıklamayı burada sayar
	clickOK := doGetClick(hitURL, searchURL, cookie, uaInfo)

	// Adım 2: POST PING — ek sinyal
	pingOK := false
	for attempt := 0; attempt < MaxHitRetries; attempt++ {
		err := doPostPing(hitURL, searchURL, result.TargetURL, cookie, uaInfo)
		if err == nil {
			pingOK = true
			break
		}
		sleepRandom(200, 500)
	}

	return clickOK || pingOK
}

// doGetClick Google'ın redirect URL'sine GET isteği gönderir
// Google bu isteği aldığında tıklamayı kaydeder ve hedef siteye 302 redirect verir
// Biz redirect'i takip edip hedef siteyi de açıyoruz (dwell time sinyali)
func doGetClick(hitURL, searchURL, cookie string, uaInfo UAInfo) bool {
	req, err := http.NewRequest("GET", hitURL, nil)
	if err != nil {
		return false
	}

	// Gerçek tarayıcı tıklama header'ları
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "tr-TR,en;q=0.9")
	req.Header.Set("Cookie", cookie)
	req.Header.Set("Referer", searchURL)
	if uaInfo.SecChUa != "" {
		req.Header.Set("Sec-Ch-Ua", uaInfo.SecChUa)
		req.Header.Set("Sec-Ch-Prefers-Color-Scheme", "dark")
		req.Header.Set("Sec-Ch-Ua-Form-Factors", `"Mobile"`)
		req.Header.Set("Sec-Ch-Ua-Mobile", "?1")
		req.Header.Set("Sec-Ch-Ua-Platform", fmt.Sprintf(`"%s"`, uaInfo.Platform))
		req.Header.Set("Sec-Fetch-Dest", "document")
		req.Header.Set("Sec-Fetch-Mode", "navigate")
		req.Header.Set("Sec-Fetch-Site", "same-origin")
		req.Header.Set("Sec-Fetch-User", "?1")
	}
	req.Header.Set("Upgrade-Insecure-Requests", "1")
	req.Header.Set("User-Agent", uaInfo.UserAgent)

	// Redirect'i takip et (Google → hedef site)
	resp, err := clientFollow.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	// Hedef site body'sinin bir kısmını oku (gerçek ziyaret simülasyonu)
	buf := make([]byte, 32*1024) // 32KB oku
	io.ReadAtLeast(resp.Body, buf, 1)

	// Dwell time: gerçek kullanıcı gibi sayfada bir süre kal
	sleepRandom(3000, 8000)

	return resp.StatusCode < 400
}

// doPostPing POST PING beacon isteği gönderir
func doPostPing(hitURL, searchURL, targetURL, cookie string, uaInfo UAInfo) error {
	body := strings.NewReader("PING")
	req, err := http.NewRequest("POST", hitURL, body)
	if err != nil {
		return err
	}

	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "tr-TR,en;q=0.9")
	req.Header.Set("Cache-Control", "max-age=0")
	req.Header.Set("Content-Type", "text/ping")
	req.Header.Set("Cookie", cookie)
	req.Header.Set("Origin", "https://www.google.com")
	req.Header.Set("Referer", "https://www.google.com/")
	req.Header.Set("Ping-From", searchURL)
	req.Header.Set("Ping-To", targetURL)
	req.Header.Set("Priority", "u=0, i")
	if uaInfo.SecChUa != "" {
		req.Header.Set("Sec-Ch-Ua", uaInfo.SecChUa)
		req.Header.Set("Sec-Ch-Prefers-Color-Scheme", "dark")
		req.Header.Set("Sec-Ch-Ua-Form-Factors", `"Mobile"`)
		req.Header.Set("Sec-Ch-Ua-Mobile", "?1")
		req.Header.Set("Sec-Ch-Ua-Platform", fmt.Sprintf(`"%s"`, uaInfo.Platform))
		req.Header.Set("Sec-Fetch-Dest", "empty")
		req.Header.Set("Sec-Fetch-Mode", "no-cors")
		req.Header.Set("Sec-Fetch-Site", "same-origin")
	}
	req.Header.Set("User-Agent", uaInfo.UserAgent)

	resp, err := clientNoFollow.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	return nil
}
