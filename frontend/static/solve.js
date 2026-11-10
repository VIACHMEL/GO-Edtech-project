const variant = JSON.parse(document.getElementById("variant-data").textContent);
const answersKey = "go-edtech:answers:" + variant.id + ":v1";
const answerInput = document.getElementById("solve-answer");
const solveStatus = document.getElementById("solve-status");
const previousButton = document.getElementById("solve-previous");
const nextButton = document.getElementById("solve-next");
const saveAnswersButton = document.getElementById("solve-save");
const finishButton = document.getElementById("solve-finish");
const finishDialog = document.getElementById("finish-dialog");
let currentIndex = 0;
let answers = {};

try {
    const savedText = localStorage.getItem(answersKey);
    if (savedText) {
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
    }
} catch (error) {
    solveStatus.textContent = "Локальный черновик недоступен. Можно решать без сохранения.";
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
    currentIndex = index;
    const task = variant.tasks[currentIndex];
    document.getElementById("solve-task-kind").textContent = task.topic + " · " + task.kind;
    document.getElementById("solve-task-title").textContent = "Задание " + (index + 1) + ". " + task.title;
    document.getElementById("solve-task-prompt").textContent = task.prompt;
    answerInput.value = answers[task.id] || "";
    answerInput.disabled = false;
    previousButton.disabled = index === 0;
    nextButton.disabled = index === variant.tasks.length - 1;
    updateNavigator();
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
saveAnswersButton.disabled = false;
saveAnswersButton.addEventListener("click", function () {
    try {
        localStorage.setItem(answersKey, JSON.stringify({answers: answers, current_index: currentIndex}));
        solveStatus.textContent = "Черновик сохранён только в этом браузере";
    } catch (error) {
        solveStatus.textContent = "Не удалось сохранить черновик. Ответы пока остаются в открытой странице.";
    }
});

function answeredCount() {
    let count = 0;
    for (const task of variant.tasks) {
        if ((answers[task.id] || "").trim()) count += 1;
    }
    return count;
}
finishButton.disabled = false;
finishButton.addEventListener("click", function () {
    document.getElementById("finish-summary").textContent = "Заполнено ответов: " + answeredCount() + " из " + variant.tasks.length + ".";
    finishDialog.showModal();
});
document.getElementById("solve-continue").addEventListener("click", function () {
    finishDialog.close();
});
document.getElementById("solve-confirm").addEventListener("click", function () {
    finishDialog.close();
    solveStatus.textContent = "Предпросмотр завершения: ответов " + answeredCount() + " из " + variant.tasks.length + ". На сервер ничего не отправлено. Черновик можно сохранить отдельно.";
});
showTask(currentIndex);
