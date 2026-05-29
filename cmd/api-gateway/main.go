package main

import (
	"encoding/json"
	"fmt"
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

	// Инициализация базы данных
	database, err := db.NewDB(cfg.DBHost, cfg.DBUser, cfg.DBPassword, cfg.DBName)
	if err != nil {
		log.Fatalf("❌ Ошибка инициализации БД: %v", err)
	}
	defer database.Close()

	// Инициализация Kafka producer
	kafkaProd, err := kafka.NewProducer(cfg.KafkaBrokers)
	if err != nil {
		log.Fatalf("❌ Ошибка инициализации Kafka: %v", err)
	}
	defer kafkaProd.Close()

	// Маршруты
	http.HandleFunc("/health", healthHandler)
	http.HandleFunc("/api/v1/menu", handleGetMenu)
	http.HandleFunc("/api/v1/book", handleBookTable(kafkaProd))

	addr := fmt.Sprintf(":%s", cfg.Port)
	log.Printf("🚀 API Gateway запущен на http://localhost%s", cfg.Port)
	log.Fatal(http.ListenAndServe(addr, nil))
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func handleGetMenu(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	mu.RLock()
	defer mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(menu)
}

func handleBookTable(producer *kafka.Producer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var booking struct {
			TableID  int    `json:"table_id"`
			Customer string `json:"customer"`
			Time     string `json:"time"`
		}

		if err := json.NewDecoder(r.Body).Decode(&booking); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		// Отправка в Kafka
		err := producer.SendBooking(booking)
		if err != nil {
			http.Error(w, "Failed to process booking", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(map[string]string{
			"status":  "booking_pending",
			"message": "Your booking is being processed",
		})
	}
}
