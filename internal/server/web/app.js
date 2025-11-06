"use strict";

document.addEventListener("DOMContentLoaded", () => {
  const state = {
    miners: [],
    models: [],
    selectedMiner: null,
    plantData: [],
    balanceStatus: null,
    balanceEvents: [],
    energyChart: null,
  };

  const refs = {
    minersTableBody: document.querySelector("#miners-table tbody"),
    modelsContainer: document.querySelector("#models-container"),
    notifications: document.querySelector("#notifications"),
    refreshMiners: document.querySelector("#refresh-miners"),
    minerModal: document.querySelector("#miner-modal"),
    minerModalContent: document.querySelector("#miner-modal-content"),
    minerModalClose: document.querySelector("#miner-modal-close"),
  };

  const AUTO_REFRESH_MS = 10_000;

  const showToast = (message, type = "success", timeout = 5000) => {
    if (!message) return;
    const toast = document.createElement("div");
    toast.className = `toast ${type}`;
    toast.innerHTML = `
      <span>${message}</span>
      <button type="button" aria-label="Dismiss">&times;</button>
    `;
    const close = () => toast.remove();
    toast.querySelector("button").addEventListener("click", close);
    refs.notifications.appendChild(toast);
    setTimeout(close, timeout);
  };

  const formatHashrate = (value) => {
    if (value === null || value === undefined) return "—";
    const units = ["GH/s", "TH/s", "PH/s"];
    let val = Number(value);
    let index = 0;
    while (val >= 1000 && index < units.length - 1) {
      val /= 1000;
      index += 1;
    }
    return `${val.toFixed(2)} ${units[index]}`;
  };

  const formatPower = (value) => {
    if (value === null || value === undefined) return "—";
    const watts = Number(value);
    if (!Number.isFinite(watts)) return "—";
    if (Math.abs(watts) >= 1000) {
      return `${(watts / 1000).toFixed(2)} kW`;
    }
    return `${watts.toFixed(0)} W`;
  };

  const formatTemperatureRange = (chains = []) => {
    if (!Array.isArray(chains) || chains.length === 0) return "—";
    let min = Number.POSITIVE_INFINITY;
    let max = Number.NEGATIVE_INFINITY;

    chains.forEach((chain) => {
      if (!chain) return;
      const readings = [
        chain.chip_temp_min,
        chain.chip_temp_max,
        chain.pcb_temp_min,
        chain.pcb_temp_max,
      ];
      readings.forEach((value) => {
        const temp = Number(value);
        if (!Number.isFinite(temp)) return;
        if (temp < min) min = temp;
        if (temp > max) max = temp;
      });
    });

    if (!Number.isFinite(min) || !Number.isFinite(max)) return "—";
    return `${Math.round(min)}&deg;-${Math.round(max)}&deg;C`;
  };

  const formatTemperature = (value, fallback = "—") => {
    const temp = Number(value);
    if (!Number.isFinite(temp)) return fallback;
    return `${temp.toFixed(1)}°C`;
  };

  const formatDuration = (seconds) => {
    if (seconds === null || seconds === undefined) return "—";
    const total = Number(seconds);
    if (!Number.isFinite(total) || total < 0) return "—";
    const days = Math.floor(total / 86400);
    const hours = Math.floor((total % 86400) / 3600);
    const minutes = Math.floor((total % 3600) / 60);
    const secs = Math.floor(total % 60);
    const parts = [];
    if (days) parts.push(`${days}d`);
    if (hours) parts.push(`${hours}h`);
    if (minutes) parts.push(`${minutes}m`);
    if (parts.length === 0) parts.push(`${secs}s`);
    return parts.slice(0, 3).join(" ");
  };

  const prettifyChainState = (state) => {
    const value = (state || "").trim();
    if (!value) return "unknown";
    return value.charAt(0).toUpperCase() + value.slice(1);
  };

  const chainStateClass = (state) => {
    const value = (state || "").trim().toLowerCase();
    if (value === "mining") return "chain-square--green";
    if (value === "auto-tuning" || value === "autotuning" || value === "tuning") {
      return "chain-square--yellow";
    }
    if (value === "starting" || value === "initializing" || value === "restarting" || value === "") {
      return "chain-square--grey";
    }
    if (value === "failure" || value === "error" || value === "offline") {
      return "chain-square--red";
    }
    return "chain-square--grey";
  };

  const renderChainSquares = (chains = []) => {
    if (!Array.isArray(chains) || chains.length === 0) {
      return '<span class="muted">—</span>';
    }
    return chains
      .map((chain, idx) => {
        if (!chain) return "";
        const titleParts = [];
        if (chain.identifier) titleParts.push(chain.identifier);
        if (chain.state) titleParts.push(prettifyChainState(chain.state));
        if (chain.hashrate) titleParts.push(formatHashrate(chain.hashrate));
        const title = titleParts.join(" | ") || `Chain ${idx + 1}`;
        return `<span class="chain-square ${chainStateClass(chain.state)}" title="${title}"></span>`;
      })
      .join("");
  };

  const renderFanBadges = (fans = []) => {
    if (!Array.isArray(fans) || fans.length === 0) {
      return '<span class="muted">—</span>';
    }
    return fans
      .map((fan, idx) => {
        if (!fan) return "";
        const label =
          (fan.identifier || `fan-${idx + 1}`).replace(/^fan-?/i, "F").toUpperCase();
        const rpm = Number(fan.rpm);
        const status = (fan.status || "").trim();
        const healthy = Number.isFinite(rpm) && rpm > 0 && (!status || status.toLowerCase() === "ok");
        const text = healthy
          ? `${label} ${rpm.toLocaleString()} RPM`
          : `${label} ${status || "offline"}`;
        const badgeClass = healthy ? "fan-badge" : "fan-badge fan-badge--alert";
        return `<span class="${badgeClass}" title="${text}">${text}</span>`;
      })
      .join("");
  };

  const formatRelativeTime = (timestamp) => {
    if (!timestamp) return "—";
    const date = new Date(timestamp);
    if (Number.isNaN(date.getTime())) return "—";
    const diff = Date.now() - date.getTime();
    if (diff < 0) return "just now";
    const minutes = Math.round(diff / 60000);
    if (minutes < 1) return "just now";
    if (minutes < 60) return `${minutes} min ago`;
    const hours = Math.round(minutes / 60);
    if (hours < 24) return `${hours} hr ago`;
    const days = Math.round(hours / 24);
    return `${days} day${days === 1 ? "" : "s"} ago`;
  };

  const formatExactTime = (timestamp) => {
    if (!timestamp) return "—";
    const date = new Date(timestamp);
    if (Number.isNaN(date.getTime())) return "—";
    return date.toLocaleString(undefined, {
      year: "numeric",
      month: "short",
      day: "2-digit",
      hour: "2-digit",
      minute: "2-digit",
      second: "2-digit",
    });
  };

  const deriveMinerStatus = (miner) => {
    if (!miner) {
      return { label: "Offline", className: "status-offline" };
    }
    const state = (miner.latest_status?.state || "").trim().toLowerCase();
    if (!miner.online || state === "stopped" || state === "shutting-down") {
      return { label: "Offline", className: "status-offline" };
    }
    if (state === "mining") {
      return { label: "Mining", className: "status-mining" };
    }
    if (
      state === "auto-tuning" ||
      state === "auto tuning" ||
      state === "initializing" ||
      state === "starting" ||
      state === "restarting"
    ) {
      return { label: "Auto-tuning", className: "status-auto-tuning" };
    }
    if (state === "failure" || state === "error") {
      return { label: "Error", className: "status-error" };
    }
    return { label: "Error", className: "status-error" };
  };

  const fetchJSON = async (url, options = {}) => {
    const res = await fetch(url, options);
    if (!res.ok) {
      const data = await res.json().catch(() => ({}));
      const error = data.error || res.statusText || "Request failed";
      throw new Error(error);
    }
    return res.json();
  };

  const fetchMiners = async (silent = false) => {
    try {
      const data = await fetchJSON("/api/miners");
      state.miners = Array.isArray(data) ? data : [];
      renderMiners();
      if (state.selectedMiner && refs.minerModal && !refs.minerModal.classList.contains("hidden")) {
        const miner = state.miners.find((m) => m.id === state.selectedMiner);
        if (miner) {
          await refreshMinerModal(miner);
        } else {
          closeMinerModal();
        }
      }
      if (!silent) {
        showToast("Miners refreshed", "success", 3000);
      }
    } catch (err) {
      if (!silent) {
        showToast(err.message, "error");
      }
    }
  };

  const fetchModels = async () => {
    try {
      const data = await fetchJSON("/api/models");
      state.models = Array.isArray(data) ? data : [];
      renderModels();
    } catch (err) {
      showToast(err.message, "error");
    }
  };

  const fetchBalanceStatus = async () => {
    try {
      const data = await fetchJSON("/api/balance/status");
      state.balanceStatus = data;
      renderBalanceStatus();
    } catch (err) {
      console.error("Failed to fetch balance status:", err);
    }
  };

  const fetchPlantHistory = async () => {
    try {
      const data = await fetchJSON("/api/plant/history?limit=50");
      state.plantData = Array.isArray(data) ? data : [];
      updateEnergyChart();
    } catch (err) {
      console.error("Failed to fetch plant history:", err);
    }
  };

  const fetchBalanceEvents = async () => {
    try {
      const data = await fetchJSON("/api/balance/events?limit=20");
      state.balanceEvents = Array.isArray(data) ? data : [];
      renderBalanceEvents();
    } catch (err) {
      console.error("Failed to fetch balance events:", err);
    }
  };

  const updateSafetyMargin = async () => {
    const input = document.getElementById("safety-margin-input");
    const value = parseFloat(input.value);

    if (isNaN(value) || value < 0 || value > 50) {
      showToast("Safety margin must be between 0 and 50%", "error");
      return;
    }

    try {
      await fetchJSON("/api/settings/safety-margin", {
        method: "PATCH",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ safety_margin_percent: value }),
      });
      showToast("Safety margin updated", "success");
      await fetchBalanceStatus();
    } catch (err) {
      showToast(err.message, "error");
    }
  };

  const updateManaged = async (minerId, value, checkbox) => {
    checkbox.disabled = true;
    try {
      await fetchJSON(`/api/miners/${encodeURIComponent(minerId)}`, {
        method: "PATCH",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ managed: value }),
      });
      showToast(`Miner ${minerId} ${value ? "enabled" : "disabled"} for automation.`, "success");
      await fetchMiners(true);
    } catch (err) {
      checkbox.checked = !value;
      showToast(err.message, "error");
    } finally {
      checkbox.disabled = false;
    }
  };

  const updateModelMaxPreset = async (alias, value, select) => {
    select.disabled = true;
    try {
      const payload = { max_preset: value || null };
      await fetchJSON(`/api/models/${encodeURIComponent(alias)}`, {
        method: "PATCH",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(payload),
      });
      showToast(`Model ${alias} max preset saved.`, "success");
      await fetchModels();
      await fetchMiners(true);
    } catch (err) {
      showToast(err.message, "error");
    } finally {
      select.disabled = false;
    }
  };

  const fetchMinerStatuses = async (minerId, silent = false) => {
    try {
      const data = await fetchJSON(`/api/miners/${encodeURIComponent(minerId)}/statuses?limit=5`);
      return Array.isArray(data) ? data : [];
    } catch (err) {
      if (!silent) {
        showToast(err.message, "error");
      }
      throw err;
    }
  };

  const fetchMinerTelemetry = async (minerId, silent = false) => {
    try {
      const data = await fetchJSON(`/api/miners/${encodeURIComponent(minerId)}/telemetry?limit=60`);
      return Array.isArray(data) ? data : [];
    } catch (err) {
      if (!silent) {
        showToast(err.message, "error");
      }
      return [];
    }
  };

  const formatTempSpan = (min, max) => {
    const minNum = Number(min);
    const maxNum = Number(max);
    const minValid = Number.isFinite(minNum);
    const maxValid = Number.isFinite(maxNum);
    if (minValid && maxValid) {
      return `${minNum.toFixed(1)}°C – ${maxNum.toFixed(1)}°C`;
    }
    if (minValid) return `${minNum.toFixed(1)}°C`;
    if (maxValid) return `${maxNum.toFixed(1)}°C`;
    return "—";
  };

  const combineChainData = (statusChains = [], telemetrySnapshots = []) => {
    const results = [];
    const telemetryMap = new Map();

    telemetrySnapshots.forEach((snapshot) => {
      const identifier = (snapshot.chain?.identifier || "").toString().trim().toLowerCase();
      const key = identifier || `telemetry-${snapshot.id}`;
      if (!telemetryMap.has(key)) {
        telemetryMap.set(key, snapshot);
      }
    });

    const usedTelemetryKeys = new Set();

    statusChains.forEach((chain, index) => {
      const identifier = (chain?.identifier || "").toString().trim().toLowerCase();
      let telemetryMatch = null;
      if (identifier && telemetryMap.has(identifier)) {
        telemetryMatch = telemetryMap.get(identifier);
        usedTelemetryKeys.add(identifier);
      }
      results.push({
        key: identifier || `status-${index}`,
        label: chain?.identifier || `Chain ${index + 1}`,
        statusChain: chain || null,
        telemetrySnapshot: telemetryMatch,
      });
    });

    telemetryMap.forEach((snapshot, key) => {
      if (usedTelemetryKeys.has(key)) return;
      results.push({
        key,
        label: snapshot.chain?.identifier || snapshot.chain?.state || "Chain",
        statusChain: null,
        telemetrySnapshot: snapshot,
      });
    });

    return results;
  };

  const buildFanSummary = (fans = []) => {
    if (!Array.isArray(fans) || fans.length === 0) {
      return '<span class="muted">—</span>';
    }
    return renderFanBadges(fans);
  };

  const buildStatusHistoryTable = (statuses = []) => {
    if (!Array.isArray(statuses) || statuses.length === 0) {
      return '<p class="muted">No recent status snapshots.</p>';
    }

    const rows = statuses
      .map((status) => {
        const recorded = formatExactTime(status.recorded_at);
        const stateLabel = status.state || "unknown";
        const preset = status.preset || "—";
        const hashrate = formatHashrate(status.hashrate);
        const powerValue =
          status.power_usage !== null && status.power_usage !== undefined
            ? status.power_usage
            : status.power_consumption;
        const power = formatPower(powerValue);
        return `
          <tr>
            <td>${recorded}</td>
            <td>${stateLabel}</td>
            <td>${preset}</td>
            <td>${hashrate}</td>
            <td>${power}</td>
          </tr>
        `;
      })
      .join("");

    return `
      <div class="scroll-table">
        <table class="mini-table">
          <thead>
            <tr>
              <th>Recorded</th>
              <th>State</th>
              <th>Preset</th>
              <th>Hashrate</th>
              <th>Power</th>
            </tr>
          </thead>
          <tbody>${rows}</tbody>
        </table>
      </div>
    `;
  };

  const buildChainSections = (statusChains = [], telemetrySnapshots = []) => {
    const chains = combineChainData(statusChains, telemetrySnapshots);
    if (!chains.length) {
      return '<p class="muted">No chain data yet.</p>';
    }

    return chains
      .map((entry, index) => {
        const statusChain = entry.statusChain;
        const telemetryChain = entry.telemetrySnapshot?.chain || null;
        const chainLabel =
          statusChain?.identifier || telemetryChain?.identifier || `Chain ${index + 1}`;
        const state = statusChain?.state || telemetryChain?.state || "unknown";
        const hashrate = telemetryChain?.hashrate || statusChain?.hashrate;
        const pcbTemps = formatTempSpan(statusChain?.pcb_temp_min, statusChain?.pcb_temp_max);
        const chipTemps = formatTempSpan(statusChain?.chip_temp_min, statusChain?.chip_temp_max);
        const snapshotTime = entry.telemetrySnapshot?.recorded_at || null;

        const chips = (telemetryChain?.chips && telemetryChain.chips.length)
          ? telemetryChain.chips
          : statusChain?.chips || [];

        const chipsMarkup = chips.length
          ? `<div class="chip-grid">${chips
              .map((chip, chipIdx) => {
                const chipLabel = chip.identifier || `Chip ${chipIdx + 1}`;
                const chipHashrate = formatHashrate(chip.hashrate);
                const chipTemp = formatTemperature(chip.temperature);
                return `
                  <div class="chip-card">
                    <div class="chip-card__label">${chipLabel}</div>
                    <div><span class="muted">Hashrate:</span> ${chipHashrate}</div>
                    <div><span class="muted">Temp:</span> ${chipTemp}</div>
                  </div>
                `;
              })
              .join("")}</div>`
          : '<p class="muted">No chip telemetry available.</p>';

        return `
          <section class="modal-section chain-card">
            <header class="chain-card__header">
              <h3>${chainLabel}</h3>
              <span class="muted">${prettifyChainState(state)}</span>
            </header>
            <div class="chain-card__meta">
              <div><span class="muted">Hashrate:</span> ${formatHashrate(hashrate)}</div>
              <div><span class="muted">PCB Temp:</span> ${pcbTemps}</div>
              <div><span class="muted">Chip Temp:</span> ${chipTemps}</div>
              ${snapshotTime ? `<div class="muted small">Telemetry ${formatRelativeTime(snapshotTime)}</div>` : ""}
            </div>
            ${chipsMarkup}
          </section>
        `;
      })
      .join("");
  };

  const closeMinerModal = () => {
    if (!refs.minerModal) return;
    refs.minerModal.classList.add("hidden");
    refs.minerModal.setAttribute("aria-hidden", "true");
    document.body.classList.remove("modal-open");
    state.selectedMiner = null;
    if (refs.minerModalContent) {
      refs.minerModalContent.innerHTML = "";
    }
  };

  const populateMinerModal = (miner, statuses = [], telemetry = []) => {
    const latestStatus = statuses[0] || miner.latest_status || {};
    const enrichedMiner = { ...miner, latest_status: latestStatus };
    const { label: statusLabel, className: statusClass } = deriveMinerStatus(enrichedMiner);
    const powerValue =
      latestStatus.power_usage !== null && latestStatus.power_usage !== undefined
        ? latestStatus.power_usage
        : latestStatus.power_consumption;

    const summaryItems = [
      { label: "Status", value: `<span class="badge ${statusClass}">${statusLabel}</span>` },
      { label: "Model", value: miner.model?.name || "Unknown model" },
      { label: "IP", value: miner.ip || "—" },
      {
        label: "Hashrate",
        value: miner.online ? formatHashrate(latestStatus.hashrate) : "—",
      },
      { label: "Power", value: miner.online ? formatPower(powerValue) : "—" },
      { label: "Preset", value: miner.online ? latestStatus.preset || "—" : "—" },
      { label: "Uptime", value: miner.online ? formatDuration(latestStatus.uptime) : "—" },
      {
        label: "Temp Range",
        value: miner.online ? formatTemperatureRange(latestStatus.chains) : "—",
      },
      {
        label: "Last Update",
        value: latestStatus.recorded_at ? formatExactTime(latestStatus.recorded_at) : "—",
      },
    ];

    const summaryMarkup = summaryItems
      .map(
        (item) => `
          <div class="summary-item">
            <span class="muted">${item.label}</span>
            <div>${item.value}</div>
          </div>
        `
      )
      .join("");

    const fanSection = `
      <section class="modal-section">
        <h3>Fans</h3>
        <div class="fan-badges">${miner.online ? buildFanSummary(latestStatus.fans) : '<span class="muted">—</span>'}</div>
      </section>
    `;

    const chainSections = buildChainSections(latestStatus.chains || [], telemetry);
    const historyTable = buildStatusHistoryTable(statuses);

    refs.minerModalContent.innerHTML = `
      <header class="modal-header">
        <div>
          <h2>Miner ${miner.id}</h2>
          <p class="muted">Detailed overview</p>
        </div>
        <button id="miner-modal-close" class="modal-close" aria-label="Close" type="button">&times;</button>
      </header>
      <section class="modal-summary">${summaryMarkup}</section>
      ${fanSection}
      <section class="modal-section">
        <h3>Chains</h3>
        ${chainSections}
      </section>
      <section class="modal-section">
        <h3>Status History</h3>
        ${historyTable}
      </section>
    `;

    refs.minerModalClose = document.querySelector("#miner-modal-close");
    if (refs.minerModalClose) {
      refs.minerModalClose.addEventListener("click", closeMinerModal, { once: true });
    }
  };

  const openMinerModal = async (miner, silent = false) => {
    if (!refs.minerModal || !refs.minerModalContent) return;
    state.selectedMiner = miner.id;
    refs.minerModal.classList.remove("hidden");
    refs.minerModal.setAttribute("aria-hidden", "false");
    document.body.classList.add("modal-open");
    refs.minerModalContent.innerHTML = '<div class="modal-loading">Loading miner data…</div>';

    try {
      const [statuses, telemetry] = await Promise.all([
        fetchMinerStatuses(miner.id, silent),
        fetchMinerTelemetry(miner.id, true),
      ]);
      populateMinerModal(miner, statuses, telemetry);
    } catch (err) {
      refs.minerModalContent.innerHTML = `
        <div class="modal-error">
          <p>Failed to load miner details.</p>
          <p class="muted">${err.message}</p>
          <button class="modal-close" type="button">Close</button>
        </div>
      `;
      const retryClose = refs.minerModalContent.querySelector(".modal-close");
      if (retryClose) {
        retryClose.addEventListener("click", closeMinerModal, { once: true });
        refs.minerModalClose = retryClose;
      }
    }
  };

  const refreshMinerModal = async (miner) => {
    if (!miner) {
      closeMinerModal();
      return;
    }
    await openMinerModal(miner, true);
  };

  const renderMiners = () => {
    refs.minersTableBody.innerHTML = "";
    state.miners.forEach((miner) => {
      const latest = miner.latest_status || {};
      const { label: statusLabel, className: statusClass } = deriveMinerStatus(miner);
      const updated = latest.recorded_at ? formatRelativeTime(latest.recorded_at) : "—";
      const powerValue =
        latest.power_usage !== null && latest.power_usage !== undefined
          ? latest.power_usage
          : latest.power_consumption;
      const isOnline = Boolean(miner.online);
      const hashrateCell = isOnline ? formatHashrate(latest.hashrate) : "—";
      const powerCell = isOnline ? formatPower(powerValue) : "—";
      const tempCell = isOnline ? formatTemperatureRange(latest.chains) : "—";
      const fanMarkup = isOnline ? renderFanBadges(latest.fans) : '<span class="muted">—</span>';
      const presetCell = isOnline ? latest.preset || "—" : "—";
      const uptimeCell = isOnline ? formatDuration(latest.uptime) : "—";
      const chainSquares = isOnline
        ? `<div class="chain-squares">${renderChainSquares(latest.chains)}</div>`
        : '<span class="muted">—</span>';
      const tr = document.createElement("tr");
      tr.dataset.minerId = miner.id;
      tr.innerHTML = `
        <td>
          <div class="miner-overview">
            <div class="miner-title">
              <strong>${miner.model?.name || "Unknown model"}</strong>
              <span class="badge ${statusClass}">${statusLabel}</span>
            </div>
            <div class="muted">${miner.ip || "IP unavailable"}</div>
            <div class="muted small">${miner.id}</div>
            ${
              latest.recorded_at
                ? `<div class="muted small" title="${latest.recorded_at}">Updated ${updated}</div>`
                : ""
            }
          </div>
        </td>
        <td>${hashrateCell}</td>
        <td>${powerCell}</td>
        <td>${tempCell}</td>
        <td>
          <div class="fan-badges">${fanMarkup}</div>
        </td>
        <td>${presetCell}</td>
        <td>${uptimeCell}</td>
        <td>
          ${chainSquares}
        </td>
        <td>
          <label class="toggle">
            <input type="checkbox" class="managed-toggle" ${miner.managed ? "checked" : ""}>
            <span>${miner.managed ? "Managed" : "Manual"}</span>
          </label>
        </td>
      `;
      refs.minersTableBody.appendChild(tr);
    });
  };

  const renderModels = () => {
    refs.modelsContainer.innerHTML = "";
    state.models.forEach((model) => {
      const card = document.createElement("div");
      card.className = "card";
      const selectId = `preset-${model.alias}`;

      // Build preset power display
      let presetPowerHTML = "";
      if (model.presets_power && model.presets_power.length > 0) {
        presetPowerHTML = `
          <div class="preset-power-list">
            <h4>Preset Power Consumption</h4>
            <table class="preset-power-table">
              <thead><tr><th>Preset</th><th>Power</th></tr></thead>
              <tbody>
                ${model.presets_power
                  .map(
                    (pp) =>
                      `<tr><td>${pp.preset}</td><td>${pp.power_w ? formatPower(pp.power_w) : "—"}</td></tr>`
                  )
                  .join("")}
              </tbody>
            </table>
          </div>
        `;
      }

      card.innerHTML = `
        <h3>${model.name}</h3>
        <p class="muted">${model.alias}</p>
        <label for="${selectId}">Max Preset</label>
        <select id="${selectId}" data-alias="${model.alias}">
          <option value="">-- not set --</option>
          ${model.presets
            .map(
              (preset) =>
                `<option value="${preset}" ${
                  model.max_preset && model.max_preset === preset ? "selected" : ""
                }>${preset}</option>`
            )
            .join("")}
        </select>
        ${presetPowerHTML}
        <p class="muted small">Last seen: ${formatRelativeTime(model.created_at)}</p>
      `;
      refs.modelsContainer.appendChild(card);
    });
  };

  const renderBalanceStatus = () => {
    if (!state.balanceStatus) return;

    const status = state.balanceStatus;
    const statusText = document.getElementById("balance-status-text");
    const statusClass = `status-${status.status.toLowerCase().replace(/_/g, "-")}`;

    statusText.textContent = status.status.replace(/_/g, " ");
    statusText.className = `status-badge ${statusClass}`;

    document.getElementById("plant-generation").textContent = status.plant_generation_kw.toFixed(2);
    document.getElementById("container-consumption").textContent = status.plant_container_kw.toFixed(2);
    document.getElementById("available-power").textContent = status.available_power_kw.toFixed(2);
    document.getElementById("target-power").textContent = status.target_power_kw.toFixed(2);
    document.getElementById("current-consumption").textContent = status.current_consumption_w.toFixed(0);
    document.getElementById("managed-count").textContent = status.managed_miners_count;
    document.getElementById("safety-margin-input").value = status.safety_margin_percent;

    // Update last update timestamp
    const lastUpdateEl = document.getElementById("balance-last-update");
    if (lastUpdateEl && status.last_reading_at) {
      const relativeTime = formatRelativeTime(status.last_reading_at);
      lastUpdateEl.textContent = `Last update: ${relativeTime}`;
    }
  };

  const updateEnergyChart = () => {
    if (!state.energyChart || !state.plantData.length) return;

    const data = state.plantData.slice().reverse().slice(-30);
    const labels = data.map((d) => {
      const date = new Date(d.recorded_at);
      return date.toLocaleTimeString();
    });

    const generation = data.map((d) => d.total_generation);
    const consumption = data.map((d) => d.total_container_consumption);

    const safetyMargin = state.balanceStatus?.safety_margin_percent || 10;
    const target = data.map((d) => d.total_generation * (1 - safetyMargin / 100));

    state.energyChart.data.labels = labels;
    state.energyChart.data.datasets[0].data = generation;
    state.energyChart.data.datasets[1].data = consumption;
    state.energyChart.data.datasets[2].data = target;
    state.energyChart.update();
  };

  const initEnergyChart = () => {
    const canvas = document.getElementById("energy-chart");
    if (!canvas) return;

    const ctx = canvas.getContext("2d");
    state.energyChart = new Chart(ctx, {
      type: "line",
      data: {
        labels: [],
        datasets: [
          {
            label: "Generation (kW)",
            data: [],
            borderColor: "rgb(75, 192, 192)",
            backgroundColor: "rgba(75, 192, 192, 0.1)",
            tension: 0.1,
            fill: true,
          },
          {
            label: "Consumption (kW)",
            data: [],
            borderColor: "rgb(255, 99, 132)",
            backgroundColor: "rgba(255, 99, 132, 0.1)",
            tension: 0.1,
            fill: true,
          },
          {
            label: "Target (kW)",
            data: [],
            borderColor: "rgb(255, 205, 86)",
            backgroundColor: "rgba(255, 205, 86, 0.1)",
            borderDash: [5, 5],
            tension: 0.1,
            fill: false,
          },
        ],
      },
      options: {
        responsive: true,
        maintainAspectRatio: true,
        aspectRatio: 2.5,
        scales: {
          y: {
            beginAtZero: true,
            title: { display: true, text: "Power (kW)" },
          },
          x: {
            title: { display: true, text: "Time" },
          },
        },
        plugins: {
          legend: { position: "top" },
          title: { display: false },
        },
      },
    });
  };

  const renderBalanceEvents = () => {
    const tbody = document.querySelector("#balance-events-table tbody");
    if (!tbody) return;

    tbody.innerHTML = "";

    if (state.balanceEvents.length === 0) {
      tbody.innerHTML = '<tr><td colspan="6" style="text-align: center;">No balance events yet</td></tr>';
      return;
    }

    state.balanceEvents.forEach((event) => {
      const row = document.createElement("tr");

      const time = new Date(event.recorded_at).toLocaleTimeString();
      const oldPreset = event.old_preset || "—";
      const newPreset = event.new_preset || "—";
      const presetChange = `${oldPreset} → ${newPreset}`;

      const oldPower = event.old_power ? event.old_power.toFixed(0) : "—";
      const newPower = event.new_power ? event.new_power.toFixed(0) : "—";
      const powerChange = `${oldPower}W → ${newPower}W`;

      const statusClass = event.success ? "success" : "failure";
      const statusText = event.success ? "✓" : "✗";

      row.innerHTML = `
        <td>${time}</td>
        <td>${event.miner_id}</td>
        <td>${presetChange}</td>
        <td>${powerChange}</td>
        <td>${event.reason}</td>
        <td class="${statusClass}">${statusText}</td>
      `;

      tbody.appendChild(row);
    });
  };

  refs.minersTableBody.addEventListener("change", (event) => {
    if (event.target.matches(".managed-toggle")) {
      const row = event.target.closest("tr");
      if (!row) return;
      const minerId = row.dataset.minerId;
      updateManaged(minerId, event.target.checked, event.target);
    }
  });

  refs.minersTableBody.addEventListener("click", (event) => {
    const isInteractive =
      event.target.closest(".toggle") ||
      event.target.tagName === "INPUT" ||
      event.target.tagName === "BUTTON" ||
      event.target.tagName === "SELECT";
    if (isInteractive) return;
    const row = event.target.closest("tr");
    if (!row) return;
    const miner = state.miners.find((m) => m.id === row.dataset.minerId);
    if (!miner) return;
    openMinerModal(miner).catch(() => {});
  });

  if (refs.minerModal) {
    refs.minerModal.addEventListener("click", (event) => {
      if (
        event.target === refs.minerModal ||
        event.target.classList.contains("modal__backdrop")
      ) {
        closeMinerModal();
      }
    });
  }

  document.addEventListener("keydown", (event) => {
    if (event.key === "Escape" && refs.minerModal && !refs.minerModal.classList.contains("hidden")) {
      closeMinerModal();
    }
  });

  refs.modelsContainer.addEventListener("change", (event) => {
    if (event.target.tagName === "SELECT") {
      const alias = event.target.dataset.alias;
      const value = event.target.value.trim();
      updateModelMaxPreset(alias, value, event.target);
    }
  });

  if (refs.refreshMiners) {
    refs.refreshMiners.addEventListener("click", () => fetchMiners());
  }

  const safetyMarginButton = document.getElementById("update-safety-margin");
  if (safetyMarginButton) {
    safetyMarginButton.addEventListener("click", updateSafetyMargin);
  }

  // Initialize energy chart
  initEnergyChart();

  // Initial data fetch
  fetchMiners();
  fetchModels();
  fetchBalanceStatus();
  fetchPlantHistory();
  fetchBalanceEvents();

  // Auto-refresh all data
  setInterval(() => {
    fetchMiners(true).catch(() => {});
    fetchBalanceStatus().catch(() => {});
    fetchPlantHistory().catch(() => {});
    fetchBalanceEvents().catch(() => {});
  }, AUTO_REFRESH_MS);
});
