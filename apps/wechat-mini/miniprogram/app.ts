import { bootstrap, ensureReady, hasSession, setForeground } from './services/session'
App({ onLaunch() { bootstrap(); if (hasSession()) void ensureReady().catch(() => {}) }, onShow() { setForeground(true); if (hasSession()) void ensureReady().catch(() => {}) }, onHide() { setForeground(false) } })
