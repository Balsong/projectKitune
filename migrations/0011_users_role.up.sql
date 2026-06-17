-- Роль пользователя для админки. 'user' по умолчанию, 'admin' — доступ к панели.
ALTER TABLE users ADD COLUMN IF NOT EXISTS role TEXT NOT NULL DEFAULT 'user';
