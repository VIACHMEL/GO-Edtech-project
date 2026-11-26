export function indexElements(elements) {
    return new Map(Array.from(elements, (element) => [element.id, element]));
}

// Excalidraw changes these values when an element is edited, removed or restored.
function fingerprint(element) {
    return [element.version, element.versionNonce, element.isDeleted, element.index].join(":");
}

export function trackScene(elements) {
    return new Map(elements.map((element) => [
        element.id,
        { element, fingerprint: fingerprint(element) },
    ]));
}

export function changedElements(previous, current) {
    const changes = [];
    for (const [id, entry] of current) {
        if (previous.get(id)?.fingerprint !== entry.fingerprint) {
            changes.push(entry.element);
        }
    }
    for (const [id, entry] of previous) {
        // Clearing the canvas can remove an element from the array entirely.
        if (!current.has(id) && !entry.element.isDeleted) {
            changes.push({ ...entry.element, isDeleted: true });
        }
    }
    return changes;
}

export function mergeElements(confirmed, inFlight, queued) {
    const elements = indexElements(confirmed);
    for (const element of inFlight) elements.set(element.id, element);
    for (const element of queued) elements.set(element.id, element);
    return [...elements.values()].sort(compareOrder);
}

function compareOrder(left, right) {
    const a = left.index ?? "";
    const b = right.index ?? "";
    if (a !== b) return a < b ? -1 : 1;
    if (left.id === right.id) return 0;
    return left.id < right.id ? -1 : 1;
}

export function sameScene(previous, current) {
    if (previous.size !== current.size) return false;
    for (const [id, entry] of current) {
        if (previous.get(id)?.fingerprint !== entry.fingerprint) return false;
    }
    return true;
}
