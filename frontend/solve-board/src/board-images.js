const IMAGE_TYPES = new Set(["image/png", "image/jpeg"]);
const MAX_IMAGE_BYTES = 2 * 1024 * 1024;
const MAX_IMAGE_SIDE = 4096;
const MAX_IMAGE_PIXELS = 8 * 1024 * 1024;
const FILE_ID = /^[A-Za-z0-9_-]{1,128}$/;
const DRAFT_MAX_AGE = 7 * 24 * 60 * 60 * 1000;
const REQUEST_TIMEOUT = 15_000;
let imageDatabase;

function openImageDatabase() {
    if (!imageDatabase) {
        imageDatabase = new Promise((resolve, reject) => {
            const request = indexedDB.open("solve-board-images", 1);
            request.onupgradeneeded = () => {
                const drafts = request.result.createObjectStore("drafts");
                drafts.createIndex("savedAt", "savedAt");
            };
            request.onsuccess = () => resolve(request.result);
            request.onerror = () => reject(request.error);
        }).catch(error => {
            imageDatabase = undefined;
            throw error;
        });
    }
    return imageDatabase;
}

async function imageDraft(mode, request) {
    const db = await openImageDatabase();
    return new Promise((resolve, reject) => {
        const transaction = db.transaction("drafts", mode);
        const operation = request(transaction.objectStore("drafts"));
        transaction.oncomplete = () => resolve(operation.result);
        transaction.onabort = () => reject(transaction.error);
    });
}

async function pruneImageDrafts() {
    const db = await openImageDatabase();
    const drafts = db.transaction("drafts", "readwrite").objectStore("drafts");
    const request = drafts.index("savedAt").openCursor(IDBKeyRange.upperBound(Date.now() - DRAFT_MAX_AGE));
    request.onsuccess = () => {
        const cursor = request.result;
        if (cursor) {
            cursor.delete();
            cursor.continue();
        }
    };
}

export function boardImageIDs(elements) {
    return [...new Set(elements.filter(element => element.type === "image" && !element.isDeleted)
        .map(element => element.fileId).filter(id => typeof id === "string" && FILE_ID.test(id)))];
}

export async function validateBoardImage(blob) {
    if (!IMAGE_TYPES.has(blob.type)) throw new Error("Выберите изображение PNG или JPEG.");
    if (!blob.size || blob.size > MAX_IMAGE_BYTES) throw new Error("Размер файла должен быть до 2 МиБ.");
    const bitmap = await createImageBitmap(blob).catch(() => {
        throw new Error("Не удалось открыть изображение.");
    });
    const { width, height } = bitmap;
    bitmap.close();
    if (width > MAX_IMAGE_SIDE || height > MAX_IMAGE_SIDE || width * height > MAX_IMAGE_PIXELS) {
        throw new Error("Изображение слишком большое. Лимит по стороне 4096 пикселей, по площади 8 388 608 пикселей.");
    }
    return { width, height };
}

function imageData(id, blob) {
    return new Promise((resolve, reject) => {
        const reader = new FileReader();
        reader.onload = () => resolve({ id, mimeType: blob.type, dataURL: reader.result, created: Date.now() });
        reader.onerror = () => reject(reader.error);
        reader.readAsDataURL(blob);
    });
}

export function createBoardImages(boardID, owner) {
    const uploaded = new Set();
    const downloads = new Map();
    const controller = new AbortController();
    const { signal } = controller;
    const key = id => [boardID, owner, id];
    const fileURL = id => `/api/v1/boards/${boardID}/files/${encodeURIComponent(id)}`;

    pruneImageDrafts().catch(() => {});

    async function requestImage(id, method, handleResponse, body) {
        const request = new AbortController();
        const abort = () => request.abort();
        signal.throwIfAborted();
        signal.addEventListener("abort", abort, { once: true });
        const timeout = setTimeout(abort, REQUEST_TIMEOUT);
        try {
            const response = await fetch(fileURL(id), {
                method, body, signal: request.signal, credentials: "same-origin", cache: "no-store",
            });
            return await handleResponse(response);
        } finally {
            clearTimeout(timeout);
            signal.removeEventListener("abort", abort);
        }
    }

    return {
        async draft(blob) {
            const size = await validateBoardImage(blob);
            const bytes = crypto.getRandomValues(new Uint8Array(16));
            const id = Array.from(bytes, byte => byte.toString(16).padStart(2, "0")).join("");
            const file = await imageData(id, blob);
            signal.throwIfAborted();
            await imageDraft("readwrite", drafts => drafts.put({ blob, savedAt: Date.now() }, key(id)))
                .catch(() => { throw new Error("Браузер не смог сохранить черновик изображения."); });
            signal.throwIfAborted();
            return { file, ...size };
        },
        async load(id) {
            if (downloads.has(id)) return downloads.get(id);
            const download = (async () => {
                const local = await imageDraft("readonly", drafts => drafts.get(key(id))).catch(() => null);
                signal.throwIfAborted();
                if (local) return imageData(id, local.blob);
                const blob = await requestImage(id, "GET", response => {
                    if (!response.ok) throw new Error("Не удалось загрузить изображение доски.");
                    return response.blob();
                });
                if (!IMAGE_TYPES.has(blob.type) || !blob.size || blob.size > MAX_IMAGE_BYTES) {
                    throw new Error("Изображение доски повреждено.");
                }
                uploaded.add(id);
                return imageData(id, blob);
            })();
            downloads.set(id, download);
            try {
                return await download;
            } finally {
                downloads.delete(id);
            }
        },
        async upload(id) {
            if (uploaded.has(id)) return;
            const local = await imageDraft("readonly", drafts => drafts.get(key(id))).catch(() => null);
            if (local) {
                await requestImage(id, "PUT", response => {
                    if (response.status === 413) throw new Error("Достигнут лимит изображений на доске.");
                    if (!response.ok) throw new Error("Сервер не принял изображение доски.");
                }, local.blob);
                uploaded.add(id);
                await imageDraft("readwrite", drafts => drafts.delete(key(id))).catch(() => {});
            } else {
                await this.load(id);
            }
        },
        close() {
            controller.abort();
        },
    };
}
