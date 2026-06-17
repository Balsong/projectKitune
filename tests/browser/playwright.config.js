// @ts-check
const { defineConfig, devices } = require('@playwright/test');

// Базовый адрес сайта (nginx + проксирование /api на gateway).
// Переопределяется через BASE_URL, напр.: BASE_URL=http://localhost:8081
const baseURL = process.env.BASE_URL || 'http://localhost:8081';

module.exports = defineConfig({
  testDir: './specs',
  timeout: 30_000,
  expect: { timeout: 7_000 },
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 2 : 0,
  reporter: process.env.CI ? 'list' : [['list'], ['html', { open: 'never' }]],
  use: {
    baseURL,
    trace: 'on-first-retry',
    screenshot: 'only-on-failure',
  },
  projects: [
    { name: 'chromium', use: { ...devices['Desktop Chrome'] } },
  ],
});
