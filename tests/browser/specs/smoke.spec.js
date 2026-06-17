// Дымовые сценарии: страницы открываются, навигация работает, SEO на месте.
const { test, expect } = require('@playwright/test');

const pages = [
  { path: '/', heading: /КИЦУНЭ|Путь чая|чай/i },
  { path: '/menu-tea.html', heading: /чай/i },
  { path: '/menu-kitchen.html', heading: /напитк|меню|кофе/i },
  { path: '/reservation.html', heading: /бронир/i },
  { path: '/delivery.html', heading: /доставк/i },
  { path: '/promos.html', heading: /акци/i },
  { path: '/about.html', heading: /о нас|атмосфер|чай/i },
  { path: '/contacts.html', heading: /контакт/i },
];

test.describe('Страницы и навигация', () => {
  for (const p of pages) {
    test(`страница ${p.path} открывается`, async ({ page }) => {
      const resp = await page.goto(p.path);
      expect(resp?.status(), `HTTP статус ${p.path}`).toBeLessThan(400);
      await expect(page.locator('h1').first()).toBeVisible();
      // Шапка и подвал присутствуют.
      await expect(page.locator('header.site-header')).toBeVisible();
      await expect(page.locator('footer.site-footer')).toBeVisible();
    });
  }

  test('переход по верхнему меню: на чайную карту', async ({ page }) => {
    await page.goto('/');
    await page.locator('header .nav__links a', { hasText: 'Чайная карта' }).click();
    await expect(page).toHaveURL(/menu-tea\.html/);
    await expect(page.locator('h1').first()).toBeVisible();
  });

  test('кнопка «Забронировать» ведёт на бронирование', async ({ page }) => {
    await page.goto('/');
    await page.locator('header a', { hasText: 'Забронировать' }).first().click();
    await expect(page).toHaveURL(/reservation\.html/);
  });
});

test.describe('SEO', () => {
  test('главная: canonical, Open Graph, описание', async ({ page }) => {
    await page.goto('/');
    await expect(page.locator('link[rel="canonical"]')).toHaveCount(1);
    await expect(page.locator('meta[property="og:title"]')).toHaveAttribute('content', /.+/);
    await expect(page.locator('meta[name="description"]')).toHaveAttribute('content', /.+/);
  });

  test('robots.txt и sitemap.xml доступны', async ({ request }) => {
    const robots = await request.get('/robots.txt');
    expect(robots.ok()).toBeTruthy();
    expect(await robots.text()).toContain('Sitemap:');

    const sitemap = await request.get('/sitemap.xml');
    expect(sitemap.ok()).toBeTruthy();
    expect(await sitemap.text()).toContain('<urlset');
  });
});
