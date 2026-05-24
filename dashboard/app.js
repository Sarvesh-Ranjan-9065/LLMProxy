const storageKey = "llmproxy_api_key";

const els = {
  loginPanel: document.getElementById("loginPanel"),
  loginError: document.getElementById("loginError"),
  apiKeyInput: document.getElementById("apiKeyInput"),
  saveKey: document.getElementById("saveKey"),
  clearKey: document.getElementById("clearKey"),
  sessionRole: document.getElementById("sessionRole"),
  userPanel: document.getElementById("userPanel"),
  adminPanel: document.getElementById("adminPanel"),
  userOwner: document.getElementById("userOwner"),
  userUpdated: document.getElementById("userUpdated"),
  userRequests: document.getElementById("userRequests"),
  userLatency: document.getElementById("userLatency"),
  userCacheRate: document.getElementById("userCacheRate"),
  userRateLimited: document.getElementById("userRateLimited"),
  userModels: document.getElementById("userModels"),
  userRecent: document.getElementById("userRecent"),
  adminUpdated: document.getElementById("adminUpdated"),
  adminRequests: document.getElementById("adminRequests"),
  adminUsers: document.getElementById("adminUsers"),
  adminCacheRate: document.getElementById("adminCacheRate"),
  adminRedis: document.getElementById("adminRedis"),
  adminBackends: document.getElementById("adminBackends"),
  adminModels: document.getElementById("adminModels"),
  adminRecent: document.getElementById("adminRecent"),
};

function getKey() {
  return localStorage.getItem(storageKey) || "";
}

function setKey(key) {
  localStorage.setItem(storageKey, key);
}

function clearKey() {
  localStorage.removeItem(storageKey);
}

function setStatus(role) {
  els.sessionRole.textContent = role || "guest";
}

function showLogin(message) {
  els.loginPanel.hidden = false;
  els.userPanel.hidden = true;
  els.adminPanel.hidden = true;
  els.loginError.textContent = message || "";
  setStatus("guest");
}

function showPanels(role) {
  els.loginPanel.hidden = true;
  els.userPanel.hidden = false;
  els.adminPanel.hidden = role === "admin" ? false : true;
  setStatus(role);
}

async function fetchJSON(path) {
  const apiKey = getKey();
  if (!apiKey) {
    throw new Error("missing key");
  }

  const res = await fetch(path, {
    headers: {
      "Authorization": "Bearer " + apiKey,
      "X-API-Key": apiKey,
    },
    cache: "no-store",
  });

  if (!res.ok) {
    const text = await res.text();
    throw new Error(text || "request failed");
  }
  return res.json();
}

function formatNumber(value) {
  return new Intl.NumberFormat().format(value || 0);
}

function formatPercent(value) {
  const pct = (value || 0) * 100;
  return pct.toFixed(1) + "%";
}

function formatLatency(value) {
  return Math.round(value || 0) + " ms";
}

function formatTime(ts) {
  if (!ts) {
    return "-";
  }
  const date = new Date(ts);
  return date.toLocaleTimeString();
}

function renderModels(listEl, usageMap) {
  listEl.innerHTML = "";
  const entries = Object.entries(usageMap || {});
  if (entries.length === 0) {
    const li = document.createElement("li");
    li.textContent = "No data yet";
    listEl.appendChild(li);
    return;
  }

  entries.sort((a, b) => b[1] - a[1]);
  entries.forEach(([model, count]) => {
    const li = document.createElement("li");
    const name = document.createElement("span");
    name.textContent = model;
    const value = document.createElement("strong");
    value.textContent = formatNumber(count);
    li.appendChild(name);
    li.appendChild(value);
    listEl.appendChild(li);
  });
}

function renderRecent(listEl, items) {
  listEl.innerHTML = "";
  if (!items || items.length === 0) {
    listEl.textContent = "No recent requests";
    return;
  }

  items.slice().reverse().forEach((item) => {
    const row = document.createElement("div");
    row.className = "recent-item";
    const time = formatTime(item.timestamp);
    const model = item.model || "unknown";
    const timeEl = document.createElement("span");
    timeEl.textContent = time;
    const detail = document.createElement("strong");
    detail.textContent = `${item.path} (${model})`;
    const status = document.createElement("span");
    status.textContent = item.status;
    row.appendChild(timeEl);
    row.appendChild(detail);
    row.appendChild(status);
    listEl.appendChild(row);
  });
}

function renderUser(summary) {
  els.userOwner.textContent = summary.owner || els.userOwner.textContent || "unknown";
  els.userUpdated.textContent = formatTime(summary.generated_at);
  els.userRequests.textContent = formatNumber(summary.requests_total);
  els.userLatency.textContent = formatLatency(summary.avg_latency_ms);
  els.userCacheRate.textContent = formatPercent(summary.cache_hit_rate);
  els.userRateLimited.textContent = formatNumber(summary.rate_limited_total);
  renderModels(els.userModels, summary.model_usage);
  renderRecent(els.userRecent, summary.recent);
}

function renderAdmin(summary) {
  els.adminUpdated.textContent = formatTime(summary.generated_at);
  els.adminRequests.textContent = formatNumber(summary.total_requests);
  els.adminUsers.textContent = formatNumber(summary.active_users);
  els.adminCacheRate.textContent = formatPercent(summary.cache_hit_rate);
  els.adminRedis.textContent = summary.redis && summary.redis.healthy ? "healthy" : "unhealthy";

  els.adminBackends.innerHTML = "";
  const backends = summary.backends || [];
  if (backends.length === 0) {
    const li = document.createElement("li");
    li.textContent = "No backends configured";
    els.adminBackends.appendChild(li);
  } else {
    backends.forEach((backend) => {
      const li = document.createElement("li");
      const state = backend.alive ? "up" : "down";
      const name = document.createElement("span");
      name.textContent = backend.url;
      const value = document.createElement("strong");
      value.textContent = state;
      li.appendChild(name);
      li.appendChild(value);
      els.adminBackends.appendChild(li);
    });
  }

  const topModels = summary.top_models || [];
  const topCounts = summary.top_model_counts || [];
  els.adminModels.innerHTML = "";
  if (topModels.length === 0) {
    const li = document.createElement("li");
    li.textContent = "No model data";
    els.adminModels.appendChild(li);
  } else {
    topModels.forEach((model, idx) => {
      const li = document.createElement("li");
      const name = document.createElement("span");
      name.textContent = model;
      const value = document.createElement("strong");
      value.textContent = formatNumber(topCounts[idx]);
      li.appendChild(name);
      li.appendChild(value);
      els.adminModels.appendChild(li);
    });
  }

  renderRecent(els.adminRecent, summary.recent_requests);
}

async function loadDashboard() {
  const apiKey = getKey();
  if (!apiKey) {
    showLogin("Enter a valid API key to continue.");
    return;
  }

  try {
    const me = await fetchJSON("/dashboard/api/me");
    const role = me.role || "user";
    showPanels(role);
    els.userOwner.textContent = me.owner || "unknown";

    const userSummary = await fetchJSON("/dashboard/api/user/summary");
    renderUser(userSummary);

    if (role === "admin") {
      const adminSummary = await fetchJSON("/dashboard/api/admin/summary");
      renderAdmin(adminSummary);
    }
  } catch (err) {
    showLogin("Invalid key or access denied.");
  }
}

els.saveKey.addEventListener("click", () => {
  const key = els.apiKeyInput.value.trim();
  if (!key) {
    els.loginError.textContent = "API key is required.";
    return;
  }
  setKey(key);
  els.apiKeyInput.value = "";
  loadDashboard();
});

els.clearKey.addEventListener("click", () => {
  clearKey();
  showLogin("Key cleared.");
});

loadDashboard();
