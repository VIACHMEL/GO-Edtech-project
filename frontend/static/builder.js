const builderData = JSON.parse(document.getElementById("builder-data").textContent);
const titleInput = document.getElementById("variant-title");
const selectedList = document.getElementById("selected-tasks");
const builderStatus = document.getElementById("builder-status");
const publishForm = document.getElementById("builder-publish");
const maxAttempts = publishForm.elements.max_attempts;
const saveButton = document.getElementById("builder-save");
const resetButton = document.getElementById("builder-reset");
const importButton = document.getElementById("builder-import");
const draftKey = "go-edtech:builder:" + builderData.user_id + ":v2";
let worksheetId = builderData.worksheet_id;
let revision = builderData.revision;
let selected = structuredClone(builderData.selected_tasks);
let busy = false;
let importedLegacy = false;
maxAttempts.value = builderData.max_attempts;

function snapshot() {
    return JSON.stringify({title: titleInput.value, attempts: maxAttempts.value, selected});
}
let savedSnapshot = snapshot();
function dirty() { return snapshot() !== savedSnapshot; }
function changed() { builderStatus.textContent = dirty() ? "Есть несохранённые изменения" : "Изменений нет"; }
function taskButton(label, action, index, disabled) {
    const button = document.createElement("button");
    button.type = "button";
    button.className = "secondary-button";
    button.textContent = label;
    button.dataset.action = action;
    button.dataset.index = index;
    button.disabled = disabled;
    return button;
}
function renderSelectedTasks() {
    selectedList.replaceChildren();
    selected.forEach((task, index) => {
        const row = document.createElement("li");
        row.className = "selected-task";
        const content = document.createElement("div");
        const title = document.createElement("h3");
        title.textContent = (index + 1) + ". " + task.subject + " · № " + task.task_number;
        const prompt = document.createElement("p");
        prompt.textContent = task.content_text;
        const controls = document.createElement("div");
        controls.className = "task-controls";
        controls.append(taskButton("Выше", "up", index, index === 0 || busy),
            taskButton("Ниже", "down", index, index === selected.length - 1 || busy),
            taskButton("Удалить", "remove", index, busy));
        content.append(title, prompt, controls);
        row.append(content);
        selectedList.append(row);
    });
    document.getElementById("builder-empty").hidden = selected.length !== 0;
    document.getElementById("builder-summary").textContent = selected.length + " заданий";
    for (const button of document.querySelectorAll("[data-add-task]")) {
        const added = selected.some(task => String(task.id) === button.dataset.addTask);
        button.disabled = busy || added;
        button.textContent = added ? "Добавлено" : "Добавить";
    }
    saveButton.disabled = busy;
    resetButton.disabled = busy;
    importButton.disabled = busy;
    titleInput.disabled = busy;
    maxAttempts.disabled = busy;
    publishForm.querySelector("button").disabled = busy || !selected.length;
}
function validateSelection() {
    const subject = selected[0]?.subject;
    if (!titleInput.value.trim() || titleInput.value.trim().length > 120 || !subject ||
        selected.length > 100 || selected.some(task => task.subject !== subject ||
            !Number.isSafeInteger(task.task_version_id) || task.task_version_id <= 0) ||
        !maxAttempts.checkValidity()) {
        throw new Error("Укажите название, лимит попыток и от 1 до 100 заданий одного предмета.");
    }
    return subject;
}
async function saveDraft(publish) {
    if (busy) return;
    try {
        const subject = validateSelection();
        const payload = { worksheet_id: worksheetId, expected_revision: revision,
            title: titleInput.value.trim(), subject, max_attempts: Number(maxAttempts.value), publish,
            items: selected.map(task => ({task_version_id: task.task_version_id,
                max_score: task.max_score ?? "1.00", settings: task.settings ?? {}})) };
        busy = true;
        renderSelectedTasks();
        builderStatus.textContent = "Сохраняем вариант…";
        const response = await fetch("/teacher/builder/save/", {
            method: "POST", headers: {"Content-Type": "application/json",
                "X-CSRFToken": publishForm.querySelector("[name=csrfmiddlewaretoken]").value},
            body: JSON.stringify(payload),
        });
        if (!response.ok) {
            if (response.status === 409) throw new Error("Черновик изменён в другой вкладке или уже опубликован. Ваши изменения остались на странице; откройте сохранённый вариант отдельно.");
            throw new Error("Не удалось сохранить вариант. Изменения остались на странице; попробуйте ещё раз.");
        }
        const saved = await response.json();
        worksheetId = saved.id;
        revision = saved.revision;
        titleInput.value = saved.title;
        savedSnapshot = snapshot();
        if (importedLegacy) {
            try { localStorage.removeItem(draftKey); } catch { /* The database save succeeded. */ }
            importedLegacy = false;
            importButton.hidden = true;
        }
        if (publish) {
            busy = false;
            window.location.assign("/teacher/builder/?worksheet=" + worksheetId);
            return;
        }
        const url = new URL(window.location.href);
        url.searchParams.set("draft", worksheetId);
        url.searchParams.delete("worksheet");
        window.history.replaceState(null, "", url);
        document.getElementById("builder-filter-draft").value = worksheetId;
        builderStatus.textContent = "Черновик сохранён на сервере";
    } catch (error) {
        builderStatus.textContent = error.message;
    } finally {
        busy = false;
        renderSelectedTasks();
    }
}
saveButton.addEventListener("click", () => saveDraft(false));
publishForm.addEventListener("submit", event => { event.preventDefault(); saveDraft(true); });
for (const button of document.querySelectorAll("[data-add-task]")) {
    button.addEventListener("click", () => {
        if (busy) return;
        const task = builderData.tasks.find(task => String(task.id) === button.dataset.addTask);
        if (task && !selected.some(item => item.id === task.id)) {
            selected.push({...task, task_version_id: task.current_version_id, max_score: "1.00", settings: {}});
            renderSelectedTasks(); changed();
        }
    });
}
selectedList.addEventListener("click", event => {
    const button = event.target.closest("button[data-action]");
    if (!button || busy) return;
    const index = Number(button.dataset.index);
    if (button.dataset.action === "remove") selected.splice(index, 1);
    else {
        const next = index + (button.dataset.action === "up" ? -1 : 1);
        if (next < 0 || next >= selected.length) return;
        [selected[index], selected[next]] = [selected[next], selected[index]];
    }
    renderSelectedTasks(); changed();
});
titleInput.addEventListener("input", changed);
maxAttempts.addEventListener("input", changed);
resetButton.addEventListener("click", () => {
    const saved = JSON.parse(savedSnapshot);
    titleInput.value = saved.title;
    maxAttempts.value = saved.attempts;
    selected = saved.selected;
    importedLegacy = false;
    renderSelectedTasks(); changed();
});
// Old browser drafts are offered explicitly and removed only after a successful server save.
try { importButton.hidden = !localStorage.getItem(draftKey); } catch { importButton.hidden = true; }
importButton.addEventListener("click", () => {
    if (busy || (dirty() && !window.confirm("Заменить несохранённые изменения старым черновиком из браузера?"))) return;
    try {
        const saved = JSON.parse(localStorage.getItem(draftKey));
        if (!saved || typeof saved.title !== "string" || !Array.isArray(saved.task_ids)) throw new Error();
        const candidates = [...(Array.isArray(saved.tasks) ? saved.tasks : []), ...builderData.tasks];
        const restored = saved.task_ids.map(id => candidates.find(task => String(task?.id) === String(id)));
        if (!restored.length || restored.some(task => !task || !Number.isSafeInteger(task.current_version_id))) throw new Error();
        selected = restored.map(task => ({...task, task_version_id: task.current_version_id, max_score: "1.00", settings: {}}));
        titleInput.value = saved.title.slice(0, 120);
        importedLegacy = true;
        renderSelectedTasks();
        builderStatus.textContent = "Старый черновик открыт. Нажмите «Сохранить черновик» для записи на сервер.";
    } catch { builderStatus.textContent = "Старый черновик не удалось прочитать. Он не удалён."; }
});
window.addEventListener("beforeunload", event => {
    if (dirty() || busy) { event.preventDefault(); event.returnValue = ""; }
});
document.getElementById("bank-search").addEventListener("input", event => {
    const query = event.target.value.trim().toLocaleLowerCase("ru");
    let count = 0;
    for (const item of document.querySelectorAll(".bank-item")) {
        item.hidden = !item.dataset.search.toLocaleLowerCase("ru").includes(query);
        if (!item.hidden) count++;
    }
    document.getElementById("bank-empty").hidden = count !== 0;
});
builderStatus.textContent = worksheetId ? "Черновик загружен с сервера" : "Черновик ещё не сохранён";
renderSelectedTasks();
