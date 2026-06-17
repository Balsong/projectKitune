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

// Защита от брутфорса: после maxLoginAttempts неудачных входов по одному
// e-mail в окне lockoutWindow вход блокируется до истечения окна.
const (
	maxLoginAttempts = 5
	lockoutWindow    = 15 * time.Minute
)

func sessionKey(token string) string { return "session:" + token }
func loginFailKey(email string) string {
	return "login_fail:" + strings.TrimSpace(strings.ToLower(email))
}

// Service реализует gRPC AccountService.
type Service struct {
	accountv1.UnimplementedAccountServiceServer

	repo        *Repository
	rdb         *redis.Client
	log         *slog.Logger
	adminEmails map[string]bool // e-mail с ролью admin (из ADMIN_EMAILS)
}

// NewService собирает сервис аккаунтов. adminEmails — e-mail, которым при
// регистрации/входе назначается роль admin (бутстрап админки).
func NewService(repo *Repository, rdb *redis.Client, log *slog.Logger, adminEmails []string) *Service {
	set := make(map[string]bool, len(adminEmails))
	for _, e := range adminEmails {
		e = strings.TrimSpace(strings.ToLower(e))
		if e != "" {
			set[e] = true
		}
	}
	return &Service{repo: repo, rdb: rdb, log: log, adminEmails: set}
}

// ensureAdmin назначает роль admin, если e-mail входит в список админов.
func (s *Service) ensureAdmin(ctx context.Context, u *User) {
	if u == nil || u.Role == "admin" || !s.adminEmails[strings.ToLower(u.Email)] {
		return
	}
	if err := s.repo.PromoteAdmins(ctx, []string{u.Email}); err != nil {
		s.log.Warn("не удалось назначить admin", "email", u.Email, "error", err)
		return
	}
	u.Role = "admin"
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

	s.ensureAdmin(ctx, u)
	token, err := s.openSession(ctx, u.ID)
	if err != nil {
		return nil, err
	}
	s.log.Info("регистрация", "user_id", u.ID, "email", u.Email)
	return &accountv1.AuthResponse{SessionToken: token, User: toProto(u)}, nil
}

// Login проверяет пароль и открывает сессию. Защищён от брутфорса: после
// серии неудач вход по этому e-mail временно блокируется.
func (s *Service) Login(ctx context.Context, req *accountv1.LoginRequest) (*accountv1.AuthResponse, error) {
	if s.loginLocked(ctx, req.GetEmail()) {
		return nil, status.Error(codes.ResourceExhausted, "слишком много попыток входа, попробуйте позже")
	}

	hash, u, err := s.repo.Credentials(ctx, req.GetEmail())
	if errors.Is(err, ErrNotFound) {
		s.recordLoginFailure(ctx, req.GetEmail())
		return nil, status.Error(codes.Unauthenticated, "неверный e-mail или пароль")
	}
	if err != nil {
		return nil, status.Errorf(codes.Internal, "credentials: %v", err)
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(req.GetPassword())) != nil {
		s.recordLoginFailure(ctx, req.GetEmail())
		return nil, status.Error(codes.Unauthenticated, "неверный e-mail или пароль")
	}

	// Успех — сбрасываем счётчик неудач.
	_ = s.rdb.Del(ctx, loginFailKey(req.GetEmail())).Err()
	s.ensureAdmin(ctx, u)
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

// loginLocked сообщает, превышен ли лимит неудачных входов по e-mail.
func (s *Service) loginLocked(ctx context.Context, email string) bool {
	n, err := s.rdb.Get(ctx, loginFailKey(email)).Int()
	if err != nil {
		return false // нет ключа или Redis недоступен — не блокируем
	}
	return n >= maxLoginAttempts
}

// recordLoginFailure инкрементит счётчик неудач и продлевает окно блокировки.
func (s *Service) recordLoginFailure(ctx context.Context, email string) {
	key := loginFailKey(email)
	if err := s.rdb.Incr(ctx, key).Err(); err != nil {
		return
	}
	_ = s.rdb.Expire(ctx, key, lockoutWindow).Err()
}

// ChangePassword меняет пароль авторизованного пользователя (по сессии),
// проверяя текущий пароль. Текущая сессия остаётся действительной.
func (s *Service) ChangePassword(ctx context.Context, req *accountv1.ChangePasswordRequest) (*accountv1.LogoutResponse, error) {
	if req.GetSessionToken() == "" {
		return nil, status.Error(codes.Unauthenticated, "нет токена сессии")
	}
	if len(req.GetNewPassword()) < 6 {
		return nil, status.Error(codes.InvalidArgument, "новый пароль должен быть не короче 6 символов")
	}

	userID, err := s.rdb.Get(ctx, sessionKey(req.GetSessionToken())).Result()
	if errors.Is(err, redis.Nil) {
		return nil, status.Error(codes.Unauthenticated, "сессия истекла или недействительна")
	}
	if err != nil {
		return nil, status.Errorf(codes.Internal, "session lookup: %v", err)
	}

	hash, err := s.repo.PasswordHash(ctx, userID)
	if errors.Is(err, ErrNotFound) {
		return nil, status.Error(codes.Unauthenticated, "пользователь не найден")
	}
	if err != nil {
		return nil, status.Errorf(codes.Internal, "password hash: %v", err)
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(req.GetOldPassword())) != nil {
		return nil, status.Error(codes.PermissionDenied, "текущий пароль неверный")
	}

	newHash, err := bcrypt.GenerateFromPassword([]byte(req.GetNewPassword()), bcrypt.DefaultCost)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "hash password: %v", err)
	}
	if err := s.repo.UpdatePassword(ctx, userID, string(newHash)); err != nil {
		return nil, status.Errorf(codes.Internal, "update password: %v", err)
	}
	s.log.Info("смена пароля", "user_id", userID)
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

// PromoteAdmins назначает роль admin по списку e-mail (бутстрап из окружения).
func (s *Service) PromoteAdmins(ctx context.Context, emails []string) error {
	return s.repo.PromoteAdmins(ctx, emails)
}

func toProto(u *User) *accountv1.User {
	role := u.Role
	if role == "" {
		role = "user"
	}
	return &accountv1.User{
		Id:        u.ID,
		Email:     u.Email,
		Name:      u.Name,
		Phone:     u.Phone,
		Role:      role,
		CreatedAt: u.CreatedAt.UTC().Format(time.RFC3339),
	}
}
