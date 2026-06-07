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
	"tea-platform/pkg/response"
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

func healthHandler(w http.ResponseWriter, _ *http.Request) {
	response.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func handleGetMenu(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		response.Error(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	mu.RLock()
	defer mu.RUnlock()

	response.WriteJSON(w, http.StatusOK, menu)
}

func handleBookTable(producer *kafka.Producer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			response.Error(w, http.StatusMethodNotAllowed, "Method not allowed")
			return
		}

		var booking struct {
			TableID  int    `json:"table_id"`
			Customer string `json:"customer"`
			Time     string `json:"time"`
		}

		if err := json.NewDecoder(r.Body).Decode(&booking); err != nil {
			response.Error(w, http.StatusBadRequest, "Invalid request body")
			return
		}

		// Отправка в Kafka
		if err := producer.SendBooking(booking); err != nil {
			response.Error(w, http.StatusInternalServerError, "Failed to process booking")
			return
		}

		response.WriteJSON(w, http.StatusAccepted, map[string]string{
			"status":  "booking_pending",
			"message": "Your booking is being processed",
		})
	}
}
