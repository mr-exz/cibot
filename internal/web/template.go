package web

import "html/template"

var ticketTmpl = template.Must(template.New("ticket").Parse(ticketHTML))

const ticketHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>Submit a ticket</title>
<style>
*{box-sizing:border-box}
body{font-family:system-ui,sans-serif;max-width:560px;margin:60px auto;padding:0 20px;color:#1a1a2e;background:#f8fafc}
h1{font-size:22px;margin-bottom:24px;color:#0f172a}
label{display:block;font-size:13px;font-weight:600;color:#374151;margin-bottom:4px}
select,input,textarea{width:100%;padding:9px 12px;border:1px solid #d1d5db;border-radius:8px;font-size:14px;color:#111827;background:#fff;margin-bottom:18px}
select:disabled{background:#f3f4f6;color:#9ca3af}
textarea{min-height:120px;resize:vertical}
button[type=submit]{width:100%;padding:11px;background:#1d4ed8;color:#fff;border:none;border-radius:8px;font-size:15px;font-weight:600;cursor:pointer}
button[type=submit]:hover{background:#1e40af}
button[type=submit]:disabled{background:#93c5fd;cursor:not-allowed}
.hidden{display:none}
.section{background:#fff;border:1px solid #e5e7eb;border-radius:12px;padding:24px;margin-bottom:20px}
.section-title{font-size:13px;font-weight:700;text-transform:uppercase;letter-spacing:.05em;color:#6b7280;margin:0 0 16px}
</style>
</head>
<body>
<h1>Submit a ticket</h1>
<form method="POST" action="/ticket" id="form">
  <input type="hidden" name="group_id" id="group_id_hidden">
  <input type="hidden" name="thread_id" id="thread_id_hidden" value="0">
  <input type="hidden" name="category_id" id="category_id_hidden">
  <input type="hidden" name="type_id" id="type_id_hidden">

  <div class="section">
    <div class="section-title">Context</div>
    <label for="group_sel">Group</label>
    <select id="group_sel" required>
      <option value="">— select group —</option>
      {{range .Groups}}
      <option value="{{.ID}}">{{.Title}}</option>
      {{end}}
    </select>

    <div id="topic_row" class="hidden">
      <label for="topic_sel">Topic</label>
      <select id="topic_sel">
        <option value="0">— whole group (no topic) —</option>
      </select>
    </div>

    <label for="category_sel">Category</label>
    <select id="category_sel" disabled required>
      <option value="">— select category —</option>
    </select>

    <label for="type_sel">Request type</label>
    <select id="type_sel" disabled required>
      <option value="">— select type —</option>
    </select>

    <label for="priority_sel">Priority</label>
    <select id="priority_sel" name="priority">
      <option value="0">No priority</option>
      <option value="1">Urgent</option>
      <option value="2">High</option>
      <option value="3">Medium</option>
      <option value="4">Low</option>
    </select>
  </div>

  <div class="section">
    <div class="section-title">Reporter</div>
    <label for="reporter_name">Your name</label>
    <input type="text" id="reporter_name" name="reporter_name" placeholder="Jane Smith" required>
  </div>

  <div class="section">
    <div class="section-title">Message</div>
    <label for="msg_url">Message link (optional)</label>
    <input type="url" id="msg_url" name="msg_url" placeholder="https://t.me/c/...">

    <label for="msg_body">Message body</label>
    <textarea id="msg_body" name="msg_body" placeholder="Describe the issue..." required></textarea>
  </div>

  <button type="submit" id="submit_btn" disabled>Submit ticket</button>
</form>

<script>
const groupSel = document.getElementById('group_sel');
const topicRow = document.getElementById('topic_row');
const topicSel = document.getElementById('topic_sel');
const categorySel = document.getElementById('category_sel');
const typeSel = document.getElementById('type_sel');
const submitBtn = document.getElementById('submit_btn');

function resetSelect(sel, placeholder) {
  sel.innerHTML = '<option value="">' + placeholder + '</option>';
  sel.disabled = true;
}

function checkReady() {
  const ready = categorySel.value && typeSel.value;
  submitBtn.disabled = !ready;
  document.getElementById('group_id_hidden').value = groupSel.value;
  document.getElementById('category_id_hidden').value = categorySel.value;
  document.getElementById('type_id_hidden').value = typeSel.value;
}

async function loadTopics(groupID) {
  resetSelect(categorySel, '— select category —');
  resetSelect(typeSel, '— select type —');
  checkReady();

  const res = await fetch('/api/topics?group=' + groupID);
  const topics = await res.json();

  topicSel.innerHTML = '<option value="0">— whole group (no topic) —</option>';
  if (topics && topics.length > 0) {
    topics.sort((a, b) => a.name.localeCompare(b.name));
    topics.forEach(t => {
      const o = document.createElement('option');
      o.value = t.id;
      o.textContent = t.name;
      topicSel.appendChild(o);
    });
    topicRow.classList.remove('hidden');
  } else {
    topicRow.classList.add('hidden');
    document.getElementById('thread_id_hidden').value = '0';
  }

  await loadCategories(groupID, 0);
}

async function loadCategories(groupID, topicID) {
  resetSelect(categorySel, '— select category —');
  resetSelect(typeSel, '— select type —');
  checkReady();

  document.getElementById('thread_id_hidden').value = topicID;

  const res = await fetch('/api/categories?group=' + groupID + '&topic=' + topicID);
  const cats = await res.json();
  if (!cats || cats.length === 0) return;

  cats.forEach(c => {
    const o = document.createElement('option');
    o.value = c.id;
    o.textContent = (c.emoji ? c.emoji + ' ' : '') + c.name;
    categorySel.appendChild(o);
  });
  categorySel.disabled = false;
}

async function loadTypes(categoryID) {
  resetSelect(typeSel, '— select type —');
  checkReady();

  const res = await fetch('/api/types?category=' + categoryID);
  const types = await res.json();
  if (!types || types.length === 0) return;

  types.forEach(t => {
    const o = document.createElement('option');
    o.value = t.id;
    o.textContent = t.name;
    typeSel.appendChild(o);
  });
  typeSel.disabled = false;
}

groupSel.addEventListener('change', () => {
  if (!groupSel.value) {
    topicRow.classList.add('hidden');
    resetSelect(categorySel, '— select category —');
    resetSelect(typeSel, '— select type —');
    checkReady();
    return;
  }
  loadTopics(groupSel.value);
});

topicSel.addEventListener('change', () => {
  loadCategories(groupSel.value, topicSel.value);
});

categorySel.addEventListener('change', () => {
  if (!categorySel.value) {
    resetSelect(typeSel, '— select type —');
    checkReady();
    return;
  }
  loadTypes(categorySel.value);
});

typeSel.addEventListener('change', checkReady);
</script>
</body>
</html>`
