import React, { useEffect, useRef, useState } from "react";
import { createRoot } from "react-dom/client";
import { CaptureUpdateAction, Excalidraw } from "@excalidraw/excalidraw";
import "@excalidraw/excalidraw/index.css";
import "./style.css";

const names = { anna: "Анна", ivan: "Иван", teacher: "Преподаватель" };
const query = new URLSearchParams(location.search);
const actor = Object.hasOwn(names, query.get("actor")) ? query.get("actor") : "anna";
const teacher = actor === "teacher";
const roleURL = (role) => "?" + new URLSearchParams({ actor: role });

async function request(path = "", options) {
  const response = await fetch("/api/" + actor + "/attempts" + path, options);
  if (!response.ok) throw new Error((await response.text()).trim());
  return response.status === 204 ? null : response.json();
}

function signature(element) {
  return [element.version, element.versionNonce, element.isDeleted].join(":");
}

function elementOrder(a, b) {
  const left = a.index ?? "";
  const right = b.index ?? "";
  if (left === right) return a.id < b.id ? -1 : Number(a.id > b.id);
  return left < right ? -1 : 1;
}

function connectBoard(boardId, editor, onStatus) {
  let socket;
  let reconnectTimer;
  let stopped = false;
  let ready = false;
  let canEdit = false;
  let dirty = new Map();
  let inFlight = null;
  let nextId = 1;
  let baseline = new Map();

  function flush() {
    if (!ready || !canEdit || inFlight || !dirty.size || socket.readyState !== WebSocket.OPEN) return;
    inFlight = { id: nextId++, elements: [...dirty.values()] };
    dirty = new Map();
    socket.send(JSON.stringify(inFlight));
  }

  function change(elements) {
    if (!ready || !canEdit) return;
    const next = new Map();
    let changed = false;
    for (const element of elements) {
      const mark = signature(element);
      next.set(element.id, { mark, element });
      if (baseline.get(element.id)?.mark !== mark) {
        dirty.set(element.id, structuredClone(element));
        changed = true;
      }
    }
    // The editor may clear the scene by dropping elements from the array.
    for (const [id, previous] of baseline) {
      if (!next.has(id) && !previous.element.isDeleted) {
        dirty.set(id, { ...previous.element, isDeleted: true });
        changed = true;
      }
    }
    baseline = next;
    if (!changed) return;
    if (!inFlight) onStatus({ ready, canEdit, busy: true, text: "Сохраняем…" });
    flush();
  }

  function open() {
    const url = new URL("/ws", location.href);
    url.protocol = location.protocol === "https:" ? "wss:" : "ws:";
    url.search = new URLSearchParams({ actor, board: boardId });
    socket = new WebSocket(url);
    socket.onmessage = ({ data }) => {
      const message = JSON.parse(data);
      if (message.ack === inFlight?.id) inFlight = null;
      ready = true;
      canEdit = message.can_edit;
      if (!canEdit) {
        inFlight = null;
        dirty.clear();
      }
      const combined = new Map((message.elements ?? []).map((e) => [e.id, e]));
      for (const element of inFlight?.elements ?? []) combined.set(element.id, element);
      for (const element of dirty.values()) combined.set(element.id, element);
      const ordered = [...combined.values()].sort(elementOrder);
      const same = baseline.size === ordered.length &&
        ordered.every((element) => baseline.get(element.id)?.mark === signature(element));
      if (!same) {
        baseline = new Map(ordered.map((element) => [element.id, { mark: signature(element), element }]));
        editor.updateScene({ elements: ordered, captureUpdate: CaptureUpdateAction.NEVER });
      }
      flush();
      const busy = Boolean(inFlight || dirty.size);
      onStatus({
        ready, canEdit, busy,
        text: message.error || (!canEdit ? "Только просмотр" : busy ? "Сохраняем…" : "Сохранено"),
      });
    };
    socket.onclose = () => {
      if (stopped) return;
      ready = canEdit = false;
      const pending = Boolean(inFlight || dirty.size);
      inFlight = null;
      dirty.clear();
      onStatus({
        ready, canEdit, busy: false,
        text: pending ? "Связь прервана до подтверждения сохранения" : "Восстанавливаем связь…",
      });
      reconnectTimer = setTimeout(open, 1000);
    };
  }

  open();
  return {
    change,
    close() {
      stopped = true;
      clearTimeout(reconnectTimer);
      socket.onmessage = null;
      socket.close();
    },
  };
}

const connecting = { ready: false, canEdit: false, busy: false, text: "Подключаем доску…" };
const editorOptions = {
  canvasActions: { loadScene: false, saveToActiveFile: false, export: false },
  tools: { image: false },
};

function Board({ id, status, setStatus }) {
  const [editor, setEditor] = useState(null);
  const room = useRef(null);
  useEffect(() => {
    if (!editor) return;
    setStatus(connecting);
    room.current = connectBoard(id, editor, setStatus);
    return () => {
      room.current.close();
      room.current = null;
    };
  }, [editor, id, setStatus]);

  return <div className="canvas" aria-busy={!status.ready}>
    <Excalidraw
      excalidrawAPI={setEditor}
      onChange={(elements) => room.current?.change(elements)}
      viewModeEnabled={!status.canEdit}
      UIOptions={editorOptions}
      langCode="ru-RU"
    />
  </div>;
}

function App() {
  const [attempts, setAttempts] = useState([]);
  const [attemptId, setAttemptId] = useState(query.get("attempt") || "");
  const [taskId, setTaskId] = useState(1);
  const [status, setStatus] = useState(connecting);
  const [error, setError] = useState("");
  const [acting, setActing] = useState(false);
  const [loading, setLoading] = useState(true);
  const attempt = attempts.find((a) => a.id === attemptId);
  const task = attempt?.tasks.find((t) => t.id === taskId);

  async function load(preferred = attemptId) {
    try {
      const list = await request();
      setAttempts(list);
      setAttemptId(list.find((a) => a.id === preferred)?.id ?? list[0]?.id ?? "");
      setError("");
    } catch (err) {
      setError(err.message);
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => { void load(); }, []);

  function selectAttempt(id) {
    if (id !== attemptId) {
      setStatus(connecting);
      setTaskId(1);
      setAttemptId(id);
    }
  }

  async function action(name) {
    setActing(true);
    setError("");
    try {
      await request("/" + attempt.id + "/" + name, { method: "POST" });
      if (name === "retry") {
        setStatus(connecting);
        setTaskId(1);
      }
      await load(name === "retry" ? actor + "-2" : attempt.id);
    } catch (err) {
      setError(err.message);
    } finally {
      setActing(false);
    }
  }

  return <>
    <header className="topbar">
      <h1>Доска для решения задач</h1>
      <nav aria-label="Тестовые роли">
        {Object.entries(names).map(([key, name]) =>
          <a key={key} href={roleURL(key)} aria-current={key === actor ? "page" : undefined}>{name}</a>
        )}
      </nav>
    </header>
    <div className="workspace">
      <aside>
        <label htmlFor="attempt">Попытка</label>
        <div className="attempt-picker">
          <select id="attempt" value={attemptId} disabled={!attempt || status.busy || acting}
            onChange={({ target }) => selectAttempt(target.value)}>
            {!attempt && <option value="">{loading ? "Загрузка…" : "Выберите попытку"}</option>}
            {attempts.map((a) => <option key={a.id} value={a.id}>{a.student} · №{a.number}</option>)}
          </select>
          {teacher && <button type="button" disabled={status.busy || acting}
            onClick={() => void load()}>Обновить</button>}
        </div>
        {attempt && <>
          <h2>Задания</h2>
          <nav className="tasks" aria-label="Задания">
            {attempt.tasks.map((t) => <button key={t.id} type="button"
              aria-current={taskId === t.id ? "step" : undefined} disabled={status.busy || acting}
              onClick={() => {
                if (t.board_id !== task.board_id) setStatus(connecting);
                setTaskId(t.id);
              }}>
              <span>Задание {t.id}</span><span>Часть {t.part}</span>
            </button>)}
          </nav>
          {!teacher && !attempt.submitted && <button className="submit" type="button"
            disabled={!status.ready || !status.canEdit || status.busy || acting}
            onClick={() => void action("submit")}>Сдать попытку</button>}
          {!teacher && attempt.submitted && attempt.number === 1 && !attempts.some((a) => a.number === 2) &&
            <button className="submit" type="button" disabled={acting}
              onClick={() => void action("retry")}>Начать попытку №2</button>}
        </>}
      </aside>
      <section className="problem">
        {error && <p role="alert">{error}</p>}
        {task && <>
          <div className="condition">
            <h2>Задание {task.id}</h2>
            <p>{task.prompt}</p>
          </div>
          <div className="board-heading">
            <h2>{task.part === 1 ? "Доска тестовой части" : "Доска задания " + task.id}</h2>
            <span role="status">{status.text}</span>
          </div>
          <Board key={task.board_id} id={task.board_id} status={status} setStatus={setStatus} />
        </>}
      </section>
    </div>
  </>;
}

createRoot(document.querySelector("#app")).render(<App />);
