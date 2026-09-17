const { defineConfig } = require('@playwright/test');

module.exports = defineConfig({
  testDir: './tests',
  fullyParallel: true,
  forbidOnly: Boolean(process.env.CI),
  retries: process.env.CI ? 1 : 0,
  workers: 2,
  reporter: 'list',
  use: {
    baseURL: 'http://127.0.0.1:8770',
    browserName: 'chromium',
    reducedMotion: 'reduce',
    trace: 'retain-on-failure'
  },
  webServer: {
    command: 'python3 -m http.server 8770 --bind 127.0.0.1 --directory ../../site',
    url: 'http://127.0.0.1:8770',
    reuseExistingServer: false
  }
});
