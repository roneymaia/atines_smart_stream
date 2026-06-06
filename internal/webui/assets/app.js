const rowsEl = document.getElementById("rows");
const emptyEl = document.getElementById("empty");
const dialog = document.getElementById("form-dialog");
const formError = document.getElementById("form-error");
let cameras = [];

async function api(method, path, body) {
  const res = await fetch(path, {
    method,
    headers: body ? { "Content-Type": "application/json" } : undefined,
    body: body ? JSON.stringify(body) : undefined,
  });
  const data = await res.json().catch(() => ({}));
  if (!res.ok) throw new Error(data.error || ("HTTP " + res.status));
  return data;
}

function statusLabel(s) { return s || "parada"; }

function render() {
  rowsEl.innerHTML = "";
  emptyEl.hidden = cameras.length > 0;
  for (const c of cameras) {
    const tr = document.createElement("tr");
    tr.innerHTML = `
      <td><span class="dot ${statusLabel(c.status)}"></span>${statusLabel(c.status)}</td>
      <td>${escapeHTML(c.name)}</td>
      <td>${escapeHTML(c.rtmp_url)}</td>
      <td><input type="checkbox" class="switch" ${c.enabled ? "checked" : ""} data-id="${c.id}"></td>
      <td class="actions">
        <button class="secondary" data-edit="${c.id}">Editar</button>
        <button class="danger" data-del="${c.id}">Excluir</button>
      </td>`;
    rowsEl.appendChild(tr);
  }
}

function escapeHTML(s) {
  return String(s).replace(/[&<>"']/g, (m) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[m]));
}

async function load() {
  cameras = await api("GET", "/api/cameras");
  render();
}

// Live status via SSE
const ev = new EventSource("/api/events");
ev.onmessage = (e) => {
  const st = JSON.parse(e.data);
  const cam = cameras.find((c) => c.id === st.id);
  if (cam) { cam.status = st.status; render(); }
};

// Open form (new)
document.getElementById("btn-new").onclick = () => openForm(null);

function openForm(cam) {
  formError.textContent = "";
  document.getElementById("form-title").textContent = cam ? "Editar câmera" : "Nova câmera";
  document.getElementById("f-id").value = cam?.id || "";
  document.getElementById("f-name").value = cam?.name || "";
  document.getElementById("f-rtsp").value = cam?.rtsp_url || "";
  document.getElementById("f-user").value = cam?.username || "";
  document.getElementById("f-pass").value = cam?.password || "";
  document.getElementById("f-rtmp").value = cam?.rtmp_url || "";
  document.getElementById("f-transcode").value = cam?.transcode_mode || "auto";
  document.getElementById("f-accel").value = cam?.accel_mode || "auto";
  dialog.showModal();
}

document.getElementById("btn-cancel").onclick = () => dialog.close();

document.getElementById("btn-save").onclick = async (e) => {
  e.preventDefault();
  const idVal = document.getElementById("f-id").value;
  const existing = cameras.find((c) => c.id === idVal);
  const payload = {
    id: idVal || "",
    name: document.getElementById("f-name").value,
    rtsp_url: document.getElementById("f-rtsp").value,
    username: document.getElementById("f-user").value,
    password: document.getElementById("f-pass").value,
    rtmp_url: document.getElementById("f-rtmp").value,
    transcode_mode: document.getElementById("f-transcode").value,
    accel_mode: document.getElementById("f-accel").value,
    enabled: existing?.enabled || false,
  };
  try {
    await api("POST", "/api/cameras", payload);
    dialog.close();
    await load();
  } catch (err) {
    formError.textContent = err.message;
  }
};

// Delegated row actions
rowsEl.addEventListener("click", async (e) => {
  const editId = e.target.getAttribute("data-edit");
  const delId = e.target.getAttribute("data-del");
  if (editId) openForm(cameras.find((c) => c.id === editId));
  if (delId && confirm("Excluir esta câmera?")) {
    try { await api("DELETE", "/api/cameras/" + delId); await load(); }
    catch (err) { alert("Erro ao excluir: " + err.message); }
  }
});

rowsEl.addEventListener("change", async (e) => {
  if (!e.target.classList.contains("switch")) return;
  const id = e.target.getAttribute("data-id");
  try {
    await api("POST", "/api/cameras/" + id + (e.target.checked ? "/enable" : "/disable"));
    await load();
  } catch (err) {
    alert("Erro: " + err.message);
    await load();
  }
});

load().catch((err) => alert("Erro ao carregar: " + err.message));
