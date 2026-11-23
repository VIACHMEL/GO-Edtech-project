import { changedElements, mergeElements, sameScene, trackScene } from "./scene.js";

export function boardSocketURL(path, pageURL) {
    if (typeof path !== "string" || !path.startsWith("/") || path.startsWith("//")) return null;
    const url = new URL(path, pageURL);
    if (url.origin !== pageURL.origin || url.hash) return null;
    url.protocol = pageURL.protocol === "https:" ? "wss:" : "ws:";
    return url.href;
}

function readSnapshot(raw) {
    const frame = JSON.parse(raw);
    if (!Number.isSafeInteger(frame?.seq) || frame.seq < 0 ||
        typeof frame.can_edit !== "boolean" || !Array.isArray(frame.elements) ||
        frame.elements.some((element) => !element || typeof element.id !== "string")) {
        throw new Error("Invalid board snapshot");
    }
    return frame;
}

export function createBoardSession(url, { onScene, onStatus }) {
    const queued = new Map();
    let visible = trackScene([]);
    let socket;
    let inFlight = null;
    let receivedSnapshot = false;
    let canEdit = false;
    let lastSeq = -1;
    let snapshotTimeout;

    function status(text, canReconnect = false) {
        onStatus({
            text,
            canReconnect,
            canEdit,
            pending: Boolean(inFlight || queued.size),
        });
    }

    function sendNext() {
        if (inFlight || !queued.size || !canEdit || socket.readyState !== WebSocket.OPEN) return;
        inFlight = {
            op_id: crypto.randomUUID(),
            payload: { elements: [...queued.values()] },
        };
        queued.clear();
        socket.send(JSON.stringify(inFlight));
    }

    function renderSnapshot(frame) {
        const elements = mergeElements(frame.elements, inFlight?.payload.elements ?? [], queued.values());
        const next = trackScene(elements);
        if (!sameScene(visible, next)) {
            visible = next;
            onScene(elements);
        }
    }

    function receiveSnapshot(frame) {
        if (frame.seq < lastSeq) return;
        const firstSnapshot = !receivedSnapshot;
        receivedSnapshot = true;
        lastSeq = frame.seq;
        canEdit = frame.can_edit;
        if (frame.ack_op_id === inFlight?.op_id) inFlight = null;

        const lostPermission = !canEdit && Boolean(inFlight || queued.size);
        if (!canEdit) {
            inFlight = null;
            queued.clear();
        }
        renderSnapshot(frame);

        if (firstSnapshot && canEdit && inFlight) {
            // The connection may have closed before the acknowledgment arrived.
            socket.send(JSON.stringify(inFlight));
        }
        sendNext();

        if (frame.error) status("Сервер отклонил правку доски.");
        else if (lostPermission) status("Только просмотр. Сервер не принял последние правки.");
        else if (!canEdit) status("Только просмотр");
        else if (inFlight) status("Сохраняем доску…");
        else status("Доска синхронизирована");
    }

    function connect() {
        if (socket && socket.readyState < WebSocket.CLOSING) return;
        receivedSnapshot = false;
        lastSeq = -1;
        canEdit = false;
        status("Подключаем доску…");
        const connection = new WebSocket(url);
        socket = connection;
        let failure = "";
        snapshotTimeout = setTimeout(() => {
            failure = "Доска не ответила вовремя.";
            connection.close();
        }, 10000);
        connection.onmessage = ({ data }) => {
            if (socket !== connection) return;
            let frame;
            try {
                frame = readSnapshot(data);
            } catch {
                failure = "Сервер прислал некорректные данные доски.";
                connection.close();
                return;
            }
            clearTimeout(snapshotTimeout);
            receiveSnapshot(frame);
        };
        connection.onclose = () => {
            if (socket !== connection) return;
            clearTimeout(snapshotTimeout);
            canEdit = false;
            const message = inFlight || queued.size
                ? "Связь прервана. Есть правки без подтверждения."
                : "Доска отключена.";
            status(failure || message, true);
        };
    }

    function edit(elements) {
        if (!canEdit) return;
        const next = trackScene(elements);
        const changes = changedElements(visible, next);
        if (!changes.length) return;
        visible = next;
        for (const element of changes) queued.set(element.id, structuredClone(element));
        sendNext();
        status("Сохраняем доску…");
    }

    return {
        connect,
        edit,
        close() {
            const previous = socket;
            socket = null;
            clearTimeout(snapshotTimeout);
            previous?.close();
        },
    };
}
