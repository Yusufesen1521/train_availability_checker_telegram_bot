# 1. Aşama: Derleme (Builder)
# Golang'in resmi imajını kullanıyoruz
FROM golang:alpine AS builder

# Çalışma klasörünü ayarla
WORKDIR /app

# Kütüphane dosyalarını kopyala ve indir
COPY go.mod go.sum ./
RUN go mod download

# Kaynak kodları kopyala
COPY . .

# Uygulamayı Linux için derle
RUN go build -o go_bot .

# 2. Aşama: Çalıştırma (Runtime)
# Çok daha küçük bir Linux sürümü (Alpine) kullanıyoruz
FROM alpine:latest

# Timezone (Saat) ayarları için gerekli paketi yükle (Tren saatleri için kritik!)
RUN apk add --no-cache tzdata
ENV TZ=Europe/Istanbul

WORKDIR /root/

# İlk aşamada derlediğimiz uygulamayı buraya al
COPY --from=builder /app/go_bot .

# Konfigürasyon ve .env dosyalarını kopyala
COPY config.yaml .
COPY .env .

# Uygulamayı çalıştır
CMD ["./go_bot"]