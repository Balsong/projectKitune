package account

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"golang.org/x/crypto/bcrypt"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	accountv1 "tea-platform/internal/genpb/account/v1"
)

// sessionTTL — срок жизни сессии в Redis.
const sessionTTL = 7 * 24 * time.Hour

func sessionKey(token string) string { return "session:" + token }

// Service реализует gRPC AccountService.
type Service struct {
	accountv1.UnimplementedAccountServiceServer

	repo *Repository
	rdb  *redis.Client
	log  *slog.Logger
}

// NewService собирает сервис аккаунтов.
func NewService(repo *Repository, rdb *redis.Client, log *slog.Logger) *Service {
	return &Service{repo: repo, rdb: rdb, log: log}
}

// Register регистрирует пользователя и сразу открывает сессию.
func (s *Service) Register(ctx context.Context, req *accountv1.RegisterRequest) (*accountv1.AuthResponse, error) {
	email := strings.TrimSpace(strings.ToLower(req.GetEmail()))
	if !strings.Contains(email, "@") {
		return nil, status.Error(codes.InvalidArgument, "введите корректный e-mail")
	}
	if len(req.GetPassword()) < 6 {
		return nil, status.Error(codes.InvalidArgument, "пароль должен быть не короче 6 символов")
	}
	if !req.GetConsentPersonalData() {
		return nil, status.Error(codes.InvalidArgument, "необходимо согласие на обработку персональных данных")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.GetPassword()), bcrypt.DefaultCost)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "hash password: %v", err)
	}

	u, err := s.repo.Create(ctx, email, string(hash), strings.TrimSpace(req.GetName()), strings.TrimSpace(req.GetPhone()), true)
	if errors.Is(err, ErrEmailTaken) {
		return nil, status.Error(codes.AlreadyExists, "пользователь с таким e-mail уже зарегистрирован")
	}
	if err != nil {
		return nil, status.Errorf(codes.Internal, "create user: %v", err)
	}

	token, err := s.openSession(ctx, u.ID)
	if err != nil {
		return nil, err
	}
	s.log.Info("регистрация", "user_id", u.ID, "email", u.Email)
	return &accountv1.AuthResponse{SessionToken: token, User: toProto(u)}, nil
}

// Login проверяет пароль и открывает сессию.
func (s *Service) Login(ctx context.Context, req *accountv1.LoginRequest) (*accountv1.AuthResponse, error) {
	hash, u, err := s.repo.Credentials(ctx, req.GetEmail())
	if errors.Is(err, ErrNotFound) {
		return nil, status.Error(codes.Unauthenticated, "неверный e-mail или пароль")
	}
	if err != nil {
		return nil, status.Errorf(codes.Internal, "credentials: %v", err)
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(req.GetPassword())) != nil {
		return nil, status.Error(codes.Unauthenticated, "неверный e-mail или пароль")
	}

	token, err := s.openSession(ctx, u.ID)
	if err != nil {
		return nil, err
	}
	s.log.Info("вход", "user_id", u.ID)
	return &accountv1.AuthResponse{SessionToken: token, User: toProto(u)}, nil
}

// GetSession валидирует токен и возвращает пользователя.
func (s *Service) GetSession(ctx context.Context, req *accountv1.SessionRequest) (*accountv1.User, error) {
	if req.GetSessionToken() == "" {
		return nil, status.Error(codes.Unauthenticated, "нет токена сессии")
	}
	userID, err := s.rdb.Get(ctx, sessionKey(req.GetSessionToken())).Result()
	if errors.Is(err, redis.Nil) {
		return nil, status.Error(codes.Unauthenticated, "сессия истекла или недействительна")
	}
	if err != nil {
		return nil, status.Errorf(codes.Internal, "session lookup: %v", err)
	}
	u, err := s.repo.GetByID(ctx, userID)
	if errors.Is(err, ErrNotFound) {
		return nil, status.Error(codes.Unauthenticated, "пользователь не найден")
	}
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get user: %v", err)
	}
	return toProto(u), nil
}

// Logout завершает сессию.
func (s *Service) Logout(ctx context.Context, req *accountv1.SessionRequest) (*accountv1.LogoutResponse, error) {
	if req.GetSessionToken() != "" {
		_ = s.rdb.Del(ctx, sessionKey(req.GetSessionToken())).Err()
	}
	return &accountv1.LogoutResponse{Ok: true}, nil
}

// openSession создаёт opaque-токен и кладёт сессию в Redis с TTL.
func (s *Service) openSession(ctx context.Context, userID string) (string, error) {
	token := uuid.NewString()
	if err := s.rdb.Set(ctx, sessionKey(token), userID, sessionTTL).Err(); err != nil {
		return "", status.Errorf(codes.Internal, "create session: %v", err)
	}
	return token, nil
}

func toProto(u *User) *accountv1.User {
	return &accountv1.User{
		Id:        u.ID,
		Email:     u.Email,
		Name:      u.Name,
		Phone:     u.Phone,
		CreatedAt: u.CreatedAt.UTC().Format(time.RFC3339),
	}
}
