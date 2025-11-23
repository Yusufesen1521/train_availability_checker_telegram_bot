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