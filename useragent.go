package main

import "fmt"

// UAInfo kullanıcı ajanı + platform bilgisi
type UAInfo struct {
	UserAgent string
	Platform  string // "iOS" veya "Android"
	SecChUa   string // Sec-Ch-Ua header (sadece Chrome için, GSA için boş)
}

// generateUA rastgele gerçekçi mobil user-agent üretir (iOS %50 / Android %50)
func generateUA() UAInfo {
	if randInt(100) < 50 {
		return generateIOSGSA()
	}
	return generateAndroidChrome()
}

// ==================== iOS GSA (Google Search App) ====================
// Örnek: Mozilla/5.0 (iPhone; CPU iPhone OS 17_4_1 like Mac OS X)
//        AppleWebKit/605.1.15 (KHTML, like Gecko) GSA/410.1.987654321
//        Mobile/15E148 Safari/604.1

func generateIOSGSA() UAInfo {
	// iOS sürümü: 14.0 - 18.x
	iosMajor := randInt(5) + 14 // 14-18
	iosMinor := randInt(10)     // 0-9
	iosVersion := fmt.Sprintf("%d_%d", iosMajor, iosMinor)
	if randInt(100) < 50 {
		iosPatch := randInt(12) // 0-11
		iosVersion += fmt.Sprintf("_%d", iosPatch)
	}

	// GSA sürümü: 390.0.xxxxxxxxx - 492.2.xxxxxxxxx
	gsaMajor := randInt(103) + 390        // 390-492
	gsaMinor := randInt(3)                // 0-2
	gsaBuild := randInt(600000001) + 800000000 // 800000000-1400000000
	gsaVersion := fmt.Sprintf("%d.%d.%d", gsaMajor, gsaMinor, gsaBuild)

	ua := fmt.Sprintf(
		"Mozilla/5.0 (iPhone; CPU iPhone OS %s like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) GSA/%s Mobile/15E148 Safari/604.1",
		iosVersion, gsaVersion,
	)
	return UAInfo{UserAgent: ua, Platform: "iOS", SecChUa: ""}
}

// ==================== Android Chrome ====================
// Örnek: Mozilla/5.0 (Linux; Android 14; Pixel 8 Pro)
//        AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.6778.135
//        Mobile Safari/537.36

func generateAndroidChrome() UAInfo {
	androidVersions := []string{"11", "12", "13", "14", "15"}

	// Gerçek cihaz modelleri
	devices := []string{
		// Google Pixel
		"Pixel 6", "Pixel 6 Pro", "Pixel 6a",
		"Pixel 7", "Pixel 7 Pro", "Pixel 7a",
		"Pixel 8", "Pixel 8 Pro", "Pixel 8a",
		"Pixel 9", "Pixel 9 Pro",
		// Samsung Galaxy S serisi
		"SM-S911B", "SM-S916B", "SM-S918B", // S23
		"SM-S921B", "SM-S926B", "SM-S928B", // S24
		// Samsung Galaxy A serisi
		"SM-A546B", "SM-A556B", "SM-A346B",
		// Xiaomi
		"2201116SG", "23127PN0CG", "24031PN0DC", "M2101K6G",
		// OnePlus
		"CPH2585", "NE2215", "PHB110",
		// Realme / Oppo
		"RMX3700", "RMX3771", "CPH2591",
	}

	androidVer := androidVersions[randInt(len(androidVersions))]
	device := devices[randInt(len(devices))]

	// Chrome sürümü: 118-133
	chromeMajor := randInt(16) + 118 // 118-133
	chromeBuild := randInt(9000) + 1000 // 1000-9999
	chromePatch := randInt(200)         // 0-199

	ua := fmt.Sprintf(
		"Mozilla/5.0 (Linux; Android %s; %s) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/%d.0.%d.%d Mobile Safari/537.36",
		androidVer, device, chromeMajor, chromeBuild, chromePatch,
	)

	// Sec-Ch-Ua header — Chrome sürümü ile tutarlı olmalı
	secChUa := fmt.Sprintf(`"Chromium";v="%d", "Google Chrome";v="%d", "Not_A Brand";v="24"`, chromeMajor, chromeMajor)

	return UAInfo{UserAgent: ua, Platform: "Android", SecChUa: secChUa}
}
