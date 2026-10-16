// Translations for the popup and the alert. Go owns the language: it resolves the setting,
// the system and navigator.languages against frontend/i18n/*.json and sends the merged
// strings (chosen language over its base over English) whenever the language changes.
import { Events } from "/wails/runtime.js";

let strings = {};
const listeners = [];
export const i18n = { lang: "en", languages: [], setting: "", version: "", trayStyle: "both", trayTitleMax: 25, platform: "", update: "",
  settings: { showDeclined: true, weekTab: true }, autostart: false, autostartError: "" };

// t("in.min", { m: 5 }); a numeric `count` picks the CLDR plural form (key_one, key_few, …).
// Translations carry no markup (CI checks), but vars are inserted as given: escape them.
export function t(key, vars = {}) {
  let s;
  if (typeof vars.count === "number") {
    s = strings[`${key}_${new Intl.PluralRules(i18n.lang).select(vars.count)}`] ?? strings[`${key}_other`];
  }
  s ??= strings[key] ?? key;
  return s.replace(/\{\{(\w+)\}\}/g, (_, k) => vars[k] ?? "");
}

// Elements with data-i18n get their text, data-i18n-title their tooltip.
function applyStatic() {
  document.querySelectorAll("[data-i18n]").forEach((el) => { el.textContent = t(el.dataset.i18n); });
  document.querySelectorAll("[data-i18n-title]").forEach((el) => { el.title = t(el.dataset.i18nTitle); });
  document.querySelectorAll("[data-i18n-placeholder]").forEach((el) => { el.placeholder = t(el.dataset.i18nPlaceholder); });
}

// Formatting shared by the popup and the alert.
export const hm = (d) => d.toLocaleTimeString(i18n.lang, { hour: "2-digit", minute: "2-digit" });
export const dur = (e) => {
  const m = Math.round((e.end - e.start) / 60000), h = Math.floor(m / 60);
  return m < 60 ? t("dur.min", { m }) : m % 60 ? t("dur.hmin", { h, m: m % 60 }) : t("dur.h", { h });
};
export const esc = (s) => String(s).replace(/[&<>"]/g, (c) => `&#${c.charCodeAt(0)};`);
// Escaped text with its links as buttons (opened through the backend, http(s) only).
export const linkify = (s) => esc(s).replace(/https?:\/\/[^\s<]+/g, (u) => `<button data-url="${u}">${u}</button>`);
// The backend names the service from the link's host; unknown services get a plain "Join".
export const joinLabel = (e) => e.service ? t("join.via", { service: e.service }) : t("join");
// Links open through the backend, which only passes http(s) on to the OS.
export const openURL = (url) => Events.Emit("open", url);

export function onLanguage(cb) { listeners.push(cb); }
export function setLanguage(code) { Events.Emit("i18n:set", code); }

Events.On("i18n", ({ data }) => {
  ({ strings } = data);
  Object.assign(i18n, { lang: data.lang, languages: data.languages, setting: data.setting, version: data.version,
    trayStyle: data.trayStyle, trayTitleMax: data.trayTitleMax, platform: data.platform, update: data.update,
    settings: data.settings, autostart: data.autostart, autostartError: data.autostartError });
  // The theme applies to every window (popup and alert), so it's set here rather than in Settings.
  if (data.settings.theme) document.documentElement.dataset.theme = data.settings.theme;
  else delete document.documentElement.dataset.theme;
  document.documentElement.lang = data.lang;
  applyStatic();
  listeners.forEach((cb) => cb());
});
Events.Emit("i18n:hello", [...navigator.languages]);
