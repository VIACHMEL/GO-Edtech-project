import React, { useEffect, useRef, useState } from "react";
import { createRoot } from "react-dom/client";
import { CaptureUpdateAction, Excalidraw } from "@excalidraw/excalidraw";
import "@excalidraw/excalidraw/index.css";
import { createBoardSession } from "./board-session.js";

const editorOptions = {
    canvasActions: { export: false, loadScene: false, saveToActiveFile: false },
    tools: { image: false },
};

function BoardEditor({ url, setPending }) {
    const [editor, setEditor] = useState(null);
    const [status, setStatus] = useState({ text: "Подключаем доску…", canEdit: false });
    const session = useRef(null);

    useEffect(() => {
        if (!editor) return;
        const board = createBoardSession(url, {
            onScene(elements) {
                editor.updateScene({ elements, captureUpdate: CaptureUpdateAction.NEVER });
            },
            onStatus(next) {
                setPending(next.pending);
                setStatus(next);
            },
        });
        session.current = board;
        board.connect();
        return () => {
            board.close();
            session.current = null;
            setPending(false);
        };
    }, [editor, url, setPending]);

    return <>
        <p className="save-status" id="solve-board-status" role="status">{status.text}</p>
        {status.canReconnect && <button type="button" className="secondary-button"
            onClick={() => session.current?.connect()}>Подключиться снова</button>}
        <div className="solve-board-canvas">
            <Excalidraw excalidrawAPI={setEditor} UIOptions={editorOptions} langCode="ru-RU"
                theme="dark" viewModeEnabled={!status.canEdit} validateEmbeddable={false}
                onChange={(elements) => session.current?.edit(elements)} />
        </div>
    </>;
}

export function mountEditor(container, setPending) {
    const root = createRoot(container);
    return {
        show(board, url) {
            // Positions sharing a board keep the same editor and connection.
            root.render(<BoardEditor key={board.id} url={url} setPending={setPending} />);
        },
        close() {
            root.unmount();
        },
    };
}
