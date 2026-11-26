const variant = JSON.parse(document.getElementById("variant-data").textContent);
const live = typeof variant.status === "string";
let canAnswer = live ? variant.can_answer : true;
const answersKey = "go-edtech:answers:" + variant.id + ":v1";
const answerInput = document.getElementById("solve-answer");
const solveStatus = document.getElementById("solve-status");
const previousButton = document.getElementById("solve-previous");
const nextButton = document.getElementById("solve-next");
const saveAnswersButton = document.getElementById("solve-save");
const finishButton = document.getElementById("solve-finish");
const finishDialog = document.getElementById("finish-dialog");
const boardContainer = document.getElementById("solve-board");
const boardNavigationStatus = document.getElementById("solve-board-navigation-status");
let currentIndex = 0;
let busy = false;
const answers = {};
const savedAnswers = new Map();
const revisions = new Map();

function restorePreviewDraft() {
    try {
        const savedText = localStorage.getItem(answersKey);
        if (!savedText) return;
        const saved = JSON.parse(savedText);
        if (!saved.answers || typeof saved.answers !== "object" || Array.isArray(saved.answers)) {
            throw new Error("Invalid answers");
        }
        for (const task of variant.tasks) {
            if (typeof saved.answers[task.id] === "string") {
                answers[task.id] = saved.answers[task.id].slice(0, 5000);
            }
        }
        if (Number.isInteger(saved.current_index) && saved.current_index >= 0 && saved.current_index < variant.tasks.length) {
            currentIndex = saved.current_index;
        }
        solveStatus.textContent = "Загружен черновик из этого браузера";
    } catch {
        solveStatus.textContent = "Локальный черновик недоступен. Можно решать без сохранения.";
    }
}

if (live) {
    for (const task of variant.tasks) {
        answers[task.id] = task.answer_text;
        savedAnswers.set(task.id, task.answer_text);
        revisions.set(task.id, task.answer_revision);
    }
    solveStatus.textContent = canAnswer ? "Ответы загружены с сервера" : "Ответы доступны для просмотра";
} else {
    solveStatus.textContent = "Ответы находятся только в открытой странице";
    restorePreviewDraft();
}

function updateNavigator() {
    for (const button of document.querySelectorAll("[data-task-index]")) {
        const index = Number(button.dataset.taskIndex);
        const task = variant.tasks[index];
        const answered = Boolean((answers[task.id] || "").trim());
        button.disabled = false;
        button.classList.toggle("answered", answered);
        button.classList.toggle("current", index === currentIndex);
        button.querySelector(".task-nav-state").textContent = answered ? "Ответ записан" : "Не отвечено";
        if (index === currentIndex) button.setAttribute("aria-current", "step");
        else button.removeAttribute("aria-current");
    }
}

function showTask(index) {
    const previousBoard = variant.tasks[currentIndex].board;
    const nextBoard = variant.tasks[index].board;
    if (previousBoard?.id !== nextBoard?.id && boardContainer.dataset.pending === "true") {
        boardNavigationStatus.textContent = "Перед сменой доски дождитесь сохранения рисунка.";
        boardNavigationStatus.hidden = false;
        return;
    }
    boardNavigationStatus.hidden = true;
    currentIndex = index;
    const task = variant.tasks[currentIndex];
    document.getElementById("solve-task-kind").textContent = [task.topic, task.kind].filter(Boolean).join(" · ");
    document.getElementById("solve-task-title").textContent = "Задание " + (index + 1) + ". " + task.title;
    document.getElementById("solve-task-prompt").textContent = task.prompt;
    answerInput.value = answers[task.id] || "";
    answerInput.disabled = !canAnswer || busy;
    previousButton.disabled = index === 0;
    nextButton.disabled = index === variant.tasks.length - 1;
    updateNavigator();
    boardContainer.dataset.currentIndex = index;
    boardContainer.dispatchEvent(new Event("solve:task-changed"));
}

answerInput.addEventListener("input", function () {
    answers[variant.tasks[currentIndex].id] = answerInput.value;
    solveStatus.textContent = "Есть несохранённые ответы";
    updateNavigator();
});
for (const button of document.querySelectorAll("[data-task-index]")) {
    button.addEventListener("click", function () {
        showTask(Number(button.dataset.taskIndex));
    });
}
previousButton.addEventListener("click", function () {
    if (currentIndex > 0) showTask(currentIndex - 1);
});
nextButton.addEventListener("click", function () {
    if (currentIndex < variant.tasks.length - 1) showTask(currentIndex + 1);
});
function savePreviewDraft() {
    try {
        localStorage.setItem(answersKey, JSON.stringify({answers: answers, current_index: currentIndex}));
        solveStatus.textContent = "Черновик сохранён только в этом браузере";
    } catch (error) {
        solveStatus.textContent = "Не удалось сохранить черновик. Ответы пока остаются в открытой странице.";
    }
}

async function attemptRequest(path, body) {
    const headers = { "X-CSRFToken": document.querySelector("[name=csrfmiddlewaretoken]").value };
    const options = { method: "POST", headers };
    if (body !== undefined) {
        headers["Content-Type"] = "application/json";
        options.body = JSON.stringify(body);
    }
    const response = await fetch("/solve/attempt/" + variant.id + path + "/", options);
    if (!response.ok) {
        const message = response.status === 409
            ? "Попытка или ответ изменились. Обновите страницу перед сохранением."
            : "Не удалось сохранить данные. Попробуйте ещё раз.";
        throw new Error(message);
    }
    return response.json();
}

async function saveAttemptAnswers() {
    for (const task of variant.tasks) {
        const text = answers[task.id];
        if (text === savedAnswers.get(task.id)) continue;
        const saved = await attemptRequest("/answer/" + task.id, {
            text, expected_revision: revisions.get(task.id),
        });
        revisions.set(task.id, saved.answer_revision);
        savedAnswers.set(task.id, text);
    }
}

function updateControls() {
    answerInput.disabled = !canAnswer || busy;
    saveAnswersButton.disabled = !canAnswer || busy;
    finishButton.disabled = !canAnswer || busy;
    document.getElementById("solve-confirm").disabled = busy;
    document.getElementById("solve-continue").disabled = busy;
}

saveAnswersButton.addEventListener("click", async function () {
    if (!live) {
        savePreviewDraft();
        return;
    }
    busy = true;
    updateControls();
    try {
        await saveAttemptAnswers();
        solveStatus.textContent = "Ответы сохранены";
    } catch (error) {
        solveStatus.textContent = error.message;
    } finally {
        busy = false;
        updateControls();
    }
});

function answeredCount() {
    let count = 0;
    for (const task of variant.tasks) {
        if ((answers[task.id] || "").trim()) count += 1;
    }
    return count;
}
finishButton.addEventListener("click", function () {
    document.getElementById("finish-summary").textContent = "Заполнено ответов: " + answeredCount() + " из " + variant.tasks.length + ".";
    finishDialog.showModal();
});
document.getElementById("solve-continue").addEventListener("click", function () {
    finishDialog.close();
});
finishDialog.addEventListener("cancel", function (event) {
    if (busy) event.preventDefault();
});
document.getElementById("solve-confirm").addEventListener("click", async function () {
    if (!live) {
        finishDialog.close();
        solveStatus.textContent = "Предпросмотр завершения: ответов " + answeredCount() + " из " + variant.tasks.length + ". На сервер ничего не отправлено. Черновик можно сохранить отдельно.";
        return;
    }
    if (boardContainer.dataset.pending === "true") {
        document.getElementById("finish-summary").textContent = "Дождитесь подтверждения сохранения рисунка перед сдачей.";
        return;
    }
    busy = true;
    updateControls();
    try {
        await saveAttemptAnswers();
        await attemptRequest("/submit");
        canAnswer = false;
        finishDialog.close();
        solveStatus.textContent = "Работа сдана";
    } catch (error) {
        document.getElementById("finish-summary").textContent = error.message;
    } finally {
        busy = false;
        updateControls();
    }
});
if (variant.tasks.length) {
    showTask(currentIndex);
    updateControls();
}
