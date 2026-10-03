"use strict";
(function () {
  /* =========================================================================
     АРЕНА ПЕРЕГОВОРОВ — интерфейс поверх REST API (OpenAPI 3.0.3, v0.3.0).
     Экраны и навигация повторяют контракт: другого источника данных нет.
       0. Утилиты
       1. Состояние и настройки
       2. Доступ к API
       3. Каркас: сайдбар, шапка, маршрутизация
       4. Экраны
       5. Аутентификация
       6. События
       7. Инициализация
     ========================================================================= */

  /* === 0. УТИЛИТЫ ========================================================= */
  const $ = (sel, root) => (root || document).querySelector(sel);

  function esc(s) {
    return String(s == null ? "" : s)
      .replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;")
      .replace(/"/g, "&quot;").replace(/'/g, "&#39;");
  }
  const MG = ["янв", "фев", "мар", "апр", "мая", "июн", "июл", "авг", "сен", "окт", "ноя", "дек"];
  function dayLabel(ts) { const d = new Date(ts); return d.getDate() + " " + MG[d.getMonth()]; }
  function hhmm(ts) { const d = new Date(ts); return String(d.getHours()).padStart(2, "0") + ":" + String(d.getMinutes()).padStart(2, "0"); }
  function fmtDateTime(iso) {
    const t = Date.parse(iso || "");
    if (!t) return "—";
    const d = new Date(t);
    return dayLabel(t) + ", " + hhmm(t) + (d.getFullYear() !== new Date().getFullYear() ? " " + d.getFullYear() : "");
  }
  function initials(name) {
    return String(name || "?").split(/\s+/).slice(0, 2).map(w => w[0] || "").join("").toUpperCase();
  }

  const ICONS = {
    menu: '<path d="M4 7h16M4 12h16M4 17h16"/>',
    close: '<path d="M6 6l12 12M18 6L6 18"/>',
    grid: '<rect x="3" y="3" width="7" height="7" rx="1.5"/><rect x="14" y="3" width="7" height="7" rx="1.5"/><rect x="3" y="14" width="7" height="7" rx="1.5"/><rect x="14" y="14" width="7" height="7" rx="1.5"/>',
    chart: '<path d="M4 20V10M10 20V4M16 20v-7M22 20H2"/>',
    edit: '<path d="M4 20h4l10-10-4-4L4 16z"/><path d="M14 6l4 4"/>',
    list: '<path d="M8 6h13M8 12h13M8 18h13M3 6h.01M3 12h.01M3 18h.01"/>',
    plus: '<path d="M12 5v14M5 12h14"/>',
    play: '<path d="M7 4l13 8-13 8z"/>',
    send: '<path d="M12 19V5M6 11l6-6 6 6"/>',
    left: '<path d="M15 6l-6 6 6 6"/>',
    right: '<path d="M9 6l6 6-6 6"/>',
    check: '<path d="M4 12l5 5L20 6"/>',
    alert: '<path d="M12 3l9 16H3z"/><path d="M12 9v5M12 17h.01"/>',
    activity: '<path d="M3 12h4l3-7 4 14 3-7h4"/>',
    scan: '<path d="M4 8V5h3M20 8V5h-3M4 16v3h3M20 16v3h-3M3 12h18"/>',
    rotate: '<path d="M4 12a8 8 0 1 0 3-6.2M4 4v5h5"/>',
    flag: '<path d="M5 21V4h13l-2 4 2 4H5"/>',
    clock: '<circle cx="12" cy="12" r="9"/><path d="M12 7v5l3 2"/>',
    user: '<circle cx="12" cy="8" r="4"/><path d="M4 21a8 8 0 0 1 16 0"/>',
    users: '<circle cx="9" cy="8" r="3.4"/><path d="M2.5 20a6.5 6.5 0 0 1 13 0"/><path d="M16 5.2a3.4 3.4 0 0 1 0 6.6M17 14.6a6.5 6.5 0 0 1 4.5 5.4"/>',
    bot: '<rect x="4" y="8" width="16" height="12" rx="3"/><path d="M12 4v4M9 14h.01M15 14h.01"/>',
    target: '<circle cx="12" cy="12" r="8"/><circle cx="12" cy="12" r="3"/>',
    shield: '<path d="M12 3l8 3v6c0 5-3.5 8-8 9-4.5-1-8-4-8-9V6z"/>',
    spark: '<path d="M12 3l2 6 6 2-6 2-2 6-2-6-6-2 6-2z"/>',
    lock: '<rect x="5" y="11" width="14" height="9" rx="2"/><path d="M8 11V8a4 4 0 0 1 8 0v3"/>',
    mic: '<rect x="9" y="3" width="6" height="11" rx="3"/><path d="M5 11a7 7 0 0 0 14 0M12 18v3M9 21h6"/>'
  };
  function ico(name, size) {
    const s = size || 18;
    return '<svg viewBox="0 0 24 24" width="' + s + '" height="' + s + '" fill="none" stroke="currentColor" ' +
      'stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true" focusable="false">' +
      (ICONS[name] || ICONS.spark) + '</svg>';
  }

  function toast(text) {
    const box = $("#toasts");
    const el = document.createElement("div");
    el.className = "toast"; el.textContent = text;
    box.appendChild(el);
    setTimeout(() => el.remove(), 2600);
  }
  function modal(html) {
    const root = $("#modalRoot");
    root.innerHTML = '<div class="ov" data-act="closeModal"></div><div class="modal" role="dialog" aria-modal="true">' + html + '</div>';
    root.classList.add("on");
    root.setAttribute("aria-hidden", "false");
    const first = $("button, [href], input, select, textarea", root);
    if (first) first.focus();
  }
  function closeModal() {
    const root = $("#modalRoot");
    root.classList.remove("on");
    root.innerHTML = "";
    root.setAttribute("aria-hidden", "true");
  }

  /* === 1. СОСТОЯНИЕ ======================================================= */
  const LS_PREF = "arena_prefs";
  const LS_IDS = "arena_client_ids";
  const LEGACY_KEYS = ["arena_v2_scenarios", "arena_v2_sessions", "arena_v2_prefs", "arena_v2_api_ids"];

  const prefs = { theme: null, rail: true, cid: "", view: "learn", authMode: "login" };

  function loadPrefs() {
    LEGACY_KEYS.forEach(k => { try { localStorage.removeItem(k); } catch (e) { } });
    let saved = null;
    try { saved = JSON.parse(localStorage.getItem(LS_PREF)); } catch (e) { saved = null; }
    if (saved && typeof saved === "object") Object.assign(prefs, saved);
    if (prefs.theme) document.documentElement.setAttribute("data-theme", prefs.theme);
  }
  function savePrefs() {
    try {
      localStorage.setItem(LS_PREF, JSON.stringify({
        theme: prefs.theme, rail: prefs.rail,
        cid: prefs.cid, view: prefs.view, authMode: prefs.authMode
      }));
    } catch (e) { }
  }

  const state = {
    boot: "checking",
    profile: null,
    companies: [],
    token: "",
    authLoading: false,
    lastEmail: "",
    inviteToken: "",
    profileOpen: false,
    draftText: "",
    typing: false,
    stepIndex: 0,
    listening: false,
    recog: null
  };

  const empty = () => ({
    modules: null, modulesErr: null, modulesLoading: false,
    module: null, moduleErr: null, moduleLoading: false, moduleId: "", explanation: null,
    cases: null, casesErr: null, casesLoading: false,
    caseDetail: null, caseDetailKey: "", caseDetailErr: null, caseDetailLoading: false,
    session: null, sessionErr: null, sessionLoading: false, sending: false,
    result: null, resultLoading: false, resultTried: false,
    hint: null, hintLoading: false,
    history: null, historyErr: null, historyLoading: false,
    progress: null, progressErr: null, progressLoading: false,
    members: null, membersErr: null, team: null, teamErr: null, teamLoading: false,
    busy: false
  });
  let data = empty();
  function resetData() { data = empty(); }

  /* === 2. ДОСТУП К API ==================================================== */
  /* Страница и API живут на одном домене (nginx проксирует /api/, /auth/, /health/),
     поэтому base пустой, а HttpOnly-cookie уходят сами. Access-токен держим только
     в памяти: после перезагрузки сессию восстанавливает cookie + POST /auth/refresh. */
  function conf(extra) {
    const base = { base: "", cid: prefs.cid, token: state.token, credentials: "same-origin" };
    return extra ? Object.assign(base, extra) : base;
  }
  ArenaApi.setTokenSink(function (accessToken) { state.token = accessToken; });

  function errText(e) {
    if (!e) return "неизвестная ошибка";
    const code = e.code || "";
    if (e.kind === "unauthorized") {
      if (code === "invalid email or password") return "Неверный email или пароль";
      return "Нужен вход — сессия истекла";
    }
    if (e.kind === "forbidden") {
      if (code === "hints_disabled") return "Подсказки отключены в проверочном кейсе";
      if (code === "origin_forbidden") return "Сервис отклонил origin страницы: проверьте, что запрос идёт на тот же адрес, что и страница";
      return "Недостаточно прав в компании";
    }
    if (e.kind === "not_found") return "Объект не найден или недоступен в этой компании";
    if (e.kind === "conflict") {
      const map = {
        assessment_locked: "Кейс закрыт: сначала пройдите теорию модуля",
        invalid_session_state: "Сессия не активна или лимит ходов исчерпан",
        session_not_evaluated: "Сессию параллельно меняет другой запрос — обновите её",
        hint_unavailable: "Подсказка недоступна: лимит исчерпан или сессия не активна",
        "email already registered": "Этот email уже зарегистрирован",
        conflict: "Конфликт состояния"
      };
      return map[code] || "Конфликт состояния (409)";
    }
    if (e.kind === "bad_request") {
      const map = {
        invalid_answer: "Ответ не принят",
        invalid_request: "Некорректный запрос",
        "invalid email": "Некорректный email",
        "password must be 8-72 bytes long": "Пароль должен быть от 8 до 72 байт"
      };
      return map[code] || ("Некорректный запрос (" + e.status + ")");
    }
    if (e.kind === "rate_limited") return "Слишком много попыток входа" + (e.retryAfter ? ", повторите через " + e.retryAfter + " с" : "");
    if (e.kind === "network") return "Сервис недоступен: запрос не прошёл";
    if (e.kind === "timeout") return "Таймаут запроса к API";
    if (e.kind === "server") return "Ошибка сервиса (" + e.status + ")";
    return e.message || ("HTTP " + (e.status || "?"));
  }

  /* Идемпотентность попыток и ходов: один и тот же (шаг, ответ) переиспользует id. */
  function clientId(scope, id, payload) {
    const str = String(payload == null ? "" : payload);
    let h = 0;
    for (let i = 0; i < str.length; i++) h = (h * 31 + str.charCodeAt(i)) | 0;
    const key = scope + "|" + id + "|" + (h >>> 0).toString(36) + "|" + str.length;
    try {
      const all = JSON.parse(localStorage.getItem(LS_IDS)) || {};
      if (all[key]) return all[key];
      const fresh = ArenaApi.newAttemptId();
      all[key] = fresh;
      const keys = Object.keys(all);
      if (keys.length > 200) keys.slice(0, keys.length - 200).forEach(k => { delete all[k]; });
      localStorage.setItem(LS_IDS, JSON.stringify(all));
      return fresh;
    } catch (e) { return ArenaApi.newAttemptId(); }
  }

  function currentCompany() { return state.companies.find(c => c.id === prefs.cid) || null; }
  function isCompanyAdmin() {
    const c = currentCompany();
    return !!c && (c.role === "company_admin" || c.role === "platform_admin");
  }
  function hasCompany() { return !!prefs.cid && !!currentCompany(); }

  /* === 3. КАРКАС ========================================================== */
  const VIEWS_BASE = ["account"];
  const VIEWS_COMPANY = ["learn", "module", "step", "cases", "case", "chat", "result", "feedback", "progress"];
  const VIEWS_ADMIN = ["team"];

  function allowedViews() {
    let list = VIEWS_BASE.slice();
    if (hasCompany()) list = list.concat(VIEWS_COMPANY);
    if (isCompanyAdmin()) list = list.concat(VIEWS_ADMIN);
    return list;
  }
  function ensureView() {
    const list = allowedViews();
    if (list.indexOf(prefs.view) === -1) prefs.view = hasCompany() ? "learn" : "account";
    const needsModule = ["module", "step"].indexOf(prefs.view) !== -1;
    const needsCase = prefs.view === "case";
    const needsSession = ["chat", "result", "feedback"].indexOf(prefs.view) !== -1;
    if (needsModule && !data.moduleId) prefs.view = "learn";
    else if (needsCase && !data.caseDetailKey) prefs.view = "cases";
    else if (needsSession && !data.session) prefs.view = "cases";
  }
  function go(v) {
    prefs.view = v;
    state.typing = false;
    stopSpeech();
    document.body.classList.remove("nav-open");
    savePrefs(); render(); closeModal();
  }

  function menuBtn() {
    return '<button class="iconbtn menu-btn" data-act="toggleNav" aria-label="Показать меню">' + ico("menu", 20) + '</button>';
  }
  function navBtn(view, icon, label, active) {
    return '<button data-act="go" data-v="' + view + '" class="' + (active ? "on" : "") + ' tip-host">'
      + ico(icon, 18) + '<span class="label">' + esc(label) + '</span>'
      + '<span class="tip">' + esc(label) + '</span></button>';
  }
  function ph(title, sub, actions) {
    return '<div class="ph">' + menuBtn()
      + '<div class="grow"><h1>' + esc(title) + '</h1>' + (sub ? '<div class="sub">' + esc(sub) + '</div>' : '')
      + '</div>' + (actions || '') + '</div>';
  }
  function card(text) {
    return '<div class="card" style="margin-top:16px"><div class="sm mut">' + esc(text) + '</div></div>';
  }
  function retryRow(act, extra) {
    return '<div class="cta-row" style="justify-content:flex-start;margin-top:12px">'
      + '<button class="btn" data-act="' + act + '">' + ico("rotate", 15) + ' Повторить</button>' + (extra || '') + '</div>';
  }

  function profileMenuHtml() {
    const p = state.profile;
    const cur = currentCompany();
    let h = '<div class="profile-wrap">';
    h += '<button class="user tip-host' + (state.profileOpen ? " open" : "") + '" data-act="toggleProfile" aria-haspopup="menu" aria-expanded="' + (!!state.profileOpen) + '" aria-label="Профиль и настройки">'
      + '<div class="av">' + esc(initials(p.name || p.email)) + '</div>'
      + '<div class="grow label" style="text-align:left"><div style="font-weight:600">' + esc(p.name || "(без имени)") + '</div>'
      + '<div class="xs mut">' + esc(cur ? ArenaApi.roleLabel(cur.role) : "Без компании") + '</div></div>'
      + '<span class="tip">' + esc(p.name || p.email) + '</span></button>';
    if (state.profileOpen) {
      h += '<div class="pmenu" role="menu">'
        + '<div class="ph-head"><div class="av">' + esc(initials(p.name || p.email)) + '</div>'
        + '<div class="grow"><div style="font-weight:600">' + esc(p.name || "(без имени)") + '</div>'
        + '<div class="xs mut">' + esc(p.email || "") + '</div></div></div>'
        + '<button role="menuitem" data-act="go" data-v="account">' + ico("user", 17) + ' Аккаунт</button>'
        + '<button role="menuitem" data-act="openSettings">' + ico("edit", 17) + ' Настройки</button>'
        + '<hr><button role="menuitem" class="danger" data-act="askLogout">' + ico("left", 17) + ' Выход</button></div>';
    }
    return h + '</div>';
  }

  function renderSidebar() {
    const v = prefs.view;
    const on = list => list.indexOf(v) !== -1;
    const collapsed = !!prefs.rail;
    let h = '<div class="brand"><button class="iconbtn collapse" data-act="toggleNav" aria-expanded="' + (!collapsed) + '" '
      + 'aria-label="' + (collapsed ? "Развернуть боковую панель" : "Свернуть боковую панель") + '">'
      + '<span class="ic-shield">' + ico("shield", 18) + '</span>'
      + '<span class="ic-hover">' + ico(collapsed ? "right" : "left", 18) + '</span>'
      + '</button></div>';
    h += '<nav class="nav" aria-label="Основная навигация">';
    if (hasCompany()) {
      h += navBtn("learn", "spark", "Обучение", on(["learn", "module", "step"]))
        + navBtn("cases", "grid", "Кейсы", on(["cases", "case", "chat", "result", "feedback"]))
        + navBtn("progress", "chart", "Прогресс", on(["progress"]));
      if (isCompanyAdmin()) h += navBtn("team", "users", "Сотрудники", on(["team"]));
    }
    h += navBtn("account", "user", "Аккаунт", on(["account"]));
    h += '</nav>';
    if (hasCompany()) {
      const list = data.history || [];
      h += '<div class="sec">Недавние сессии</div><div class="recents">';
      if (!list.length) h += '<div class="xs mut" style="padding:6px 10px">' + (data.historyLoading ? "Загрузка…" : "Пока нет сессий") + '</div>';
      list.slice(0, 6).forEach(s => {
        h += '<button class="rec' + (s.id === (data.session && data.session.sessionId) ? " on" : "") + '" data-act="openSession" data-id="' + esc(s.id) + '" title="' + esc(s.title) + '">'
          + '<div class="t">' + esc(s.title || "Кейс") + '</div>'
          + '<div class="s">' + esc(ArenaApi.caseModeLabel(s.mode) + " · " + ArenaApi.sessionStatusLabel(s.status)) + '</div></button>';
      });
      h += '</div>';
    }
    h += '<div class="grow"></div>' + profileMenuHtml();
    $("#sidebar").innerHTML = h;
  }

  function render() {
    const main = $("#main");
    if (state.boot === "checking") {
      document.body.classList.add("logged-out");
      $("#sidebar").innerHTML = "";
      main.innerHTML = '<div class="page" style="display:grid;place-items:center"><div class="card" style="max-width:380px;text-align:center">'
        + '<div class="empty" style="padding:8px 0"><div class="ic">' + ico("shield", 26) + '</div>'
        + '<b>Проверяем сессию…</b><div class="sm">Секунду — восстанавливаем вход.</div></div></div></div>';
      return;
    }
    if (!state.profile) {
      document.body.classList.add("logged-out");
      document.body.classList.remove("nav-open");
      $("#sidebar").innerHTML = "";
      main.innerHTML = viewAuth();
      return;
    }
    document.body.classList.remove("logged-out");
    ensureView();
    renderSidebar();
    const v = prefs.view;
    if (v === "learn") main.innerHTML = viewModules();
    else if (v === "module") main.innerHTML = viewModule();
    else if (v === "step") main.innerHTML = viewStep();
    else if (v === "cases") main.innerHTML = viewCases();
    else if (v === "case") main.innerHTML = viewCase();
    else if (v === "chat") main.innerHTML = viewChat();
    else if (v === "result") main.innerHTML = viewResult();
    else if (v === "feedback") main.innerHTML = viewFeedback();
    else if (v === "progress") main.innerHTML = viewProgress();
    else if (v === "team") main.innerHTML = viewTeam();
    else main.innerHTML = viewAccount();
    if (v === "chat") { autosizeCompose(); scrollMsgs(); }
  }
  function scrollMsgs() { const m = $("#msgs"); if (m) m.scrollTop = m.scrollHeight; }

  const COMPOSE_MAX_H = 220;
  function autosizeCompose(el) {
    const ta = el || $("#compose");
    if (!ta) return;
    ta.style.height = "auto";
    ta.style.height = Math.min(ta.scrollHeight, COMPOSE_MAX_H) + "px";
  }

  /* === 4. ЭКРАНЫ ========================================================== */

  /* --- Вход, регистрация, приглашение (единственное, что видно до входа) --- */
  function viewAuth() {
    const reg = prefs.authMode === "register";
    let h = '<div class="page"><div class="inner" style="max-width:460px">';
    h += '<div style="text-align:center;margin:8px 0 20px">'
      + '<div class="av" style="margin:0 auto 10px;width:46px;height:46px">' + ico("shield", 22) + '</div>'
      + '<h1 style="font-size:24px;margin:0">Арена переговоров</h1>'
      + '<div class="sub" style="margin-top:6px">Тренажёр переговоров с ИИ-оппонентом. Войдите, чтобы открыть обучение и кейсы компании.</div></div>';
    h += '<div class="card"><div class="topline"></div>';
    h += '<div class="seg2" style="margin-bottom:14px">'
      + '<button data-act="authMode" data-v="login" class="' + (reg ? "" : "on") + '">Вход</button>'
      + '<button data-act="authMode" data-v="register" class="' + (reg ? "on" : "") + '">Регистрация</button></div>';
    h += '<form data-act="' + (reg ? "authRegister" : "authLogin") + '">';
    if (reg) h += '<div class="field"><label for="ap-name">Имя</label><input class="input" id="ap-name" name="name" maxlength="100" placeholder="Как к вам обращаться" required></div>';
    h += '<div class="field"><label for="ap-email">Email</label>'
      + '<input class="input" id="ap-email" name="email" type="email" maxlength="254" placeholder="name@company.ru" value="' + esc(state.lastEmail || "") + '" required></div>'
      + '<div class="field"><label for="ap-password">Пароль</label>'
      + '<input class="input" id="ap-password" name="password" type="password" placeholder="' + (reg ? "Минимум 8 символов" : "Пароль") + '"' + (reg ? ' minlength="8"' : '') + ' required></div>'
      + '<button type="submit" class="btn pri wide lg"' + (state.authLoading ? " disabled" : "") + '>'
      + (state.authLoading ? "Отправляем…" : (reg ? "Создать аккаунт" : "Войти")) + '</button></form>';
    h += '<div style="border-top:1px solid var(--line);margin-top:16px;padding-top:14px">'
      + '<form data-act="authInvite">'
      + '<div class="field"><label for="ap-invite">Есть код приглашения?</label>'
      + '<input class="input" id="ap-invite" name="token" placeholder="Вставьте токен из ссылки" value="' + esc(state.inviteToken || "") + '"></div>'
      + '<button type="submit" class="btn wide">Принять приглашение</button></form></div>';
    h += '</div>';
    h += '<div class="xs mut" style="text-align:center;margin-top:14px">Доступ к обучению и кейсам даёт приглашение администратора компании.</div>';
    return h + '</div></div>';
  }

  /* --- Обучение: список модулей (GET /modules) --- */
  function loadModules(force) {
    if (!hasCompany() || data.modulesLoading) return;
    if (data.modules && !force) return;
    if (data.modulesErr && !force) return;
    data.modulesLoading = true; data.modulesErr = null; render();
    ArenaApi.listModules(conf()).then(r => {
      data.modulesLoading = false;
      if (r.ok) { data.modules = ArenaApi.mapModulesToTracks(r.data); data.modulesErr = null; }
      else { data.modules = null; data.modulesErr = r.error; toast("Обучение: " + errText(r.error)); }
      render();
    });
  }
  function viewModules() {
    if (!data.modules && !data.modulesLoading && !data.modulesErr) setTimeout(() => loadModules(), 0);
    let h = '<div class="page"><div class="inner">';
    h += ph("Обучение", "Дорожка модулей компании: теория, вопросы и проверочный кейс.",
      '<button class="btn" data-act="reloadModules">' + ico("rotate", 15) + ' Обновить</button>');
    if (!data.modules) {
      h += card(data.modulesLoading ? "Загружаем модули…" : ("Не удалось получить модули: " + errText(data.modulesErr)));
      if (!data.modulesLoading) h += retryRow("reloadModules");
      return h + '</div></div>';
    }
    if (!data.modules.length) {
      h += '<div class="empty"><div class="ic">' + ico("spark", 26) + '</div><b>Модулей пока нет</b>'
        + '<div class="sm">В этой компании ещё не опубликовано ни одного учебного модуля.</div></div>';
      return h + '</div></div>';
    }
    h += '<div class="grid">';
    data.modules.forEach(m => {
      const total = m.totalSteps || m.steps.length || 1;
      const done = m.completedSteps || 0;
      const pct = Math.round(done / total * 100);
      h += '<button class="sc" data-act="openModule" data-id="' + esc(m.id) + '"' + (m.locked ? ' disabled' : '') + ' style="min-height:206px">'
        + '<div class="who"><div class="av">' + ico(m.locked ? "lock" : "target", 18) + '</div>'
        + '<div class="grow"><h3>' + esc(m.title) + '</h3>'
        + '<div class="xs mut">Модуль ' + m.order + ' · ' + esc((m.skillIds || []).join(" · ")) + '</div></div></div>'
        + '<div class="q sm">' + esc(statusHint(m.status)) + '</div>'
        + '<div class="ft mod">'
        + '<div class="mod-row"><div class="bar"><i class="' + (pct === 100 ? "g" : "") + '" style="width:' + pct + '%"></i></div>'
        + '<span class="badge' + (m.status === "passed" ? " g" : "") + '">' + esc(ArenaApi.statusToLabel(m.status)) + '</span></div>'
        + '<div class="xs mut">' + done + ' из ' + total + ' шагов</div></div></button>';
    });
    return h + '</div></div></div>';
  }
  function statusHint(status) {
    const M = {
      locked: "Откроется после предыдущих модулей.",
      available: "Модуль доступен — можно начинать.",
      in_progress: "Модуль в процессе: продолжайте шаги.",
      assessment_ready: "Теория пройдена, открыт проверочный кейс.",
      needs_retry: "Проверочный кейс не зачтён — нужна ещё попытка.",
      passed: "Модуль пройден."
    };
    return M[status] || "";
  }

  /* --- Модуль и шаги (GET /modules/{id}) --- */
  function loadModule(id, force) {
    if (!hasCompany() || !id || data.moduleLoading) return;
    if (data.module && data.module.id === id && !force) return;
    if (data.moduleErr && data.moduleId === id && !force) return;
    data.moduleLoading = true; data.moduleErr = null; data.moduleId = id; data.explanation = null;
    render();
    ArenaApi.getModule(conf({ moduleId: id })).then(r => {
      data.moduleLoading = false;
      if (r.ok) { data.module = ArenaApi.mapModule(r.data); data.moduleErr = null; }
      else { data.module = null; data.moduleErr = r.error; toast("Модуль: " + errText(r.error)); }
      render();
    });
  }
  function stepParts(s) {
    const paras = String((s && s.content) || "").split(/\n{2,}/)
      .map(p => p.replace(/\s+/g, " ").trim()).filter(Boolean);
    if (!paras.length) return { title: "", body: [] };
    if (s && s.type === "choice") return { title: paras.join(" "), body: [] };
    const first = paras[0];
    const m = first.match(/^.*?[.!?…](?=\s|$)/);
    const title = m ? m[0].trim() : first;
    const tail = first.slice(title.length).trim();
    return { title: title, body: (tail ? [tail] : []).concat(paras.slice(1)) };
  }
  function stepTitle(s, i) {
    return stepParts(s).title || ("Шаг " + (i + 1));
  }
  function viewModule() {
    const id = data.moduleId;
    const m = (data.module && data.module.id === id) ? data.module : null;
    if (!m && !data.moduleLoading && !data.moduleErr) setTimeout(() => loadModule(id), 0);
    let h = '<div class="page"><div class="inner" style="max-width:720px">';
    if (!m) {
      h += ph("Модуль", "Загрузка из API", '<button class="btn" data-act="go" data-v="learn">' + ico("left", 16) + ' Ко всем модулям</button>');
      h += card(data.moduleLoading ? "Загружаем модуль…" : ("Не удалось получить модуль: " + errText(data.moduleErr)));
      if (!data.moduleLoading) h += retryRow("reloadModule");
      return h + '</div></div>';
    }
    const total = m.totalSteps || m.steps.length || 1;
    const done = m.completedSteps || 0;
    h += ph(m.title, (m.skillIds || []).join(" · "), '<button class="btn" data-act="go" data-v="learn">' + ico("left", 16) + ' Ко всем модулям</button>');
    h += '<div class="card" style="margin-bottom:18px"><div style="display:flex;justify-content:space-between;gap:12px;margin-bottom:8px">'
      + '<b>Прогресс модуля</b><span class="xs mut">' + done + ' из ' + total + ' · ' + esc(ArenaApi.statusToLabel(m.status)) + '</span></div>'
      + '<div class="bar"><i class="' + (done >= total ? "g" : "") + '" style="width:' + Math.round(done / total * 100) + '%"></i></div></div>';
    h += '<div class="path">';
    m.steps.forEach((s, i) => {
      h += '<div class="step"><div class="dot ' + (s.completed ? "done" : "now") + '">'
        + (s.completed ? ico("check", 18) : (s.type === "choice" ? "?" : String(i + 1))) + '</div>'
        + '<div class="body"><div class="st-t">' + esc(stepTitle(s, i)) + '</div>'
        + '<div class="xs mut">' + (s.type === "choice" ? "Вопрос с вариантами ответа" : "Теория") + (s.completed ? " · пройдено" : "") + '</div></div>'
        + '<button class="btn ' + (s.completed ? "" : "pri") + '" data-act="openStep" data-i="' + i + '">'
        + (s.completed ? "Повторить" : "Открыть") + '</button></div>';
    });
    h += '</div>';
    const ready = m.status === "assessment_ready" || m.status === "needs_retry" || m.status === "passed";
    let note;
    if (m.status === "passed") note = "Модуль пройден. Проверочный кейс можно пройти ещё раз.";
    else if (m.status === "needs_retry") note = "Прошлая попытка не зачтена — нужна ещё одна.";
    else if (ready) note = "Теория пройдена — можно браться за проверочный кейс. Подсказки в нём отключены.";
    else note = "Откроется, когда будут пройдены все шаги модуля.";
    h += '<div class="card" style="margin-top:18px"><div class="topline"></div>'
      + '<div style="display:flex;gap:12px;align-items:center">'
      + '<div class="grow"><b>Проверочный кейс</b><div class="xs mut" style="margin-top:4px">' + esc(note) + '</div></div>'
      + '<button class="btn ' + (ready ? "pri" : "") + '" data-act="startAssessment" data-id="' + esc(m.id) + '"'
      + (ready && !data.busy ? '' : ' disabled') + '>' + (data.busy ? "Создаём…" : "Начать") + '</button></div></div>';
    return h + '</div></div>';
  }

  function viewStep() {
    const m = data.module;
    if (!m) { prefs.view = "learn"; return viewModules(); }
    const i = state.stepIndex || 0;
    const s = m.steps[i];
    if (!s) { prefs.view = "module"; return viewModule(); }
    const ex = (data.explanation && data.explanation.stepId === s.id) ? data.explanation : null;
    const parts = stepParts(s);
    const pct = Math.round((i + 1) / m.steps.length * 100);
    let h = '<div class="page"><div class="inner" style="max-width:720px">';
    h += '<div style="display:flex;align-items:center;gap:12px;margin-bottom:16px">' + menuBtn()
      + '<div class="grow"><div class="bar"><i style="width:' + pct + '%"></i></div></div>'
      + '<button class="btn ghost" data-act="go" data-v="module">Выйти из модуля</button></div>';
    h += '<div class="card"><div class="topline"></div>'
      + '<div class="xs mut">' + esc(m.title) + ' · шаг ' + (i + 1) + ' из ' + m.steps.length + ' · ' + (s.type === "choice" ? "вопрос" : "теория") + '</div>'
      + '<h1 style="font-size:24px;margin:6px 0 ' + (parts.body.length ? 16 : 6) + 'px">' + esc(parts.title || ("Шаг " + (i + 1))) + '</h1>'
      + parts.body.map(p => '<p class="theory-p">' + esc(p) + '</p>').join('');
    if (s.type === "choice" && !ex && (s.options || []).length) {
      h += '<div style="display:flex;flex-direction:column;gap:8px;margin-top:16px">'
        + s.options.map(o => '<button class="btn" data-act="answerStep" data-i="' + i + '" data-oid="' + esc(o.id) + '"'
          + (data.busy ? ' disabled' : '') + '>' + esc(o.text) + '</button>').join('')
        + '</div>';
      if (s.completed) h += '<div class="xs mut" style="margin-top:10px">Шаг уже пройден — можно ответить ещё раз.</div>';
    } else {
      if (ex) {
        const head = s.type === "choice" ? (ex.correct ? "Верно" : "Пока неверно") : "Разбор";
        const body = ex.text || (s.type === "choice"
          ? (ex.correct ? "Вариант засчитан." : "Этот вариант не засчитан — вернитесь к теории и попробуйте ещё раз.")
          : "Шаг отмечен как пройденный.");
        h += '<div class="hint" style="margin-top:12px"><b>' + esc(head) + '</b><div style="margin-top:6px">' + esc(body) + '</div></div>';
      }
      h += '<div class="cta-row" style="margin-top:18px;justify-content:flex-end">'
        + '<button class="btn pri lg" data-act="nextStep" data-i="' + i + '"' + (data.busy ? ' disabled' : '') + '>'
        + (data.busy ? "Отправляем…" : "Продолжить") + ' ' + ico("right", 16) + '</button></div>';
    }
    h += '</div>';
    return h + '</div></div>';
  }

  function submitStep(step, answer) {
    if (!step || !step.id || data.busy) return Promise.resolve(null);
    const cid = clientId("step:" + prefs.cid, step.id, answer);
    data.busy = true; render();
    return ArenaApi.submitAttempt(conf({ stepId: step.id, answer: String(answer), clientAttemptId: cid })).then(r => {
      data.busy = false;
      if (!r.ok) { toast("Шаг: " + errText(r.error)); render(); return null; }
      const d = r.data || {};
      if (d.explanation != null || typeof d.correct === "boolean") {
        data.explanation = { stepId: step.id, text: d.explanation || "", correct: !!d.correct };
      }
      const m = data.module;
      if (m) {
        if (typeof d.completed_steps === "number") m.completedSteps = d.completed_steps;
        if (typeof d.total_steps === "number") m.totalSteps = d.total_steps;
        if (d.module_status) m.status = d.module_status;
        const local = m.steps.find(x => x.id === step.id);
        if (local && d.completed) local.completed = true;
      }
      data.modules = null;
      render();
      return d;
    });
  }
  function advanceStep(i) {
    const m = data.module;
    data.explanation = null;
    if (m && i + 1 < m.steps.length) { state.stepIndex = i + 1; prefs.view = "step"; }
    else { prefs.view = "module"; toast("Шаги модуля пройдены"); }
    savePrefs(); render();
  }

  /* --- Кейсы (GET /cases, GET /cases/{id}) --- */
  function loadCases(force) {
    if (!hasCompany() || data.casesLoading) return;
    if (data.cases && !force) return;
    if (data.casesErr && !force) return;
    data.casesLoading = true; data.casesErr = null; render();
    ArenaApi.listCases(conf()).then(r => {
      data.casesLoading = false;
      if (r.ok) { data.cases = ArenaApi.mapCases(r.data); data.casesErr = null; }
      else { data.cases = null; data.casesErr = r.error; toast("Кейсы: " + errText(r.error)); }
      render();
    });
  }
  function diffDots(d) {
    const n = d === "easy" ? 1 : d === "hard" ? 3 : 2;
    let h = '<span class="dots" aria-hidden="true">';
    for (let i = 1; i <= 3; i++) h += '<i class="' + (i <= n ? "on" : "") + '"></i>';
    return h + '</span><span class="xs mut">' + esc(ArenaApi.difficultyLabel(d)) + '</span>';
  }
  function viewCases() {
    if (!data.cases && !data.casesLoading && !data.casesErr) setTimeout(() => loadCases(), 0);
    let h = '<div class="page"><div class="inner">';
    h += ph("Кейсы", "Каталог опубликованных кейсов компании. Диалог ведёт ИИ-оппонент, разбор считает сервер по рубрике.",
      '<button class="btn" data-act="reloadCases">' + ico("rotate", 15) + ' Обновить</button>');
    if (!data.cases) {
      h += card(data.casesLoading ? "Загружаем кейсы…" : ("Не удалось получить кейсы: " + errText(data.casesErr)));
      if (!data.casesLoading) h += retryRow("reloadCases");
      return h + '</div></div>';
    }
    if (!data.cases.length) {
      h += '<div class="empty"><div class="ic">' + ico("grid", 26) + '</div><b>Каталог кейсов пуст</b>'
        + '<div class="sm">Опубликованных кейсов пока нет — загляните позже.</div></div>';
      return h + '</div></div>';
    }
    h += '<div class="grid">';
    data.cases.forEach(c => {
      const assessment = c.mode === "assessment";
      const disabled = c.locked || assessment;
      h += '<button class="sc' + (c.locked ? " locked" : "") + '" data-act="openCase" data-key="' + esc(c.key) + '"' + (disabled ? ' disabled' : '') + ' style="min-height:212px">'
        + '<div class="who"><div class="av o">' + ico(c.locked ? "lock" : "bot", 18) + '</div>'
        + '<div class="grow"><h3>' + esc(c.title) + '</h3>'
        + '<div class="xs mut">' + esc(ArenaApi.caseModeLabel(c.mode)) + ' · до ' + c.maxTurns + ' ходов</div></div></div>'
        + '<div class="q sm">' + esc(c.briefing.length > 96 ? c.briefing.slice(0, 96) + "…" : c.briefing) + '</div>'
        + '<div class="ft">' + diffDots(c.difficulty)
        + '<span class="ft-i">' + ico("target", 13) + '<span>' + esc((c.skillIds || []).join(", ")) + '</span></span>'
        + '<span class="ft-i">' + ico("spark", 13) + '<span>' + (c.hintsAvailable ? "подсказки есть" : "без подсказок") + '</span></span>'
        + (c.locked ? '' : '<span class="badge">' + (assessment ? "Из модуля" : "Начать") + '</span>') + '</div>'
        + (c.locked
          ? '<div class="xs mut">' + (c.unlockedBy ? "Откроется после модуля: " + esc(c.unlockedBy) : "Кейс закрыт до прохождения теории") + '</div>'
          : (assessment ? '<div class="xs mut">Проверочный кейс запускается со страницы модуля</div>' : ''))
        + '</button>';
    });
    return h + '</div></div></div>';
  }

  function loadCaseDetail(key, force) {
    if (!key || !hasCompany() || data.caseDetailLoading) return;
    if (data.caseDetailKey === key && (data.caseDetail || data.caseDetailErr) && !force) return;
    data.caseDetailLoading = true; data.caseDetailKey = key; data.caseDetailErr = null;
    render();
    ArenaApi.getCase(conf({ caseKey: key })).then(r => {
      data.caseDetailLoading = false;
      data.caseDetail = r.ok ? ArenaApi.mapCase(r.data) : null;
      data.caseDetailErr = r.ok ? null : r.error;
      if (!r.ok) toast("Кейс: " + errText(r.error));
      render();
    });
  }
  function viewCase() {
    const key = data.caseDetailKey;
    if (!key) { prefs.view = "cases"; return viewCases(); }
    const c = data.caseDetail;
    let h = '<div class="page"><div class="inner" style="max-width:760px">';
    h += ph("Карточка кейса", "Задание игрока. Закрытые данные сценария оппонент держит при себе.",
      '<button class="btn" data-act="go" data-v="cases">' + ico("left", 16) + ' К кейсам</button>');
    if (!c) {
      h += card(data.caseDetailLoading ? "Загружаем кейс…" : ("Не удалось получить кейс: " + errText(data.caseDetailErr)));
      if (!data.caseDetailLoading) h += retryRow("reloadCase");
      return h + '</div></div>';
    }
    h += '<div class="card"><div class="topline"></div>'
      + '<div class="kv"><span class="mut">Кейс</span><b>' + esc(c.title) + '</b></div>'
      + '<div class="kv"><span class="mut">Режим</span><b>' + esc(ArenaApi.caseModeLabel(c.mode)) + '</b></div>'
      + '<div class="kv"><span class="mut">Сложность</span><b>' + esc(ArenaApi.difficultyLabel(c.difficulty)) + '</b></div>'
      + '<div class="kv"><span class="mut">Лимит ходов</span><b>' + c.maxTurns + '</b></div>'
      + '<div class="kv"><span class="mut">Навыки</span><b>' + esc((c.skillIds || []).join(", ") || "—") + '</b></div>'
      + '<div class="kv"><span class="mut">Подсказки</span><b>' + (c.hintsAvailable ? "до 3 за сессию" : "отключены") + '</b></div>'
      + '<div class="hint" style="margin-top:14px"><b>Задание</b><div style="margin-top:6px">' + esc(c.briefing || "—") + '</div></div>'
      + '<div class="cta-row" style="margin-top:16px;justify-content:space-between">'
      + '<button class="btn ghost" data-act="go" data-v="cases">' + ico("left", 16) + ' К кейсам</button>'
      + '<button class="btn pri lg" data-act="startSession" data-key="' + esc(c.key) + '"' + (data.busy || c.locked ? ' disabled' : '') + '>'
      + (data.busy ? "Создаём сессию…" : "Войти в переговорную") + ' ' + ico("play", 16) + '</button></div></div>';
    h += '<div class="xs mut" style="margin-top:12px">Сессию создаёт сервер, первый ход делает ИИ-оппонент.</div>';
    return h + '</div></div>';
  }

  /* --- Сессия переговоров --- */
  function applySession(s) {
    data.session = s;
    data.result = null; data.resultTried = false; data.hint = null;
    state.draftText = ""; state.typing = false;
  }
  function loadSession(id, force) {
    if (!hasCompany() || !id || data.sessionLoading) return;
    if (data.session && data.session.sessionId === id && !force) return;
    data.sessionLoading = true; data.sessionErr = null; render();
    ArenaApi.getSession(conf({ sessionId: id })).then(r => {
      data.sessionLoading = false;
      if (!r.ok) { data.sessionErr = r.error; toast("Сессия: " + errText(r.error)); render(); return; }
      const s = ArenaApi.mapSession(r.data);
      if (!data.session || data.session.sessionId !== s.sessionId) applySession(s);
      else data.session = s;
      prefs.view = s.status === "active" ? "chat" : "result";
      savePrefs(); render();
    });
  }
  function renderMsg(m) {
    if (m.role === "user") {
      return '<div class="row u"><div class="line"><div class="bub">' + esc(m.text) + '</div></div>'
        + '<div class="meta"><span>Вы</span>' + (m.turnNumber ? '<span class="tag i">ход ' + m.turnNumber + '</span>' : '') + '</div></div>';
    }
    return '<div class="row o"><div class="meta"><span>ИИ-оппонент</span></div>'
      + '<div class="line"><div class="bub">' + esc(m.text) + '</div></div></div>';
  }
  function viewChat() {
    const s = data.session;
    if (!s) {
      let h = '<div class="page"><div class="inner">' + ph("Переговоры");
      h += '<div class="empty"><div class="ic">' + ico("bot", 26) + '</div><b>Сессия не выбрана</b>'
        + '<div class="sm">Выберите кейс, чтобы начать диалог.</div>'
        + '<button class="btn pri" data-act="go" data-v="cases">К кейсам</button></div>';
      return h + '</div></div>';
    }
    const active = s.status === "active";
    const hintsLeft = typeof s.hintsLeft === "number" ? s.hintsLeft : 0;
    let h = '<div class="chat">';
    h += '<div class="chat-head">' + menuBtn()
      + '<div class="av o">' + ico("bot", 18) + '</div>'
      + '<div class="ch-t"><div class="tt">' + esc(s.title || "Кейс") + '</div>'
      + '<div class="xs mut">ИИ-оппонент · ' + esc(ArenaApi.caseModeLabel(s.mode)) + ' · ход ' + (s.turnsUsed || 0) + '/' + (s.maxTurns || "?") + ' · попытка ' + (s.attempt || 1) + '</div></div>'
      + '<div class="pill ' + (active ? "mid" : "ok") + '">' + esc(ArenaApi.sessionStatusLabel(s.status)) + '</div>'
      + '<button class="btn" data-act="askFinish"' + (active ? '' : ' disabled') + '>' + ico("flag", 16) + ' Завершить</button></div>';

    h += '<div class="chat-body">';
    h += '<div class="chat-col"><div class="msgs" id="msgs" role="log" aria-live="polite"><div class="msgs-in" id="msgsIn">';
    (s.messages || []).forEach(m => { h += renderMsg(m); });
    if (state.typing || data.sending) h += '<div class="row o"><div class="line"><div class="bub typing"><i></i><i></i><i></i></div></div></div>';
    h += '</div></div>';

    h += '<div class="composer"><div class="composer-in">';
    h += '<div class="box"><label class="sr-only" for="compose">Сообщение оппоненту</label>'
      + '<textarea id="compose" rows="1" placeholder="' + (active ? "Написать сообщение ИИ-оппоненту…" : "Сессия завершена") + '"'
      + (active ? '' : ' disabled') + ' data-oninput="compose">' + esc(state.draftText || "") + '</textarea>'
      + (speechSupported()
        ? '<button class="mic' + (state.listening ? " on" : "") + '" data-act="mic"'
        + ' aria-label="' + (state.listening ? "Остановить диктовку" : "Надиктовать сообщение") + '"'
        + ' title="' + (state.listening ? "Остановить диктовку" : "Надиктовать сообщение") + '"'
        + (active ? '' : ' disabled') + '>' + ico("mic", 20) + '</button>'
        : '')
      + '<button class="send" data-act="send" aria-label="Отправить сообщение"'
      + ((active && (state.draftText || "").trim() && !data.sending) ? "" : " disabled") + '>' + ico("send", 20) + '</button></div>';
    h += '<div class="xs mut" style="text-align:center;margin-top:6px">Enter — отправить, Shift+Enter — новая строка.'
      + (speechSupported() ? ' Микрофон надиктует реплику в поле — текст можно поправить перед отправкой.' : '') + '</div>';
    h += '</div></div></div>';

    h += '<div class="panel">'
      + '<div class="card" style="margin-bottom:12px"><h3>Кейс</h3>'
      + '<div class="kv"><span class="mut">Режим</span><b style="text-align:right">' + esc(ArenaApi.caseModeLabel(s.mode)) + '</b></div>'
      + '<div class="kv"><span class="mut">Ходы</span><b style="text-align:right">' + (s.turnsUsed || 0) + ' / ' + (s.maxTurns || "?") + '</b></div>'
      + '<div class="kv"><span class="mut">Подсказки</span><b style="text-align:right">' + hintsLeft + '</b></div>'
      + (s.briefing ? '<div class="sm mut" style="margin-top:8px">' + esc(s.briefing) + '</div>' : '') + '</div>'
      + '<div class="card" style="margin-bottom:12px"><h3>Подсказка тренера</h3>'
      + (data.hint ? '<div class="sm">' + esc(data.hint) + '</div>'
        : '<div class="xs mut">Одна подсказка о недостающем действии. Не более трёх за сессию, в историю диалога не попадает.</div>')
      + '<div class="cta-row" style="margin-top:10px;flex-wrap:wrap">'
      + '<button class="btn" data-act="hint"' + ((data.hintLoading || !active || hintsLeft <= 0) ? ' disabled' : '') + '>' + ico("spark", 15) + ' Подсказка</button>'
      + '<button class="btn" data-act="refreshSession">' + ico("rotate", 15) + ' Обновить</button></div></div>'
      + '<button class="btn wide" data-act="askFinish"' + (active ? '' : ' disabled') + '>' + ico("flag", 16) + ' Завершить и получить разбор</button>'
      + '</div>';
    return h + '</div></div>';
  }

  function sendMessage() {
    const s = data.session;
    if (!s || s.status !== "active" || data.sending) return;
    const ta = $("#compose"); if (!ta) return;
    const text = ta.value.trim(); if (!text) return;
    stopSpeech();
    const cid = clientId("msg:" + prefs.cid, s.sessionId, text);
    s.messages = (s.messages || []).concat([{ role: "user", turnNumber: (s.turnsUsed || 0) + 1, text: text, createdAt: "" }]);
    state.draftText = ""; state.typing = true;
    data.sending = true;
    render(); scrollMsgs();
    ArenaApi.sendMessage(conf({ sessionId: s.sessionId, clientMessageId: cid, text: text })).then(r => {
      data.sending = false; state.typing = false;
      if (!r.ok) {
        toast("Ход: " + errText(r.error));
        render(); return;
      }
      const t = ArenaApi.mapTurn(r.data);
      if (t.opponentMessage && t.opponentMessage.text) s.messages = s.messages.concat([t.opponentMessage]);
      if (t.turnsUsed) s.turnsUsed = t.turnsUsed;
      if (t.maxTurns) s.maxTurns = t.maxTurns;
      if (t.status) s.status = t.status;
      if (s.status !== "active") {
        prefs.view = "result"; savePrefs();
        loadResult(true);
      }
      render(); scrollMsgs();
    });
  }
  function refreshSession() {
    const s = data.session; if (!s) return;
    ArenaApi.getSession(conf({ sessionId: s.sessionId })).then(r => {
      if (!r.ok) { toast("Сессия: " + errText(r.error)); return; }
      const fresh = ArenaApi.mapSession(r.data);
      data.session = fresh;
      if (fresh.status !== "active") { prefs.view = "result"; savePrefs(); data.resultTried = false; loadResult(true); }
      render();
    });
  }
  function requestHint() {
    const s = data.session; if (!s || data.hintLoading) return;
    data.hintLoading = true; render();
    ArenaApi.requestHint(conf({ sessionId: s.sessionId })).then(r => {
      data.hintLoading = false;
      if (!r.ok) { toast("Подсказка: " + errText(r.error)); render(); return; }
      const hnt = ArenaApi.mapHint(r.data);
      data.hint = hnt.hint;
      s.hintsLeft = hnt.hintsLeft;
      render();
    });
  }
  function askFinish() {
    const s = data.session; if (!s || s.status !== "active") return;
    modal('<h3>Завершить диалог?</h3><div class="mut">Сервер посчитает разбор по рубрике. Продолжить эту попытку будет нельзя — но можно начать новую.</div>'
      + '<div style="display:flex;gap:8px;justify-content:flex-end;margin-top:14px">'
      + '<button class="btn" data-act="closeModal">Продолжить диалог</button>'
      + '<button class="btn pri" data-act="doFinish">Завершить и получить разбор</button></div>');
  }
  function doFinish() {
    const s = data.session; if (!s) return;
    closeModal();
    data.busy = true; render();
    ArenaApi.finishSession(conf({ sessionId: s.sessionId })).then(r => {
      data.busy = false;
      if (!r.ok) { toast("Завершение: " + errText(r.error)); render(); return; }
      data.result = ArenaApi.mapResult(r.data);
      data.resultTried = true;
      if (data.result.status) s.status = data.result.status;
      data.history = null;
      prefs.view = "result"; savePrefs(); render();
      loadHistory(true);
    });
  }
  function retrySession() {
    const s = data.session; if (!s || data.busy) return;
    data.busy = true; render();
    ArenaApi.retrySession(conf({ sessionId: s.sessionId })).then(r => {
      data.busy = false;
      if (!r.ok) { toast("Повтор: " + errText(r.error)); render(); return; }
      applySession(ArenaApi.mapSession(r.data));
      data.history = null;
      prefs.view = "chat"; savePrefs(); render();
      loadHistory(true);
      toast("Новая попытка № " + (data.session.attempt || 1));
    });
  }
  function loadResult(force) {
    const s = data.session;
    if (!s || data.resultLoading) return;
    if (data.result && !force) return;
    if (data.resultTried && !force) return;
    data.resultLoading = true; render();
    ArenaApi.getFeedback(conf({ sessionId: s.sessionId })).then(r => {
      data.resultLoading = false;
      data.resultTried = true;
      if (r.ok) data.result = ArenaApi.mapResult(r.data);
      else toast("Разбор: " + errText(r.error));
      render();
    });
  }
  function viewResult() {
    const s = data.session;
    if (!s) { prefs.view = "cases"; return viewCases(); }
    if (!data.result && !data.resultLoading && !data.resultTried) setTimeout(() => loadResult(), 0);
    const r = data.result;
    let h = '<div class="page"><div class="inner" style="max-width:820px">';
    h += ph("Результат сессии", (s.title || "Кейс") + " · попытка " + (s.attempt || 1) + " · " + esc(ArenaApi.caseModeLabel(s.mode)));
    if (!r) {
      h += card(data.resultLoading ? "Получаем разбор…" : "Разбор ещё не получен.");
      h += '<div class="cta-row" style="justify-content:flex-start;margin-top:12px">'
        + '<button class="btn" data-act="reloadResult">' + ico("rotate", 15) + ' Обновить</button>'
        + '<button class="btn ghost" data-act="go" data-v="cases">К кейсам</button></div>';
      return h + '</div></div>';
    }
    const ev = r.evaluationStatus;
    const badge = ev === "evaluated"
      ? (r.passed ? '<span class="badge g">Рубрика пройдена</span>' : '<span class="badge w">Рубрика не пройдена</span>')
      : (ev === "needs_retry" ? '<span class="badge b">Разбор не получен — нужна повторная попытка</span>' : '<span class="badge">Разбор в процессе</span>');
    const met = (r.criteria || []).filter(c => c.met).length, total = (r.criteria || []).length;
    h += '<div class="card"><div class="topline"></div>'
      + '<div class="verdict">' + badge + '<span class="big">Критериев выполнено: ' + met + ' из ' + total + '</span></div>'
      + (r.moduleStatus ? '<p class="sm mut" style="margin-top:8px">Статус модуля: ' + esc(ArenaApi.statusToLabel(r.moduleStatus)) + '</p>' : '')
      + '<div class="kv" style="margin-top:12px"><span class="mut">Ходов использовано</span><b>' + (r.turnsUsed || 0) + ' из ' + (r.maxTurns || "?") + '</b></div>'
      + '<p class="sm" style="margin-top:10px"><b>Что сработало.</b> ' + esc(r.feedback.whatWorked || "—") + '</p>'
      + '<p class="sm" style="margin-top:6px"><b>Что попробовать дальше.</b> ' + esc(r.feedback.whatToTryNext || "—") + '</p>'
      + '<div class="cta-row" style="margin-top:16px">'
      + '<button class="btn pri lg" data-act="go" data-v="feedback">' + ico("scan", 18) + ' Разбор по критериям</button>'
      + (r.canRetry ? '<button class="btn" data-act="retry"' + (data.busy ? ' disabled' : '') + '>' + ico("rotate", 16) + ' Пройти заново</button>' : '')
      + '<button class="linkbtn" data-act="go" data-v="cases">К кейсам</button></div></div>';
    return h + '</div></div>';
  }
  function viewFeedback() {
    const s = data.session;
    if (!s) { prefs.view = "cases"; return viewCases(); }
    if (!data.result && !data.resultLoading && !data.resultTried) setTimeout(() => loadResult(), 0);
    const r = data.result;
    let h = '<div class="page"><div class="inner" style="max-width:820px">';
    h += ph("Разбор сессии", "Критерии рубрики считает сервер по стенограмме диалога.");
    if (!r) {
      h += card(data.resultLoading ? "Получаем разбор…" : "Разбор ещё не получен.");
      h += '<div class="cta-row" style="justify-content:flex-start;margin-top:12px">'
        + '<button class="btn" data-act="reloadResult">' + ico("rotate", 15) + ' Обновить</button>'
        + '<button class="btn ghost" data-act="go" data-v="cases">К кейсам</button></div>';
      return h + '</div></div>';
    }
    h += '<div class="card"><div class="xray-head">' + ico("scan", 20) + '<h3 style="margin:0">Критерии рубрики (в. ' + esc(String(r.rubricVersion || 1)) + ')</h3></div>';
    if (!(r.criteria || []).length) {
      h += '<div class="block"><div><div class="bt">Критерии не переданы</div>'
        + '<div class="bd">' + esc(ArenaApi.EVAL_STATUS_LABEL[r.evaluationStatus] || "Разбор ещё не готов.") + '</div></div></div>';
    }
    (r.criteria || []).forEach(c => {
      h += '<div class="block ' + (c.met ? "good" : "") + '"><div class="bi">' + ico(c.met ? "check" : "alert", 16) + '</div>'
        + '<div><div class="bt">' + esc(c.behavior || c.id) + (c.required ? ' <span class="xs mut">· обязательный</span>' : '') + '</div>'
        + '<div class="bd">' + (c.met ? "Выполнен" : "Не выполнен")
        + (c.evidence ? ' · цитата: «' + esc(c.evidence) + '»' : '')
        + (c.comment ? ' · ' + esc(c.comment) : '') + '</div></div></div>';
    });
    h += '<div class="cta-row" style="margin-top:18px">'
      + '<button class="btn" data-act="go" data-v="result">К результату</button>'
      + (r.canRetry ? '<button class="btn pri" data-act="retry"' + (data.busy ? ' disabled' : '') + '>' + ico("rotate", 16) + ' Пройти заново</button>' : '')
      + '<button class="btn ghost" data-act="go" data-v="cases">К кейсам</button></div></div>';
    return h + '</div></div>';
  }

  /* --- Прогресс: GET /me/progress + GET /me/sessions --- */
  function loadProgress(force) {
    if (!hasCompany() || data.progressLoading) return;
    if (data.progress && !force) return;
    if (data.progressErr && !force) return;
    data.progressLoading = true; data.progressErr = null; render();
    ArenaApi.myProgress(conf()).then(r => {
      data.progressLoading = false;
      if (r.ok) { data.progress = ArenaApi.mapModulesToTracks(r.data); data.progressErr = null; }
      else { data.progress = null; data.progressErr = r.error; }
      render();
    });
  }
  function loadHistory(force) {
    if (!hasCompany() || data.historyLoading) return;
    if (data.history && !force) return;
    if (data.historyErr && !force) return;
    data.historyLoading = true; data.historyErr = null;
    ArenaApi.mySessions(conf()).then(r => {
      data.historyLoading = false;
      if (r.ok) { data.history = ArenaApi.mapSessionHistory(r.data); data.historyErr = null; }
      else { data.history = null; data.historyErr = r.error; }
      render();
    });
  }
  function viewProgress() {
    if (!data.progress && !data.progressLoading && !data.progressErr) setTimeout(() => loadProgress(), 0);
    if (!data.history && !data.historyLoading && !data.historyErr) setTimeout(() => loadHistory(), 0);
    let h = '<div class="page"><div class="inner" style="max-width:820px">';
    h += ph("Прогресс", "Статусы модулей и история сессий в этой компании.",
      '<button class="btn" data-act="reloadProgress">' + ico("rotate", 15) + ' Обновить</button>');
    h += '<div class="card"><b>Модули</b>';
    if (data.progressLoading && !data.progress) h += '<div class="sm mut" style="margin-top:10px">Загрузка…</div>';
    else if (!data.progress) h += '<div class="sm mut" style="margin-top:10px">Прогресс недоступен: ' + esc(errText(data.progressErr)) + '</div>';
    else if (!data.progress.length) h += '<div class="sm mut" style="margin-top:10px">Модулей пока нет.</div>';
    else {
      const passed = data.progress.filter(m => m.status === "passed").length;
      h += '<div class="xs mut" style="margin:8px 0 12px">' + passed + ' из ' + data.progress.length + ' модулей пройдено</div>';
      data.progress.forEach(m => {
        const total = m.totalSteps || 1, done = m.completedSteps || 0;
        h += '<div style="margin-bottom:12px"><div style="display:flex;justify-content:space-between;gap:12px">'
          + '<span class="sm">' + esc(m.title) + '</span>'
          + '<span class="xs mut">' + done + '/' + total + ' · ' + esc(ArenaApi.statusToLabel(m.status)) + '</span></div>'
          + '<div class="bar"><i class="' + (m.status === "passed" ? "g" : "") + '" style="width:' + Math.round(done / total * 100) + '%"></i></div></div>';
      });
    }
    h += '</div>';
    h += '<div class="card" style="margin-top:16px"><b>История сессий</b>';
    if (data.historyLoading && !data.history) h += '<div class="sm mut" style="margin-top:10px">Загрузка…</div>';
    else if (!data.history) h += '<div class="sm mut" style="margin-top:10px">История недоступна: ' + esc(errText(data.historyErr)) + '</div>';
    else if (!data.history.length) h += '<div class="sm mut" style="margin-top:10px">Сессий пока нет.</div>';
    else {
      h += '<div class="tablewrap" style="margin-top:10px"><table><thead><tr><th>Кейс</th><th>Режим</th><th>Статус</th><th>Начата</th><th class="n"></th></tr></thead><tbody>';
      data.history.forEach(s => {
        h += '<tr><td><b>' + esc(s.title || "—") + '</b></td>'
          + '<td>' + esc(ArenaApi.caseModeLabel(s.mode)) + '</td>'
          + '<td><span class="badge' + (s.status === "evaluated" ? " g" : "") + '">' + esc(ArenaApi.sessionStatusLabel(s.status)) + '</span></td>'
          + '<td class="xs mut">' + esc(fmtDateTime(s.createdAt)) + '</td>'
          + '<td class="n"><button class="btn" data-act="openSession" data-id="' + esc(s.id) + '">Открыть</button></td></tr>';
      });
      h += '</tbody></table></div>';
    }
    h += '</div>';
    return h + '</div></div>';
  }

  /* --- Сотрудники: GET /members, GET /team/progress, POST /invitations, PATCH /members/{uid} --- */
  function loadTeam(force) {
    if (!isCompanyAdmin() || data.teamLoading) return;
    if (data.members && !force) return;
    if (data.membersErr && !force) return;
    data.teamLoading = true; data.membersErr = null; data.teamErr = null; render();
    Promise.all([ArenaApi.listMembers(conf()), ArenaApi.teamProgress(conf())]).then(res => {
      data.teamLoading = false;
      const m = res[0], t = res[1];
      data.members = m.ok ? ArenaApi.mapMembers(m.data) : null;
      data.membersErr = m.ok ? null : m.error;
      data.team = t.ok ? ArenaApi.mapTeamProgress(t.data) : null;
      data.teamErr = t.ok ? null : t.error;
      render();
    });
  }
  function viewTeam() {
    if (!data.members && !data.teamLoading && !data.membersErr) setTimeout(() => loadTeam(), 0);
    let h = '<div class="page"><div class="inner">';
    h += ph("Сотрудники", "Участники компании и их прогресс по модулям.",
      '<button class="btn" data-act="reloadTeam">' + ico("rotate", 15) + ' Обновить</button>'
      + '<button class="btn pri" data-act="askInvite">' + ico("plus", 15) + ' Пригласить</button>');
    if (!data.members) {
      h += card(data.teamLoading ? "Загружаем участников…" : ("Не удалось получить участников: " + errText(data.membersErr)));
      if (!data.teamLoading) h += retryRow("reloadTeam");
      return h + '</div></div>';
    }
    const byUser = {};
    (data.team || []).forEach(p => { byUser[p.userId] = p; });
    h += '<div class="card"><div class="tablewrap"><table><thead><tr>'
      + '<th>Сотрудник</th><th>Роль</th><th>Статус</th><th class="n">Модулей</th><th>Прогресс</th><th class="n"></th>'
      + '</tr></thead><tbody>';
    data.members.forEach(m => {
      const tp = byUser[m.userId];
      let prog = "—";
      if (tp && tp.modules.length) {
        const done = tp.modules.filter(x => x.status === "passed").length;
        const inprog = tp.modules.filter(x => x.status === "in_progress").length;
        prog = done + " из " + tp.modules.length + " пройдено" + (inprog ? " · " + inprog + " в процессе" : "");
      } else if (data.teamErr) prog = "нет доступа к прогрессу";
      h += '<tr><td><b>' + esc(m.name || "(без имени)") + '</b><div class="xs mut">' + esc(m.email || "") + '</div></td>'
        + '<td>' + esc(ArenaApi.roleLabel(m.role)) + '</td>'
        + '<td><span class="badge ' + (m.status === "active" ? "g" : "") + '">' + esc(m.status === "active" ? "активен" : "не активен") + '</span></td>'
        + '<td class="n">' + (tp ? tp.modules.length : "—") + '</td>'
        + '<td>' + esc(prog) + '</td>'
        + '<td class="n"><button class="btn" data-act="editMember" data-id="' + esc(m.userId) + '">Изменить</button></td></tr>';
    });
    h += '</tbody></table></div></div>';
    if (data.teamErr) h += '<div class="hint" style="margin-top:14px">Прогресс команды недоступен: ' + esc(errText(data.teamErr)) + '</div>';
    return h + '</div></div>';
  }
  function askInvite() {
    modal('<h3>Пригласить сотрудника</h3>'
      + '<div class="mut sm">Приглашение одноразовое и живёт 7 дней. Токен показывается один раз.</div>'
      + '<form data-act="createInvite" style="margin-top:12px">'
      + '<div class="field"><label for="inv-email">Email</label><input class="input" id="inv-email" name="email" type="email" placeholder="name@company.ru" required></div>'
      + '<div class="field"><label for="inv-role">Роль</label><select class="select" id="inv-role" name="role">'
      + '<option value="employee">Сотрудник</option><option value="company_admin">Админ компании</option></select></div>'
      + '<div class="cta-row" style="justify-content:flex-end;margin-top:10px">'
      + '<button type="button" class="btn" data-act="closeModal">Отмена</button>'
      + '<button type="submit" class="btn pri">Создать приглашение</button></div></form>');
  }
  function createInvite(form) {
    const f = new FormData(form);
    const email = String(f.get("email") || "").trim();
    const role = String(f.get("role") || "employee");
    if (!email) { toast("Укажите email"); return; }
    ArenaApi.createInvitation(conf({ email: email, role: role })).then(r => {
      if (!r.ok) { toast("Приглашение: " + errText(r.error)); return; }
      const d = r.data || {};
      const origin = (location && String(location.origin).indexOf("http") === 0) ? location.origin : "";
      const link = origin + "/?invite=" + encodeURIComponent(d.token || "");
      modal('<h3>Приглашение создано</h3>'
        + '<div class="sm mut">Токен показывается один раз — отправьте сотруднику ссылку. Срок действия: до ' + esc(fmtDateTime(d.expires_at)) + '.</div>'
        + '<div class="card" style="margin-top:10px;word-break:break-all"><div class="xs mut">Ссылка для принятия</div>'
        + '<div class="sm mono">' + esc(link) + '</div></div>'
        + '<div class="cta-row" style="justify-content:flex-end;margin-top:10px">'
        + '<button class="btn" data-act="copyInvite" data-token="' + esc(link) + '">Скопировать ссылку</button>'
        + '<button class="btn pri" data-act="closeModal">Готово</button></div>');
    });
  }
  function editMember(uid) {
    const m = (data.members || []).find(x => x.userId === uid);
    if (!m) return;
    const opt = (val, label, cur) => '<option value="' + val + '"' + (cur === val ? " selected" : "") + '>' + label + '</option>';
    modal('<h3>' + esc(m.name || m.email || "Участник") + '</h3>'
      + '<div class="mut sm">Роль и статус участия в компании. Последнего активного администратора снять нельзя.</div>'
      + '<form data-act="saveMember" data-id="' + esc(uid) + '" style="margin-top:12px">'
      + '<div class="field"><label for="mb-role">Роль</label><select class="select" id="mb-role" name="role">'
      + opt("employee", "Сотрудник", m.role) + opt("company_admin", "Админ компании", m.role) + '</select></div>'
      + '<div class="field"><label for="mb-status">Статус</label><select class="select" id="mb-status" name="status">'
      + opt("active", "Активен", m.status) + opt("inactive", "Не активен", m.status) + '</select></div>'
      + '<div class="cta-row" style="justify-content:flex-end;margin-top:10px">'
      + '<button type="button" class="btn" data-act="closeModal">Отмена</button>'
      + '<button type="submit" class="btn pri">Сохранить</button></div></form>');
  }
  function saveMember(form) {
    const uid = form.getAttribute("data-id");
    const f = new FormData(form);
    ArenaApi.updateMember(conf({
      memberId: uid, role: String(f.get("role") || "employee"), status: String(f.get("status") || "active")
    })).then(r => {
      if (!r.ok) { toast("Участник: " + errText(r.error)); return; }
      closeModal();
      toast("Участник обновлён");
      loadTeam(true);
    });
  }

  /* --- Аккаунт: GET/PATCH /api/v1/me, выбор компании --- */
  function viewAccount() {
    const p = state.profile;
    const cur = currentCompany();
    let h = '<div class="page"><div class="inner" style="max-width:720px">';
    h += ph("Аккаунт", p.email || "");
    h += '<div class="card"><div class="topline"></div><form data-act="renameMe">'
      + '<div class="kv"><span class="mut">Почта</span><b>' + esc(p.email || "—") + '</b></div>'
      + '<div class="field" style="margin-top:10px"><label for="ac-name">Имя</label>'
      + '<input class="input" id="ac-name" name="name" maxlength="100" value="' + esc(p.name || "") + '" required></div>'
      + '<div class="cta-row" style="justify-content:flex-start">'
      + '<button type="submit" class="btn pri">Сохранить имя</button>'
      + '<button type="button" class="btn" data-act="reloadMe">' + ico("rotate", 15) + ' Обновить профиль</button>'
      + '<button type="button" class="btn danger" data-act="askLogout">' + ico("left", 15) + ' Выйти</button>'
      + '</div></form></div>';

    h += '<div class="card" style="margin-top:16px"><b>Компания</b>';
    if (!state.companies.length) {
      h += '<div class="sm mut" style="margin-top:8px">Активных компаний нет. Обучение и кейсы откроются после принятия приглашения.</div>'
        + '<form data-act="authInvite" style="margin-top:12px">'
        + '<div class="field"><label for="ac-invite">Код приглашения</label>'
        + '<input class="input" id="ac-invite" name="token" placeholder="Вставьте токен из ссылки"></div>'
        + '<button type="submit" class="btn pri">Принять приглашение</button></form>';
    } else {
      h += '<div class="xs mut" style="margin-top:6px">Обучение, кейсы и сессии относятся к выбранной компании.</div>'
        + '<div class="cta-row" style="justify-content:flex-start;margin-top:10px;flex-wrap:wrap">';
      state.companies.forEach(c => {
        h += '<button class="btn' + (prefs.cid === c.id ? " pri" : "") + '" data-act="pickCompany" data-id="' + esc(c.id) + '">'
          + esc(c.name || c.id) + ' · ' + esc(ArenaApi.roleLabel(c.role)) + '</button>';
      });
      h += '</div>';
      if (isCompanyAdmin()) {
        h += '<div class="cta-row" style="justify-content:flex-start;margin-top:12px">'
          + '<button class="btn pri" data-act="askInvite">' + ico("plus", 15) + ' Пригласить сотрудника</button>'
          + '<button class="btn" data-act="go" data-v="team">' + ico("users", 15) + ' Участники компании</button></div>';
      } else if (cur) {
        h += '<div class="xs mut" style="margin-top:12px">Приглашать сотрудников может администратор компании. Ваша роль: ' + esc(ArenaApi.roleLabel(cur.role)) + '.</div>';
      }
    }
    h += '</div>';
    return h + '</div></div>';
  }

  /* --- Настройки --- */
  function openSettings() {
    const th = prefs.theme || "auto";
    modal('<h3>Настройки</h3>'
      + '<div class="toggle-row" style="border-top:0"><div><div class="t">Тема</div><div class="d">Светлая, тёмная или как в системе</div></div>'
      + '<div class="seg2">'
      + '<button data-act="setTheme" data-v="light" class="' + (th === "light" ? "on" : "") + '">Светлая</button>'
      + '<button data-act="setTheme" data-v="dark" class="' + (th === "dark" ? "on" : "") + '">Тёмная</button>'
      + '<button data-act="setTheme" data-v="auto" class="' + (th === "auto" ? "on" : "") + '">Система</button></div></div>'
      + '<div class="cta-row" style="margin-top:16px;justify-content:flex-end"><button class="btn pri" data-act="closeModal">Готово</button></div>');
  }

  /* === 5. АУТЕНТИФИКАЦИЯ ================================================== */
  function applyProfile(p) {
    const prof = ArenaApi.mapProfile(p);
    state.profile = prof;
    state.companies = prof.companies || [];
    if (prefs.cid && !state.companies.some(c => c.id === prefs.cid)) prefs.cid = "";
    if (!prefs.cid && state.companies.length) prefs.cid = state.companies[0].id;
    savePrefs();
  }
  function afterLogin() {
    resetData();
    ensureView();
    render();
    if (hasCompany()) loadHistory(true);
  }
  function loadMe(silent) {
    return ArenaApi.me(conf()).then(r => {
      if (!r.ok) {
        if (!silent) toast("Профиль: " + errText(r.error));
        return false;
      }
      applyProfile(r.data);
      return true;
    });
  }
  function authSubmit(form, kind) {
    if (state.authLoading) return;
    const f = new FormData(form);
    const email = String(f.get("email") || "").trim();
    const password = String(f.get("password") || "");
    const name = String(f.get("name") || "").trim();
    if (!email || !password) { toast("Укажите email и пароль"); return; }
    if (kind === "register" && (!name || password.length < 8)) { toast("Для регистрации нужны имя и пароль от 8 символов"); return; }
    state.authLoading = true; state.lastEmail = email; render();
    const call = kind === "register"
      ? ArenaApi.register(conf({ email: email, password: password, name: name }))
      : ArenaApi.login(conf({ email: email, password: password }));
    call.then(r => {
      state.authLoading = false;
      if (!r.ok) {
        const st = r.error && r.error.status;
        if (kind === "register" && (st === 404 || st === 405)) toast("Регистрация на этом сервере не включена — попросите приглашение у администратора");
        else toast((kind === "register" ? "Регистрация: " : "Вход: ") + errText(r.error));
        render(); return;
      }
      if (r.data && r.data.accessToken) state.token = String(r.data.accessToken);
      loadMe(true).then(ok => {
        if (!ok) { toast("Вход выполнен, но профиль не загрузился"); render(); return; }
        toast(kind === "register" ? "Аккаунт создан" : "Вход выполнен");
        afterLogin();
      });
    });
  }
  function askLogout() {
    state.profileOpen = false;
    modal('<h3>Выйти из аккаунта?</h3><div class="mut">Сессия будет завершена, refresh-токен отозван. Прогресс хранится на сервере и не потеряется.</div>'
      + '<div style="display:flex;gap:8px;justify-content:flex-end;margin-top:14px">'
      + '<button class="btn" data-act="closeModal">Отмена</button>'
      + '<button class="btn danger" data-act="doLogout">Выйти</button></div>');
  }
  function doLogout() {
    closeModal();
    ArenaApi.logout(conf()).then(function () { });
    state.profile = null; state.companies = []; state.token = "";
    state.profileOpen = false; state.draftText = "";
    prefs.cid = ""; prefs.view = "learn"; savePrefs();
    resetData();
    render();
    toast("Вы вышли из аккаунта");
  }
  function acceptInvite(token, name, password) {
    const opts = conf({ inviteToken: token });
    if (name) opts.name = name;
    if (password) opts.password = password;
    return ArenaApi.acceptInvitation(opts).then(r => {
      if (!r.ok) {
        const st = r.error && r.error.status;
        toast(st === 409 ? "Приглашение уже принято или просрочено"
          : (st === 401 ? "Токен принадлежит другому аккаунту — выйдите и примите приглашение заново"
            : "Приглашение: " + errText(r.error)));
        return false;
      }
      state.inviteToken = "";
      closeModal();
      const wasLoggedIn = !!state.profile;
      applyProfile(r.data);
      if (wasLoggedIn || !password) {
        toast("Приглашение принято");
        afterLogin();
        return true;
      }
      /* Новый пользователь создан без сессии: cookie не выставлены — входим паролем. */
      const email = state.profile.email;
      state.profile = null;
      state.lastEmail = email;
      return ArenaApi.login(conf({ email: email, password: password })).then(lr => {
        if (!lr.ok) { toast("Аккаунт создан — войдите с этим паролем"); render(); return false; }
        if (lr.data && lr.data.accessToken) state.token = String(lr.data.accessToken);
        return loadMe(true).then(ok => {
          if (!ok) { render(); return false; }
          toast("Приглашение принято");
          afterLogin();
          return true;
        });
      });
    });
  }
  function askAcceptInvite(token) {
    state.inviteToken = String(token || "");
    const logged = !!state.profile;
    modal('<h3>Принять приглашение</h3>'
      + '<div class="mut sm">Приглашение одноразовое. '
      + (logged ? "Аккаунт уже опознан — имя и пароль не нужны." : "Если аккаунта ещё нет, он будет создан из имени и пароля ниже.") + '</div>'
      + '<form data-act="submitInvite" style="margin-top:12px">'
      + (logged ? '' : '<div class="field"><label for="iv-name">Имя</label><input class="input" id="iv-name" name="name" maxlength="100" placeholder="Как к вам обращаться"></div>'
        + '<div class="field"><label for="iv-password">Пароль</label><input class="input" id="iv-password" name="password" type="password" minlength="8" placeholder="Минимум 8 символов"></div>')
      + '<div class="cta-row" style="justify-content:flex-end;margin-top:10px">'
      + '<button type="button" class="btn" data-act="closeModal">Позже</button>'
      + '<button type="submit" class="btn pri">Принять приглашение</button></div></form>');
  }
  function submitInvite(form) {
    const token = state.inviteToken;
    if (!token) { toast("Вставьте токен приглашения"); return; }
    const f = new FormData(form);
    const name = String(f.get("name") || "").trim();
    const password = String(f.get("password") || "");
    if (!state.profile) {
      if (!name || password.length < 8) { toast("Укажите имя и пароль (от 8 символов)"); return; }
    }
    acceptInvite(token, name, password);
  }

  /* === 6. СОБЫТИЯ ========================================================= */
  function onCompose(el) {
    state.draftText = el.value;
    autosizeCompose(el);
    const b = $(".send");
    if (b) b.disabled = !String(el.value || "").trim() || data.sending;
  }

  /* Распознавание речи — встроенный Web Speech API браузера, без сторонних библиотек.
     Работает только в защищённом контексте (https либо localhost). */
  function speechRecognizer() {
    return window.SpeechRecognition || window.webkitSpeechRecognition || null;
  }
  function speechSupported() { return !!speechRecognizer(); }

  function setDraft(text) {
    state.draftText = text;
    const ta = $("#compose");
    if (ta) { ta.value = text; autosizeCompose(ta); }
    const b = $(".send");
    if (b) b.disabled = !text.trim() || data.sending;
  }

  function micState(on) {
    const m = $(".mic");
    if (!m) return;
    const label = on ? "Остановить диктовку" : "Надиктовать сообщение";
    m.classList.toggle("on", on);
    m.setAttribute("aria-label", label);
    m.setAttribute("title", label);
  }

  function stopSpeech() {
    const rec = state.recog;
    state.recog = null;
    state.listening = false;
    micState(false);
    if (rec) {
      try { rec.stop(); } catch (e) { }
    }
  }

  const SPEECH_ERRORS = {
    "not-allowed": "Доступ к микрофону запрещён — разрешите его в настройках браузера",
    "service-not-allowed": "Распознавание речи недоступно в этом браузере",
    "no-speech": "Речь не распознана — попробуйте ещё раз",
    "audio-capture": "Микрофон не найден",
    aborted: "",
    network: "Распознавание речи недоступно без сети"
  };

  function startSpeech() {
    const Recognizer = speechRecognizer();
    if (!Recognizer) return;
    const rec = new Recognizer();
    const base = String(state.draftText || "").replace(/\s+$/, "");
    rec.lang = "ru-RU";
    rec.continuous = true;
    rec.interimResults = true;
    rec.maxAlternatives = 1;
    rec.onresult = ev => {
      let spoken = "";
      for (let i = 0; i < ev.results.length; i++) spoken += ev.results[i][0].transcript;
      spoken = spoken.trim();
      setDraft(base ? (spoken ? base + " " + spoken : base) : spoken);
    };
    rec.onerror = ev => {
      const text = SPEECH_ERRORS[ev.error];
      if (text !== "") toast(text || ("Распознавание речи: " + ev.error));
      if (state.recog === rec) stopSpeech();
    };
    rec.onend = () => { if (state.recog === rec) stopSpeech(); };
    state.recog = rec;
    state.listening = true;
    micState(true);
    try {
      rec.start();
    } catch (e) {
      stopSpeech();
      toast("Не удалось включить микрофон");
    }
  }

  function toggleMic() {
    const s = data.session;
    if (!s || s.status !== "active") return;
    if (state.listening) stopSpeech();
    else startSpeech();
  }

  document.addEventListener("click", e => {
    const el = e.target.closest ? e.target.closest("[data-act]") : null;
    if (state.profileOpen) {
      const inside = e.target.closest(".profile-wrap");
      const a0 = el ? el.getAttribute("data-act") : "";
      if (!inside && a0 !== "toggleProfile") { state.profileOpen = false; if (state.profile) renderSidebar(); }
    }
    if (!el) return;
    const act = el.getAttribute("data-act");
    const id = el.getAttribute("data-id");
    const v = el.getAttribute("data-v");

    if (act === "toggleNav") {
      if (window.innerWidth <= 900) document.body.classList.toggle("nav-open");
      else {
        prefs.rail = !prefs.rail;
        document.body.classList.toggle("rail", !!prefs.rail);
        savePrefs();
      }
      if (state.profile) renderSidebar();
    }
    else if (act === "go") { go(v); }
    else if (act === "toggleProfile") { state.profileOpen = !state.profileOpen; renderSidebar(); }
    else if (act === "openSettings") { state.profileOpen = false; renderSidebar(); openSettings(); }
    else if (act === "closeModal") { closeModal(); }
    else if (act === "setTheme") {
      if (v === "auto") { document.documentElement.removeAttribute("data-theme"); prefs.theme = null; }
      else { document.documentElement.setAttribute("data-theme", v); prefs.theme = v; }
      savePrefs(); openSettings();
    }
    else if (act === "authMode") { prefs.authMode = v === "register" ? "register" : "login"; savePrefs(); render(); }
    else if (act === "askLogout") { askLogout(); }
    else if (act === "doLogout") { doLogout(); }
    else if (act === "reloadMe") { loadMe(false).then(() => { resetData(); render(); if (hasCompany()) loadHistory(true); }); }
    else if (act === "pickCompany") {
      if (prefs.cid === id) return;
      prefs.cid = id; savePrefs();
      resetData();
      prefs.view = "learn"; savePrefs();
      render(); loadHistory(true);
      const c = currentCompany();
      toast("Компания: " + ((c && c.name) || id));
    }
    else if (act === "reloadModules") { data.modules = null; data.modulesErr = null; loadModules(true); }
    else if (act === "openModule") {
      data.module = null; data.moduleErr = null; data.moduleId = id; data.explanation = null;
      state.stepIndex = 0;
      prefs.view = "module"; savePrefs(); render(); loadModule(id, true);
    }
    else if (act === "reloadModule") { loadModule(data.moduleId, true); }
    else if (act === "openStep") {
      state.stepIndex = parseInt(el.getAttribute("data-i"), 10) || 0;
      data.explanation = null;
      prefs.view = "step"; savePrefs(); render();
    }
    else if (act === "answerStep") {
      const m = data.module; if (!m) return;
      const i = parseInt(el.getAttribute("data-i"), 10) || 0;
      const s = m.steps[i]; const oid = el.getAttribute("data-oid");
      if (!s || !oid) return;
      submitStep(s, oid).then(d => { if (d) toast(d.correct ? "Верный вариант" : "Ответ отправлен: пока неверно"); });
    }
    else if (act === "nextStep") {
      const m = data.module; if (!m) return;
      const i = parseInt(el.getAttribute("data-i"), 10) || 0;
      const s = m.steps[i]; if (!s) return;
      const ex = (data.explanation && data.explanation.stepId === s.id) ? data.explanation : null;
      if (ex || s.completed) { advanceStep(i); return; }
      submitStep(s, "confirmed").then(d => {
        if (!d) return;
        if (!String(d.explanation || "").trim()) advanceStep(i);
      });
    }
    else if (act === "startAssessment") {
      if (data.busy) return;
      data.busy = true; render();
      ArenaApi.startAssessment(conf({ moduleId: id })).then(r => {
        data.busy = false;
        if (!r.ok) { toast("Проверочный кейс: " + errText(r.error)); render(); return; }
        applySession(ArenaApi.mapSession(r.data));
        data.history = null;
        prefs.view = "chat"; savePrefs(); render();
        loadHistory(true);
        toast("Проверочная сессия создана");
      });
    }
    else if (act === "reloadCases") { data.cases = null; data.casesErr = null; loadCases(true); }
    else if (act === "openCase") {
      const key = el.getAttribute("data-key");
      data.caseDetail = null; data.caseDetailErr = null; data.caseDetailKey = key;
      prefs.view = "case"; savePrefs(); render(); loadCaseDetail(key, true);
    }
    else if (act === "reloadCase") { loadCaseDetail(data.caseDetailKey, true); }
    else if (act === "startSession") {
      if (data.busy) return;
      const key = el.getAttribute("data-key");
      data.busy = true; render();
      ArenaApi.startSession(conf({ caseKey: key })).then(r => {
        data.busy = false;
        if (!r.ok) { toast("Сессия: " + errText(r.error)); render(); return; }
        applySession(ArenaApi.mapSession(r.data));
        data.history = null;
        prefs.view = "chat"; savePrefs(); render();
        loadHistory(true);
        toast("Сессия создана: попытка " + (data.session.attempt || 1));
      });
    }
    else if (act === "openSession") { loadSession(id, true); }
    else if (act === "refreshSession") { refreshSession(); }
    else if (act === "hint") { requestHint(); }
    else if (act === "askFinish") { askFinish(); }
    else if (act === "doFinish") { doFinish(); }
    else if (act === "retry") { retrySession(); }
    else if (act === "reloadResult") { data.resultTried = false; loadResult(true); }
    else if (act === "send") { sendMessage(); }
    else if (act === "mic") { toggleMic(); }
    else if (act === "reloadProgress") { data.progress = null; data.progressErr = null; data.history = null; data.historyErr = null; loadProgress(true); loadHistory(true); }
    else if (act === "reloadTeam") { data.members = null; data.membersErr = null; loadTeam(true); }
    else if (act === "askInvite") { askInvite(); }
    else if (act === "editMember") { editMember(id); }
    else if (act === "copyInvite") {
      const text = el.getAttribute("data-token") || "";
      try { navigator.clipboard.writeText(text).then(() => toast("Ссылка скопирована"), () => toast("Не удалось скопировать")); }
      catch (err) { toast("Копирование недоступно в этом браузере"); }
    }
  });

  document.addEventListener("submit", e => {
    const form = e.target.closest ? e.target.closest("[data-act]") : null;
    if (!form) return;
    const act = form.getAttribute("data-act");
    if (act === "authLogin") { e.preventDefault(); authSubmit(form, "login"); }
    else if (act === "authRegister") { e.preventDefault(); authSubmit(form, "register"); }
    else if (act === "authInvite") {
      e.preventDefault();
      const token = String(new FormData(form).get("token") || "").trim();
      if (!token) { toast("Вставьте токен приглашения"); return; }
      askAcceptInvite(token);
    }
    else if (act === "submitInvite") { e.preventDefault(); submitInvite(form); }
    else if (act === "renameMe") {
      e.preventDefault();
      const name = String(new FormData(form).get("name") || "").trim();
      if (!name) { toast("Имя не может быть пустым"); return; }
      ArenaApi.updateMe(conf({ name: name })).then(r => {
        if (!r.ok) { toast("Имя: " + errText(r.error)); return; }
        applyProfile(r.data);
        toast("Имя обновлено");
        render();
      });
    }
    else if (act === "createInvite") { e.preventDefault(); createInvite(form); }
    else if (act === "saveMember") { e.preventDefault(); saveMember(form); }
  });

  document.addEventListener("input", e => {
    const el = e.target;
    if (el && el.getAttribute && el.getAttribute("data-oninput") === "compose") onCompose(el);
  });

  document.addEventListener("keydown", e => {
    if (e.key === "Escape") {
      closeModal();
      if (state.profileOpen) { state.profileOpen = false; if (state.profile) renderSidebar(); }
    }
    const ta = document.getElementById("compose");
    if (ta && e.target === ta && e.key === "Enter" && !e.shiftKey) { e.preventDefault(); sendMessage(); }
  });

  $("#backdrop").addEventListener("click", () => document.body.classList.remove("nav-open"));

  /* === 7. ИНИЦИАЛИЗАЦИЯ =================================================== */
  (function init() {
    loadPrefs();
    $("#backdrop").hidden = true;
    if (prefs.rail && window.innerWidth > 900) document.body.classList.add("rail");
    render();

    let invite = "";
    try {
      const qs = new URLSearchParams((window.location && window.location.search) || "");
      invite = qs.get("invite") || "";
    } catch (e) { }

    /* Пока сессия не подтверждена запросом /api/v1/me, интерфейс не показывается. */
    loadMe(true).then(ok => {
      state.boot = "ready";
      if (ok) { ensureView(); render(); if (hasCompany()) loadHistory(true); }
      else { state.profile = null; render(); }
      if (invite) setTimeout(() => askAcceptInvite(invite), 250);
    });
  })();
})();
