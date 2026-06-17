package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	accountv1 "tea-platform/internal/genpb/account/v1"
	"tea-platform/pkg/response"
)

// bearerToken достаёт токен сессии из заголовка Authorization: Bearer <token>.
func bearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if strings.HasPrefix(h, "Bearer ") {
		return strings.TrimSpace(h[len("Bearer "):])
	}
	return ""
}

// sessionUserID возвращает id пользователя по токену сессии или "" для гостя.
func sessionUserID(ctx context.Context, account accountv1.AccountServiceClient, r *http.Request) string {
	token := bearerToken(r)
	if token == "" {
		return ""
	}
	u, err := account.GetSession(ctx, &accountv1.SessionRequest{SessionToken: token})
	if err != nil {
		return ""
	}
	return u.GetId()
}

type userDTO struct {
	ID        string `json:"id"`
	Email     string `json:"email"`
	Name      string `json:"name"`
	Phone     string `json:"phone"`
	Role      string `json:"role"`
	CreatedAt string `json:"created_at"`
}

func toUserDTO(u *accountv1.User) userDTO {
	return userDTO{
		ID:        u.GetId(),
		Email:     u.GetEmail(),
		Name:      u.GetName(),
		Phone:     u.GetPhone(),
		Role:      u.GetRole(),
		CreatedAt: u.GetCreatedAt(),
	}
}

type authResponseDTO struct {
	SessionToken string  `json:"session_token"`
	User         userDTO `json:"user"`
}

// handleRegister регистрирует пользователя (с согласием на обработку ПДн).
func handleRegister(log *slog.Logger, client accountv1.AccountServiceClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Email    string `json:"email"`
			Password string `json:"password"`
			Name     string `json:"name"`
			Phone    string `json:"phone"`
			Consent  bool   `json:"consent_personal_data"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			response.Error(w, http.StatusBadRequest, "Invalid request body")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		resp, err := client.Register(ctx, &accountv1.RegisterRequest{
			Email: body.Email, Password: body.Password, Name: body.Name,
			Phone: body.Phone, ConsentPersonalData: body.Consent,
		})
		if err != nil {
			writeGRPCError(w, log, "Register", err)
			return
		}
		response.WriteJSON(w, http.StatusCreated, authResponseDTO{
			SessionToken: resp.GetSessionToken(), User: toUserDTO(resp.GetUser()),
		})
	}
}

// handleLogin выполняет вход по e-mail и паролю.
func handleLogin(log *slog.Logger, client accountv1.AccountServiceClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Email    string `json:"email"`
			Password string `json:"password"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			response.Error(w, http.StatusBadRequest, "Invalid request body")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		resp, err := client.Login(ctx, &accountv1.LoginRequest{Email: body.Email, Password: body.Password})
		if err != nil {
			writeGRPCError(w, log, "Login", err)
			return
		}
		response.WriteJSON(w, http.StatusOK, authResponseDTO{
			SessionToken: resp.GetSessionToken(), User: toUserDTO(resp.GetUser()),
		})
	}
}

// handleMe возвращает текущего пользователя по токену сессии.
func handleMe(log *slog.Logger, client accountv1.AccountServiceClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		u, err := client.GetSession(ctx, &accountv1.SessionRequest{SessionToken: bearerToken(r)})
		if err != nil {
			writeGRPCError(w, log, "GetSession", err)
			return
		}
		response.WriteJSON(w, http.StatusOK, toUserDTO(u))
	}
}

// handleChangePassword меняет пароль авторизованного пользователя.
func handleChangePassword(log *slog.Logger, client accountv1.AccountServiceClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			OldPassword string `json:"old_password"`
			NewPassword string `json:"new_password"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			response.Error(w, http.StatusBadRequest, "Invalid request body")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		if _, err := client.ChangePassword(ctx, &accountv1.ChangePasswordRequest{
			SessionToken: bearerToken(r), OldPassword: body.OldPassword, NewPassword: body.NewPassword,
		}); err != nil {
			writeGRPCError(w, log, "ChangePassword", err)
			return
		}
		response.WriteJSON(w, http.StatusOK, map[string]bool{"ok": true})
	}
}

// handleLogout завершает сессию.
func handleLogout(log *slog.Logger, client accountv1.AccountServiceClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		if _, err := client.Logout(ctx, &accountv1.SessionRequest{SessionToken: bearerToken(r)}); err != nil {
			writeGRPCError(w, log, "Logout", err)
			return
		}
		response.WriteJSON(w, http.StatusOK, map[string]bool{"ok": true})
	}
}
