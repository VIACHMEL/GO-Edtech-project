import { sameScene, trackScene } from "./scene.js";

// Leave room for the operation envelope and server-side JSON serialization.
const batchBytes = 480 * 1024;
const batchElements = 2000;
const encoder = new TextEncoder();
export const boardMessageBytes = 512 * 1024;

function operationID() {
    if (crypto.randomUUID) return crypto.randomUUID();
    const bytes = crypto.getRandomValues(new Uint8Array(16));
    bytes[6] = (bytes[6] & 15) | 64;
    bytes[8] = (bytes[8] & 63) | 128;
    const hex = Array.from(bytes, byte => byte.toString(16).padStart(2, "0")).join("");
    return `${hex.slice(0, 8)}-${hex.slice(8, 12)}-${hex.slice(12, 16)}-${hex.slice(16, 20)}-${hex.slice(20)}`;
}

export class BoardDraft {
    queued = new Map();
    inFlight = null;
    error = null;
    storageFailed = false;

    constructor(url, owner) {
        this.key = owner && `edtech:board-draft:v1:${new URL(url).pathname}:${owner}`;
        if (!this.key) return;
        try {
            const saved = JSON.parse(sessionStorage.getItem(this.key) ?? "null");
            if (saved) {
                this.queued = new Map(saved.queued);
                this.inFlight = saved.inFlight;
                this.error = saved.error;
            }
        } catch {
            this.storageFailed = true;
        }
    }

    get pending() {
        return this.inFlight !== null || this.queued.size > 0;
    }

    get sentElements() {
        return this.inFlight?.operation.payload.elements ?? [];
    }

    get queuedElements() {
        return Array.from(this.queued.values(), ({ element }) => element);
    }

    persist() {
        if (!this.key) {
            this.storageFailed = this.pending;
            return;
        }
        try {
            if (this.pending) {
                sessionStorage.setItem(this.key, JSON.stringify({
                    queued: [...this.queued], inFlight: this.inFlight, error: this.error,
                }));
            } else {
                sessionStorage.removeItem(this.key);
            }
            this.storageFailed = false;
        } catch {
            this.storageFailed = true;
        }
    }

    add(elements, seq) {
        for (const element of elements) {
            const base = this.queued.get(element.id)?.base ?? seq;
            this.queued.set(element.id, { base, element: structuredClone(element) });
        }
        this.persist();
    }

    startOperation() {
        const elements = [];
        const base = this.queued.values().next().value.base;
        let bytes = 2;
        for (const [id, entry] of this.queued) {
            if (entry.base !== base) break;
            const size = encoder.encode(JSON.stringify(entry.element)).length + 1;
            if (elements.length && (bytes + size > batchBytes || elements.length >= batchElements)) break;
            elements.push(entry.element);
            bytes += size;
            this.queued.delete(id);
        }
        this.inFlight = {
            operation: { op_id: operationID(), base_seq: base, payload: { elements } },
            prepared: false,
            attempted: false,
        };
        this.persist();
    }

    acknowledge(confirmed, seq, deletionConfirmed) {
        for (const sent of this.sentElements) {
            const next = this.queued.get(sent.id);
            const server = confirmed.get(sent.id);
            const unchanged = server && sameScene(trackScene([server]), trackScene([sent]));
            if (next && (unchanged || (sent.isDeleted && deletionConfirmed && !server))) next.base = seq;
        }
        this.inFlight = null;
        this.persist();
    }

    retry(seq) {
        if (this.error === "prepare_failed") {
            this.error = null;
            this.persist();
            return;
        }
        const queue = this.queued;
        this.queued = new Map();
        for (const sent of this.sentElements) {
            const element = queue.get(sent.id)?.element ?? sent;
            this.queued.set(sent.id, { base: seq, element });
            queue.delete(sent.id);
        }
        for (const [id, entry] of queue) this.queued.set(id, entry);
        this.inFlight = null;
        this.error = null;
        this.persist();
    }

    discard() {
        this.inFlight = null;
        this.queued.clear();
        this.error = null;
        this.persist();
    }
}
