import { boardSocketURL } from "./board-session.js";

const container = document.getElementById("solve-board");
const navigationStatus = document.getElementById("solve-board-navigation-status");
const variant = JSON.parse(document.getElementById("variant-data").textContent);
const pageURL = new URL(location.href);
let editor;
let selection = 0;

function setPending(pending) {
    container.dataset.pending = String(pending);
    if (!pending) navigationStatus.hidden = true;
}

function showStatus(text) {
    editor?.close();
    editor = null;
    const label = document.createElement("p");
    label.className = "field-hint";
    label.setAttribute("role", "status");
    label.textContent = text;
    container.replaceChildren(label);
}

async function showBoard() {
    const currentSelection = ++selection;
    const task = variant.tasks[Number(container.dataset.currentIndex)];
    const board = task?.board;
    const validID = Number.isSafeInteger(board?.id) && board.id > 0;
    const url = validID ? boardSocketURL(board.ws_path, pageURL) : null;
    if (!url) {
        showStatus("Для этого задания доска пока недоступна.");
        return;
    }

    if (!editor) {
        showStatus("Загружаем редактор доски…");
        try {
            const { mountEditor } = await import("./editor.jsx");
            if (currentSelection !== selection) return;
            editor = mountEditor(container, setPending);
        } catch {
            if (currentSelection === selection) showStatus("Не удалось загрузить редактор доски.");
            return;
        }
    }
    editor.show(board, url);
}

container.addEventListener("solve:task-changed", showBoard);
showBoard();
