// Кликовый сценарий корзины: добавление позиции из меню обновляет счётчик
// и позиция появляется на странице корзины.
const { test, expect } = require('@playwright/test');

test('добавление позиции из меню обновляет корзину', async ({ page }) => {
  // Чистим состояние корзины перед прогоном.
  await page.goto('/');
  await page.evaluate(() => {
    localStorage.removeItem('kitsune_cart_id');
    localStorage.removeItem('kitsune_cart_v1');
  });

  await page.goto('/menu-kitchen.html');

  // Дожидаемся отрисовки карточек из /api/v1/menu.
  const addBtn = page.locator('.add-btn[data-add]').first();
  await expect(addBtn).toBeVisible({ timeout: 10_000 });
  await addBtn.click();

  // Счётчик в шапке становится положительным.
  const count = page.locator('[data-cart-count]').first();
  await expect(count).toHaveText(/[1-9]/, { timeout: 7_000 });

  // На странице корзины есть хотя бы одна позиция (не пустая корзина).
  await page.goto('/cart.html');
  await expect(page.locator('body')).not.toContainText(/корзина пуста/i, { timeout: 7_000 });
});
