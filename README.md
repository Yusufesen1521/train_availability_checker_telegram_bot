# 🚄 Train Availability Watcher - Telegram Bot

Bu proje, belirli tren seferleri için doluluk oranlarını kontrol eden, boş yer bulunduğunda Telegram üzerinden bildirim gönderen ve Docker üzerinde çalışan, Go (Golang) ile yazılmış yüksek performanslı bir bottur.

![Go](https://img.shields.io/badge/Go-1.22+-00ADD8?style=for-the-badge&logo=go)
![Docker](https://img.shields.io/badge/Docker-Enabled-2496ED?style=for-the-badge&logo=docker)
![License](https://img.shields.io/badge/License-MIT-green?style=for-the-badge)

## 🌟 Özellikler

- **Otomatik Takip:** Dolu seferleri arka planda (belirlenen aralıklarla) kontrol eder.
- **Akıllı Bildirim:** Sadece boş yer ("EKONOMİ" vb.) bulunduğunda mesaj atar.
- **Anti-Ban Mekanizması:** Jitter (Rastgele Gecikme) ve Exponential Backoff (Hata durumunda artan bekleme süresi) ile "insani" davranış sergiler.
- **Kalıcılık (Persistence):** Bot veya sunucu kapansa bile aktif aramalar `jobs.json` üzerinden geri yüklenir.
- **Çoklu Kullanıcı Desteği:** Sadece izin verilen kullanıcılar (`/adduser`) botu kullanabilir.
- **Docker Desteği:** Tek komutla kurulum ve çalıştırma.

---

## 🛠️ Kurulum Rehberi

Bu botu kendi sunucunuzda çalıştırmak için aşağıdaki adımları takip edin.

### Ön Gereksinimler

- [Docker](https://docs.docker.com/get-docker/) yüklü bir sunucu (veya bilgisayar).
- Bir **Telegram Bot Token**'ı.
- Takip edilecek Servisin API Bilgileri (URL ve Auth Token).

### Adım 1: Telegram Botunu Oluşturma

1.  Telegram'da **[@BotFather](https://t.me/BotFather)**'ı bulun.
2.  `/newbot` komutunu gönderin ve botunuza bir isim verin.
3.  Size verilen **HTTP API Token**'ı bir yere not edin.
4.  Kendi ID'nizi öğrenmek için **[@userinfobot](https://t.me/userinfobot)**'a bir mesaj atın ve `Id` değerini not edin (Bu sizin Admin ID'niz olacak).

### Adım 2: Projeyi İndirme ve Hazırlık

Repoyu klonlayın ve dizine girin:

```bash
git clone [https://github.com/KULLANICI_ADINIZ/REPO_ADINIZ.git](https://github.com/KULLANICI_ADINIZ/REPO_ADINIZ.git)
cd REPO_ADINIZ
```

Örnek konfigürasyon dosyalarının ismini değiştirin:

```bash
cp .env.example .env
cp config.example.yaml config.yaml
```

### Adım 3: Konfigürasyon

#### `.env` Dosyası

Bu dosya hassas API bilgilerini içerir. Dosyayı açın ve doldurun:

```ini
TELEGRAM_TOKEN=123456:ABC-DEF... (BotFather'dan aldığınız token)
API_URL=[https://api.hedef-servis.com/stations](https://api.hedef-servis.com/stations)
SEARCH_URL=[https://api.hedef-servis.com/availability](https://api.hedef-servis.com/availability)
AUTH_KEY=Servis_Auth_Key (Bearer Token)
UNIT_ID=Varsa_Unit_ID
```

#### `config.yaml` Dosyası

Botun davranış ayarlarını buradan yapabilirsiniz:

```yaml
app:
  db_file: "jobs.json" # Görevlerin kaydedileceği dosya
  users_file: "users.json" # İzinli kullanıcıların listesi
  job_timeout_hours: 6 # Bir arama en fazla ne kadar sürsün?
  confirmation_timeout_minutes: 1
  admin_id: 000000000 # <-- BURAYA KENDİ TELEGRAM ID'NİZİ YAZIN

anti_ban:
  base_interval_seconds: 60 # Kontrol aralığı (saniye)
  max_backoff_minutes: 15 # Hata durumunda maksimum bekleme
  jitter_seconds: 30 # Rastgele eklenecek ek süre (0-30 sn)
```

---

## 🐳 Docker ile Çalıştırma (Önerilen)

Botu Docker ile çalıştırmak en temiz ve kararlı yöntemdir.

1.  **İmajı Oluşturun (Build):**

<!-- end list -->

```bash
docker build -t train-bot .
```

2.  **Kalıcılık için Dosyaları Oluşturun:**
    Docker kapansa bile verilerin kaybolmaması için sunucuda boş dosyalar oluşturun:

<!-- end list -->

```bash
touch jobs.json users.json
```

3.  **Konteyneri Başlatın (Run):**

<!-- end list -->

```bash
docker run -d \
  --name train-bot \
  --restart always \
  -v $(pwd)/jobs.json:/root/jobs.json \
  -v $(pwd)/users.json:/root/users.json \
  -v $(pwd)/config.yaml:/root/config.yaml \
  -v $(pwd)/.env:/root/.env \
  train-bot
```

_Artık botunuz arka planda çalışıyor\!_

Durumu kontrol etmek için:

```bash
docker logs -f train-bot
```

---

## 🎮 Kullanım Komutları

Bot sadece `admin_id` sahibi veya `/adduser` ile eklenen kullanıcılar tarafından kullanılabilir.

| Komut      | Açıklama                                 | Örnek                               |
| :--------- | :--------------------------------------- | :---------------------------------- |
| `/find`    | Bilet araması başlatır.                  | `/find Eskisehir Ankara 24.11.2025` |
| `/iptal`   | Aktif aramayı durdurur.                  | `/iptal`                            |
| `/devam`   | Süre dolduğunda uzatmak için kullanılır. | `/devam`                            |
| `/adduser` | (Admin) Yeni kullanıcı ekler.            | `/adduser 98765432`                 |
| `/deluser` | (Admin) Kullanıcı siler.                 | `/deluser 98765432`                 |
| `/users`   | (Admin) İzinli kullanıcıları listeler.   | `/users`                            |

---

## ⚠️ Yasal Uyarı (Disclaimer)

Bu proje tamamen **eğitim ve kişisel kullanım amaçlı** geliştirilmiştir. Herhangi bir kurum, kuruluş veya ticari yapı ile resmi bir bağı yoktur.

- Bu yazılımı kullanırken ilgili servis sağlayıcısının **Kullanım Koşullarına (Terms of Service)** uymak kullanıcının sorumluluğundadır.
- Aşırı istek gönderimi (spamming) veya sistemin kötüye kullanımı yasal yaptırımlara yol açabilir. "Anti-Ban" özellikleri sunucuları yormamak için eklenmiştir, bu limitleri değiştirmemeniz önerilir.
- Geliştirici, bu yazılımın kullanımından doğabilecek herhangi bir yasal veya teknik sorundan sorumlu tutulamaz.

---

## 📄 Lisans

Bu proje [MIT License](https://www.google.com/search?q=LICENSE) altında lisanslanmıştır.
