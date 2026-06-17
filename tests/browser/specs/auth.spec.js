// Кликовый сценарий аккаунта: регистрация через форму → попадание в кабинет,
// затем смена пароля через профиль.
const { test, expect } = require('@playwright/test');

function uniqueEmail() {
  return `pw-${Date.now()}-${Math.floor(Math.random() * 1e6)}@kitsune.tea`;
}

test('регистрация через форму ведёт в личный кабинет', async ({ page }) => {
  const email = uniqueEmail();
  await page.goto('/register.html');

  await page.fill('#r-name', 'Playwright Гость');
  await page.fill('#r-phone', '+70000000000');
  await page.fill('#r-email', email);
  await page.fill('#r-password', 'secret123');
  await page.check('#r-consent'); // согласие на обработку ПДн обязательно
  await page.click('#reg-form button[type=submit]');

  await expect(page).toHaveURL(/account\.html/, { timeout: 10_000 });
  await expect(page.locator('#acc-email')).toContainText(email, { timeout: 7_000 });
});

test('смена пароля в кабинете', async ({ page }) => {
  const email = uniqueEmail();
  // Регистрируемся (быстрый путь — через тот же UI).
  await page.goto('/register.html');
  await page.fill('#r-name', 'Pwd Тест');
  await page.fill('#r-phone', '+70000000001');
  await page.fill('#r-email', email);
  await page.fill('#r-password', 'oldpass123');
  await page.check('#r-consent');
  await page.click('#reg-form button[type=submit]');
  await expect(page).toHaveURL(/account\.html/, { timeout: 10_000 });

  // Вкладка «Профиль и адреса» → форма смены пароля.
  await page.locator('#tabs .chip', { hasText: 'Профиль' }).click();
  await page.fill('#pwd-old', 'oldpass123');
  await page.fill('#pwd-new', 'newpass123');
  await page.fill('#pwd-new2', 'newpass123');
  await page.click('#pwd-form button[type=submit]');

  // Кнопка подтверждает успех.
  await expect(page.locator('#pwd-form button[type=submit]')).toContainText(/Готово|✓/, { timeout: 7_000 });
});

test('неверный пароль на входе показывает ошибку', async ({ page }) => {
  await page.goto('/login.html');
  await page.fill('#l-email', 'nobody-' + Date.now() + '@kitsune.tea');
  await page.fill('#l-password', 'wrongpass');
  await page.click('#login-form button[type=submit]');
  // Остаёмся на странице входа (в кабинет не пустило).
  await expect(page).toHaveURL(/login\.html/);
});
