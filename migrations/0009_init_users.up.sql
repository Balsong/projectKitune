-- Пользователи: регистрация/вход. Пароль — bcrypt-хэш.
-- Согласие на обработку персональных данных фиксируется флагом и временем.

CREATE TABLE IF NOT EXISTS users (
    id                    UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email                 TEXT        NOT NULL,
    password_hash         TEXT        NOT NULL,
    name                  TEXT        NOT NULL DEFAULT '',
    phone                 TEXT        NOT NULL DEFAULT '',
    consent_personal_data BOOLEAN     NOT NULL DEFAULT FALSE,
    consent_at            TIMESTAMPTZ,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Уникальность e-mail без учёта регистра.
CREATE UNIQUE INDEX IF NOT EXISTS idx_users_email ON users (lower(email));
