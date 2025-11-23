package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	tele "gopkg.in/telebot.v3"
	"gopkg.in/yaml.v3"
)

// --- KONFİGÜRASYON YAPISI ---
type Config struct {
	App struct {
		DBFile             string `yaml:"db_file"`
		UsersFile          string `yaml:"users_file"` // Kullanıcı listesi dosyası
		JobTimeoutHours    int    `yaml:"job_timeout_hours"`
		ConfirmTimeoutMins int    `yaml:"confirmation_timeout_minutes"`
		AdminID            int64  `yaml:"admin_id"`
	} `yaml:"app"`
	AntiBan struct {
		BaseIntervalSec int `yaml:"base_interval_seconds"`
		MaxBackoffMin   int `yaml:"max_backoff_minutes"`
		JitterSec       int `yaml:"jitter_seconds"`
	} `yaml:"anti_ban"`
}

var cfg Config

// --- GLOBAL DEĞİŞKENLER ---

// Job Yapısı
type Job struct {
	ChatID      int64     `json:"chat_id"`
	FromID      int       `json:"from_id"`
	FromName    string    `json:"from_name"`
	ToID        int       `json:"to_id"`
	ToName      string    `json:"to_name"`
	Date        string    `json:"date"`
	FilterStart int       `json:"filter_start"`
	FilterEnd   int       `json:"filter_end"`
	StartTime   time.Time `json:"start_time"`

	StopChan     chan struct{} `json:"-"`
	ContinueChan chan struct{} `json:"-"`
}

var activeJobs = make(map[int64]*Job)
var jobsMutex sync.Mutex

// İzinli Kullanıcılar Listesi
var allowedUsers = make(map[int64]bool)
var usersMutex sync.Mutex

// --- AYARLARI YÜKLE ---
func loadConfig() {
	f, err := os.Open("config.yaml")
	if err != nil {
		log.Fatal("❌ config.yaml okunamadı:", err)
	}
	defer f.Close()
	decoder := yaml.NewDecoder(f)
	if err := decoder.Decode(&cfg); err != nil {
		log.Fatal("❌ YAML hatası:", err)
	}
	// Varsayılan dosya adı (Config'de yoksa)
	if cfg.App.UsersFile == "" {
		cfg.App.UsersFile = "users.json"
	}
	fmt.Println("⚙️  Ayarlar yüklendi.")
}

// --- PERSISTENCE: USER MANAGEMENT ---

func saveUsers() {
	usersMutex.Lock()
	defer usersMutex.Unlock()

	// Map'i listeye çevir
	var userList []int64
	for id := range allowedUsers {
		userList = append(userList, id)
	}

	data, err := json.MarshalIndent(userList, "", "  ")
	if err != nil {
		fmt.Println("❌ Kullanıcı kayıt hatası:", err)
		return
	}
	_ = os.WriteFile(cfg.App.UsersFile, data, 0644)
}

func loadUsers() {
	usersMutex.Lock()
	defer usersMutex.Unlock()

	fileData, err := os.ReadFile(cfg.App.UsersFile)
	if err != nil {
		if os.IsNotExist(err) {
			return // Dosya yoksa sorun yok, liste boş başlar
		}
		fmt.Println("❌ Kullanıcı dosyası okuma hatası:", err)
		return
	}

	var userList []int64
	if err := json.Unmarshal(fileData, &userList); err != nil {
		fmt.Println("❌ Kullanıcı JSON hatası:", err)
		return
	}

	for _, id := range userList {
		allowedUsers[id] = true
	}
	fmt.Printf("👥 %d izinli kullanıcı yüklendi.\n", len(allowedUsers))
}

// --- PERSISTENCE: JOB MANAGEMENT ---

func saveJobs() {
	jobsMutex.Lock()
	defer jobsMutex.Unlock()
	var jobList []*Job
	for _, job := range activeJobs {
		jobList = append(jobList, job)
	}
	data, _ := json.MarshalIndent(jobList, "", "  ")
	_ = os.WriteFile(cfg.App.DBFile, data, 0644)
}

func loadAndRecoverJobs(b *tele.Bot) {
	jobsMutex.Lock()
	defer jobsMutex.Unlock()

	fileData, err := os.ReadFile(cfg.App.DBFile)
	if err != nil {
		return
	}

	var jobList []*Job
	if err := json.Unmarshal(fileData, &jobList); err != nil {
		return
	}

	fmt.Printf("🔄 %d kayıtlı görev geri yükleniyor...\n", len(jobList))
	timeoutDuration := time.Duration(cfg.App.JobTimeoutHours) * time.Hour

	for _, job := range jobList {
		if time.Since(job.StartTime) > timeoutDuration {
			continue
		}
		trainDate, err := time.Parse("02-01-2006 15:04:05", job.Date)
		if err == nil {
			if trainDate.Add(24 * time.Hour).Before(time.Now()) {
				continue
			}
		}

		job.StopChan = make(chan struct{})
		job.ContinueChan = make(chan struct{})
		activeJobs[job.ChatID] = job

		filterText := "Tüm Gün"
		if job.FilterStart != -1 && job.FilterEnd != -1 {
			startH := job.FilterStart / 60
			startM := job.FilterStart % 60
			endH := job.FilterEnd / 60
			endM := job.FilterEnd % 60
			filterText = fmt.Sprintf("%02d:%02d - %02d:%02d", startH, startM, endH, endM)
		}
		infoMsg := fmt.Sprintf("🔄 **Sistem Yeniden Başlatıldı.**\nArama devam ediyor:\n📍 %s -> %s\n📅 %s\n🕒 %s",
			job.FromName, job.ToName, job.Date, filterText)

		go b.Send(&tele.Chat{ID: job.ChatID}, infoMsg, tele.ModeMarkdown)
		go startMonitoring(b, job)
	}
}

// --- GÜVENLİK MIDDLEWARE ---
// Hem Admin'e hem de Ekli Kullanıcılara izin verir
func authMiddleware(next tele.HandlerFunc) tele.HandlerFunc {
	return func(c tele.Context) error {
		userID := c.Sender().ID

		// 1. Süper Admin mi?
		if userID == cfg.App.AdminID {
			return next(c)
		}

		// 2. İzinli listede var mı?
		usersMutex.Lock()
		allowed := allowedUsers[userID]
		usersMutex.Unlock()

		if allowed {
			return next(c)
		}

		// İzin yoksa sessizce reddet (Log düş)
		fmt.Printf("⛔ Yetkisiz Erişim: %d (%s)\n", userID, c.Sender().Username)
		return nil
	}
}

// --- SERVİS FONKSİYONLARI ---

func getStations() ([]Station, error) {
	apiUrl := os.Getenv("API_URL")
	authKey := os.Getenv("AUTH_KEY")
	unitId := os.Getenv("UNIT_ID")
	req, _ := http.NewRequest("GET", apiUrl, nil)
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Authorization", "Bearer "+authKey)
	req.Header.Add("unit-id", unitId)
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("Status: %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	var stations []Station
	json.Unmarshal(body, &stations)
	return stations, nil
}

func getTrains(fromId int, fromName string, toId int, toName string, date string) (*TrainResponse, error) {
	searchUrl := os.Getenv("SEARCH_URL")
	authKey := os.Getenv("AUTH_KEY")
	unitId := os.Getenv("UNIT_ID")
	payload := SearchRequest{
		SearchRoutes:      []SearchRoute{{DepartureStationId: fromId, DepartureStationName: fromName, ArrivalStationId: toId, ArrivalStationName: toName, DepartureDate: date}},
		PassengerCounts:   []PassengerTypeCount{{Id: 0, Count: 1}},
		SearchReservation: false,
	}
	jsonData, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", searchUrl, bytes.NewBuffer(jsonData))
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Authorization", "Bearer "+authKey)
	req.Header.Add("unit-id", unitId)
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		if resp.StatusCode == 400 {
			return nil, fmt.Errorf("400")
		}
		return nil, fmt.Errorf("API Error: %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	var trainResp TrainResponse
	if err := json.Unmarshal(body, &trainResp); err != nil {
		return nil, err
	}
	return &trainResp, nil
}

// --- OTOMASYON FONKSİYONU ---

func startMonitoring(b *tele.Bot, job *Job) {
	recipient := &tele.Chat{ID: job.ChatID}
	shouldAbort, isApiError := checkAndNotify(b, recipient, job.FromID, job.FromName, job.ToID, job.ToName, job.Date, job.FilterStart, job.FilterEnd, true)
	if shouldAbort {
		cleanupJob(job.ChatID)
		return
	}
	consecutiveErrors := 0
	if isApiError {
		consecutiveErrors = 1
	}

	baseInterval := time.Duration(cfg.AntiBan.BaseIntervalSec) * time.Second
	maxBackoff := time.Duration(cfg.AntiBan.MaxBackoffMin) * time.Minute
	timeoutDuration := time.Duration(cfg.App.JobTimeoutHours) * time.Hour
	confirmTimeout := time.Duration(cfg.App.ConfirmTimeoutMins) * time.Minute

	for {
		waitDuration := baseInterval
		if consecutiveErrors > 0 {
			multiplier := 1 << consecutiveErrors
			backoffWait := baseInterval * time.Duration(multiplier)
			if backoffWait > maxBackoff {
				backoffWait = maxBackoff
			}
			waitDuration = backoffWait
		}
		jitter := time.Duration(rand.Intn(cfg.AntiBan.JitterSec)) * time.Second
		totalWait := waitDuration + jitter

		select {
		case <-job.StopChan:
			return
		case <-time.After(totalWait):
			if time.Since(job.StartTime) > timeoutDuration {
				msg := fmt.Sprintf("⏳ **Süre Doldu!**\nDevam etmek için %d dakika içinde **/devam** yazın.", cfg.App.ConfirmTimeoutMins)
				b.Send(recipient, msg, tele.ModeMarkdown)
				select {
				case <-job.ContinueChan:
					job.StartTime = time.Now()
					saveJobs()
					b.Send(recipient, "✅ **Süre uzatıldı!**")
					consecutiveErrors = 0
				case <-time.After(confirmTimeout):
					b.Send(recipient, "🛑 **Zaman Aşımı.**")
					cleanupJob(job.ChatID)
					return
				case <-job.StopChan:
					return
				}
			} else {
				abort, apiErr := checkAndNotify(b, recipient, job.FromID, job.FromName, job.ToID, job.ToName, job.Date, job.FilterStart, job.FilterEnd, false)
				if abort {
					cleanupJob(job.ChatID)
					return
				}
				if apiErr {
					consecutiveErrors++
					fmt.Printf("⚠️ API Hatası (%d). Sayı: %d.\n", job.ChatID, consecutiveErrors)
				} else {
					consecutiveErrors = 0
				}
			}
		}
	}
}

func cleanupJob(chatID int64) {
	jobsMutex.Lock()
	if _, exists := activeJobs[chatID]; exists {
		delete(activeJobs, chatID)
	}
	jobsMutex.Unlock()
	saveJobs()
}

func checkAndNotify(b *tele.Bot, recipient tele.Recipient, fromID int, fromName string, toID int, toName string, date string, fStart, fEnd int, isFirstRun bool) (bool, bool) {
	result, err := getTrains(fromID, fromName, toID, toName, date)
	if err != nil {
		if err.Error() == "400" {
			if isFirstRun {
				b.Send(recipient, "⚠️ **Hatalı İstek!** Formatı kontrol edin.")
			}
			return true, false
		}
		if isFirstRun {
			b.Send(recipient, "❌ Sunucu hatası.")
		}
		return false, true
	}

	var buffer bytes.Buffer
	foundAny := false
	buffer.WriteString(fmt.Sprintf("🚨 **BİLET BULUNDU!** 🚨\n📅 %s\n\n", date))

	for _, leg := range result.TrainLegs {
		for _, availability := range leg.TrainAvailabilities {
			for _, train := range availability.Trains {
				trainTimeMinutes := -1
				departureTimeStr := "--:--"
				if len(train.Segments) > 0 {
					ts := train.Segments[0].DepartureTime
					t := time.UnixMilli(ts)
					departureTimeStr = t.Format("15:04")
					trainTimeMinutes = (t.Hour() * 60) + t.Minute()
				}
				if fStart != -1 && fEnd != -1 {
					if trainTimeMinutes < fStart || trainTimeMinutes > fEnd {
						continue
					}
				}
				economySeats := 0
				price := 0.0
				if len(train.AvailableFareInfo) > 0 {
					for _, cabin := range train.AvailableFareInfo[0].CabinClasses {
						if cabin.CabinClass != nil && cabin.CabinClass.Name == "EKONOMİ" {
							economySeats = int(cabin.AvailabilityCount)
							price = cabin.MinPrice
						}
					}
				}
				if price == 0 && train.MinPrice != nil {
					price = train.MinPrice.PriceAmount
				}

				if economySeats > 0 {
					foundAny = true
					seatIcon := "🟢"
					if economySeats < 5 {
						seatIcon = "🔴"
					}
					buffer.WriteString(fmt.Sprintf("🕒 **%s** - %s\n", departureTimeStr, train.Name))
					buffer.WriteString(fmt.Sprintf("%s Yer: **%d** | %.2f TL\n", seatIcon, economySeats, price))
					buffer.WriteString("-------------------------\n")
				}
			}
		}
	}
	if foundAny {
		b.Send(recipient, buffer.String(), tele.ModeMarkdown)
	} else if isFirstRun {
		b.Send(recipient, "❌ Şu an boş yer yok.\n🔄 **Otomatik takip başlatıldı.**\n(6 Saat boyunca aranacak)\nDurdurmak için: /iptal")
	}
	return false, false
}