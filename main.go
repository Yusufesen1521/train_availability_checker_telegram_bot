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