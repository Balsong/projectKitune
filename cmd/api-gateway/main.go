package main

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"

	"tea-platform/internal/config"
	"tea-platform/internal/db"
	"tea-platform/internal/kafka"
	"tea-platform/pkg/models"
)

var (
	menu = []models.MenuItem{
		{ID: 1, Name: "Да Хун Пао", Type: "tea", Price: 850},
		{ID: 2, Name: "Дим-самы с креветками", Type: "dim_sum", Price: 420},
	}
	mu sync.RWMutex
)

func main() {
	cfg := config.Load()

	// Инициализация инфраструктуры
	database, err := db.NewDB(nil, cfg.DBHost, cfg.DBUser, cfg.DBPassword, cfg.DBName)
	if err != nil {
		log.Fatalf("❌ Ошибка инициализации БД: %v", err)
	}

	kafkaProd, err := kafka.NewProducer(cfg.KafkaBrokers)
	if err != nil {
		log.Fatalf("❌ Ошибка инициализации Kafka: %v", err)
	}

	_ = database // пока используем для демонстрации
	_ = kafkaProd

	// Маршруты
	http.HandleFunc("GET /api/v1/menu", handleGetMenu)
	http.HandleFunc("POST /api/v1/book", handleBookTable)

	log.Printf("🍵 API Gateway запущен на http://localhost:%s", cfg.Port)
	log.Fatal(http.ListenAndServe(":"+cfg.Port, nil))
}

func handleGetMenu(w http.ResponseWriter, r *http.Request) {
	mu.RLock()
	defer mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(menu)
}

func handleBookTable(w http.ResponseWriter, r *http.Request) {
	var req models.BookingRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}

	if req.Date == "" || req.Time == "" || req.Guests <= 0 {
		http.Error(w, `{"error":"date, time and guests required"}`, http.StatusBadRequest)
		return
	}

	log.Printf("📅 Бронирование: %s %s | %d гостей", req.Date, req.Time, req.Guests)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{
		"status": "confirmed",
		"msg":    "Запрос принят. Ожидайте подтверждения.",
	})
}
