import { BoardDraft, boardMessageBytes } from "./board-draft.js";
import { changedElements, indexElements, mergeElements, sameScene, trackScene } from "./scene.js";

const snapshotTimeout = 10000;
const idleTimeout = 25000;
const ackTimeout = 12000;
const batchDelay = 100;
const encoder = new TextEncoder();

const draftErrors = {
    element_conflict: "Объект уже изменил другой участник. Ваши правки остались в этой вкладке.",
    history_expired: "Доска изменилась, пока вы были отключены. Ваши правки остались в этой вкладке.",
    operation_rejected: "Сервер отклонил правку доски. Ваши правки остались в этой вкладке.",
    operation_too_large: "Правка превышает лимит 512 КиБ. Ваши правки остались в этой вкладке.",
    prepare_failed: "Не удалось загрузить изображение. Ваши правки остались в этой вкладке.",
};

const retryableErrors = new Set(["element_conflict", "history_expired", "prepare_failed"]);

export function boardSocketURL(path, pageURL) {
    if (typeof path !== "string" || !path.startsWith("/") || path.startsWith("//")) return null;
    const url = new URL(path, pageURL);
    if (url.origin !== pageURL.origin || url.hash) return null;
    url.protocol = pageURL.protocol === "https:" ? "wss:" : "ws:";
    return url.href;
}

function readFrame(raw) {
    const frame = JSON.parse(raw);
    const type = frame?.type ?? "snapshot";
    if (type === "access_revoked") return { type };
    const owner = frame?.draft_owner ?? null;
    if (!Number.isSafeInteger(frame?.seq) || frame.seq < 0 || typeof frame.can_edit !== "boolean" ||
        (owner !== null && (!Number.isSafeInteger(owner) || owner < 1)) ||
        !["snapshot", "delta", "heartbeat"].includes(type)) {
        throw new Error("Invalid board frame");
    }
    if (type !== "heartbeat" && (!Array.isArray(frame.elements) ||
        frame.elements.some((element) => !element || typeof element.id !== "string"))) {
        throw new Error("Invalid board elements");
    }
    return { ...frame, type, owner: owner === null ? null : String(owner) };
}

export function createBoardSession(url, { onScene, onStatus, prepareOperation }) {
    let visible = trackScene([]);
    let confirmed = new Map();
    let draft;
    let draftOwner = null;
    let lastSeq = -1;
    let canEdit = false;
    let socket;
    let connected = false;
    let stopped = false;
    let denied = false;
    let preparing;
    let reconnectCount = 0;
    let reconnectTimer;
    let idleTimer;
    let ackTimer;
    let flushTimer;
    let lastStatus;

    function status(text) {
        const pending = draft?.pending ?? false;
        const error = draft?.error;
        const canDiscardDraft = !denied && pending && (Boolean(error) || !canEdit);
        const canRetryDraft = connected && canEdit && retryableErrors.has(error);
        if (draft?.storageFailed && pending) {
            text += " Браузер не сохранил черновик. Оставьте вкладку открытой.";
        }
        const next = { text, canEdit: canEdit && !error, pending, canReconnect: !connected,
            canDiscardDraft, canRetryDraft, ready: Boolean(draft) && !denied, draftOwner,
            retryOverwritesObjects: error === "element_conflict" || error === "history_expired",
        };
        if (!lastStatus || Object.keys(next).some((key) => lastStatus[key] !== next[key])) {
            lastStatus = next;
            onStatus(next);
        }
    }

    function showCurrentStatus() {
        if (denied) status("Доступ к доске прекращён.");
        else if (!connected && draft?.pending) status("Переподключаем доску. Есть локальные правки.");
        else if (!connected) status("Подключаем доску…");
        else if (!canEdit && draft.pending) status("Только просмотр. Есть локальные правки.");
        else if (!canEdit) status("Только просмотр");
        else if (draft.error) status(draftErrors[draft.error]);
        else if (draft.pending) status("Сохраняем доску…");
        else status("Доска синхронизирована");
    }

    function render(force = false) {
        const elements = mergeElements(confirmed.values(), draft?.sentElements ?? [], draft?.queuedElements ?? []);
        const next = trackScene(elements);
        if (force || !sameScene(visible, next)) {
            visible = next;
            onScene(elements, { draftOwner });
        }
    }

    function canFlush() {
        return connected && draft.pending && !draft.error && (canEdit || draft.inFlight?.attempted);
    }

    async function flush() {
        flushTimer = undefined;
        if (!canFlush() || ackTimer || preparing) return;
        if (!draft.inFlight) draft.startOperation();
        const currentDraft = draft;
        const entry = currentDraft.inFlight;
        if (!entry.prepared && prepareOperation) {
            preparing = entry;
            try {
                const elements = await prepareOperation(entry.operation.payload.elements, { draftOwner });
                if (!Array.isArray(elements)) throw new Error("Invalid prepared elements");
                if (draft !== currentDraft || draft.inFlight !== entry || stopped) return;
                entry.operation.payload.elements = elements;
            } catch {
                if (draft === currentDraft && draft.inFlight === entry && !stopped) {
                    draft.error = "prepare_failed";
                    draft.persist();
                    showCurrentStatus();
                }
                return;
            } finally {
                if (preparing === entry) preparing = undefined;
            }
        }
        entry.prepared = true;
        const message = JSON.stringify(entry.operation);
        if (encoder.encode(message).length > boardMessageBytes) {
            currentDraft.error = "operation_too_large";
            currentDraft.persist();
            showCurrentStatus();
            return;
        }
        if (!canFlush() || socket.readyState !== WebSocket.OPEN) {
            currentDraft.persist();
            return;
        }
        entry.attempted = true;
        currentDraft.persist();
        const connection = socket;
        ackTimer = setTimeout(() => disconnect(connection), ackTimeout);
        try {
            connection.send(message);
        } catch {
            disconnect(connection);
        }
    }

    function scheduleFlush() {
        if (!flushTimer && canFlush()) {
            flushTimer = setTimeout(flush, batchDelay);
        }
    }

    function revokeAccess(connection) {
        denied = true;
        connected = false;
        canEdit = false;
        lastSeq = -1;
        draft = undefined;
        draftOwner = null;
        confirmed.clear();
        render(true);
        disconnect(connection);
    }

    function receive(frame, connection) {
        if (frame.type === "access_revoked" || (draft && draftOwner !== frame.owner)) {
            revokeAccess(connection);
            return;
        }
        if ((!connected && frame.type !== "snapshot") ||
            (connected && frame.seq < lastSeq) ||
            (frame.type === "delta" && frame.seq !== lastSeq + 1) ||
            (frame.type === "heartbeat" && frame.seq !== lastSeq)) {
            disconnect(connection);
            return;
        }

        const firstScene = !draft;
        if (firstScene) {
            draftOwner = frame.owner;
            draft = new BoardDraft(url, draftOwner);
        }
        if (frame.type === "snapshot") {
            confirmed = indexElements(frame.elements.filter((element) => !element.isDeleted));
        }
        if (frame.type === "delta") {
            for (const element of frame.elements) {
                if (element.isDeleted) confirmed.delete(element.id);
                else confirmed.set(element.id, element);
            }
        }
        canEdit = frame.can_edit;
        lastSeq = frame.seq;
        reconnectCount = 0;
        connected = true;

        if (draft.inFlight && frame.ack_op_id === draft.inFlight.operation.op_id) {
            clearTimeout(ackTimer);
            ackTimer = undefined;
            if (frame.error) {
                draft.error = Object.hasOwn(draftErrors, frame.error) ? frame.error : "operation_rejected";
                draft.persist();
            } else {
                draft.acknowledge(confirmed, lastSeq, frame.type === "delta");
            }
        }
        render(firstScene);
        scheduleFlush();
        showCurrentStatus();
    }

    function scheduleReconnect() {
        if (stopped || denied) return;
        const delay = Math.min(15000, 500 * 2 ** Math.min(reconnectCount++, 5));
        reconnectTimer = setTimeout(connect, delay * (0.8 + Math.random() * 0.2));
    }

    function clearTimers() {
        clearTimeout(idleTimer);
        clearTimeout(ackTimer);
        clearTimeout(flushTimer);
        ackTimer = undefined;
        flushTimer = undefined;
    }

    function disconnect(connection) {
        if (socket !== connection) return;
        socket = undefined;
        connected = false;
        clearTimers();
        if (connection.readyState < WebSocket.CLOSING) connection.close();
        showCurrentStatus();
        scheduleReconnect();
    }

    function connect() {
        if (stopped || (socket && socket.readyState < WebSocket.CLOSING)) return;
        clearTimeout(reconnectTimer);
        clearTimers();
        denied = false;
        connected = false;
        showCurrentStatus();
        let connection;
        try {
            connection = new WebSocket(url);
        } catch {
            scheduleReconnect();
            return;
        }
        socket = connection;
        idleTimer = setTimeout(() => disconnect(connection), snapshotTimeout);
        connection.onmessage = ({ data }) => {
            if (socket !== connection) return;
            let frame;
            try {
                frame = readFrame(data);
            } catch {
                disconnect(connection);
                return;
            }
            clearTimeout(idleTimer);
            idleTimer = setTimeout(() => disconnect(connection), idleTimeout);
            receive(frame, connection);
        };
        connection.onclose = ({ code }) => {
            if (socket !== connection) return;
            if (code === 1008) revokeAccess(connection);
            else disconnect(connection);
        };
    }

    function edit(elements) {
        if (!canEdit || draft.error) return;
        const next = trackScene(elements);
        const changes = changedElements(visible, next);
        if (!changes.length) return;
        visible = next;
        draft.add(changes, lastSeq);
        scheduleFlush();
        showCurrentStatus();
    }

    return {
        connect,
        edit,
        retryDraft() {
            if (!connected || !canEdit || !retryableErrors.has(draft.error)) return;
            draft.retry(lastSeq);
            scheduleFlush();
            showCurrentStatus();
        },
        discardDraft() {
            if (!draft?.pending || (!draft.error && canEdit)) return;
            clearTimeout(ackTimer);
            ackTimer = undefined;
            draft.discard();
            render();
            showCurrentStatus();
        },
        close() {
            stopped = true;
            clearTimeout(reconnectTimer);
            clearTimers();
            const previous = socket;
            socket = undefined;
            previous?.close();
        },
    };
}
