const builderData = JSON.parse(document.getElementById("builder-data").textContent);
for (const task of builderData.tasks) task.id = String(task.id);
const draftKey = "go-edtech:builder:" + builderData.user_id + ":v2";
const titleInput = document.getElementById("variant-title");
const selectedList = document.getElementById("selected-tasks");
const builderStatus = document.getElementById("builder-status");
let selectedIds = builderData.selected_ids.slice();

function findTask(id) {
    for (const task of builderData.tasks) {
        if (task.id === id) return task;
    }
    return null;
}

function taskButton(label, action, id, disabled) {
    const button = document.createElement("button");
    button.type = "button";
    button.className = "secondary-button task-action";
    button.textContent = label;
    button.dataset.action = action;
    button.dataset.task = id;
    button.disabled = disabled;
    return button;
}

function renderSelectedTasks() {
    selectedList.replaceChildren();
    for (let index = 0; index < selectedIds.length; index += 1) {
        const task = findTask(selectedIds[index]);
        const row = document.createElement("li");
        row.className = "selected-task";
        const number = document.createElement("span");
        number.className = "task-number";
        number.textContent = index + 1;
        const content = document.createElement("div");
        const title = document.createElement("h3");
        title.textContent = task.subject + " · № " + task.task_number;
        const detail = document.createElement("p");
        detail.textContent = task.task_type || "";
        const prompt = document.createElement("p");
        prompt.textContent = task.content_text;
        const controls = document.createElement("div");
        controls.className = "task-controls";
        controls.append(
            taskButton("Выше", "up", task.id, index === 0),
            taskButton("Ниже", "down", task.id, index === selectedIds.length - 1),
            taskButton("Удалить", "remove", task.id, false)
        );
        content.append(title, detail, prompt, controls);
        row.append(number, content);
        selectedList.append(row);
    }
    document.getElementById("builder-empty").hidden = selectedIds.length !== 0;
    document.getElementById("builder-summary").textContent = selectedIds.length + " заданий";
    for (const button of document.querySelectorAll("[data-add-task]")) {
        const selected = selectedIds.includes(button.dataset.addTask);
        button.disabled = selected;
        button.textContent = selected ? "Добавлено" : "Добавить";
    }
}

try {
    const savedText = localStorage.getItem(draftKey);
    if (savedText) {
        const saved = JSON.parse(savedText);
        if (typeof saved.title !== "string" || !Array.isArray(saved.task_ids)) {
            throw new Error("Invalid draft");
        }
        // Keep selected tasks when the next server filter returns a different set.
        if (Array.isArray(saved.tasks)) {
            for (const task of saved.tasks) {
                if (task && typeof task.id === "string" && saved.task_ids.includes(task.id) && !findTask(task.id)) {
                    builderData.tasks.push(task);
                }
            }
        }
        const validIds = [];
        for (const id of saved.task_ids) {
            if (findTask(id) && !validIds.includes(id)) validIds.push(id);
        }
        selectedIds = validIds;
        titleInput.value = saved.title.slice(0, 120);
        builderStatus.textContent = "Загружен черновик из этого браузера";
    }
} catch (error) {
    builderStatus.textContent = "Локальный черновик недоступен. Можно работать без сохранения.";
}
renderSelectedTasks();

for (const button of document.querySelectorAll("[data-add-task]")) {
    button.addEventListener("click", function () {
        const id = button.dataset.addTask;
        if (findTask(id) && !selectedIds.includes(id)) {
            selectedIds.push(id);
            renderSelectedTasks();
            builderStatus.textContent = "Есть несохранённые изменения";
        }
    });
}

selectedList.addEventListener("click", function (event) {
    const button = event.target.closest("button[data-action]");
    if (!button) return;
    const index = selectedIds.indexOf(button.dataset.task);
    if (index < 0) return;
    if (button.dataset.action === "remove") {
        selectedIds.splice(index, 1);
    } else {
        const nextIndex = button.dataset.action === "up" ? index - 1 : index + 1;
        if (nextIndex < 0 || nextIndex >= selectedIds.length) return;
        const otherId = selectedIds[nextIndex];
        selectedIds[nextIndex] = selectedIds[index];
        selectedIds[index] = otherId;
    }
    renderSelectedTasks();
    builderStatus.textContent = "Есть несохранённые изменения";
});

titleInput.addEventListener("input", function () {
    builderStatus.textContent = "Есть несохранённые изменения";
});

const saveButton = document.getElementById("builder-save");
saveButton.disabled = false;
saveButton.addEventListener("click", function () {
    if (!titleInput.value.trim()) {
        titleInput.focus();
        builderStatus.textContent = "Введите название варианта";
        return;
    }
    try {
        localStorage.setItem(draftKey, JSON.stringify({title: titleInput.value.trim(), task_ids: selectedIds, tasks: selectedIds.map(findTask)}));
        builderStatus.textContent = "Черновик сохранён в этом браузере";
    } catch (error) {
        builderStatus.textContent = "Не удалось сохранить черновик в браузере";
    }
});

const resetButton = document.getElementById("builder-reset");
resetButton.disabled = false;
resetButton.addEventListener("click", function () {
    try {
        localStorage.removeItem(draftKey);
    } catch (error) {
        builderStatus.textContent = "Не удалось удалить локальный черновик";
        return;
    }
    selectedIds = builderData.selected_ids.slice();
    titleInput.value = builderData.draft_title;
    renderSelectedTasks();
    builderStatus.textContent = "Черновик очищен";
});

document.getElementById("bank-search").addEventListener("input", function (event) {
    const query = event.target.value.trim().toLocaleLowerCase("ru");
    let visibleCount = 0;
    for (const item of document.querySelectorAll(".bank-item")) {
        item.hidden = !item.dataset.search.toLocaleLowerCase("ru").includes(query);
        if (!item.hidden) visibleCount += 1;
    }
    document.getElementById("bank-empty").hidden = visibleCount !== 0;
});
