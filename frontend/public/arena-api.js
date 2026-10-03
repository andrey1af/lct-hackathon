  /* === ARENA_API_LAYER_START === */
  /* DOM-независимый слой доступа к API «Арена переговоров» (https://vozduhanchiki.ru).
     Полный контракт OpenAPI 3.0.3 (v0.3.0):
       • /health/live, /health/ready
       • /auth/register (в КОРНЕ), /api/v1/auth/login|refresh|logout, /api/v1/me, /api/v1/invitations/accept
       • /api/v1/companies/{cid}/…: members, invitations, modules, steps/{id}/attempts, me/progress,
         team/progress, me/sessions, cases, sessions (+ messages/hint/retry/finish/feedback),
         modules/{id}/assessment-sessions.
     Аутентификация: заголовок `Authorization: Bearer <token>` ИЛИ HttpOnly cookie access_token
     (на том же домене куки уходят автоматически). Refresh-токен одноразовый и ротируется.
     Слой не хранит состояние: base/cid/token/credentials передаются вызывающим кодом.
     Все ошибки — унифицированный объект {kind, статус, code, message, retryAfter}. */
  (function (global) {
    "use strict";

    var DEFAULT_TIMEOUT = 12000;

    function buildUrl(base, path) {
      var b = String(base == null ? "" : base).trim().replace(/\/+$/, "");
      var p = String(path == null ? "" : path).replace(/^\/+/, "");
      return (b ? b + "/" : "/") + p;
    }

    function isNetworkFailure(message) {
      return /failed to fetch|networkerror|load failed|fetch failed|enotfound|econnrefused|econnreset|cors|blocked by|network request failed/i.test(String(message || ""));
    }

    function normalizeError(e, timeoutMs) {
      var m = String((e && e.message) || e || "");
      if (e && (e.name === "AbortError" || /abort/i.test(m))) {
        return { kind: "timeout", status: 0, code: "timeout", message: "Превышено время ожидания ответа API (" + (timeoutMs || DEFAULT_TIMEOUT) + " мс)" };
      }
      if (isNetworkFailure(m)) {
        return { kind: "network", status: 0, code: "network", message: "Запрос заблокирован: сеть или CORS (откройте страницу с домена API)" };
      }
      return { kind: "http", status: 0, code: "unknown", message: m || "Неизвестная ошибка запроса" };
    }

    var KIND_BY_STATUS = { 400: "bad_request", 401: "unauthorized", 403: "forbidden", 404: "not_found", 409: "conflict", 422: "bad_request", 429: "rate_limited" };
    function kindForStatus(status) {
      if (KIND_BY_STATUS[status]) return KIND_BY_STATUS[status];
      if (status >= 500) return "server";
      return "http";
    }

    function newAttemptId() {
      try {
        if (global.crypto && typeof global.crypto.randomUUID === "function") return global.crypto.randomUUID();
      } catch (e) { }
      return "a-" + Date.now().toString(36) + "-" + Math.random().toString(36).slice(2, 10);
    }

    function requestOnce(o) {
      o = o || {};
      var url = buildUrl(o.base, o.path);
      var timeoutMs = typeof o.timeoutMs === "number" && o.timeoutMs > 0 ? o.timeoutMs : DEFAULT_TIMEOUT;
      var fetchFn = o.fetchImpl || global.fetch;
      if (typeof fetchFn !== "function") {
        return Promise.resolve({ ok: false, status: 0, error: { kind: "network", status: 0, code: "no_fetch", message: "fetch недоступен в этом окружении" } });
      }
      var headers = { Accept: "application/json" };
      if (o.token) headers.Authorization = "Bearer " + o.token;
      var init = { method: o.method || "GET", headers: headers };
      if (o.credentials) init.credentials = o.credentials;
      if (o.body !== undefined && o.body !== null) {
        headers["Content-Type"] = "application/json";
        init.body = JSON.stringify(o.body);
      }
      var timer = null;
      try {
        if (typeof global.AbortController === "function") {
          var ctl = new global.AbortController();
          init.signal = ctl.signal;
          timer = setTimeout(function () { ctl.abort(); }, timeoutMs);
        }
      } catch (e) { }

      return fetchFn(url, init).then(function (res) {
        var retryAfter = "";
        try { retryAfter = (res.headers && res.headers.get) ? (res.headers.get("Retry-After") || "") : ""; } catch (e2) { }
        return res.text().then(function (text) {
          var payload = null;
          if (text) { try { payload = JSON.parse(text); } catch (e) { payload = null; } }
          if (res.ok) return { ok: true, status: res.status, data: payload };
          var code = payload && payload.error && payload.error.code ? payload.error.code : "";
          var msg = payload && payload.error && payload.error.message ? payload.error.message : ("HTTP " + res.status);
          return { ok: false, status: res.status, error: { kind: kindForStatus(res.status), status: res.status, code: code, message: msg, retryAfter: retryAfter } };
        });
      }).catch(function (e) {
        return { ok: false, status: 0, error: normalizeError(e, timeoutMs) };
      }).then(function (r) {
        if (timer) { clearTimeout(timer); timer = null; }
        return r;
      });
    }

    var tokenSink = null;
    var refreshInFlight = null;
    var NO_REFRESH = /^\/(?:auth\/register|api\/v1\/auth\/(?:login|refresh|logout))$/;

    function setTokenSink(fn) { tokenSink = typeof fn === "function" ? fn : null; }

    function runRefresh(o) {
      if (refreshInFlight) return refreshInFlight;
      refreshInFlight = refresh({
        base: o.base, credentials: o.credentials, timeoutMs: o.timeoutMs,
        fetchImpl: o.fetchImpl, refreshToken: o.refreshToken
      }).then(function (r) {
        var t = (r && r.ok && r.data && r.data.accessToken) ? String(r.data.accessToken) : "";
        if (t && tokenSink) { try { tokenSink(t); } catch (e) { } }
        refreshInFlight = null;

        return t;
      }, function () {
        refreshInFlight = null;

        return "";
      });

      return refreshInFlight;
    }

    function request(o) {
      o = o || {};

      return requestOnce(o).then(function (r) {
        if (!r || r.ok || r.status !== 401 || o.noRefresh || NO_REFRESH.test(String(o.path || ""))) return r;

        return runRefresh(o).then(function (t) {
          if (!t) return r;
          var retry = Object.assign({}, o, { token: t, noRefresh: true });

          return requestOnce(retry).then(function (r2) { return (r2 && r2.status !== 401) ? r2 : r; });
        });
      });
    }

    function withCid(cid, tail) { return "/api/v1/companies/" + encodeURIComponent(cid || "") + tail; }
    function q(o) { return { base: o.base, token: o.token, credentials: o.credentials, timeoutMs: o.timeoutMs, fetchImpl: o.fetchImpl }; }

    /* ---- Health ---- */
    function healthLive(o) { o = o || {}; return request({ base: o.base, token: o.token, credentials: o.credentials, timeoutMs: o.timeoutMs, fetchImpl: o.fetchImpl, path: "/health/live" }); }
    function healthReady(o) { o = o || {}; return request({ base: o.base, token: o.token, credentials: o.credentials, timeoutMs: o.timeoutMs, fetchImpl: o.fetchImpl, path: "/health/ready" }); }

    /* ---- Auth ---- */
    function register(o) {
      o = o || {};
      return request(Object.assign(q(o), { method: "POST", path: "/auth/register", body: { email: o.email || "", password: o.password || "", name: o.name || "" } }));
    }
    function login(o) {
      o = o || {};
      return request(Object.assign(q(o), { method: "POST", path: "/api/v1/auth/login", body: { email: o.email || "", password: o.password || "" } }));
    }
    function refresh(o) {
      o = o || {};
      var body = o.refreshToken ? { refresh_token: o.refreshToken } : undefined;
      return request(Object.assign(q(o), { method: "POST", path: "/api/v1/auth/refresh", body: body }));
    }
    function logout(o) {
      o = o || {};
      var body = o.refreshToken ? { refresh_token: o.refreshToken } : undefined;
      return request(Object.assign(q(o), { method: "POST", path: "/api/v1/auth/logout", body: body }));
    }
    function me(o) { o = o || {}; return request(Object.assign(q(o), { path: "/api/v1/me" })); }
    function updateMe(o) { o = o || {}; return request(Object.assign(q(o), { method: "PATCH", path: "/api/v1/me", body: { name: o.name || "" } })); }
    function acceptInvitation(o) {
      o = o || {};
      var body = { token: o.inviteToken || "" };
      if (o.name) body.name = o.name;
      if (o.password) body.password = o.password;
      return request(Object.assign(q(o), { method: "POST", path: "/api/v1/invitations/accept", body: body }));
    }

    /* ---- Learning ---- */
    function listModules(o) { o = o || {}; return request(Object.assign(q(o), { path: withCid(o.cid, "/modules") })); }
    function getModule(o) { o = o || {}; return request(Object.assign(q(o), { path: withCid(o.cid, "/modules/" + encodeURIComponent(o.moduleId || "")) })); }
    function submitAttempt(o) {
      o = o || {};
      return request(Object.assign(q(o), {
        method: "POST", path: withCid(o.cid, "/steps/" + encodeURIComponent(o.stepId || "") + "/attempts"),
        body: { client_attempt_id: o.clientAttemptId || newAttemptId(), answer: String(o.answer == null ? "" : o.answer) }
      }));
    }
    function myProgress(o) { o = o || {}; return request(Object.assign(q(o), { path: withCid(o.cid, "/me/progress") })); }
    function teamProgress(o) { o = o || {}; return request(Object.assign(q(o), { path: withCid(o.cid, "/team/progress") })); }
    function startAssessment(o) { o = o || {}; return request(Object.assign(q(o), { method: "POST", path: withCid(o.cid, "/modules/" + encodeURIComponent(o.moduleId || "") + "/assessment-sessions") })); }

    /* ---- Negotiation ---- */
    function listCases(o) { o = o || {}; return request(Object.assign(q(o), { path: withCid(o.cid, "/cases") })); }
    function getCase(o) { o = o || {}; return request(Object.assign(q(o), { path: withCid(o.cid, "/cases/" + encodeURIComponent(o.caseKey || "")) })); }
    function startSession(o) {
      o = o || {};
      var body = o.caseKey ? { case_key: o.caseKey } : undefined;
      return request(Object.assign(q(o), { method: "POST", path: withCid(o.cid, "/sessions"), body: body }));
    }
    function getSession(o) { o = o || {}; return request(Object.assign(q(o), { path: withCid(o.cid, "/sessions/" + encodeURIComponent(o.sessionId || "")) })); }
    function sendMessage(o) {
      o = o || {};
      return request(Object.assign(q(o), {
        method: "POST", path: withCid(o.cid, "/sessions/" + encodeURIComponent(o.sessionId || "") + "/messages"),
        body: { client_message_id: o.clientMessageId || newAttemptId(), text: String(o.text == null ? "" : o.text) }
      }));
    }
    function requestHint(o) { o = o || {}; return request(Object.assign(q(o), { method: "POST", path: withCid(o.cid, "/sessions/" + encodeURIComponent(o.sessionId || "") + "/hint") })); }
    function retrySession(o) { o = o || {}; return request(Object.assign(q(o), { method: "POST", path: withCid(o.cid, "/sessions/" + encodeURIComponent(o.sessionId || "") + "/retry") })); }
    function finishSession(o) { o = o || {}; return request(Object.assign(q(o), { method: "POST", path: withCid(o.cid, "/sessions/" + encodeURIComponent(o.sessionId || "") + "/finish") })); }
    function getFeedback(o) { o = o || {}; return request(Object.assign(q(o), { path: withCid(o.cid, "/sessions/" + encodeURIComponent(o.sessionId || "") + "/feedback") })); }

    /* ---- Platform (админ-контуры) ---- */
    function listMembers(o) { o = o || {}; return request(Object.assign(q(o), { path: withCid(o.cid, "/members") })); }
    function updateMember(o) {
      o = o || {};
      return request(Object.assign(q(o), { method: "PATCH", path: withCid(o.cid, "/members/" + encodeURIComponent(o.memberId || "")), body: { role: o.role || "employee", status: o.status || "active" } }));
    }
    function createInvitation(o) {
      o = o || {};
      return request(Object.assign(q(o), { method: "POST", path: withCid(o.cid, "/invitations"), body: { email: o.email || "", role: o.role || "employee" } }));
    }
    function mySessions(o) { o = o || {}; return request(Object.assign(q(o), { path: withCid(o.cid, "/me/sessions") })); }

    /* ---- Мапперы (в локальную модель интерфейса) ---- */
    function mapCase(c) {
      c = c || {};
      return {
        key: c.key, title: c.title == null ? "" : String(c.title),
        mode: c.mode || "practice", difficulty: c.difficulty || "medium",
        skillIds: Array.isArray(c.skill_ids) ? c.skill_ids.slice() : [],
        maxTurns: typeof c.max_turns === "number" ? c.max_turns : 0,
        briefing: c.briefing == null ? "" : String(c.briefing),
        hintsAvailable: !!c.hints_available, locked: !!c.locked,
        unlockedBy: c.unlocked_by == null ? "" : String(c.unlocked_by)
      };
    }
    function mapCases(payload) { var list = payload && Array.isArray(payload.cases) ? payload.cases : []; return list.map(mapCase); }
    function mapMessage(m) {
      m = m || {};
      return { role: m.role === "user" ? "user" : "opponent", turnNumber: m.turn_number || 0, text: m.text == null ? "" : String(m.text), createdAt: m.created_at || "" };
    }
    function mapSession(s) {
      s = s || {};
      return {
        sessionId: s.session_id, status: s.status || "active", mode: s.mode || "practice",
        caseKey: s.case_key == null ? "" : String(s.case_key), title: s.title == null ? "" : String(s.title),
        briefing: s.briefing == null ? "" : String(s.briefing), attempt: s.attempt || 1,
        turnsUsed: s.turns_used || 0, maxTurns: s.max_turns || 0, hintsUsed: s.hints_used || 0, hintsLeft: s.hints_left || 0,
        messages: Array.isArray(s.messages) ? s.messages.map(mapMessage) : []
      };
    }
    function mapTurn(t) {
      t = t || {};
      return {
        sessionId: t.session_id, status: t.status || "active", turnNumber: t.turn_number || 0,
        playerMessage: mapMessage(t.player_message), opponentMessage: mapMessage(t.opponent_message),
        turnsUsed: t.turns_used || 0, maxTurns: t.max_turns || 0
      };
    }
    function mapHint(h) { h = h || {}; return { sessionId: h.session_id, hint: h.hint == null ? "" : String(h.hint), hintsUsed: h.hints_used || 0, hintsLeft: h.hints_left || 0 }; }
    function mapCriterion(c) {
      c = c || {};
      return { id: c.id, behavior: c.behavior == null ? "" : String(c.behavior), required: !!c.required, met: !!c.met, evidence: c.evidence == null ? "" : String(c.evidence), comment: c.comment == null ? "" : String(c.comment) };
    }
    function mapResult(r) {
      r = r || {};
      return {
        sessionId: r.session_id, status: r.status || "evaluated", mode: r.mode || "practice",
        caseKey: r.case_key == null ? "" : String(r.case_key), title: r.title == null ? "" : String(r.title),
        passed: !!r.passed, evaluationStatus: r.evaluation_status || "pending", rubricVersion: r.rubric_version || 1,
        moduleStatus: r.module_status || "", criteria: Array.isArray(r.criteria) ? r.criteria.map(mapCriterion) : [],
        feedback: { whatWorked: (r.feedback && r.feedback.what_worked) || "", whatToTryNext: (r.feedback && r.feedback.what_to_try_next) || "" },
        attempt: r.attempt || 1, turnsUsed: r.turns_used || 0, maxTurns: r.max_turns || 0, canRetry: !!r.can_retry
      };
    }
    function mapProfile(p) {
      p = p || {};
      return {
        id: p.id, email: p.email == null ? "" : String(p.email), name: p.name == null ? "" : String(p.name),
        companies: Array.isArray(p.companies) ? p.companies.map(function (c) { c = c || {}; return { id: c.id, name: c.name == null ? "" : String(c.name), role: c.role || "employee" }; }) : []
      };
    }
    function mapMember(m) { m = m || {}; return { userId: m.user_id, email: m.email == null ? "" : String(m.email), name: m.name == null ? "" : String(m.name), role: m.role || "employee", status: m.status || "active" }; }
    function mapMembers(payload) { var list = payload && Array.isArray(payload.members) ? payload.members : []; return list.map(mapMember); }
    function mapSessionHistoryItem(s) {
      s = s || {};
      return {
        id: s.id, title: s.title == null ? "" : String(s.title),
        mode: s.mode || "practice", status: s.status || "active",
        createdAt: s.created_at || ""
      };
    }
    function mapSessionHistory(payload) { var list = payload && Array.isArray(payload.sessions) ? payload.sessions : []; return list.map(mapSessionHistoryItem); }
    function mapTeamMember(m) { m = m || {}; var out = mapMember(m); out.modules = Array.isArray(m.modules) ? m.modules.map(mapModule) : []; return out; }
    function mapTeamProgress(payload) { var list = payload && Array.isArray(payload.members) ? payload.members : []; return list.map(mapTeamMember); }

    /* ---- Обучение: шаги и модули ---- */
    function mapStep(st) {
      st = st || {};
      return {
        id: st.id, order: typeof st.order === "number" ? st.order : 0,
        type: st.type === "choice" ? "choice" : "theory",
        content: st.content == null ? "" : String(st.content),
        options: Array.isArray(st.options) ? st.options.map(function (o) { o = o || {}; return { id: o.id, text: o.text == null ? "" : String(o.text) }; }) : [],
        completed: !!st.completed
      };
    }
    function mapSteps(payload) {
      var steps = payload && Array.isArray(payload.steps) ? payload.steps : [];
      return steps.map(mapStep).sort(function (a, b) { return a.order - b.order; });
    }
    function mapModule(m) {
      m = m || {};
      return {
        id: m.id, version: m.version, title: m.title == null ? "" : String(m.title),
        order: typeof m.order === "number" ? m.order : 0,
        skillIds: Array.isArray(m.skill_ids) ? m.skill_ids.slice() : [],
        status: m.status || "available",
        completedSteps: typeof m.completed_steps === "number" ? m.completed_steps : 0,
        totalSteps: typeof m.total_steps === "number" ? m.total_steps : 0,
        locked: m.status === "locked",
        steps: mapSteps(m)
      };
    }
    function mapModulesToTracks(payload) {
      var list = payload && Array.isArray(payload.modules) ? payload.modules : [];
      return list.map(mapModule).sort(function (a, b) { return a.order - b.order; });
    }

    var STATUS_LABEL = { locked: "Закрыто", available: "Начать", in_progress: "Продолжить", assessment_ready: "Готово к оценке", needs_retry: "Нужен повтор", passed: "Пройден" };
    var CASE_MODE_LABEL = { practice: "Практика", assessment: "Проверочный кейс" };
    var DIFFICULTY_LABEL = { easy: "Лёгкий", medium: "Средний", hard: "Сложный" };
    var SESSION_STATUS_LABEL = { active: "Идёт", finished: "Завершена", evaluated: "Разобрана" };
    var EVAL_STATUS_LABEL = { pending: "Разбор в процессе", evaluated: "Разбор получен", needs_retry: "Разбор не удался — нужен повтор" };
    var ROLE_LABEL = { employee: "Сотрудник", company_admin: "Админ компании", platform_admin: "Платформа" };
    function statusToLabel(s) { return STATUS_LABEL[s] || String(s || ""); }
    function caseModeLabel(m) { return CASE_MODE_LABEL[m] || String(m || ""); }
    function difficultyLabel(d) { return DIFFICULTY_LABEL[d] || String(d || ""); }
    function sessionStatusLabel(s) { return SESSION_STATUS_LABEL[s] || String(s || ""); }
    function roleLabel(r) { return ROLE_LABEL[r] || String(r || ""); }

    global.ArenaApi = {
      DEFAULT_TIMEOUT: DEFAULT_TIMEOUT,
      buildUrl: buildUrl,
      normalizeError: normalizeError,
      newAttemptId: newAttemptId,
      request: request,
      setTokenSink: setTokenSink,
      healthLive: healthLive,
      healthReady: healthReady,
      register: register,
      login: login,
      refresh: refresh,
      logout: logout,
      me: me,
      updateMe: updateMe,
      acceptInvitation: acceptInvitation,
      listModules: listModules,
      getModule: getModule,
      submitAttempt: submitAttempt,
      myProgress: myProgress,
      teamProgress: teamProgress,
      startAssessment: startAssessment,
      listCases: listCases,
      getCase: getCase,
      startSession: startSession,
      getSession: getSession,
      sendMessage: sendMessage,
      requestHint: requestHint,
      retrySession: retrySession,
      finishSession: finishSession,
      getFeedback: getFeedback,
      listMembers: listMembers,
      updateMember: updateMember,
      createInvitation: createInvitation,
      mySessions: mySessions,
      mapCase: mapCase,
      mapCases: mapCases,
      mapMessage: mapMessage,
      mapSession: mapSession,
      mapTurn: mapTurn,
      mapHint: mapHint,
      mapCriterion: mapCriterion,
      mapResult: mapResult,
      mapProfile: mapProfile,
      mapMember: mapMember,
      mapMembers: mapMembers,
      mapSessionHistoryItem: mapSessionHistoryItem,
      mapSessionHistory: mapSessionHistory,
      mapTeamMember: mapTeamMember,
      mapTeamProgress: mapTeamProgress,
      mapStep: mapStep,
      mapSteps: mapSteps,
      mapModule: mapModule,
      mapModulesToTracks: mapModulesToTracks,
      statusToLabel: statusToLabel,
      caseModeLabel: caseModeLabel,
      difficultyLabel: difficultyLabel,
      sessionStatusLabel: sessionStatusLabel,
      roleLabel: roleLabel,
      STATUS_LABEL: STATUS_LABEL,
      EVAL_STATUS_LABEL: EVAL_STATUS_LABEL
    };
  })(typeof window !== "undefined" ? window : globalThis);
  /* === ARENA_API_LAYER_END === */
