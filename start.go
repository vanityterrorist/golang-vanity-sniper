package main

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	token      = ""
	pass       = ""  
	serverID   = "1317607350353399950"
	gatewayURL = "wss://gateway-us-east1-b.discord.gg"
	webhookURL = ""
)

var (
	guilds      = make(map[string]string)
	mfaToken    string
	mu          sync.RWMutex
	tlsConfig   = &tls.Config{
		MinVersion:         tls.VersionTLS12,
		MaxVersion:         tls.VersionTLS12,
		InsecureSkipVerify: true,
	}
)

type discordPayload struct {
	Op int         `json:"op"`
	D  interface{} `json:"d"`
	T  string      `json:"t"`
}

type identifyData struct {
	Token      string                 `json:"token"`
	Intents    int                   `json:"intents"`
	Properties map[string]interface{} `json:"properties"`
}

type mfaResponse struct {
	Token string `json:"token"`
}

type vanityResponse struct {
	MFA struct {
		Ticket string `json:"ticket"`
	} `json:"mfa"`
}

func main() {
	log.Println("MFA token alınıyor...")
	if newToken := handleMFA(); newToken != "" {
		mfaToken = newToken
		log.Printf("MFA Token alındı: %s", mfaToken)
	}

	
	go func() {
		ticker := time.NewTicker(15 * time.Minute)
		for range ticker.C {
			log.Println("MFA token yenileniyor...")
			if newToken := handleMFA(); newToken != "" {
				mfaToken = newToken
				log.Printf("MFA Token yenilendi: %s", mfaToken)
			}
		}
	}()

	go startHTTPServer()
	
	go connectToDiscord()

	select {}
}

func handleMFA() string {
	req, _ := http.NewRequest("PATCH", "https://discord.com/api/v9/guilds/0/vanity-url", nil)
	setHeaders(req)

	client := &http.Client{Transport: &http.Transport{TLSClientConfig: tlsConfig}}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("Ticket alma hatası: %v", err)
		return ""
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		return ""
	}

	var vanityResp vanityResponse
	if err := json.NewDecoder(resp.Body).Decode(&vanityResp); err != nil {
		log.Printf("Ticket parse hatası: %v", err)
		return ""
	}

	mfaPayload := map[string]string{
		"ticket":   vanityResp.MFA.Ticket,
		"mfa_type": "password",
		"data":     pass,
	}

	jsonData, _ := json.Marshal(mfaPayload)
	req, _ = http.NewRequest("POST", "https://discord.com/api/v9/mfa/finish", bytes.NewBuffer(jsonData))
	setHeaders(req)

	resp, err = client.Do(req)
	if err != nil {
		log.Printf("MFA isteği hatası: %v", err)
		return ""
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("MFA hatası: %d", resp.StatusCode)
		return ""
	}

	var mfaResp mfaResponse
	if err := json.NewDecoder(resp.Body).Decode(&mfaResp); err != nil {
		log.Printf("MFA token parse hatası: %v", err)
		return ""
	}

	return mfaResp.Token
}

func setHeaders(req *http.Request) {
	req.Header.Set("Authorization", token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.6367.243 Safari/537.36")
	req.Header.Set("X-Super-Properties", "eyJvcyI6IkFuZHJvaWQiLCJicm93c2VyIjoiQW5kcm9pZCBDaHJvbWUiLCJkZXZpY2UiOiJBbmRyb2lkIiwic3lzdGVtX2xvY2FsZSI6InRyLVRSIiwiYnJvd3Nlcl91c2VyX2FnZW50IjoiTW96aWxsYS81LjAgKExpbnV4OyBBbmRyb2lkIDYuMDsgTmV4dXMgNSBCdWlsZC9NUkE1OE4pIEFwcGxlV2ViS2l0LzUzNy4zNiAoS0hUTUwsIGxpa2UgR2Vja28pIENocm9tZS8xMzEuMC4wLjAgTW9iaWxlIFNhZmFyaS81MzcuMzYiLCJicm93c2VyX3ZlcnNpb24iOiIxMzEuMC4wLjAiLCJvc192ZXJzaW9uIjoiNi4wIiwicmVmZXJyZXIiOiJodHRwczovL2Rpc2NvcmQuY29tL2NoYW5uZWxzL0BtZS8xMzAzMDQ1MDIyNjQzNTIzNjU1IiwicmVmZXJyaW5nX2RvbWFpbiI6ImRpc2NvcmQuY29tIiwicmVmZXJyaW5nX2N1cnJlbnQiOiIiLCJyZWxlYXNlX2NoYW5uZWwiOiJzdGFibGUiLCJjbGllbnRfYnVpbGRfbnVtYmVyIjozNTU2MjQsImNsaWVudF9ldmVudF9zb3VyY2UiOm51bGwsImhhc19jbGllbnRfbW9kcyI6ZmFsc2V9")
	req.Header.Set("X-Discord-Timezone", "Europe/Istanbul")
	req.Header.Set("X-Discord-Locale", "tr")
	if mfaToken != "" {
		req.Header.Set("X-Discord-MFA-Authorization", mfaToken)
	}
}

func connectToDiscord() {
	for {
		dialer := websocket.Dialer{
			TLSClientConfig: tlsConfig,
			Proxy:          http.ProxyFromEnvironment,
		}

		conn, _, err := dialer.Dial(gatewayURL, nil)
		if err != nil {
			log.Printf("WebSocket bağlantı hatası: %v", err)
			time.Sleep(time.Second * 2)
			continue
		}

		identify := discordPayload{
			Op: 2,
			D: identifyData{
				Token:   token,
				Intents: 1,
				Properties: map[string]interface{}{
					"os":      "linux",
					"browser": "firefox",
					"device":  "hairo",
				},
			},
		}

		if err := conn.WriteJSON(identify); err != nil {
			log.Printf("Identify gönderme hatası: %v", err)
			conn.Close()
			continue
		}

		go func() {
			ticker := time.NewTicker(41250 * time.Millisecond)
			defer ticker.Stop()

			for {
				select {
				case <-ticker.C:
					if err := conn.WriteJSON(map[string]interface{}{"op": 1, "d": nil}); err != nil {
						return
					}
				}
			}
		}()

		for {
			var message discordPayload
			if err := conn.ReadJSON(&message); err != nil {
				log.Printf("Mesaj okuma hatası: %v", err)
				break
			}

			switch message.T {
			case "GUILD_UPDATE":
				handleGuildUpdate(message.D)
			case "READY":
				handleReady(message.D)
			}

			if message.Op == 7 {
				break
			}
		}

		conn.Close()
		time.Sleep(time.Second * 2) 
	}
}

func handleGuildUpdate(data interface{}) {
	if guildData, ok := data.(map[string]interface{}); ok {
		guildID := guildData["guild_id"].(string)
		
		mu.RLock()
		oldVanity := guilds[guildID]
		mu.RUnlock()

		
		newVanityRaw, exists := guildData["vanity_url_code"]
		if !exists || newVanityRaw == nil { 
			if oldVanity != "" {
				
				time.Sleep(1 * time.Second)
				go performPatchRequest(oldVanity)
				log.Printf("Vanity URL silindi, eski URL'yi almaya çalışıyorum: %s", oldVanity)
			}
			return
		}

		
		newVanity, ok := newVanityRaw.(string)
		if !ok {
			return
		}

		if oldVanity != "" && newVanity != oldVanity {
			
			time.Sleep(1 * time.Second)
			go performPatchRequest(oldVanity)
			log.Printf("Vanity URL değişti: %s -> %s, eski URL'yi almaya çalışıyorum", oldVanity, newVanity)
		}

		
		mu.Lock()
		guilds[guildID] = newVanity
		mu.Unlock()
	}
}

func handleReady(data interface{}) {
	if readyData, ok := data.(map[string]interface{}); ok {
		if guildsData, ok := readyData["guilds"].([]interface{}); ok {
			mu.Lock()
			for _, guild := range guildsData {
				if g, ok := guild.(map[string]interface{}); ok {
					if id, ok := g["id"].(string); ok {
						if vanity, ok := g["vanity_url_code"].(string); ok && vanity != "" {
							guilds[id] = vanity
							log.Printf("GUILD => %s || VANITY => %s", id, vanity)
						}
					}
				}
			}
			mu.Unlock()
		}
	}
}

func performPatchRequest(vanityCode string) {
	
	time.Sleep(2 * time.Second)
	
	startTime := time.Now()
	url := fmt.Sprintf("https://canary.discord.com/api/v9/guilds/%s/vanity-url", serverID)
	payload := map[string]string{"code": vanityCode}
	jsonData, _ := json.Marshal(payload)

	req, _ := http.NewRequest("PATCH", url, bytes.NewBuffer(jsonData))
	setHeaders(req)

	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: tlsConfig,
		},
		Timeout: 10 * time.Second, 
	}

	resp, err := client.Do(req)
	if err != nil {
		log.Printf("Vanity URL değiştirme hatası: %v", err)
		return
	}
	defer resp.Body.Close()

	elapsedMs := time.Since(startTime).Milliseconds()

	if resp.StatusCode == 429 {
		log.Println("Rate limit yendi, 5 saniye bekleniyor...")
		time.Sleep(5 * time.Second)
		return
	}

	go notifyWebhook(vanityCode, resp, elapsedMs)
}

func notifyWebhook(vanityCode string, resp *http.Response, elapsedMs int64) {
	body, _ := io.ReadAll(resp.Body)
	
	webhook := map[string]interface{}{
		"content": "@everyone",
		"embeds": []map[string]interface{}{
			{
				"description": fmt.Sprintf("```%s```", string(body)),
				"color":      0x000000,
				"image": map[string]string{
					"url": "https://cdn.discordapp.com/attachments/1321903890328719404/1358138850756792592/dd8aaf02bdb1fc5919242f97bd548860.gif?ex=67f4bb1f&is=67f3699f&hm=3699a126f3029a645d348b386cbd797c7407bc8b619745512f8b15f06e589481&",
				},
				"fields": []map[string]interface{}{
					{"name": "Vanity", "value": fmt.Sprintf("`%s`", vanityCode), "inline": true},
					{"name": "Guild", "value": fmt.Sprintf("`%s`", serverID), "inline": true},
					{"name": "Gateway", "value": fmt.Sprintf("`%s`", gatewayURL), "inline": true},
					{"name": "Ping", "value": fmt.Sprintf("`%dms`", elapsedMs), "inline": true},
				},
				"author": map[string]interface{}{
					"name": "Hairo Split Checker",
					"icon_url": "https://cdn.discordapp.com/emojis/1000750307373596702.webp?size=96&quality=lossless",
				},
				"footer": map[string]interface{}{
					"text":     fmt.Sprintf("vanityterrorist | %s", time.Now().Format("15:04:05")),
					"icon_url": "https://cdn.discordapp.com/attachments/1279593311350296748/1345523497119518730/togif-9.gif?ex=67f4faa8&is=67f3a928&hm=3010079938a7c6e9b09a962965a66e8c999e1ad93f7084913c6bf2952f016dc6&",
				},
				"timestamp": time.Now().Format(time.RFC3339),
			},
		},
	}

	jsonData, _ := json.Marshal(webhook)
	http.Post(webhookURL, "application/json", bytes.NewBuffer(jsonData))
}

func startHTTPServer() {
	http.HandleFunc("/hairo", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var data struct {
			MFAToken string `json:"mfaToken"`
		}

		if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}

		if data.MFAToken == "" {
			http.Error(w, "Missing mfaToken", http.StatusBadRequest)
			return
		}

		mfaToken = data.MFAToken
		log.Printf("[%s] > MFA Token Alindi: %s", time.Now().Format("15:04:05"), data.MFAToken)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"message": "MFA token received and set."})
	})

	log.Fatal(http.ListenAndServe(":6931", nil))
}
