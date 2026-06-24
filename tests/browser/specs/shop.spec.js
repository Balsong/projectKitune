// Кликовый сценарий чайного магазина: витрина → корзина → оформление заказа.
const { test, expect } = require('@playwright/test');

test('магазин: каталог открывается и фильтруется по категориям', async ({ page }) => {
  await page.goto('/shop.html');
  await expect(page.locator('h1')).toContainText(/магазин/i);
  // карточки подгружаются из /api/v1/shop/menu
  await expect(page.locator('.shop-card').first()).toBeVisible({ timeout: 10_000 });

  // фильтр по категории «Пуэр»
  await page.locator('#cats .chip', { hasText: 'Пуэр' }).click();
  await expect(page.locator('.shop-card').first()).toBeVisible();
});

test('магазин: добавление в корзину обновляет счётчик', async ({ page }) => {
  await page.goto('/shop.html');
  await page.evaluate(() => localStorage.removeItem('kitsune_shop_cart_id'));
  await page.reload();

  const addBtn = page.locator('.shop-card .add-btn:not([disabled])').first();
  await expect(addBtn).toBeVisible({ timeout: 10_000 });
  await addBtn.click();

  const count = page.locator('[data-shop-cart-count]').first();
  await expect(count).toHaveText(/[1-9]/, { timeout: 7_000 });
});

test('магазин: оформление заказа с доставкой', async ({ page }) => {
  // Кладём товар в корзину магазина.
  await page.goto('/shop.html');
  await page.evaluate(() => localStorage.removeItem('kitsune_shop_cart_id'));
  await page.reload();
  const addBtn = page.locator('.shop-card .add-btn:not([disabled])').first();
  await expect(addBtn).toBeVisible({ timeout: 10_000 });
  await addBtn.click();
  await expect(page.locator('[data-shop-cart-count]').first()).toHaveText(/[1-9]/);

  // Переходим в корзину и оформляем.
  await page.goto('/shop-cart.html');
  await expect(page.locator('#cart-body')).toBeVisible({ timeout: 7_000 });
  await page.fill('#c-name', 'Playwright Покупатель');
  await page.fill('#c-phone', '+70000000000');
  await page.fill('#c-city', 'Москва');
  await page.fill('#c-address', 'ул. Тверская, 1');
  await page.click('#checkout-form button[type=submit]');

  // Подтверждение заказа.
  await expect(page.locator('#order-ok')).toBeVisible({ timeout: 10_000 });
  await expect(page.locator('#order-ok-text')).toContainText(/KT-/);
});
