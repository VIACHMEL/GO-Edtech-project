import React, { useEffect, useRef, useState } from "react";
import { createRoot } from "react-dom/client";
import {
    CaptureUpdateAction, convertToExcalidrawElements, Excalidraw, exportToBlob, exportToSvg,
} from "@excalidraw/excalidraw";
import "@excalidraw/excalidraw/index.css";
import { createBoardSession } from "./board-session.js";
import { boardImageIDs, createBoardImages } from "./board-images.js";
import "./editor.css";

const editorOptions = {
    canvasActions: { export: false, loadScene: false, saveToActiveFile: false },
    tools: { image: false },
};
const INITIAL_STATUS = { text: "Подключаем доску…", canEdit: false, ready: false };

function useExpandedBoard(panel, button) {
    const [fullscreen, setFullscreen] = useState(false);
    const [expanded, setExpanded] = useState(false);

    useEffect(() => {
        function onFullscreen() {
            setFullscreen(document.fullscreenElement === panel.current);
        }
        document.addEventListener("fullscreenchange", onFullscreen);
        return () => document.removeEventListener("fullscreenchange", onFullscreen);
    }, [panel]);

    useEffect(() => {
        if (!expanded) return;
        function onEscape(event) {
            if (event.key !== "Escape" || event.defaultPrevented) return;
            setExpanded(false);
            button.current?.focus();
        }
        window.addEventListener("keydown", onEscape);
        return () => window.removeEventListener("keydown", onEscape);
    }, [expanded, button]);

    async function toggle() {
        if (fullscreen) await document.exitFullscreen();
        else if (expanded) setExpanded(false);
        else {
            try {
                if (!panel.current.requestFullscreen) throw new Error();
                await panel.current.requestFullscreen();
            } catch {
                setExpanded(true);
            }
        }
    }

    return { fullscreen, expanded, toggle };
}

function changeZoom(editor, multiplier) {
    const { zoom, scrollX, scrollY, width, height } = editor.getAppState();
    const value = Math.min(4, Math.max(0.1, zoom.value * multiplier));
    const offset = (1 / value - 1 / zoom.value) / 2;
    editor.updateScene({ appState: {
        zoom: { ...zoom, value }, scrollX: scrollX + width * offset, scrollY: scrollY + height * offset,
    }, captureUpdate: CaptureUpdateAction.NEVER });
}

function addImage(editor, { file, width, height }) {
    const appState = editor.getAppState();
    const scale = Math.min(1, 420 / Math.max(width, height));
    const [image] = convertToExcalidrawElements([{
        type: "image",
        fileId: file.id,
        status: "saved",
        x: appState.width / (2 * appState.zoom.value) - appState.scrollX - width * scale / 2,
        y: appState.height / (2 * appState.zoom.value) - appState.scrollY - height * scale / 2,
        width: width * scale,
        height: height * scale,
    }]);
    editor.addFiles([file]);
    editor.updateScene({
        elements: [...editor.getSceneElementsIncludingDeleted(), image],
        appState: { selectedElementIds: { [image.id]: true } },
        captureUpdate: CaptureUpdateAction.IMMEDIATELY,
    });
}

async function loadSceneImages(editor, images, elements) {
    const files = editor.getFiles();
    const missing = boardImageIDs(elements).filter(id => !Object.hasOwn(files, id));
    return Promise.allSettled(missing.map(id => images.load(id)));
}

function downloadBoard(blob, filename) {
    const href = URL.createObjectURL(blob);
    const link = document.createElement("a");
    link.href = href;
    link.download = filename;
    document.body.append(link);
    link.click();
    link.remove();
    setTimeout(() => URL.revokeObjectURL(href), 1000);
}

function BoardEditor({ boardID, url, setPending }) {
    const [editor, setEditor] = useState(null);
    const [status, setStatus] = useState(INITIAL_STATUS);
    const [assetMessage, setAssetMessage] = useState("");
    const [imagesUnavailable, setImagesUnavailable] = useState(false);
    const [addingImage, setAddingImage] = useState(false);
    const [exporting, setExporting] = useState(false);
    const [zoom, setZoom] = useState(100);
    const panel = useRef(null);
    const fullscreenButton = useRef(null);
    const input = useRef(null);
    const session = useRef(null);
    const assets = useRef(null);
    const activity = useRef({ addingImage: false, pending: false });
    const connection = useRef(INITIAL_STATUS);
    const loadRequest = useRef(0);
    const { fullscreen, expanded, toggle } = useExpandedBoard(panel, fullscreenButton);

    async function refreshImages(elements, currentAssets) {
        const request = ++loadRequest.current;
        const loaded = await loadSceneImages(editor, currentAssets.images, elements);
        if (assets.current !== currentAssets || loadRequest.current !== request) return;
        const available = loaded.filter(result => result.status === "fulfilled").map(result => result.value);
        if (available.length) editor.addFiles(available);
        setImagesUnavailable(loaded.some(result => result.status === "rejected"));
    }

    useEffect(() => {
        if (!editor) return;
        function setAssetOwner(owner) {
            if ((assets.current?.owner ?? null) === owner) return;
            assets.current?.images.close();
            editor.resetScene();
            setImagesUnavailable(false);
            setAssetMessage("");
            assets.current = owner ? { owner, images: createBoardImages(boardID, owner) } : null;
        }

        const board = createBoardSession(url, {
            onScene(elements, { draftOwner }) {
                setAssetOwner(draftOwner);
                editor.updateScene({ elements, captureUpdate: CaptureUpdateAction.NEVER });
                if (assets.current) void refreshImages(elements, assets.current);
            },
            async prepareOperation(elements, { draftOwner }) {
                const ids = boardImageIDs(elements);
                if (!ids.length) return elements;
                const current = assets.current;
                if (!current || current.owner !== draftOwner) throw new Error("Доска недоступна.");
                try {
                    for (const id of ids) await current.images.upload(id);
                } catch (error) {
                    if (assets.current === current && error.name === "Error") setAssetMessage(error.message);
                    throw error;
                }
                if (assets.current === current) setAssetMessage("");
                return elements;
            },
            onStatus(next) {
                setAssetOwner(next.draftOwner ?? null);
                connection.current = next;
                activity.current.pending = next.pending;
                setPending(next.pending || activity.current.addingImage);
                setStatus(next);
            },
        });
        session.current = board;
        board.connect();
        return () => {
            board.close();
            assets.current?.images.close();
            assets.current = null;
            session.current = null;
            setPending(false);
        };
    }, [boardID, editor, url, setPending]);

    async function insertImage(blob) {
        const current = assets.current;
        if (!current || !connection.current.canEdit || activity.current.addingImage || !blob) return;
        activity.current.addingImage = true;
        setPending(true);
        setAddingImage(true);
        setAssetMessage("");
        try {
            const draft = await current.images.draft(blob);
            if (assets.current !== current || !connection.current.canEdit) return;
            addImage(editor, draft);
            session.current?.edit(editor.getSceneElementsIncludingDeleted());
        } catch (error) {
            if (assets.current === current) setAssetMessage(error.message || "Не удалось добавить изображение.");
        } finally {
            activity.current.addingImage = false;
            setAddingImage(false);
            if (session.current) setPending(activity.current.pending);
        }
    }

    function insertTransferredImage(event, files) {
        if (!files?.length || event.target.closest?.("input, textarea, [contenteditable=true]")) return;
        event.preventDefault();
        event.stopPropagation();
        void insertImage(files[0]);
    }

    async function exportBoard(format) {
        const current = assets.current;
        if (!current || exporting) return;
        setExporting(true);
        setAssetMessage("");
        try {
            const elements = editor.getSceneElements();
            if (!elements.length) throw new Error("На доске пока нечего экспортировать.");
            const loaded = await loadSceneImages(editor, current.images, elements);
            if (loaded.some(result => result.status === "rejected")) throw new Error("Загрузите изображения перед экспортом.");
            if (assets.current !== current) return;
            editor.addFiles(loaded.map(result => result.value));
            const options = { elements, files: editor.getFiles(), appState: { exportBackground: true, viewBackgroundColor: "#ffffff" } };
            let blob;
            if (format === "png") {
                blob = await exportToBlob({ ...options, mimeType: "image/png", maxWidthOrHeight: 4096 });
            } else {
                const svg = await exportToSvg({ ...options, renderEmbeddables: false });
                blob = new Blob([new XMLSerializer().serializeToString(svg)], { type: "image/svg+xml" });
            }
            if (assets.current === current) downloadBoard(blob, `board-${boardID}.${format}`);
        } catch (error) {
            if (assets.current === current) setAssetMessage(error.message || "Не удалось экспортировать доску.");
        } finally {
            setExporting(false);
        }
    }

    return <div className="board-editor" data-expanded={expanded} ref={panel}>
        <p className="save-status" id="solve-board-status" role="status">{status.text}</p>
        <div className="board-toolbar" role="group" aria-label="Управление доской">
            <button type="button" className="secondary-button" disabled={!status.canEdit || addingImage}
                onClick={() => input.current.click()}>Изображение</button>
            <input ref={input} type="file" accept="image/png,image/jpeg" hidden onChange={event => {
                void insertImage(event.target.files[0]);
                event.target.value = "";
            }} />
            <button type="button" className="secondary-button" disabled={!status.ready || exporting}
                onClick={() => exportBoard("png")}>PNG</button>
            <button type="button" className="secondary-button" disabled={!status.ready || exporting}
                onClick={() => exportBoard("svg")}>SVG</button>
            <button type="button" className="secondary-button" ref={fullscreenButton} aria-pressed={fullscreen || expanded}
                onClick={toggle}>{fullscreen || expanded ? "Свернуть доску" : "На весь экран"}</button>
            <div className="board-zoom" role="group" aria-label="Масштаб доски">
                <button type="button" className="secondary-button" aria-label="Уменьшить масштаб" disabled={!editor || zoom <= 10}
                    onClick={() => changeZoom(editor, 1 / 1.25)}>−</button>
                <output aria-label="Текущий масштаб">{zoom}%</output>
                <button type="button" className="secondary-button" aria-label="Увеличить масштаб" disabled={!editor || zoom >= 400}
                    onClick={() => changeZoom(editor, 1.25)}>+</button>
                <button type="button" className="secondary-button" disabled={!status.ready}
                    onClick={() => editor.scrollToContent(undefined, { fitToContent: true })}>Уместить</button>
            </div>
        </div>
        <div className="board-recovery">
            {status.canReconnect && <button type="button" className="secondary-button"
                onClick={() => session.current?.connect()}>Подключиться снова</button>}
            {status.canRetryDraft && <button type="button" className="secondary-button" onClick={() => {
                if (!status.retryOverwritesObjects || window.confirm("Применить свои правки поверх текущего состояния доски?")) {
                    session.current?.retryDraft();
                }
            }}>{status.retryOverwritesObjects ? "Применить мои правки" : "Повторить сохранение"}</button>}
            {status.canDiscardDraft && <button type="button" className="secondary-button" onClick={() => {
                if (window.confirm("Удалить несохранённые правки этой доски?")) session.current?.discardDraft();
            }}>Отменить мои правки</button>}
            {imagesUnavailable && <button type="button" className="secondary-button" onClick={() => {
                if (assets.current) void refreshImages(editor.getSceneElements(), assets.current);
            }}>Загрузить изображения снова</button>}
        </div>
        {(addingImage || assetMessage || imagesUnavailable) && <p className="save-status" role="status">
            {addingImage ? "Добавляем изображение…" : assetMessage || "Часть изображений недоступна."}
        </p>}
        <div className="solve-board-canvas"
            onPasteCapture={event => insertTransferredImage(event, event.clipboardData.files)}
            onDragOver={event => { if (event.dataTransfer.types.includes("Files")) event.preventDefault(); }}
            onDropCapture={event => insertTransferredImage(event, event.dataTransfer.files)}>
            <Excalidraw excalidrawAPI={setEditor} UIOptions={editorOptions} langCode="ru-RU"
                theme="dark" viewModeEnabled={!status.canEdit} validateEmbeddable={false}
                onPaste={data => !boardImageIDs(data.elements || []).some(id => !Object.hasOwn(editor?.getFiles() || {}, id))}
                onChange={(elements, appState) => {
                    session.current?.edit(elements);
                    setZoom(Math.round(appState.zoom.value * 100));
                }} />
        </div>
    </div>;
}

export function mountEditor(container, setPending) {
    const root = createRoot(container);
    return {
        show(board, url) {
            root.render(<BoardEditor key={board.id} boardID={board.id} url={url} setPending={setPending} />);
        },
        close() {
            root.unmount();
        },
    };
}
