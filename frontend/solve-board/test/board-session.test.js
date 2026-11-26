import assert from "node:assert/strict";
import { beforeEach, afterEach, test } from "node:test";
import { boardSocketURL, createBoardSession } from "../src/board-session.js";

const address = "wss://class.example/ws/boards/7";
const initialSocket = Object.getOwnPropertyDescriptor(globalThis, "WebSocket");
const initialStorage = Object.getOwnPropertyDescriptor(globalThis, "sessionStorage");

class Socket {
    static OPEN = 1;
    static CLOSING = 2;
    static opened = [];
    readyState = Socket.OPEN;
    sent = [];

    constructor(url) {
        assert.equal(url, address);
        Socket.opened.push(this);
    }

    send(raw) {
        this.sent.push(JSON.parse(raw));
    }

    close() {
        if (this.readyState >= Socket.CLOSING) return;
        this.readyState = 3;
        this.onclose({ code: 1006 });
    }

    receive(seq, elements = [], options = {}) {
        this.onmessage({ data: JSON.stringify({
            seq, elements, can_edit: true, draft_owner: 32, type: "snapshot", ...options,
        }) });
    }
}

function rectangle(id, version = 1) {
    return { id, version, type: "rectangle" };
}

const sessions = [];
function open(options) {
    const client = { elements: [], status: null };
    client.session = createBoardSession(address, {
        onScene(elements) { client.elements = elements; },
        onStatus(status) { client.status = status; },
        ...options,
    });
    sessions.push(client.session);
    client.session.connect();
    client.socket = Socket.opened.at(-1);
    return client;
}

beforeEach(({ mock }) => {
    const storage = new Map();
    Object.defineProperty(globalThis, "WebSocket", { value: Socket, configurable: true });
    Object.defineProperty(globalThis, "sessionStorage", { configurable: true, value: {
        getItem(key) { return storage.get(key) ?? null; },
        setItem(key, value) { storage.set(key, value); },
        removeItem(key) { storage.delete(key); },
    } });
    mock.timers.enable({ apis: ["setTimeout"] });
});

afterEach(() => {
    for (const session of sessions) session.close();
    sessions.length = 0;
    Socket.opened.length = 0;
    if (initialSocket) Object.defineProperty(globalThis, "WebSocket", initialSocket);
    else delete globalThis.WebSocket;
    if (initialStorage) Object.defineProperty(globalThis, "sessionStorage", initialStorage);
    else delete globalThis.sessionStorage;
});

test("constructs a same-origin socket address", () => {
    const page = new URL("https://class.example/solve/");
    assert.equal(boardSocketURL("/ws/boards/7", page), address);
    assert.equal(boardSocketURL("//another.example/ws/boards/7", page), null);
    assert.equal(boardSocketURL("/\\another.example/ws/boards/7", page), null);
});

test("coalesces edits, keeps a draft until ACK, and receives deletion deltas", ({ mock }) => {
    const client = open();
    const first = rectangle("first");
    client.socket.receive(4, [first]);
    client.session.edit([first, rectangle("local")]);
    client.session.edit([first, rectangle("local", 2)]);
    assert.equal(client.status.pending, true);
    assert.deepEqual(client.socket.sent, []);
    mock.timers.tick(100);
    const [operation] = client.socket.sent;
    assert.equal(operation.base_seq, 4);
    assert.deepEqual(operation.payload.elements, [rectangle("local", 2)]);
    client.socket.receive(5, operation.payload.elements, { type: "delta", ack_op_id: operation.op_id });
    assert.equal(client.status.pending, false);
    client.socket.receive(6, [{ ...first, isDeleted: true }], { type: "delta" });
    assert.deepEqual(client.elements, [rectangle("local", 2)]);
});

test("reconnects automatically and restores the same operation across page reload", ({ mock }) => {
    const initial = open();
    initial.socket.receive(5);
    initial.session.edit([rectangle("local")]);
    mock.timers.tick(100);
    const [operation] = initial.socket.sent;
    initial.socket.close();
    mock.timers.tick(500);
    const resumed = Socket.opened.at(-1);
    assert.notEqual(resumed, initial.socket);
    assert.deepEqual(resumed.sent, []);
    resumed.receive(6, [rectangle("local")]);
    mock.timers.tick(100);
    assert.deepEqual(resumed.sent, [operation]);
    initial.session.close();

    const reloaded = open();
    assert.equal(reloaded.status.pending, false);
    reloaded.socket.receive(6, [rectangle("local")]);
    assert.equal(reloaded.status.pending, true);
    mock.timers.tick(100);
    assert.deepEqual(reloaded.socket.sent, [operation]);
    reloaded.socket.receive(6, [rectangle("local")], { ack_op_id: operation.op_id });
    assert.equal(reloaded.status.pending, false);
});

test("retries a half-open connection whose operation never receives an ACK", ({ mock }) => {
    const client = open();
    client.socket.receive(2);
    client.session.edit([rectangle("local")]);
    mock.timers.tick(100);
    const [operation] = client.socket.sent;
    mock.timers.tick(12000);
    mock.timers.tick(500);
    const resumed = Socket.opened.at(-1);
    assert.notEqual(resumed, client.socket);
    resumed.receive(2);
    mock.timers.tick(100);
    assert.deepEqual(resumed.sent, [operation]);
});

test("confirms an already-sent operation if access becomes read-only before reconnect", ({ mock }) => {
    const client = open();
    client.socket.receive(0);
    client.session.edit([rectangle("published")]);
    mock.timers.tick(100);
    const [operation] = client.socket.sent;
    client.session.edit([rectangle("published"), rectangle("never-sent")]);
    client.socket.close();
    mock.timers.tick(500);
    const resumed = Socket.opened.at(-1);
    resumed.receive(1, [rectangle("published")], { can_edit: false });
    mock.timers.tick(100);
    assert.deepEqual(resumed.sent, [operation]);
    resumed.receive(1, [rectangle("published")], { can_edit: false, ack_op_id: operation.op_id });
    mock.timers.tick(100);
    assert.deepEqual(resumed.sent, [operation]);
    assert.equal(client.status.pending, true);
    client.session.discardDraft();
    assert.equal(client.status.pending, false);
    assert.deepEqual(client.elements, [rectangle("published")]);
});

test("requests a fresh scene after a gap and after a missing heartbeat", ({ mock }) => {
    const client = open();
    client.socket.receive(2);
    client.socket.receive(4, [rectangle("gap")], { type: "delta" });
    assert.deepEqual(client.elements, []);
    mock.timers.tick(500);
    const resumed = Socket.opened.at(-1);
    resumed.receive(4, [rectangle("existing"), rectangle("gap")]);
    resumed.receive(4, [], { type: "heartbeat" });
    assert.equal(client.elements.length, 2);
    mock.timers.tick(25000);
    mock.timers.tick(500);
    assert.notEqual(Socket.opened.at(-1), resumed);
});

test("keeps conflicts separate from published edits and rebases only after explicit retry", ({ mock }) => {
    const client = open();
    client.socket.receive(6, [rectangle("shared")]);
    client.session.edit([rectangle("shared", 2)]);
    mock.timers.tick(100);
    const [first] = client.socket.sent;
    client.session.edit([rectangle("shared", 3), rectangle("other")]);
    client.socket.receive(7, [rectangle("shared", 9)], {
        ack_op_id: first.op_id, error: "element_conflict", conflicts: ["shared"],
    });
    assert.equal(client.status.canRetryDraft, true);
    assert.equal(client.status.retryOverwritesObjects, true);
    assert.equal(client.status.pending, true);
    mock.timers.tick(100);
    assert.equal(client.socket.sent.length, 1);
    client.session.retryDraft();
    mock.timers.tick(100);
    const retry = client.socket.sent[1];
    assert.notEqual(retry.op_id, first.op_id);
    assert.equal(retry.base_seq, 7);
    assert.deepEqual(retry.payload.elements, [rectangle("shared", 3)]);
    client.socket.receive(8, retry.payload.elements, { type: "delta", ack_op_id: retry.op_id });
    mock.timers.tick(100);
    assert.equal(client.socket.sent[2].base_seq, 6);
    assert.deepEqual(client.socket.sent[2].payload.elements, [rectangle("other")]);
});

test("preserves permission-lost edits and never opens another user's draft", ({ mock }) => {
    const client = open();
    client.socket.receive(0);
    client.session.edit([rectangle("private")]);
    client.socket.receive(0, [], { type: "heartbeat", can_edit: false });
    assert.equal(client.status.canEdit, false);
    assert.equal(client.status.canDiscardDraft, true);
    mock.timers.tick(100);
    assert.deepEqual(client.socket.sent, []);
    client.socket.receive(0, [], { type: "access_revoked" });
    assert.deepEqual(client.elements, []);
    assert.equal(client.status.ready, false);
    client.session.connect();
    Socket.opened.at(-1).receive(0, [], { draft_owner: 34 });
    assert.equal(client.status.pending, false);
    client.session.close();
    const originalOwner = open();
    originalOwner.socket.receive(0);
    assert.deepEqual(originalOwner.elements, [rectangle("private")]);
});

test("requires re-approval for expired offline edits and can discard them", ({ mock }) => {
    const client = open();
    client.socket.receive(2);
    client.socket.close();
    client.session.edit([rectangle("offline")]);
    assert.equal(client.status.pending, true);
    mock.timers.tick(500);
    const resumed = Socket.opened.at(-1);
    resumed.receive(600, [rectangle("published")]);
    mock.timers.tick(100);
    const [operation] = resumed.sent;
    assert.equal(operation.base_seq, 2);
    resumed.receive(600, [rectangle("published")], {
        ack_op_id: operation.op_id, error: "history_expired",
    });
    assert.equal(client.status.canDiscardDraft, true);
    client.session.discardDraft();
    assert.equal(client.status.pending, false);
    assert.deepEqual(client.elements, [rectangle("published")]);
});

test("does not send changes with missing images and can retry their preparation", async ({ mock }) => {
    let uploads = 0;
    const client = open({
        async prepareOperation(elements, { draftOwner }) {
            assert.equal(draftOwner, "32");
            if (++uploads === 1) throw new Error("Upload interrupted");
            return elements.map((element) => ({ ...element, status: "saved" }));
        },
    });
    client.socket.receive(0);
    const image = { id: "image", type: "image", status: "pending", fileId: "file" };
    client.session.edit([image]);
    mock.timers.tick(100);
    await Promise.resolve();
    assert.deepEqual(client.socket.sent, []);
    assert.equal(client.status.canRetryDraft, true);
    assert.equal(client.status.retryOverwritesObjects, false);
    client.session.retryDraft();
    mock.timers.tick(100);
    await Promise.resolve();
    assert.deepEqual(client.socket.sent[0].payload.elements, [{ ...image, status: "saved" }]);
});

test("keeps a single oversized element recoverable without sending a disconnecting frame", ({ mock }) => {
    const client = open();
    client.socket.receive(0);
    client.session.edit([{ id: "text", type: "text", text: "ш".repeat(270000) }]);
    mock.timers.tick(100);
    assert.deepEqual(client.socket.sent, []);
    assert.equal(client.status.pending, true);
    assert.equal(client.status.canRetryDraft, false);
    assert.equal(client.status.canDiscardDraft, true);
    assert.match(client.status.text, /512/);
});
